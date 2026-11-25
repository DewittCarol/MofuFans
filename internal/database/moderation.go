package database

import "time"

// Separate from public question data. Loaded only for authenticated administrators.
type QuestionModeration struct {
	QuestionID     uint       `gorm:"primaryKey" json:"question_id"`
	Status         string     `gorm:"index;size:24" json:"status"`
	Reason         string     `gorm:"type:text" json:"reason"`
	Error          string     `gorm:"type:text" json:"error"`
	Model          string     `json:"model"`
	ImageStatus    string     `json:"image_status"`
	ImagesChecked  int        `json:"images_checked"`
	DecisionLocked bool       `json:"decision_locked"`
	Token          string     `json:"-"`
	Attempts       int        `json:"attempts"`
	CheckedAt      *time.Time `json:"checked_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func InitializeModeration() error {
	// Add only AI columns; do not rebuild unrelated legacy SQLite config fields.
	for _, field := range []string{"AIConfig", "AIKeyCiphertext"} {
		if !DB.Migrator().HasColumn(&Config{}, field) {
			if err := DB.Migrator().AddColumn(&Config{}, field); err != nil {
				return err
			}
		}
	}
	if err := DB.AutoMigrate(&QuestionModeration{}); err != nil {
		return err
	}
	return InitializeAI()
}
