package database

import "time"

// Delivery data is separate from the public gift catalog and roster responses.
type VoyageDelivery struct {
	RecipientID      uint       `gorm:"primaryKey" json:"-"`
	AddressEncrypted string     `gorm:"type:text" json:"-"`
	AddressSource    string     `gorm:"size:24;not null;default:''" json:"-"`
	LongTermVersion  int        `gorm:"not null;default:0" json:"-"`
	LongTermSavedAt  *time.Time `json:"-"`
	LongTermOrigin   string     `gorm:"size:16;not null;default:''" json:"-"`
	Courier          string     `gorm:"size:80" json:"courier"`
	Tracking         string     `gorm:"size:100" json:"tracking"`
	ShippingNote     string     `gorm:"size:500" json:"shipping_note"`
	SubmittedAt      *time.Time `json:"submitted_at"`
	ShippedAt        *time.Time `json:"shipped_at"`
	Version          int        `gorm:"not null;default:1" json:"version"`
}
