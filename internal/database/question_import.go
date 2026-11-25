package database

// Kept after a question is deleted so re-importing a backup cannot resurrect it.
type QuestionImport struct {
	BaseModel
	Source     string `gorm:"size:80;uniqueIndex:question_import_source"`
	LegacyID   uint   `gorm:"uniqueIndex:question_import_source"`
	QuestionID uint
}
