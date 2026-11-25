package database

import (
	"gorm.io/gorm"
	"time"
)

// A collection owns eligibility and an editable address; a plan owns parcels.
type VoyageShipmentPlan struct {
	BaseModel
	Title   string `gorm:"size:160" json:"title"`
	Version int    `gorm:"not null;default:1" json:"version"`
}

type VoyageParcelItem struct {
	VoyageBatchItem
	RecipientID  uint `json:"recipient_id"`
	CollectionID uint `json:"collection_id"`
}

type VoyageParcel struct {
	BaseModel
	LegacyRecipientID  *uint              `gorm:"uniqueIndex" json:"-"`
	PlanID             uint               `gorm:"not null;index" json:"plan_id"`
	UID                string             `gorm:"size:20;not null;index" json:"uid"`
	Name               string             `gorm:"size:160" json:"name"`
	Items              []VoyageParcelItem `gorm:"serializer:json;type:text" json:"items"`
	AddressRecipientID uint               `json:"address_recipient_id"`
	AddressVersion     int                `json:"address_version"`
	AddressEncrypted   string             `gorm:"type:text" json:"-"`
	ConfirmedAt        *time.Time         `json:"confirmed_at"`
	ExportedAt         *time.Time         `json:"exported_at"`
	ShippedAt          *time.Time         `json:"shipped_at"`
	CancelledAt        *time.Time         `json:"cancelled_at"`
	Courier            string             `gorm:"size:80" json:"courier"`
	Tracking           string             `gorm:"size:100" json:"tracking"`
	Note               string             `gorm:"type:text" json:"note"`
	Version            int                `gorm:"not null;default:1" json:"version"`
}

func InitializeVoyageFulfillment() error {
	if e := DB.AutoMigrate(&VoyageShipmentPlan{}, &VoyageParcel{}); e != nil {
		return e
	}
	if !DB.Migrator().HasTable(&VoyageDelivery{}) {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var deliveries []VoyageDelivery
		if e := tx.Where("shipped_at IS NOT NULL OR tracking <> ''").Find(&deliveries).Error; e != nil {
			return e
		}
		plans := map[uint]uint{}
		for _, d := range deliveries {
			var count int64
			if e := tx.Model(&VoyageParcel{}).Where("legacy_recipient_id = ?", d.RecipientID).Count(&count).Error; e != nil {
				return e
			}
			if count > 0 {
				continue
			}
			var r VoyageRecipient
			if e := tx.First(&r, d.RecipientID).Error; e != nil {
				return e
			}
			var b VoyageBatch
			if e := tx.First(&b, r.IssueID).Error; e != nil {
				return e
			}
			planID := plans[b.ID]
			if planID == 0 {
				p := VoyageShipmentPlan{Title: b.Title, Version: 1}
				if e := tx.Create(&p).Error; e != nil {
					return e
				}
				planID = p.ID
				plans[b.ID] = p.ID
			}
			items := []VoyageParcelItem{}
			for _, i := range r.BatchItems {
				items = append(items, VoyageParcelItem{VoyageBatchItem: i, RecipientID: r.ID, CollectionID: b.ID})
			}
			id := r.ID
			p := VoyageParcel{LegacyRecipientID: &id, PlanID: planID, UID: r.UID, Name: r.Name, Items: items, AddressRecipientID: r.ID, AddressVersion: d.Version, AddressEncrypted: d.AddressEncrypted, ConfirmedAt: d.SubmittedAt, ShippedAt: d.ShippedAt, Courier: d.Courier, Tracking: d.Tracking, Note: d.ShippingNote, Version: 1}
			if e := tx.Create(&p).Error; e != nil {
				return e
			}
		}
		return nil
	})
}
