package controller

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"gorm.io/gorm"
	"mofufans/internal/database"
)

type VoyageController struct{ Directory string }

// Material references are admin-only; public visibility still uses display references.
type voyageAdminIssue struct {
	database.VoyageIssue
	AssetImages []string `json:"asset_images"`
}

func voyageIssueMaterials(issue database.VoyageIssue) []string {
	images := append([]string{}, issue.AssetImages...)
	images = append(images, issue.Images...)
	for _, gift := range issue.Gifts {
		images = append(images, gift.Images...)
	}
	result := []string{}
	seen := map[string]bool{}
	for _, image := range images {
		if !seen[image] {
			seen[image] = true
			result = append(result, image)
		}
	}
	return result
}

func (v *VoyageController) images() *BackgroundController {
	directory := v.Directory
	if directory == "" {
		directory = viper.GetString("voyage_directory")
	}
	if directory == "" {
		directory = "data/voyage"
	}
	return &BackgroundController{Directory: directory, AssetPrefix: "/api/voyage-assets/", Label: "礼物", Private: true}
}
func (v *VoyageController) Upload(c *gin.Context) { v.images().Upload(c) }

// Assets lists retained uploads for admin reuse, including currently unused artwork.
// Visibility to visitors is still determined only by published display references in File.
func (v *VoyageController) Assets(c *gin.Context) {
	page := 1
	if raw := c.Query("page"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 1000000 {
			Fail(c, 400, "图片页码无效")
			return
		}
		page = value
	}
	entries, err := v.images().assets().List(c.Request.Context())
	if err != nil {
		Fail(c, 500, "图片加载失败，请重试")
		return
	}
	type asset struct {
		name     string
		modified time.Time
	}
	assets := []asset{}
	for _, entry := range entries {
		if !backgroundFilename.MatchString(entry.Name()) || !entry.Mode().IsRegular() {
			continue
		}
		assets = append(assets, asset{entry.Name(), entry.ModTime()})
	}
	sort.Slice(assets, func(i, j int) bool {
		if assets[i].modified.Equal(assets[j].modified) {
			return assets[i].name < assets[j].name
		}
		return assets[i].modified.After(assets[j].modified)
	})
	const pageSize = 24
	pages := max(1, (len(assets)+pageSize-1)/pageSize)
	page = min(page, pages)
	images := []string{}
	for _, item := range assets[(page-1)*pageSize : min(page*pageSize, len(assets))] {
		images = append(images, "/api/voyage-assets/"+item.name)
	}
	c.Header("Cache-Control", "private, no-store")
	Success(c, gin.H{"images": images, "total": len(assets), "page": page, "page_size": pageSize})
}
func (v *VoyageController) File(c *gin.Context) {
	// Public access only serves artwork referenced by a published issue.
	// Draft and unused uploads remain available solely to administrators.
	if !c.GetBool("authed") {
		c.Header("Cache-Control", "private, no-store")
		var issues []database.VoyageIssue
		if err := database.DB.Select("images", "gifts").Where("published = ?", true).Find(&issues).Error; err != nil {
			c.Status(503)
			return
		}
		resource := "/api/voyage-assets/" + c.Param("filename")
		visible := false
		for _, issue := range issues {
			visible = visible || slices.Contains(issue.Images, resource)
			for _, gift := range issue.Gifts {
				visible = visible || slices.Contains(gift.Images, resource)
			}
		}
		if !visible {
			c.Status(404)
			return
		}
	}
	v.images().File(c)
}

func (*VoyageController) list(c *gin.Context, admin bool) {
	issues := []database.VoyageIssue{}
	query := database.DB.Where("kind <> ?", "batch").Order("month desc, id desc")
	if !admin {
		query = query.Where("published = ?", true)
	}
	if err := query.Find(&issues).Error; err != nil {
		Fail(c, 500, "回馈加载失败，请重试")
		return
	}
	c.Header("Cache-Control", "no-store")
	if admin {
		result := make([]voyageAdminIssue, 0, len(issues))
		for _, issue := range issues {
			result = append(result, voyageAdminIssue{issue, voyageIssueMaterials(issue)})
		}
		Success(c, result)
		return
	}
	Success(c, issues)
}
func (v *VoyageController) List(c *gin.Context)      { v.list(c, false) }
func (v *VoyageController) AdminList(c *gin.Context) { v.list(c, true) }

