package controller

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"mofufans/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

const (
	websiteAssetMaxFiles        = 8
	websiteAssetMaxFileBytes    = int64(512 << 20)
	websiteAssetMaxRequestBytes = int64(520 << 20)
	websiteAssetMaxDescription  = 4000
	websiteAssetMaxInstructions = 4000
)

type WebsiteAssetController struct {
	Directory string
}

type websiteAssetList struct {
	Items          []database.WebsiteAssetRequest `json:"items"`
	Total          int64                          `json:"total"`
	PendingCount   int64                          `json:"pending_count"`
	CompletedCount int64                          `json:"completed_count"`
}

func (ctl *WebsiteAssetController) directory() string {
	if ctl.Directory != "" {
		return ctl.Directory
	}
	if configured := viper.GetString("website_asset_directory"); configured != "" {
		return configured
	}
	if sqlitePath := viper.GetString("sqlite"); sqlitePath != "" && sqlitePath != ":memory:" {
		return filepath.Join(filepath.Dir(sqlitePath), "website-assets")
	}
	return "data/website-assets"
}

func cleanUploadName(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	value = strings.TrimSpace(path.Base(value))
	if !utf8.ValidString(value) || value == "." || value == "" || strings.ContainsFunc(value, unicode.IsControl) {
		return "未命名文件"
	}
	runes := []rune(value)
	if len(runes) > 255 {
		value = string(runes[:255])
	}
	return value
}

