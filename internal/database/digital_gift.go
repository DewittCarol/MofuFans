package database

import (
	"fmt"
	"gorm.io/gorm"
	"time"
)

// Credentials and storage keys are deliberately excluded from every JSON response.
type DigitalGift struct {
	BaseModel
	Title             string            `gorm:"size:120" json:"title"`
	Intro             string            `gorm:"size:500" json:"intro"`
	Content           string            `gorm:"type:text" json:"content"`
	Mode              string            `gorm:"size:16;not null" json:"mode"`
	Published         bool              `gorm:"not null;default:false;index" json:"published"`
	Version           int               `gorm:"not null;default:1" json:"version"`
	RosterManaged     bool              `gorm:"not null;default:false" json:"-"`
	AccessVersion     int               `gorm:"not null;default:1" json:"-"`
	PasswordEncrypted string            `gorm:"type:text" json:"-"`
	Files             []DigitalGiftFile `gorm:"foreignKey:GiftID" json:"files"`
}

type DigitalGiftFile struct {
	BaseModel
	GiftID     uint   `gorm:"not null;index" json:"gift_id"`
	Name       string `gorm:"size:255" json:"name"`
	StorageKey string `gorm:"size:64;uniqueIndex" json:"-"`
	Size       int64  `json:"size"`
	MediaType  string `gorm:"size:100" json:"media_type"`
}

type DigitalGiftRecipient struct {
	BaseModel
	GiftID            uint       `gorm:"not null;uniqueIndex:gift_uid" json:"gift_id"`
	UID               string     `gorm:"size:19;not null;uniqueIndex:gift_uid" json:"uid"`
	Name              string     `gorm:"size:160" json:"name"`
	PasswordEncrypted string     `gorm:"type:text" json:"-"`
	RosterActive      bool       `gorm:"not null;default:true" json:"roster_active"`
	Disabled          bool       `gorm:"not null;default:false" json:"disabled"`
	Version           int        `gorm:"not null;default:1" json:"version"`
	AccessVersion     int        `gorm:"not null;default:1" json:"-"`
	SentAt            *time.Time `json:"sent_at"`
}

type DigitalGiftRoster struct {
	GiftID   uint `gorm:"primaryKey;autoIncrement:false" json:"gift_id"`
	RosterID uint `gorm:"primaryKey;autoIncrement:false;index" json:"roster_id"`
}

type DigitalGiftUpload struct {
	Token      string    `gorm:"primaryKey;size:64" json:"token"`
	GiftID     uint      `gorm:"not null;index" json:"-"`
	Name       string    `gorm:"size:255" json:"name"`
	StorageKey string    `gorm:"size:64" json:"-"`
	Size       int64     `json:"size"`
	FileID     uint      `json:"file_id,omitempty"`
	Received   int64     `gorm:"not null;default:0" json:"received"`
	UpdatedAt  time.Time `gorm:"index" json:"-"`
}

func InitializeDigitalGifts() error {
	if err := DB.AutoMigrate(&DigitalGift{}, &DigitalGiftFile{}, &DigitalGiftRecipient{}, &DigitalGiftRoster{}, &DigitalGiftUpload{}); err != nil {
		return err
	}
	if !DB.Migrator().HasTable(&VoyageRoster{}) || !DB.Migrator().HasTable(&VoyageRosterMember{}) {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var gifts []DigitalGift
		if err := tx.Where("mode = ? AND roster_managed = ?", "individual", false).Find(&gifts).Error; err != nil {
			return err
		}
		for _, gift := range gifts {
			var rows []DigitalGiftRecipient
			if err := tx.Where("gift_id = ?", gift.ID).Find(&rows).Error; err != nil {
				return err
			}
			if len(rows) > 0 {
				roster := VoyageRoster{Title: fmt.Sprintf("电子礼物领取名单 #%d", gift.ID), Category: "event", Version: 1}
				if err := tx.Create(&roster).Error; err != nil {
					return err
				}
				for _, row := range rows {
					member := VoyageRosterMember{RosterID: roster.ID, UID: row.UID, Name: row.Name, Version: 1}
					if err := tx.Create(&member).Error; err != nil {
						return err
					}
				}
				if err := tx.Create(&DigitalGiftRoster{GiftID: gift.ID, RosterID: roster.ID}).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&gift).Update("roster_managed", true).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
