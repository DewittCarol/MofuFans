package database

import "mofufans/internal/theme"
import "encoding/json"

// Separate tables intentionally keep preview theme work independent of existing settings.
type SiteTheme struct {
	ID       uint   `gorm:"primaryKey"`
	Revision uint   `json:"revision"`
	Config   string `gorm:"type:text"`
}
type ThemePreset struct {
	BaseModel
	Name   string `gorm:"size:80" json:"name"`
	Config string `gorm:"type:text" json:"-"`
}

func InitializeTheme() error {
	if err := DB.AutoMigrate(&SiteTheme{}, &ThemePreset{}); err != nil {
		return err
	}
	raw, err := json.Marshal(theme.Default())
	if err != nil {
		return err
	}
	return DB.Where("id = ?", 1).Attrs(SiteTheme{Revision: 1, Config: string(raw)}).FirstOrCreate(&SiteTheme{ID: 1}).Error
}
