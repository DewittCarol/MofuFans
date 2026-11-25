package controller

import (
	"encoding/json"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"mofufans/internal/database"
	"mofufans/internal/theme"
	"path/filepath"
	"testing"
)

func TestThemeSignatureBrandUsesCurrentSiteTheme(t *testing.T) {
	original := database.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "theme.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	database.DB = db
	t.Cleanup(func() {
		database.DB = original
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&database.SiteTheme{}); err != nil {
		t.Fatal(err)
	}
	if got := ThemeSignatureBrand(); got != theme.Default().Brand.SiteName {
		t.Fatal("missing theme lost site default")
	}
	config := theme.Default()
	config.Brand.SiteName = "CustomFans"
	raw, _ := json.Marshal(config)
	row := database.SiteTheme{ID: 1, Revision: 1, Config: string(raw)}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if got := ThemeSignatureBrand(); got != "CustomFans" {
		t.Fatal("configured brand was not used")
	}
	config.Brand.SiteName = "UpdatedFans"
	raw, _ = json.Marshal(config)
	if err := db.Model(&row).Update("config", string(raw)).Error; err != nil {
		t.Fatal(err)
	}
	if got := ThemeSignatureBrand(); got != "UpdatedFans" {
		t.Fatal("brand did not follow theme update")
	}
	if err := db.Model(&row).Update("config", "invalid").Error; err != nil {
		t.Fatal(err)
	}
	if got := ThemeSignatureBrand(); got != theme.Default().Brand.SiteName {
		t.Fatal("invalid theme lost site default")
	}
}
