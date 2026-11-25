package database

import "time"

const (
	WebsiteAssetPending             = "pending"
	WebsiteAssetCompleted           = "completed"
	WebsiteAssetDefaultInstructions = "想加新功能、反馈 Bug、调整使用体验，或者上传新内容，都可以在这里发布。说说你想怎么改；如果遇到问题，写下出现问题的页面和操作步骤。截图、素材等附件按需添加，不上传文件也可以发布。"
)

// WebsiteAssetRequest is a private hand-off from a streamer to the site owner.
// Files are optional, deliberately stored outside the public web root, and only served
// through an authenticated download endpoint.
type WebsiteAssetRequest struct {
	BaseModel
	Description     string             `gorm:"type:text;not null" json:"description"`
	Status          string             `gorm:"size:16;index;not null;default:pending" json:"status"`
	SubmittedByID   uint               `gorm:"index;not null" json:"submitted_by_id"`
	SubmittedByName string             `gorm:"size:255;not null" json:"submitted_by_name"`
	CompletedAt     *time.Time         `json:"completed_at"`
	CompletedByID   *uint              `json:"completed_by_id,omitempty"`
	CompletedByName string             `gorm:"size:255" json:"completed_by_name,omitempty"`
	Files           []WebsiteAssetFile `gorm:"foreignKey:RequestID;constraint:OnDelete:CASCADE" json:"files"`
}

type WebsiteAssetFile struct {
	BaseModel
	RequestID    uint   `gorm:"index;not null" json:"request_id"`
	OriginalName string `gorm:"size:255;not null" json:"original_name"`
	StoredName   string `gorm:"size:64;uniqueIndex;not null" json:"-"`
	ContentType  string `gorm:"size:255;not null" json:"content_type"`
	Size         int64  `gorm:"not null" json:"size"`
}

type WebsiteAssetSettings struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Instructions  string    `gorm:"type:text;not null" json:"instructions"`
	UpdatedByID   uint      `json:"updated_by_id"`
	UpdatedByName string    `gorm:"size:255" json:"updated_by_name"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
