package controller

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"image"
	"io"
	"mofufans/internal/database"
	"mofufans/internal/imagevariant"
	"mofufans/internal/storage"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
)

const maxBackgroundBytes = 12 << 20

var backgroundFilename = regexp.MustCompile(`^[a-f0-9]{64}\.(png|jpg|webp)$`)

type BackgroundController struct {
	Directory, AssetPrefix, Label string
	Private                       bool
}

func (b *BackgroundController) label() string {
	if b.Label != "" {
		return b.Label
	}
	return "背景"
}
func (b *BackgroundController) assetPrefix() string {
	if b.AssetPrefix != "" {
		return b.AssetPrefix
	}
	return "/api/background-assets/"
}

func (b *BackgroundController) directory() string {
	if b.Directory != "" {
		return b.Directory
	}
	if dir := viper.GetString("background_directory"); dir != "" {
		return dir
	}
	return "data/backgrounds"
}

// Upload first; selecting a file does not change the live background until Save.
func (b *BackgroundController) Upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBackgroundBytes+(64<<10))
	if err := c.Request.ParseMultipartForm(maxBackgroundBytes); err != nil {
		Fail(c, 400, b.label()+"图片不能超过 12 MB")
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		Fail(c, 400, "请选择"+b.label()+"图片")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBackgroundBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxBackgroundBytes {
		Fail(c, 400, "图片为空或超过 12 MB")
		return
	}
	info, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg" && format != "webp") {
		Fail(c, 400, "请选择有效的 JPG、PNG 或 WebP 图片")
		return
	}
	if info.Width < 1 || info.Height < 1 || info.Width > 8192 || info.Height > 8192 || int64(info.Width)*int64(info.Height) > 32000000 {
		Fail(c, 400, "图片过大，请缩小至 3200 万像素以内，边长不超过 8192 像素")
		return
	}
	if format == "jpeg" {
		format = "jpg"
	}
	filename := fmt.Sprintf("%x.%s", sha256.Sum256(data), format)
	if err := b.assets().Put(c.Request.Context(), filename, bytes.NewReader(data)); err != nil {
		Fail(c, 500, "图片保存失败，请重试")
		return
	}

	Success(c, gin.H{"url": b.assetPrefix() + filename})
}

func (b *BackgroundController) Put(c *gin.Context) {
	var request struct {
		URL     string `json:"url"`
		Opacity *int   `json:"opacity"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.Opacity == nil || *request.Opacity < 0 || *request.Opacity > 100 {
		Fail(c, 400, "背景清晰度应在 0% 到 100% 之间")
		return
	}
	if request.URL != "" {
		filename := strings.TrimPrefix(request.URL, "/api/background-assets/")
		if !strings.HasPrefix(request.URL, "/api/background-assets/") || !backgroundFilename.MatchString(filename) {
			Fail(c, 400, "请选择已上传的背景图片")
			return
		}
		if file, err := b.assets().Stat(c.Request.Context(), filename); err != nil || !file.Mode().IsRegular() {
			Fail(c, 400, "背景图片不存在，请重新上传")
			return
		}
	}
	var config database.Config
	if err := database.DB.First(&config).Error; err != nil {
		Fail(c, 500, "设置读取失败")
		return
	}
	if err := database.DB.Model(&config).Updates(map[string]any{"background_url": request.URL, "background_opacity": *request.Opacity}).Error; err != nil {
		Fail(c, 500, "背景设置保存失败")
		return
	}
	Success(c, gin.H{"url": request.URL, "opacity": *request.Opacity})
}

func (b *BackgroundController) File(c *gin.Context) {
	filename := c.Param("filename")
	if !backgroundFilename.MatchString(filename) {
		c.Status(404)
		return
	}
	target, err := b.assets().Materialize(c.Request.Context(), filename, maxBackgroundBytes)
	if err != nil {
		c.Status(404)
		return
	}
	if variant := c.Query("variant"); variant != "" {
		if variant != "background-v1" && variant != "background-small-v1" {
			c.Status(400)
			return
		}
		var err error
		target, err = imagevariant.File(c.Request.Context(), target, variant)
		if err != nil {
			c.Status(503)
			return
		}
	}
	if b.Private {
		c.Header("Cache-Control", "private, no-store")
	} else {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("ETag", fmt.Sprintf("%q", filepath.Base(target)))
	c.File(target)
}

func (b *BackgroundController) assets() *storage.Assets {
	namespace := "backgrounds"
	switch b.assetPrefix() {
	case "/api/theme-assets/":
		namespace = "branding"
	case "/api/voyage-assets/":
		namespace = "voyage"
	}
	return storage.NewAssets(namespace, b.directory())
}
