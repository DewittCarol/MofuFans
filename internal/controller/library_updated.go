package controller

import (
	"time"

	"gorm.io/gorm"
)

// The newest edit among currently published content, independent of list filters
// and pagination. Empty libraries have no timestamp; private drafts are excluded
// by the caller.
func libraryLastUpdated(query *gorm.DB) (*time.Time, error) {
	var row struct{ UpdatedAt time.Time }
	if err := query.Select("updated_at").Order("updated_at DESC").Limit(1).Scan(&row).Error; err != nil {
		return nil, err
	}
	if row.UpdatedAt.IsZero() {
		return nil, nil
	}
	return &row.UpdatedAt, nil
}
