package controller

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	_ "golang.org/x/image/webp"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mofufans/internal/adminauth"
	"mofufans/internal/database"
	"mofufans/internal/imagevariant"
	"mofufans/internal/storage"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const galleryPrefix = "/api/gallery/assets/"
const galleryMaxFile = 12 << 20
const galleryMaxTotal = 36 << 20

var galleryFilename = regexp.MustCompile(`^[a-f0-9]{32}\.(jpg|png|webp|gif)$`)

type GalleryController struct {
	Directory string
	mu        sync.Mutex
	limits    map[string]*claimLimit
	global    *rate.Limiter
	slots     chan struct{}
}

func (ctl *GalleryController) directory() string {
	if ctl.Directory != "" {
		return ctl.Directory
	}
	if dir := viper.GetString("gallery_directory"); dir != "" {
		return dir
	}
	return "data/gallery"
}
func (ctl *GalleryController) acquire(c *gin.Context, admin bool) bool {
	ctl.mu.Lock()
	defer ctl.mu.Unlock()
	if ctl.slots == nil {
		ctl.slots = make(chan struct{}, 2)
		ctl.limits = map[string]*claimLimit{}
		ctl.global = rate.NewLimiter(rate.Every(2*time.Second), 10)
	}
	if !admin {
		now := time.Now()
		ip := c.ClientIP()
		entry := ctl.limits[ip]
		if entry == nil {
			if len(ctl.limits) >= 10000 {
				for k, v := range ctl.limits {
					if now.Sub(v.seen) > 10*time.Minute {
						delete(ctl.limits, k)
					}
				}
				if len(ctl.limits) >= 10000 {
					return false
				}
			}
			entry = &claimLimit{limiter: rate.NewLimiter(rate.Every(time.Minute), 3)}
			ctl.limits[ip] = entry
		}
		entry.seen = now
		if !entry.limiter.Allow() || !ctl.global.Allow() {
			return false
		}
	}
	select {
	case ctl.slots <- struct{}{}:
		return true
	default:
		return false
	}
}
func galleryURLs(rows []database.GalleryWork) {
	for i := range rows {
		for j := range rows[i].Images {
			rows[i].Images[j].URL = galleryPrefix + rows[i].Images[j].Filename
		}
	}
}
func (ctl *GalleryController) List(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	admin := c.GetBool("authed")
	page, size := libraryPage(c)
	query := database.DB.Model(&database.GalleryWork{})
	if source := c.Query("source"); source == "streamer" || source == "fan" {
		query = query.Where("source = ?", source)
	}
	counts := map[string]int64{"pending": 0, "published": 0, "rejected": 0}
	if !admin {
		query = query.Where("status = ?", "published")
	} else {
		for status := range counts {
			var count int64
			if query.Session(&gorm.Session{}).Where("status = ?", status).Count(&count).Error != nil {
				Fail(c, 500, "画廊加载失败")
				return
			}
			counts[status] = count
		}
		if status := c.Query("status"); status != "" {
			if _, ok := counts[status]; !ok {
				Fail(c, 400, "审核状态无效")
				return
			}
			query = query.Where("status = ?", status)
		}
	}
	var total int64
	rows := []database.GalleryWork{}
	if query.Session(&gorm.Session{}).Count(&total).Error != nil || query.Preload("Images", func(tx *gorm.DB) *gorm.DB { return tx.Order("id asc") }).Order("id desc").Offset((page-1)*size).Limit(size).Find(&rows).Error != nil {
		Fail(c, 500, "画廊加载失败，请重试")
		return
	}
	galleryURLs(rows)
	updated, err := libraryLastUpdated(database.DB.Model(&database.GalleryWork{}).Where("status = ?", "published"))
	if err != nil {
		Fail(c, 500, "画廊加载失败，请重试")
		return
	}
	result := gin.H{"items": rows, "total": total, "page": page, "page_size": size, "last_updated_at": updated}
	if admin {
		result["counts"] = counts
	}
	Success(c, result)
}

