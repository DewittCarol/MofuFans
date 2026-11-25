package database

import (
	_ "embed"
	"encoding/json"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type EmojiCollection struct {
	BaseModel
	Name      string  `gorm:"size:64" json:"name"`
	SystemKey *string `gorm:"size:32;uniqueIndex" json:"-"`
	IsDeleted bool    `gorm:"not null;default:false" json:"-"`
}

type EmojiAsset struct {
	BaseModel
	Kind         string `gorm:"size:16;uniqueIndex:emoji_value" json:"kind"`
	Name         string `gorm:"size:64" json:"name"`
	Value        string `gorm:"size:191;uniqueIndex:emoji_value" json:"value"`
	URL          string `gorm:"size:512" json:"url"`
	IsDeleted    bool   `gorm:"not null;default:false" json:"is_deleted"`
	Builtin      bool   `json:"builtin"`
	CollectionID uint   `gorm:"not null;default:0;index" json:"collection_id"`
	SortOrder    int    `gorm:"not null;default:0" json:"sort_order"`
}

//go:embed emoji-defaults.json
var emojiDefaults []byte

func InitializeEmojiCatalog() error {
	var defaults []EmojiAsset
	if err := json.Unmarshal(emojiDefaults, &defaults); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		key := "legacy-joi"
		collection := EmojiCollection{Name: "常用表情", SystemKey: &key}
		if err := tx.Where("system_key = ?", key).FirstOrCreate(&collection).Error; err != nil {
			return err
		}
		joiKey := "joi-original"
		var joi EmojiCollection
		if err := tx.Where("system_key = ?", joiKey).Limit(1).Find(&joi).Error; err != nil {
			return err
		}
		restoreReactionImages := joi.ID == 0
		if restoreReactionImages {
			joi = EmojiCollection{Name: "轴伊Joi", SystemKey: &joiKey}
			if err := tx.Create(&joi).Error; err != nil {
				return err
			}
		}
		for i := range defaults {
			defaults[i].Builtin = true
			defaults[i].SortOrder = i + 1
			if defaults[i].Kind == "input" {
				defaults[i].CollectionID = collection.ID
				defaults[i].IsDeleted = collection.IsDeleted
				if strings.HasPrefix(defaults[i].URL, "/joi-emojis/") {
					defaults[i].CollectionID = joi.ID
					defaults[i].IsDeleted = joi.IsDeleted
				}
			}
		}
		// Preserve names, removals, stable values and all previously saved ordering.
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "kind"}, {Name: "value"}}, DoNothing: true}).Create(&defaults).Error; err != nil {
			return err
		}
		// The early generic release kept reaction values but omitted their images.
		// Repair only missing builtin URLs once, without changing IDs or user choices.
		if restoreReactionImages {
			for _, item := range defaults {
				if item.Kind != "reaction" || item.URL == "" {
					continue
				}
				if err := tx.Model(&EmojiAsset{}).Where("kind = ? AND value = ? AND builtin = ? AND url = ?", item.Kind, item.Value, true, "").Update("url", item.URL).Error; err != nil {
					return err
				}
			}
		}
		legacyUpdates := map[string]any{"collection_id": collection.ID}
		if collection.IsDeleted {
			legacyUpdates["is_deleted"] = true
		}
		if err := tx.Model(&EmojiAsset{}).Where("kind = ? AND collection_id = 0", "input").Updates(legacyUpdates).Error; err != nil {
			return err
		}
		return tx.Model(&EmojiAsset{}).Where("sort_order = 0").Update("sort_order", gorm.Expr("id")).Error
	})
}

// New uploads append to their own collection, including when older items were removed.
func NextEmojiSort(tx *gorm.DB, kind string, collectionID uint) (int, error) {
	var maximum int
	err := tx.Model(&EmojiAsset{}).Where("kind = ? AND collection_id = ?", kind, collectionID).Select("COALESCE(MAX(sort_order), 0)").Scan(&maximum).Error
	return maximum + 1, err
}
