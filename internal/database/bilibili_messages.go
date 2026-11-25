package database

import "time"

// Login and message bodies are encrypted with owner-bound authenticated data.
type BilibiliLogin struct {
	AdminID     uint      `gorm:"primaryKey;autoIncrement:false" json:"-"`
	UID         string    `json:"uid"`
	Name        string    `json:"name"`
	Credentials string    `gorm:"type:text" json:"-"`
	UpdatedAt   time.Time `json:"-"`
}

type BilibiliMessageQueue struct {
	BaseModel
	AdminID     uint       `gorm:"index" json:"-"`
	Scope       string     `gorm:"size:16;index" json:"scope"`
	ScopeID     uint       `gorm:"index" json:"scope_id"`
	TestUID     string     `json:"test_uid"`
	SenderUID   string     `json:"sender_uid"`
	Status      string     `gorm:"size:32;index" json:"status"`
	TestStatus  string     `json:"test_status"`
	TestBody    string     `gorm:"type:text" json:"-"`
	Note        string     `json:"note"`
	Interval    int        `json:"interval"`
	TestAt      *time.Time `json:"test_at"`
	ConfirmedAt *time.Time `json:"confirmed_at"`
}

type BilibiliMessageItem struct {
	BaseModel
	QueueID     uint       `gorm:"index" json:"queue_id"`
	UID         string     `json:"uid"`
	Body        string     `gorm:"type:text" json:"-"`
	Fingerprint string     `json:"-"`
	Status      string     `gorm:"size:16;index" json:"status"`
	Note        string     `json:"note"`
	MessageKey  string     `json:"-"`
	SentAt      *time.Time `json:"sent_at"`
}

// A single clock covers both modules, including tests, retries and account changes.
type BilibiliMessageClock struct {
	ID     uint `gorm:"primaryKey;autoIncrement:false"`
	NextAt time.Time
}

func InitializeBilibiliMessages() error {
	return DB.AutoMigrate(&BilibiliLogin{}, &BilibiliMessageQueue{}, &BilibiliMessageItem{}, &BilibiliMessageClock{})
}
