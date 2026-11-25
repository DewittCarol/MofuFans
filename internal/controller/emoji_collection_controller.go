package controller

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mofufans/internal/database"
)

// Catalog reads both tables in one snapshot. /emojis remains compatible with older clients.
func (*EmojiAssetController) Catalog(c *gin.Context) {
	assets := []database.EmojiAsset{}
	collections := []database.EmojiCollection{}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("is_deleted = ?", false).Order("id asc").Find(&collections).Error; err != nil {
			return err
		}
		return tx.Order("collection_id asc, sort_order asc, id asc").Find(&assets).Error
	})
	if err != nil {
		Fail(c, 500, "表情加载失败，请重试")
		return
	}
	c.Header("Cache-Control", "no-store")
	Success(c, gin.H{"emojis": assets, "collections": collections})
}

func (*EmojiAssetController) SaveCollection(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var request struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, 400, "请输入合集名称")
		return
	}
	name := strings.TrimSpace(request.Name)
	if !validEmojiDisplayName(name) {
		Fail(c, 400, "合集名称需为 1–32 个字，不能包含换行或控制字符")
		return
	}
	var collection database.EmojiCollection
	if raw := c.Param("id"); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || id == 0 {
			Fail(c, 400, "请选择有效的合集")
			return
		}
		result := database.DB.Model(&database.EmojiCollection{}).Where("id = ? AND is_deleted = ?", id, false).Update("name", name)
		if result.Error != nil {
			Fail(c, 500, "合集保存失败，请重试")
			return
		}
		if result.RowsAffected == 0 {
			Fail(c, 404, "合集不存在或已删除，请刷新后重试")
			return
		}
		collection.ID, collection.Name = uint(id), name
		Success(c, collection)
		return
	}
	collection.Name = name
	if err := database.DB.Save(&collection).Error; err != nil {
		Fail(c, 500, "合集保存失败，请重试")
		return
	}
	Success(c, collection)
}

// Keep collection identities (including builtin system keys) and asset metadata
// for historical posts. The startup seed must respect these deletion markers.
func (*EmojiAssetController) DeleteCollection(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		Fail(c, 400, "请选择有效的合集")
		return
	}
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		var collection database.EmojiCollection
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&collection, id).Error; err != nil {
			return err
		}
		if err := tx.Model(&collection).Update("is_deleted", true).Error; err != nil {
			return err
		}
		return tx.Model(&database.EmojiAsset{}).Where("kind = ? AND collection_id = ?", "input", id).Update("is_deleted", true).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, 404, "合集不存在，请刷新后重试")
		return
	}
	if err != nil {
		Fail(c, 500, "合集删除失败，请重试")
		return
	}
	Success(c, nil)
}

var errEmojiCollectionRemoved = errors.New("emoji collection removed")

// Upload, restore and reorder take the same collection lock as deletion so a
// request from a stale admin page cannot add active assets to a removed pack.
func lockActiveEmojiCollection(tx *gorm.DB, id uint) error {
	var collection database.EmojiCollection
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND is_deleted = ?", id, false).First(&collection).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errEmojiCollectionRemoved
	}
	return err
}

var errEmojiOrderChanged = errors.New("emoji catalog changed")
var errEmojiOrderInvalid = errors.New("invalid emoji order")

func (*EmojiAssetController) Reorder(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
	var request struct {
		Kind         string `json:"kind"`
		CollectionID uint   `json:"collection_id"`
		IDs          []uint `json:"ids"`
		ExpectedIDs  []uint `json:"expected_ids"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || (request.Kind != "input" && request.Kind != "reaction") || len(request.IDs) == 0 || len(request.IDs) > 5000 || len(request.ExpectedIDs) != len(request.IDs) || (request.Kind == "reaction" && request.CollectionID != 0) || (request.Kind == "input" && request.CollectionID == 0) {
		Fail(c, 400, "排序参数无效，请刷新后重试")
		return
	}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if request.Kind == "input" {
			if err := lockActiveEmojiCollection(tx, request.CollectionID); err != nil {
				return err
			}
		}
		var assets []database.EmojiAsset
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("kind = ? AND collection_id = ? AND is_deleted = ?", request.Kind, request.CollectionID, false).Order("sort_order asc, id asc").Find(&assets).Error; err != nil {
			return err
		}
		current := make([]uint, len(assets))
		allowed := make(map[uint]bool, len(assets))
		for i, asset := range assets {
			current[i] = asset.ID
			allowed[asset.ID] = true
		}
		if !slices.Equal(current, request.ExpectedIDs) {
			return errEmojiOrderChanged
		}
		for _, id := range request.IDs {
			if !allowed[id] {
				return errEmojiOrderInvalid
			}
			delete(allowed, id)
		}
		if len(allowed) != 0 {
			return errEmojiOrderInvalid
		}
		for i, id := range request.IDs {
			if err := tx.Model(&database.EmojiAsset{}).Where("id = ?", id).Update("sort_order", i+1).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, errEmojiCollectionRemoved) {
		Fail(c, 409, "合集已删除，请刷新后选择其他合集")
		return
	}
	if errors.Is(err, errEmojiOrderChanged) {
		Fail(c, 409, "表情列表已变化，请刷新后重新排序")
		return
	}
	if errors.Is(err, errEmojiOrderInvalid) {
		Fail(c, 400, "排序包含重复或不属于当前合集的表情")
		return
	}
	if err != nil {
		Fail(c, 500, "顺序保存失败，请重试")
		return
	}
	Success(c, nil)
}