func randomAssetName() (string, error) {
	value := make([]byte, 20)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func websiteAssetDescription(value string) (string, bool) {
	value = strings.TrimSpace(value)
	return value, value != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= websiteAssetMaxDescription && !strings.ContainsFunc(value, func(r rune) bool {
		return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t'
	})
}

func (ctl *WebsiteAssetController) List(c *gin.Context) {
	status := c.DefaultQuery("status", "pending")
	if status != "all" && status != database.WebsiteAssetPending && status != database.WebsiteAssetCompleted {
		Fail(c, 400, "完成状态无效")
		return
	}
	query := database.DB.Model(&database.WebsiteAssetRequest{})
	if status != "all" {
		query = query.Where("status = ?", status)
	}
	var result websiteAssetList
	if err := query.Count(&result.Total).Error; err != nil {
		Fail(c, 500, "需求记录读取失败")
		return
	}
	if err := database.DB.Model(&database.WebsiteAssetRequest{}).Where("status = ?", database.WebsiteAssetPending).Count(&result.PendingCount).Error; err != nil {
		Fail(c, 500, "需求记录读取失败")
		return
	}
	if err := database.DB.Model(&database.WebsiteAssetRequest{}).Where("status = ?", database.WebsiteAssetCompleted).Count(&result.CompletedCount).Error; err != nil {
		Fail(c, 500, "需求记录读取失败")
		return
	}
	order := "CASE WHEN status = 'pending' THEN 0 ELSE 1 END, created_at DESC, id DESC"
	if err := query.Preload("Files").Order(order).Limit(200).Find(&result.Items).Error; err != nil {
		Fail(c, 500, "需求记录读取失败")
		return
	}
	Success(c, result)
}

func saveWebsiteAssetFile(directory string, headerName string, source io.Reader) (database.WebsiteAssetFile, error) {
	if err := os.MkdirAll(directory, 0750); err != nil {
		return database.WebsiteAssetFile{}, err
	}
	storedName, err := randomAssetName()
	if err != nil {
		return database.WebsiteAssetFile{}, err
	}
	temp, err := os.CreateTemp(directory, ".upload-*")
	if err != nil {
		return database.WebsiteAssetFile{}, err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	written, copyErr := io.Copy(temp, io.LimitReader(source, websiteAssetMaxFileBytes+1))
	closeErr := temp.Close()
	if copyErr != nil || closeErr != nil {
		return database.WebsiteAssetFile{}, errors.New("copy upload")
	}
	if written < 1 || written > websiteAssetMaxFileBytes {
		return database.WebsiteAssetFile{}, errors.New("invalid upload size")
	}
	probe := make([]byte, 512)
	file, err := os.Open(tempName)
	if err != nil {
		return database.WebsiteAssetFile{}, err
	}
	n, readErr := file.Read(probe)
	_ = file.Close()
	if readErr != nil && readErr != io.EOF {
		return database.WebsiteAssetFile{}, readErr
	}
	if err := os.Rename(tempName, filepath.Join(directory, storedName)); err != nil {
		return database.WebsiteAssetFile{}, err
	}
	return database.WebsiteAssetFile{
		OriginalName: cleanUploadName(headerName),
		StoredName:   storedName,
		ContentType:  http.DetectContentType(probe[:n]),
		Size:         written,
	}, nil
}

func (ctl *WebsiteAssetController) Create(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, websiteAssetMaxRequestBytes)
	form, err := c.MultipartForm()
	if err != nil {
		Fail(c, 400, "发布内容过大或格式无效")
		return
	}
	defer form.RemoveAll()
	description, valid := websiteAssetDescription(c.PostForm("description"))
	files := form.File["files"]
	if !valid {
		Fail(c, 400, "请填写 1–4000 字的需求内容")
		return
	}
	if len(files) > websiteAssetMaxFiles {
		Fail(c, 400, "每次最多上传 8 个附件")
		return
	}
	var declaredTotal int64
	for _, header := range files {
		if header.Size < 1 || header.Size > websiteAssetMaxFileBytes {
			Fail(c, 400, "附件不能为空文件，单个文件不能超过 512 MB")
			return
		}
		declaredTotal += header.Size
	}
	if declaredTotal > websiteAssetMaxFileBytes {
		Fail(c, 400, "单次上传总大小不能超过 512 MB")
		return
	}
	actor := c.MustGet("user").(database.Admin)
	request := database.WebsiteAssetRequest{
		Description:     description,
		Status:          database.WebsiteAssetPending,
		SubmittedByID:   actor.ID,
		SubmittedByName: actor.Username,
	}
	stored := make([]database.WebsiteAssetFile, 0, len(files))
	cleanup := func() {
		for _, file := range stored {
			_ = os.Remove(filepath.Join(ctl.directory(), file.StoredName))
		}
	}
	var actualTotal int64
	for _, header := range files {
		source, err := header.Open()
		if err != nil {
			cleanup()
			Fail(c, 400, "文件读取失败，请重新选择")
			return
		}
		file, saveErr := saveWebsiteAssetFile(ctl.directory(), header.Filename, source)
		_ = source.Close()
		if saveErr != nil {
			cleanup()
			if saveErr.Error() == "invalid upload size" {
				Fail(c, 400, "文件为空或单次上传超过 512 MB")
			} else {
				Fail(c, 500, "文件保存失败，请重试")
			}
			return
		}
		actualTotal += file.Size
		if actualTotal > websiteAssetMaxFileBytes {
			stored = append(stored, file)
			cleanup()
			Fail(c, 400, "单次上传总大小不能超过 512 MB")
			return
		}
		stored = append(stored, file)
	}
	request.Files = stored
	if err := database.DB.Create(&request).Error; err != nil {
		cleanup()
		logrus.Errorf("failed to save website asset request: %v", err)
		Fail(c, 500, "需求保存失败，请重试")
		return
	}
	Success(c, request)
}

func parseWebsiteAssetID(c *gin.Context) (uint, bool) {
	value, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || value == 0 {
		Fail(c, 400, "记录编号无效")
		return 0, false
	}
	return uint(value), true
}

func (ctl *WebsiteAssetController) Update(c *gin.Context) {
	id, ok := parseWebsiteAssetID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var body struct {
		Description *string `json:"description"`
		Status      *string `json:"status"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || (body.Description == nil && body.Status == nil) {
		Fail(c, 400, "没有需要保存的修改")
		return
	}
	actor := c.MustGet("user").(database.Admin)
	var request database.WebsiteAssetRequest
	if err := database.DB.Preload("Files").First(&request, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, 404, "需求记录不存在")
		} else {
			Fail(c, 500, "需求记录读取失败")
		}
		return
	}
	updates := map[string]any{}
	if body.Description != nil {
		description, valid := websiteAssetDescription(*body.Description)
		if !valid {
			Fail(c, 400, "请填写 1–4000 字的需求内容")
			return
		}
		if !actor.IsSuperAdmin && actor.ID != request.SubmittedByID {
			Fail(c, 403, "只能修改自己提交的需求")
			return
		}
		updates["description"] = description
	}
	if body.Status != nil {
		if !actor.IsSuperAdmin {
			Fail(c, 403, "只有超级管理员可以修改完成情况")
			return
		}
		if *body.Status != database.WebsiteAssetPending && *body.Status != database.WebsiteAssetCompleted {
			Fail(c, 400, "完成状态无效")
			return
		}
		updates["status"] = *body.Status
		if *body.Status == database.WebsiteAssetCompleted {
			now := time.Now()
			updates["completed_at"] = &now
			updates["completed_by_id"] = actor.ID
			updates["completed_by_name"] = actor.Username
		} else {
			updates["completed_at"] = nil
			updates["completed_by_id"] = nil
			updates["completed_by_name"] = ""
		}
	}
	if err := database.DB.Model(&request).Updates(updates).Error; err != nil {
		Fail(c, 500, "需求保存失败")
		return
	}
	if err := database.DB.Preload("Files").First(&request, id).Error; err != nil {
		Fail(c, 500, "需求记录读取失败")
		return
	}
	Success(c, request)
}

func (ctl *WebsiteAssetController) Delete(c *gin.Context) {
	id, ok := parseWebsiteAssetID(c)
	if !ok {
		return
	}
	actor := c.MustGet("user").(database.Admin)
	if !actor.IsSuperAdmin {
		Fail(c, 403, "只有超级管理员可以删除需求记录")
		return
	}
	var request database.WebsiteAssetRequest
	if err := database.DB.Preload("Files").First(&request, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, 404, "需求记录不存在")
		} else {
			Fail(c, 500, "需求记录读取失败")
		}
		return
	}
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("request_id = ?", request.ID).Delete(&database.WebsiteAssetFile{}).Error; err != nil {
			return err
		}
		return tx.Delete(&request).Error
	}); err != nil {
		Fail(c, 500, "需求记录删除失败")
		return
	}
	for _, file := range request.Files {
		if err := os.Remove(filepath.Join(ctl.directory(), file.StoredName)); err != nil && !errors.Is(err, os.ErrNotExist) {
			logrus.Warnf("failed to remove website asset file %s: %v", file.StoredName, err)
		}
	}
	Success(c, nil)
}

func (ctl *WebsiteAssetController) Download(c *gin.Context) {
	id, ok := parseWebsiteAssetID(c)
	if !ok {
		return
	}
	var file database.WebsiteAssetFile
	if err := database.DB.First(&file, id).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	target := filepath.Join(ctl.directory(), file.StoredName)
	info, err := os.Stat(target)
	if err != nil || !info.Mode().IsRegular() {
		c.Status(http.StatusNotFound)
		return
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": file.OriginalName})
	c.Header("Content-Disposition", disposition)
	c.Header("Content-Type", "application/octet-stream")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, no-store")
	c.File(target)
}

func (*WebsiteAssetController) GetSettings(c *gin.Context) {
	var settings database.WebsiteAssetSettings
	if err := database.DB.First(&settings).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			settings.Instructions = database.WebsiteAssetDefaultInstructions
			if createErr := database.DB.Create(&settings).Error; createErr != nil {
				Fail(c, 500, "页面说明读取失败")
				return
			}
		} else {
			Fail(c, 500, "页面说明读取失败")
			return
		}
	}
	Success(c, settings)
}

func (*WebsiteAssetController) PutSettings(c *gin.Context) {
	actor := c.MustGet("user").(database.Admin)
	if !actor.IsSuperAdmin {
		Fail(c, 403, "只有超级管理员可以修改页面说明")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var body struct {
		Instructions string `json:"instructions"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		Fail(c, 400, "页面说明格式无效")
		return
	}
	value := strings.TrimSpace(body.Instructions)
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > websiteAssetMaxInstructions || strings.ContainsFunc(value, func(r rune) bool {
		return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t'
	}) {
		Fail(c, 400, "页面说明需为 1–4000 字")
		return
	}
	var settings database.WebsiteAssetSettings
	if err := database.DB.First(&settings).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, 500, "页面说明读取失败")
		return
	}
	settings.Instructions = value
	settings.UpdatedByID = actor.ID
	settings.UpdatedByName = actor.Username
	if err := database.DB.Save(&settings).Error; err != nil {
		Fail(c, 500, "页面说明保存失败")
		return
	}
	Success(c, settings)
}
