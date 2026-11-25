package database

import "time"

// Public identity cache only; business names remain optional per-record overrides.
type BilibiliProfile struct {
	UID             string     `gorm:"size:20;primaryKey" json:"uid"`
	Name            string     `gorm:"size:160" json:"name"`
	Avatar          string     `gorm:"type:text" json:"avatar"`
	Error           string     `gorm:"size:160" json:"error"`
	CheckedAt       time.Time  `json:"checked_at"`
	RetryAt         *time.Time `json:"retry_at"`
	AvatarSourceURL string     `gorm:"type:text;not null;default:''" json:"-"`
}

func InitializeBilibiliProfiles() error { return DB.AutoMigrate(&BilibiliProfile{}) }
