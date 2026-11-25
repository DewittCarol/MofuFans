package controller

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mofufans/internal/storage"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	_ "golang.org/x/image/webp"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mofufans/internal/appearance"
	"mofufans/internal/database"
)

const maxEmojiBytes = 5 << 20

type EmojiAssetController struct {
	// Optional directory for tests; production is independent of attachment/OSS settings.
	Directory string
}

func (controller *EmojiAssetController) directory() string {
	if controller.Directory != "" {
		return controller.Directory
	}
	if configured := viper.GetString("emoji_directory"); configured != "" {
		return configured
	}
	return "data/emojis"
}

func (*EmojiAssetController) List(c *gin.Context) {
	var assets []database.EmojiAsset
	if err := database.DB.Order("collection_id asc, sort_order asc, id asc").Find(&assets).Error; err != nil {
		Fail(c, 500, "表情包加载失败")
		return
	}
	c.Header("Cache-Control", "no-store")
	// Include deleted metadata so historical posts and reaction counts keep rendering.
	Success(c, assets)
}

func emojiName(filename string) string {
	name := path.Base(strings.ReplaceAll(filename, "\\", "/"))
	name = strings.TrimSuffix(name, path.Ext(name))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune("[]<>\"'\\/", r) {
			return -1
		}
		return r
	}, name)
	runes := []rune(strings.TrimSpace(name))
	if len(runes) > 32 {
		runes = runes[:32]
	}
	if len(runes) == 0 {
		return "新表情"
	}
	return string(runes)
}

func (controller *EmojiAssetController) Upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxEmojiBytes+(64<<10))
	if err := c.Request.ParseMultipartForm(maxEmojiBytes + (64 << 10)); err != nil {
		Fail(c, 400, "上传失败：每张图片不能超过 5 MB")
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	kind := c.PostForm("kind")
	if kind != "input" && kind != "reaction" {
		Fail(c, 400, "请选择表情类型")
		return
	}
	var collectionID uint
	if kind == "input" {
		var collection database.EmojiCollection
		query := database.DB.Where("system_key = ?", "legacy-joi")
		if raw := c.PostForm("collection_id"); raw != "" {
			id, err := strconv.ParseUint(raw, 10, 32)
			if err != nil || id == 0 {
				Fail(c, 400, "请选择有效的投稿表情合集")
				return
			}
			query = database.DB.Where("id = ?", id)
		}
		if err := query.Where("is_deleted = ?", false).First(&collection).Error; err != nil {
			Fail(c, 400, "合集不存在，请刷新后重试")
			return
		}
		collectionID = collection.ID
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		Fail(c, 400, "请选择一张图片")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxEmojiBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxEmojiBytes {
		Fail(c, 400, "图片为空或超过 5 MB")
		return
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg" && format != "gif" && format != "webp") {
		Fail(c, 400, "请选择有效的 PNG、JPG、GIF 或 WebP 图片")
		return
	}
	if config.Width < 1 || config.Height < 1 || config.Width > 4096 || config.Height > 4096 {
		Fail(c, 400, "图片边长不能超过 4096 像素")
		return
	}
	// Preserve the original bytes, including transparency and animated GIF/WebP frames.
	ext := format
	if ext == "jpeg" {
		ext = "jpg"
	}
	filename := fmt.Sprintf("%x.%s", sha256.Sum256(data), ext)
	if err := controller.assets().Put(c.Request.Context(), filename, bytes.NewReader(data)); err != nil {
		Fail(c, 500, "图片保存失败，请重试")
		return
	}

	name := emojiName(header.Filename)
	var saved database.EmojiAsset
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		if kind == "input" {
			if err := lockActiveEmojiCollection(tx, collectionID); err != nil {
				return err
			}
		}
		order, err := database.NextEmojiSort(tx, kind, collectionID)
		if err != nil {
			return err
		}
		for suffix := 1; suffix <= 100; suffix++ {
			display := name
			if suffix > 1 {
				display = fmt.Sprintf("%s %d", name, suffix)
			}
			asset := database.EmojiAsset{Kind: kind, Name: display, Value: "[" + display + "]", URL: "/api/emoji-assets/" + filename, CollectionID: collectionID, SortOrder: order}
			result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "kind"}, {Name: "value"}}, DoNothing: true}).Create(&asset)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected > 0 {
				saved = asset
				return nil
			}
		}
		return nil
	})
	if errors.Is(err, errEmojiCollectionRemoved) {
		Fail(c, 409, "合集已删除，请刷新后选择其他合集")
		return
	}
	if err != nil {
		Fail(c, 500, "表情保存失败，请重试")
		return
	}
	if saved.ID != 0 {
		Success(c, saved)
		return
	}
	Fail(c, 400, "同名表情过多，请更改图片文件名后上传")
}