// One multipart request creates the work and its files. Public requests never
// accept a source/status override; even an admin posting here is a fan submission.
func (ctl *GalleryController) Create(c *gin.Context) {
	if !libraryRequest(c, galleryMaxTotal+(1<<20)) {
		return
	}
	admin := c.GetBool("authed")
	if !ctl.acquire(c, admin) {
		Fail(c, 429, "上传较频繁，请稍后再试")
		return
	}
	defer func() { <-ctl.slots }()
	err := c.Request.ParseMultipartForm(1 << 20)
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	if err != nil {
		Fail(c, 400, "图片总大小不能超过 36 MB")
		return
	}
	title := strings.TrimSpace(c.PostForm("title"))
	files := c.Request.MultipartForm.File["images"]
	if utf8.RuneCountInString(title) > 100 || len(files) < 1 || len(files) > 9 {
		Fail(c, 400, "作品名最多 100 字，每次上传 1–9 张图片")
		return
	}
	var total int64
	for _, file := range files {
		total += file.Size
		if file.Size < 1 || file.Size > galleryMaxFile {
			Fail(c, 400, "每张图片不能超过 12 MB")
			return
		}
	}
	if total > galleryMaxTotal {
		Fail(c, 400, "图片总大小不能超过 36 MB")
		return
	}
	paths := []string{}
	saved := false
	defer func() {
		if !saved {
			for _, path := range paths {
				_ = ctl.assets().Delete(c.Request.Context(), path)
			}
		}
	}()
	row := database.GalleryWork{Title: title, Source: "fan", Status: "pending", Version: 1, Images: []database.GalleryImage{}}
	if admin {
		row.Source = "streamer"
		row.Status = "published"
	}
	var existing database.GalleryWork
	if admin {
		row.Credit = strings.TrimSpace(c.PostForm("credit"))
		row.SourceURL = strings.TrimSpace(c.PostForm("source_url"))
		if utf8.RuneCountInString(row.Credit) > 300 || !gallerySourceValid(row.SourceURL) {
			Fail(c, 400, "署名最多 300 字，来源请填写 B 站链接")
			return
		}
		if c.PostForm("source") == "fan" {
			row.Source = "fan"
		}
		if c.Param("id") != "" {
			id, ok := libraryID(c)
			if !ok {
				return
			}
			version, _ := strconv.Atoi(c.PostForm("version"))
			if database.DB.Preload("Images").First(&existing, id).Error != nil {
				Fail(c, 404, "作品不存在")
				return
			}
			if version != existing.Version {
				Fail(c, 409, "作品已更新，请刷新后再添加")
				return
			}
			if len(existing.Images)+len(files) > 24 {
				Fail(c, 400, "同一作品最多 24 张图片")
				return
			}
		}
	}
	for _, file := range files {
		f, err := file.Open()
		if err != nil {
			Fail(c, 400, "图片读取失败")
			return
		}
		data, err := io.ReadAll(io.LimitReader(f, galleryMaxFile+1))
		_ = f.Close()
		if err != nil || len(data) > galleryMaxFile {
			Fail(c, 400, "图片读取失败或过大")
			return
		}
		info, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || (format != "jpeg" && format != "png" && format != "webp" && format != "gif") {
			Fail(c, 400, "请上传 JPG、PNG、WebP 或 GIF 图片")
			return
		}
		if info.Width < 1 || info.Height < 1 || info.Width > 8192 || info.Height > 8192 || int64(info.Width)*int64(info.Height) > 32000000 {
			Fail(c, 400, "图片边长最多 8192 像素，总像素最多 3200 万")
			return
		}
		if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
			Fail(c, 400, "图片文件不完整，请重新选择")
			return
		}
		if format == "jpeg" {
			format = "jpg"
		}
		var token [16]byte
		if _, err = rand.Read(token[:]); err != nil {
			Fail(c, 500, "图片保存失败")
			return
		}
		name := fmt.Sprintf("%x.%s", token, format)
		if err := ctl.assets().Put(c.Request.Context(), name, bytes.NewReader(data)); err != nil {
			Fail(c, 500, "图片保存失败，请重试")
			return
		}
		paths = append(paths, name)

		row.Images = append(row.Images, database.GalleryImage{Filename: name, Width: info.Width, Height: info.Height})
	}
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		if existing.ID == 0 {
			return tx.Create(&row).Error
		}
		changed := tx.Model(&database.GalleryWork{}).Where("id = ? AND version = ?", existing.ID, existing.Version).Update("version", existing.Version+1)
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return errors.New("conflict")
		}
		for i := range row.Images {
			row.Images[i].WorkID = existing.ID
		}
		if err := tx.Create(&row.Images).Error; err != nil {
			return err
		}
		existing.Images = append(existing.Images, row.Images...)
		existing.Version++
		row = existing
		return nil
	})
	if err != nil {
		if err.Error() == "conflict" {
			Fail(c, 409, "作品已更新，请刷新后再添加")
			return
		}
		Fail(c, 500, "作品保存失败，请重试")
		return
	}
	saved = true
	// Pending image URLs are not exposed to the submitter.
	if admin {
		rows := []database.GalleryWork{row}
		galleryURLs(rows)
		Success(c, rows[0])
	} else {
		Success(c, gin.H{"id": row.ID, "status": "pending"})
	}
}
func gallerySourceValid(raw string) bool {
	if raw == "" {
		return true
	}
	if len(raw) > 500 {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.User == nil && (u.Host == "t.bilibili.com" || u.Host == "www.bilibili.com" || u.Host == "space.bilibili.com")
}
func (*GalleryController) Update(c *gin.Context) {
	if !libraryRequest(c, 16<<10) {
		return
	}
	id, ok := libraryID(c)
	if !ok {
		return
	}
	var input struct {
		SourceURL *string `json:"source_url"`
		Credit    *string `json:"credit"`
		Title     *string `json:"title"`
		Status    string  `json:"status"`
		Version   int     `json:"version"`
	}
	if c.ShouldBindJSON(&input) != nil || input.Version < 1 {
		Fail(c, 400, "作品格式无效")
		return
	}
	updates := map[string]any{"version": input.Version + 1}
	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if utf8.RuneCountInString(title) > 100 {
			Fail(c, 400, "作品名最多 100 字")
			return
		}
		updates["title"] = title
	}
	if input.SourceURL != nil {
		if !gallerySourceValid(*input.SourceURL) {
			Fail(c, 400, "请填写有效的 B 站来源链接")
			return
		}
		updates["source_url"] = *input.SourceURL
	}
	if input.Credit != nil {
		if utf8.RuneCountInString(*input.Credit) > 300 {
			Fail(c, 400, "署名最多 300 字")
			return
		}
		updates["credit"] = strings.TrimSpace(*input.Credit)
	}
	if input.Status != "" {
		if input.Status != "published" && input.Status != "rejected" && input.Status != "pending" {
			Fail(c, 400, "审核状态无效")
			return
		}
		updates["status"] = input.Status
	}
	result := database.DB.Model(&database.GalleryWork{}).Where("id = ? AND version = ?", id, input.Version).Updates(updates)
	if result.Error != nil {
		Fail(c, 500, "保存失败，请重试")
		return
	}
	if result.RowsAffected == 0 {
		Fail(c, 409, "作品已更新或删除，请刷新列表后再操作")
		return
	}
	Success(c, nil)
}
func (ctl *GalleryController) Delete(c *gin.Context) {
	if !libraryRequest(c, 1024) {
		return
	}
	id, ok := libraryID(c)
	if !ok {
		return
	}
	images := []database.GalleryImage{}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("work_id = ?", id).Find(&images).Error; err != nil {
			return err
		}
		if err := tx.Where("work_id = ?", id).Delete(&database.GalleryImage{}).Error; err != nil {
			return err
		}
		return tx.Delete(&database.GalleryWork{}, id).Error
	})
	if err != nil {
		Fail(c, 500, "删除失败，请重试")
		return
	}
	for _, img := range images {
		if galleryFilename.MatchString(img.Filename) {
			imagevariant.Remove(filepath.Join(ctl.directory(), img.Filename))
			_ = ctl.assets().Delete(c.Request.Context(), img.Filename)
		}
	}
	Success(c, nil)
}
func (ctl *GalleryController) File(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	name := c.Param("filename")
	if !galleryFilename.MatchString(name) {
		c.Status(404)
		return
	}
	var img database.GalleryImage
	if database.DB.Where("filename = ?", name).First(&img).Error != nil {
		c.Status(404)
		return
	}
	var work database.GalleryWork
	if database.DB.First(&work, img.WorkID).Error != nil {
		c.Status(404)
		return
	}
	if work.Status != "published" {
		if _, ok := adminauth.Current(c); !ok {
			c.Status(404)
			return
		}
	}
	path, err := ctl.assets().Materialize(c.Request.Context(), name, galleryMaxFile)
	if err != nil {
		c.Status(404)
		return
	}
	if variant := c.Query("variant"); variant != "" {
		if variant != "small-v1" && variant != "thumb-v1" && variant != "preview-v1" {
			c.Status(400)
			return
		}
		var err error
		path, err = imagevariant.File(c.Request.Context(), path, variant)
		if err != nil {
			c.Status(503)
			return
		}
	}
	// Revalidate against the work's current publication status on every visit.
	// Browsers may reuse the bytes after a 304, but shared caches must not retain them.
	c.Header("Cache-Control", "private, no-cache")
	c.Header("ETag", fmt.Sprintf("%q", filepath.Base(path)))
	c.File(path)
}

func (ctl *GalleryController) assets() *storage.Assets {
	return storage.NewAssets("gallery", ctl.directory())
}
