package database

import "time"

// A recipient belongs to one issue. Credentials never appear in regular JSON lists.
type VoyageRecipient struct {
	BaseModel
	BatchItems       []VoyageBatchItem `gorm:"serializer:json;type:text" json:"batch_items"`
	IssueID          uint              `gorm:"not null;uniqueIndex:idx_voyage_issue_uid" json:"issue_id"`
	UID              string            `gorm:"size:20;not null;uniqueIndex:idx_voyage_issue_uid" json:"uid"`
	Tier             string            `gorm:"size:16;not null" json:"tier"`
	Name             string            `gorm:"size:160" json:"name"`
	Avatar           string            `gorm:"type:text" json:"avatar"`
	ProfileCheckedAt *time.Time        `json:"profile_checked_at"`
	ProfileError     string            `gorm:"size:160" json:"profile_error"`
	CodeEncrypted    string            `gorm:"type:text;not null" json:"-"`
	CodeHash         string            `gorm:"size:64;not null" json:"-"`
	Disabled         bool              `gorm:"not null;default:false" json:"disabled"`
	PreparedAt       *time.Time        `json:"prepared_at"`
	SentAt           *time.Time        `json:"sent_at"`
	Version          int               `gorm:"not null;default:1" json:"version"`
}

type VoyageDispatchConfig struct {
	IssueID  uint   `gorm:"primaryKey" json:"issue_id"`
	Template string `gorm:"type:text" json:"template"`
	EntryURL string `gorm:"type:text" json:"entry_url"`
	Version  int    `gorm:"not null;default:1" json:"version"`
}
