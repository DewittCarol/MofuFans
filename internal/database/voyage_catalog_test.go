package database

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func catalogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "catalog.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	t.Cleanup(func() { sql.Close() })
	if err := db.AutoMigrate(&VoyageIssue{}); err != nil {
		t.Fatal(err)
	}
	return db
}
func catalogSnapshot(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var issues []VoyageIssue
	if err := db.Order("id").Find(&issues).Error; err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(issues)
	return string(raw)
}
func TestMigrateVoyageCatalogsSplitsAndIsIdempotent(t *testing.T) {
	db := catalogTestDB(t)
	original := VoyageIssue{Kind: "catalog", Title: "九月回馈", Month: "2026-09", Version: 4, Published: true, Note: "介绍", ShippingNote: "分批寄出", Images: []string{"poster", "gift"}, Gifts: []VoyageGift{
		{Section: "guard", Name: "徽章", Tiers: []string{"captain", "admiral"}, Images: []string{"gift"}, Audience: "discard", Event: "discard"},
		{Section: "other", Event: "转盘", Name: "立牌", Audience: "中奖者", Tiers: []string{"governor"}},
		{Section: "other", Event: "转盘", Name: "贴纸"},
		{Section: "other", Event: "生日抽奖", Name: "挂件"},
	}}
	if err := db.Create(&original).Error; err != nil {
		t.Fatal(err)
	}
	batch := VoyageIssue{Kind: "batch", Title: "收集批次", Version: 3}
	if err := db.Create(&batch).Error; err != nil {
		t.Fatal(err)
	}
	var batchBefore VoyageIssue
	db.First(&batchBefore, batch.ID)
	if err := MigrateVoyageCatalogs(db); err != nil {
		t.Fatal(err)
	}
	var cards []VoyageIssue
	db.Where("kind <> ?", "batch").Order("id").Find(&cards)
	if len(cards) != 3 {
		t.Fatalf("want guard and two independent activities, got %d", len(cards))
	}
	if cards[0].ID != original.ID || cards[0].Version != 5 || cards[0].Kind != "guard" || len(cards[0].Gifts) != 1 {
		t.Fatalf("original identity or conflict version lost: %+v", cards[0])
	}
	if cards[0].Gifts[0].Audience != "" || cards[0].Gifts[0].Event != "" {
		t.Fatal("activity metadata leaked into voyage")
	}
	if cards[1].Title != "转盘" || len(cards[1].Gifts) != 2 || cards[2].Title != "生日抽奖" {
		t.Fatal("event groups not retained")
	}
	for _, card := range cards {
		if card.Month != "2026-09" || !card.Published || card.Note != original.Note || card.ShippingNote != original.ShippingNote || !reflect.DeepEqual(card.Images, []string{"poster"}) {
			t.Fatal("card content lost or assigned image promoted")
		}
		if card.Kind == "lottery" {
			for _, gift := range card.Gifts {
				if len(gift.Tiers) != 0 || gift.Event != "" || gift.Section != "other" {
					t.Fatal("lottery linked to voyage")
				}
			}
		}
	}
	var unchanged VoyageIssue
	db.First(&unchanged, batch.ID)
	if !reflect.DeepEqual(batchBefore, unchanged) {
		t.Fatal("batch scope was modified")
	}
	before := catalogSnapshot(t, db)
	if err := MigrateVoyageCatalogs(db); err != nil {
		t.Fatal(err)
	}
	if before != catalogSnapshot(t, db) {
		t.Fatal("migration is not idempotent")
	}
}
func TestMigrateVoyageCatalogsRollbackAndMonth(t *testing.T) {
	db := catalogTestDB(t)
	original := VoyageIssue{Kind: "catalog", Title: "旧卡片", Version: 2, Gifts: []VoyageGift{{Name: "舰礼", Tiers: []string{"captain"}}, {Section: "other", Name: "奖品"}}}
	original.CreatedAt = time.Date(2026, 8, 31, 18, 0, 0, 0, time.UTC)
	if err := db.Create(&original).Error; err != nil {
		t.Fatal(err)
	}
	before := catalogSnapshot(t, db)
	db.Callback().Create().Before("gorm:create").Register("test:fail-split", func(tx *gorm.DB) { tx.AddError(errors.New("injected split failure")) })
	if err := MigrateVoyageCatalogs(db); err == nil {
		t.Fatal("expected failure")
	}
	if before != catalogSnapshot(t, db) {
		t.Fatal("partial migration committed")
	}
	db.Callback().Create().Remove("test:fail-split")
	if err := MigrateVoyageCatalogs(db); err != nil {
		t.Fatal(err)
	}
	var card VoyageIssue
	db.First(&card, original.ID)
	if card.Month != "2026-09" {
		t.Fatalf("missing month must use original creation time in China, got %s", card.Month)
	}
}
