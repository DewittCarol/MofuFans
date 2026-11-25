package database

import "time"

// Entries are immutable address versions. The single default and its revocable
// permission remain in VoyageLongTermAddress, never a flag on each entry.
type VoyageAddressBookEntry struct {
	ID               uint      `gorm:"primaryKey" json:"-"`
	UID              string    `gorm:"size:19;not null;index" json:"-"`
	AddressEncrypted string    `gorm:"type:text;not null" json:"-"`
	CreatedAt        time.Time `json:"-"`
	LastUsedAt       time.Time `json:"-"`
}

// Batch notes are encrypted separately and cannot enter the default address.
type VoyageAddressBookUse struct {
	ID            uint      `gorm:"primaryKey" json:"-"`
	EntryID       uint      `gorm:"not null;index" json:"-"`
	RecipientID   uint      `gorm:"not null;index" json:"-"`
	IssueID       uint      `gorm:"not null" json:"-"`
	NoteEncrypted string    `gorm:"type:text" json:"-"`
	Source        string    `gorm:"size:24;not null" json:"-"`
	RecordedAt    time.Time `json:"-"`
}

type VoyageAddressConsent struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	UID               string     `gorm:"size:19;not null;index" json:"-"`
	EntryID           uint       `json:"entry_id"`
	RecipientID       uint       `json:"recipient_id,omitempty"`
	Action            string     `gorm:"size:32;not null" json:"action"`
	PreferenceVersion int        `json:"preference_version"`
	AdminID           uint       `json:"admin_id,omitempty"`
	Channel           string     `gorm:"size:40" json:"channel,omitempty"`
	ConfirmedAt       *time.Time `json:"confirmed_at,omitempty"`
	OccurredAt        time.Time  `json:"occurred_at"`
}

// Additive migration only. It never creates permissions from old addresses.
func InitializeVoyageAddressBook() error {
	return DB.AutoMigrate(&VoyageAddressBookEntry{}, &VoyageAddressBookUse{}, &VoyageAddressConsent{},
		&VoyageLongTermAddress{}, &VoyageDelivery{}, &BilibiliProfile{})
}
