package database

import (
	"encoding/json"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"path/filepath"
	"testing"
)

type legacyEmojiAsset struct {
	BaseModel
	Kind      string `gorm:"size:16;uniqueIndex:emoji_value"`
	Name      string
	Value     string `gorm:"size:191;uniqueIndex:emoji_value"`
	URL       string
	IsDeleted bool
	Builtin   bool
}

func TestOriginalJoiAssetsUpgradePreservesExistingCatalog(t *testing.T) {
	var err error
	DB, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "restore.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	connection, _ := DB.DB()
	t.Cleanup(func() { connection.Close() })
	if err = DB.AutoMigrate(&EmojiAsset{}, &EmojiCollection{}, &Question{}); err != nil {
		t.Fatal(err)
	}
	key := "legacy-joi"
	common := EmojiCollection{Name: "我的收藏", SystemKey: &key}
	DB.Create(&common)
	old := []EmojiAsset{
		{Kind: "reaction", Value: "👍", Name: "我的点赞", Builtin: true, IsDeleted: true, SortOrder: 77},
		{Kind: "reaction", Value: "👎", Name: "自定义图片", URL: "/custom.png", Builtin: true, SortOrder: 30},
		{Kind: "reaction", Value: "💩", Name: "自己添加的 Emoji", Builtin: false, SortOrder: 90},
		{Kind: "input", Value: "😊", Name: "我的微笑", Builtin: true, CollectionID: common.ID, SortOrder: 80},
		{Kind: "input", Value: "[自定义]", Name: "自定义", URL: "/custom-input.gif", CollectionID: common.ID, SortOrder: 91},
	}
	DB.Create(&old)
	question := Question{Content: "keep content", Emojis: `[{"value":"👍","count":12}]`}
	DB.Create(&question)
	if err = InitializeEmojiCatalog(); err != nil {
		t.Fatal(err)
	}
	for i, before := range old {
		var after EmojiAsset
		DB.First(&after, before.ID)
		wantURL := before.URL
		if i == 0 {
			wantURL = "/emojis/76.png"
		}
		if after.URL != wantURL || after.Kind != before.Kind || after.Value != before.Value || after.Name != before.Name || after.IsDeleted != before.IsDeleted || after.SortOrder != before.SortOrder || after.CollectionID != before.CollectionID {
			t.Fatalf("existing emoji changed unexpectedly: %+v", after)
		}
	}
	DB.First(&common, common.ID)
	if common.Name != "我的收藏" {
		t.Fatal("renamed collection overwritten")
	}
	var pack EmojiCollection
	if err = DB.Where("system_key = ?", "joi-original").First(&pack).Error; err != nil {
		t.Fatal(err)
	}
	var stickers []EmojiAsset
	DB.Where("collection_id = ?", pack.ID).Find(&stickers)
	if len(stickers) != 15 || pack.Name != "轴伊Joi" {
		t.Fatalf("missing original pack: %d", len(stickers))
	}
	DB.Model(&stickers[0]).Updates(map[string]any{"name": "我的轴伊", "is_deleted": true, "sort_order": 100, "collection_id": common.ID})
	DB.Model(&pack).Update("name", "改名的轴伊合集")
	// Choosing a native Unicode reaction after this upgrade must survive later restarts.
	DB.Model(&old[0]).Update("url", "")
	for range 2 {
		if err = InitializeEmojiCatalog(); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	DB.Model(&EmojiAsset{}).Where("kind = ? AND value = ?", "reaction", "👍").Count(&count)
	if count != 1 {
		t.Fatal("reaction identity duplicated")
	}
	var restored EmojiAsset
	DB.First(&restored, old[0].ID)
	if restored.URL != "" {
		t.Fatal("one-time URL repair ran again")
	}
	restored = EmojiAsset{}
	DB.First(&restored, stickers[0].ID)
	if restored.Name != "我的轴伊" || !restored.IsDeleted || restored.SortOrder != 100 || restored.CollectionID != common.ID {
		t.Fatal("Joi edits lost on restart")
	}
	DB.First(&pack, pack.ID)
	if pack.Name != "改名的轴伊合集" {
		t.Fatal("Joi collection name reset")
	}
	DB.First(&question, question.ID)
	if question.Emojis != `[{"value":"👍","count":12}]` {
		t.Fatal("historical reaction counts changed")
	}
	var defaults []EmojiAsset
	if err = json.Unmarshal(emojiDefaults, &defaults); err != nil {
		t.Fatal(err)
	}
	DB.Model(&EmojiAsset{}).Count(&count)
	if count != int64(len(defaults)+1) {
		t.Fatal("restart created duplicate defaults")
	}
}

func (legacyEmojiAsset) TableName() string { return "emoji_assets" }

func TestCollectionUpgradeFromLegacySchema(t *testing.T) {
	var err error
	DB, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := DB.DB()
	t.Cleanup(func() { conn.Close() })
	if err := DB.AutoMigrate(&legacyEmojiAsset{}); err != nil {
		t.Fatal(err)
	}
	legacy := legacyEmojiAsset{Kind: "input", Name: "自定义旧表情", Value: "[old]", URL: "/old.png", IsDeleted: true}
	if err := DB.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.AutoMigrate(&EmojiAsset{}, &EmojiCollection{}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := InitializeEmojiCatalog(); err != nil {
			t.Fatal(err)
		}
	}
	var migrated EmojiAsset
	DB.First(&migrated, legacy.ID)
	if migrated.CollectionID == 0 || migrated.SortOrder != int(legacy.ID) || migrated.Name != legacy.Name || migrated.Value != legacy.Value || migrated.URL != legacy.URL || !migrated.IsDeleted {
		t.Fatalf("migration changed legacy content: %+v", migrated)
	}
	var count int64
	DB.Model(&EmojiCollection{}).Count(&count)
	if count != 2 {
		t.Fatal("migration created duplicate collections")
	}
}

func TestBuiltinCommonPackPreservesAdministratorChanges(t *testing.T) {
	var err error
	DB, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "joi.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := DB.DB()
	t.Cleanup(func() { conn.Close() })
	if err = DB.AutoMigrate(&EmojiAsset{}, &EmojiCollection{}); err != nil {
		t.Fatal(err)
	}
	if err = InitializeEmojiCatalog(); err != nil {
		t.Fatal(err)
	}
	var pack EmojiCollection
	if err = DB.Where("system_key = ?", "legacy-joi").First(&pack).Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	DB.Model(&EmojiAsset{}).Where("kind = ? AND collection_id = ? AND builtin = ?", "input", pack.ID, true).Count(&count)
	if count != 4 {
		t.Fatalf("expected 4 default emoji, got %d", count)
	}
	var asset EmojiAsset
	DB.Where("kind = ?", "input").First(&asset)
	DB.Model(&asset).Updates(map[string]interface{}{"name": "自定义名称", "is_deleted": true, "sort_order": 77})
	DB.Model(&pack).Update("name", "我的收藏")
	if err = InitializeEmojiCatalog(); err != nil {
		t.Fatal(err)
	}
	DB.First(&asset, asset.ID)
	DB.First(&pack, pack.ID)
	if asset.Name != "自定义名称" || !asset.IsDeleted || asset.SortOrder != 77 || pack.Name != "我的收藏" {
		t.Fatal("initialization overwrote administrator choices")
	}
	DB.Model(&EmojiAsset{}).Where("kind = ? AND collection_id = ?", "input", pack.ID).Count(&count)
	if count != 4 {
		t.Fatal("initialization duplicated Joi stickers")
	}
}
