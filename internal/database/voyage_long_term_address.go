package database

import "time"

// One explicit, revocable consent per UID in this site's database. Never seeded
// from historical deliveries. The ciphertext has a distinct authenticated scope.
type VoyageLongTermAddress struct {
	UID              string     `gorm:"size:19;primaryKey" json:"-"`
	AddressEncrypted string     `gorm:"type:text;not null" json:"-"`
	Enabled          bool       `gorm:"not null;default:false" json:"-"`
	Version          int        `gorm:"not null;default:1" json:"-"`
	UpdatedAt        time.Time  `json:"-"`
	EntryID          uint       `gorm:"not null;default:0" json:"-"`
	Source           string     `gorm:"size:16;not null;default:''" json:"-"`
	AdminID          uint       `gorm:"not null;default:0" json:"-"`
	ConfirmedAt      *time.Time `json:"-"`
	ConfirmedVia     string     `gorm:"size:40;not null;default:''" json:"-"`
}