func (v *VoyageController) Save(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 512<<10)
	var request struct {
		Kind         string                `json:"kind"`
		Month        string                `json:"month"`
		Images       []string              `json:"images"`
		AssetImages  []string              `json:"asset_images"`
		ShippingNote string                `json:"shipping_note"`
		Title        string                `json:"title"`
		Note         string                `json:"note"`
		Published    bool                  `json:"published"`
		Version      int                   `json:"version"`
		Gifts        []database.VoyageGift `json:"gifts"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, 400, "内容格式无效，请检查后重试")
		return
	}
	request.Title = strings.TrimSpace(request.Title)
	if request.Kind != "guard" && request.Kind != "lottery" {
		Fail(c, 400, "请从月份下创建大航海回馈或抽奖活动")
		return
	}
	if request.Title == "" || utf8.RuneCountInString(request.Title) > 80 || utf8.RuneCountInString(request.Note) > 3000 || len(request.Gifts) > 100 {
		Fail(c, 400, "请填写名称（最多 80 字），介绍最多 3000 字，最多 100 件礼物或奖品")
		return
	}
	if request.Published && len(request.Gifts) == 0 {
		Fail(c, 400, "请至少添加一件礼物再发布")
		return
	}

	if _, err := time.Parse("2006-01", request.Month); err != nil || request.Month < "0001-01" {
		Fail(c, 400, "请选择所属月份")
		return
	}
	if len(request.Images) > 24 || utf8.RuneCountInString(request.ShippingNote) > 3000 {
		Fail(c, 400, "海报最多 24 张，发货说明最多 3000 字")
		return
	}
	if len(request.AssetImages) > 2000 {
		Fail(c, 400, "本期图片素材最多 2000 张")
		return
	}
	for _, image := range request.AssetImages {
		if !v.validImage(image) {
			Fail(c, 400, "本期图片素材不存在，请重新上传")
			return
		}
	}
	for _, image := range request.Images {
		if !v.validImage(image) {
			Fail(c, 400, "海报图片不存在，请重新上传")
			return
		}
	}
	if request.Images == nil {
		request.Images = []string{}
	}
	for i := range request.Gifts {
		gift := &request.Gifts[i]
		gift.Event = ""
		if request.Kind == "guard" {
			gift.Section = "guard"
			gift.Audience = ""
		} else {
			gift.Section = "other"
			if len(gift.Tiers) != 0 {
				Fail(c, 400, "抽奖活动不关联大航海档位，请刷新页面后重试")
				return
			}
		}
		if gift.Tiers == nil {
			gift.Tiers = []string{}
		}
		if gift.Images == nil {
			gift.Images = []string{}
		}
		gift.Name = strings.TrimSpace(gift.Name)
		if gift.Name == "" || utf8.RuneCountInString(gift.Name) > 80 || utf8.RuneCountInString(gift.Description) > 2000 || (gift.Section == "guard" && len(gift.Tiers) < 1) || len(gift.Tiers) > 3 || len(gift.Images) > 6 || utf8.RuneCountInString(gift.Event) > 80 || utf8.RuneCountInString(gift.Audience) > 160 || utf8.RuneCountInString(gift.ShippingNote) > 1000 {
			Fail(c, 400, "请填写礼物名称；航海回馈需选择档位。每项最多 6 张图片，说明最多 2000 字")
			return
		}
		seen := map[string]bool{}
		for _, tier := range gift.Tiers {
			if !slices.Contains([]string{"captain", "admiral", "governor"}, tier) || seen[tier] {
				Fail(c, 400, "请选择有效且不重复的航海档位")
				return
			}
			seen[tier] = true
		}
		for _, image := range gift.Images {
			name := strings.TrimPrefix(image, "/api/voyage-assets/")
			if !strings.HasPrefix(image, "/api/voyage-assets/") || !backgroundFilename.MatchString(name) {
				Fail(c, 400, "请使用已上传的礼物图片")
				return
			}
			if info, err := v.images().assets().Stat(context.Background(), name); err != nil || !info.Mode().IsRegular() {
				Fail(c, 400, "礼物图片不存在，请重新上传")
				return
			}
		}
	}
	if request.Gifts == nil {
		request.Gifts = []database.VoyageGift{}
	}
	issue := database.VoyageIssue{Kind: request.Kind, Month: request.Month, Images: request.Images, AssetImages: request.AssetImages, ShippingNote: request.ShippingNote, Title: request.Title, Note: request.Note, Published: request.Published, Gifts: request.Gifts, Version: 1}
	if raw := c.Param("id"); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || id == 0 {
			Fail(c, 400, "期次无效")
			return
		}
		var existing database.VoyageIssue
		if err := database.DB.First(&existing, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				Fail(c, 404, "本期已被删除，请刷新后重试")
			} else {
				Fail(c, 500, "读取失败，请重试")
			}
			return
		}

		if existing.Kind == "batch" {
			Fail(c, 409, "请从收集与发货管理批次")
			return
		}
		if existing.Kind != request.Kind {
			Fail(c, 409, "回馈与抽奖活动分别管理，请刷新后从对应月份打开")
			return
		}
		issue.AssetImages = append(issue.AssetImages, voyageIssueMaterials(existing)...)
		if !v.retainMaterials(c, &issue) {
			return
		}
		issue.ID = existing.ID
		issue.CreatedAt = existing.CreatedAt
		issue.Version = request.Version + 1
		result := database.DB.Model(&database.VoyageIssue{}).Where("id = ? AND version = ?", id, request.Version).Select("kind", "month", "images", "asset_images", "shipping_note", "title", "note", "published", "gifts", "version", "updated_at").Updates(&issue)
		if result.Error != nil {
			Fail(c, 500, "保存失败，请重试")
			return
		}
		if result.RowsAffected == 0 {
			Fail(c, 409, "本期已被其他页面修改，请重新加载后再编辑")
			return
		}
	} else {
		if !v.retainMaterials(c, &issue) {
			return
		}
		if err := database.DB.Create(&issue).Error; err != nil {
			Fail(c, 500, "创建失败，请重试")
			return
		}
	}
	Success(c, voyageAdminIssue{issue, issue.AssetImages})
}

func (v *VoyageController) retainMaterials(c *gin.Context, issue *database.VoyageIssue) bool {
	retained := []string{}
	for _, image := range voyageIssueMaterials(*issue) {
		if v.validImage(image) {
			retained = append(retained, image)
		}
	}
	if len(retained) > 2000 {
		Fail(c, 400, "本期图片素材最多 2000 张")
		return false
	}
	issue.AssetImages = retained
	return true
}

func (v *VoyageController) validImage(image string) bool {
	name := strings.TrimPrefix(image, "/api/voyage-assets/")
	if !strings.HasPrefix(image, "/api/voyage-assets/") || !backgroundFilename.MatchString(name) {
		return false
	}
	info, err := v.images().assets().Stat(context.Background(), name)
	return err == nil && info.Mode().IsRegular()
}

func (*VoyageController) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		Fail(c, 400, "期次无效")
		return
	}
	blocked := false
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		if _, ok, e := batchFor(tx, uint(id)); e != nil {
			return e
		} else if ok {
			blocked = true
			return errors.New("issue is a batch credential scope")
		}
		var count int64
		if err := tx.Model(&database.VoyageRecipient{}).Where("issue_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			blocked = true
			return errors.New("issue has recipients")
		}
		if err := tx.Where("issue_id = ?", id).Delete(&database.VoyageDispatchConfig{}).Error; err != nil {
			return err
		}
		return tx.Delete(&database.VoyageIssue{}, id).Error
	})
	if err != nil {
		if blocked {
			Fail(c, 409, "本期仍有收件记录，不能删除；可以取消发布以隐藏展示")
		} else {
			Fail(c, 500, "删除失败，请重试")
		}
		return
	}
	Success(c, nil)
}
