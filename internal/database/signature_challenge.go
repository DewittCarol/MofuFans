package database

import "time"

// SignatureChallenge is deliberately separate from the retired follow verifier.
// Neither the public marker nor the browser's secret is persisted in plaintext.
type SignatureChallenge struct {
	ID                uint       `gorm:"primaryKey" json:"-"`
	BilibiliUID       int64      `gorm:"index;not null" json:"-"`
	Purpose           string     `gorm:"size:16;not null" json:"-"`
	Audience          string     `gorm:"size:255;not null" json:"-"`
	SecretHash        string     `gorm:"size:64;not null" json:"-"`
	CodeHash          string     `gorm:"size:64;not null" json:"-"`
	AccountStamp      string     `gorm:"size:64" json:"-"`
	Status            string     `gorm:"size:16;index;not null" json:"-"`
	RequestedAt       time.Time  `json:"-"`
	ExpiresAt         time.Time  `gorm:"index;not null" json:"-"`
	LastCheckedAt     *time.Time `json:"-"`
	CheckCount        int        `json:"-"`
	VerifiedAt        *time.Time `json:"-"`
	ConfirmationUntil *time.Time `json:"-"`
	ConsumedAt        *time.Time `json:"-"`
	BilibiliName      string     `gorm:"size:255" json:"-"`
	BilibiliAvatar    string     `gorm:"size:1024" json:"-"`
}

func InitializeMemberIdentity() error {
	return DB.AutoMigrate(&User{}, &SignatureChallenge{})
}