func (*EmojiAssetController) SetStatus(c *gin.Context) {
	var request struct {
		IDs       []uint `json:"ids"`
		IsDeleted *bool  `json:"is_deleted"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || len(request.IDs) == 0 || len(request.IDs) > 500 || request.IsDeleted == nil {
		Fail(c, 400, "请选择需要操作的表情")
		return
	}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if !*request.IsDeleted {
			var collectionIDs []uint
			if err := tx.Model(&database.EmojiAsset{}).Where("id IN ? AND kind = ?", request.IDs, "input").Distinct("collection_id").Order("collection_id asc").Pluck("collection_id", &collectionIDs).Error; err != nil {
				return err
			}
			for _, id := range collectionIDs {
				if err := lockActiveEmojiCollection(tx, id); err != nil {
					return err
				}
			}
		}
		return tx.Model(&database.EmojiAsset{}).Where("id IN ?", request.IDs).Update("is_deleted", *request.IsDeleted).Error
	})
	if errors.Is(err, errEmojiCollectionRemoved) {
		Fail(c, 409, "表情所属合集已删除，无法撤销删除；请刷新后选择其他合集")
		return
	}
	if err != nil {
		Fail(c, 500, "表情修改失败，请重试")
		return
	}
	// Keep files for historical questions; removing a picker option is reversible.
	Success(c, nil)
}

func validEmojiDisplayName(name string) bool {
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 32 {
		return false
	}
	return !strings.ContainsFunc(name, unicode.IsControl)
}

func (*EmojiAssetController) Rename(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var request struct {
		Name string `json:"name"`
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		Fail(c, 400, "请选择需要改名的表情")
		return
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, 400, "请输入表情名称")
		return
	}
	name := strings.TrimSpace(request.Name)
	if !validEmojiDisplayName(name) {
		Fail(c, 400, "名称需为 1–32 个字，不能包含换行或控制字符")
		return
	}
	var asset database.EmojiAsset
	if err := database.DB.First(&asset, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, 404, "表情不存在，请刷新后重试")
		} else {
			Fail(c, 500, "表情读取失败，请重试")
		}
		return
	}
	// Value is a permanent identifier used by question text and reaction counts.
	if err := database.DB.Model(&asset).Update("name", name).Error; err != nil {
		Fail(c, 500, "改名失败，请重试")
		return
	}
	asset.Name = name
	Success(c, asset)
}

func (*EmojiAssetController) AddUnicode(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var request struct {
		Value string `json:"value"`
		Name  string `json:"name"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, 400, "请输入一个 Emoji 表情")
		return
	}
	value, valid := appearance.NormalizeEmoji(request.Value)
	if !valid {
		Fail(c, 400, "请只输入一个 Emoji 表情，例如 🐶、👍🏽 或 ❤️")
		return
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		name = value
	}
	if !validEmojiDisplayName(name) {
		Fail(c, 400, "名称最多 32 个字，不能包含换行或控制字符")
		return
	}
	var existing database.EmojiAsset
	err := database.DB.Where("kind = ? AND REPLACE(value, ?, '') = ?", "reaction", "\ufe0f", strings.ReplaceAll(value, "\ufe0f", "")).First(&existing).Error
	if err == nil {
		if !existing.IsDeleted {
			Fail(c, 409, "已存在同样的互动表情，无需重复添加")
			return
		}
		// Reuse the stable identifier and its counts, now displaying native Emoji.
		result := database.DB.Model(&existing).Where("is_deleted = ?", true).Updates(map[string]any{"name": name, "url": "", "is_deleted": false})
		if result.Error != nil {
			Fail(c, 500, "表情添加失败，请重试")
			return
		}
		if result.RowsAffected == 0 {
			Fail(c, 409, "表情已被恢复，请刷新后查看")
			return
		}
		existing.Name, existing.URL, existing.IsDeleted = name, "", false
		Success(c, existing)
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, 500, "表情读取失败，请重试")
		return
	}
	order, err := database.NextEmojiSort(database.DB, "reaction", 0)
	if err != nil {
		Fail(c, 500, "无法读取表情顺序，请重试")
		return
	}
	asset := database.EmojiAsset{Kind: "reaction", Name: name, Value: value, SortOrder: order}
	result := database.DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "kind"}, {Name: "value"}}, DoNothing: true}).Create(&asset)
	if result.Error != nil {
		Fail(c, 500, "表情添加失败，请重试")
		return
	}
	if result.RowsAffected == 0 {
		Fail(c, 409, "已存在同样的互动表情，无需重复添加")
		return
	}
	Success(c, asset)
}

var emojiFilename = regexp.MustCompile(`^[a-f0-9]{64}\.(png|jpg|gif|webp)$`)

func (controller *EmojiAssetController) File(c *gin.Context) {
	filename := c.Param("filename")
	if !emojiFilename.MatchString(filename) {
		c.Status(http.StatusNotFound)
		return
	}

	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	controller.assets().Serve(c.Writer, c.Request, filename)
}

func (controller *EmojiAssetController) assets() *storage.Assets {
	return storage.NewAssets("emojis", controller.directory())
}
