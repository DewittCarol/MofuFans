package controller

import (
	"gorm.io/gorm"
	"mofufans/internal/database"
)

func voyageRecipients(tx *gorm.DB, issueID uint) ([]database.VoyageRecipient, error) {
	rows := []database.VoyageRecipient{}
	err := tx.Where("issue_id = ?", issueID).Order("id asc").Find(&rows).Error
	return rows, err
}
func voyageRecipient(tx *gorm.DB, issueID, recipientID uint) (database.VoyageRecipient, error) {
	var row database.VoyageRecipient
	err := tx.Where("issue_id = ? AND id = ?", issueID, recipientID).First(&row).Error
	return row, err
}
