package controller

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mofufans/internal/database"
	"mofufans/internal/secret"
	"time"
)

func (ctl *DigitalGiftController) Recipients(c *gin.Context) {
	gift, ok := ctl.load(c, false)
	if !ok {
		return
	}
	rows := []database.DigitalGiftRecipient{}
	if err := database.DB.Where("gift_id = ?", gift.ID).Order("length(uid), uid, id").Find(&rows).Error; err != nil {
		giftError(c, err)
		return
	}
	members, err := giftSourceMembers(database.DB, gift.ID)
	if err != nil {
		giftError(c, err)
		return
	}
	sources := map[string][]giftRosterMember{}
	for _, member := range members {
		sources[member.UID] = append(sources[member.UID], member)
	}
	result := make([]gin.H, 0, len(rows))
	states, e := latestMessageStates(database.DB, "gift", gift.ID)
	if e != nil {
		giftError(c, e)
		return
	}
	for _, row := range rows {
		result = append(result, gin.H{"id": row.ID, "uid": row.UID, "name": row.Name, "disabled": row.Disabled, "roster_active": row.RosterActive, "version": row.Version, "sent_at": row.SentAt, "sources": sources[row.UID], "message": states[row.UID]})
	}
	Success(c, result)
}

func (ctl *DigitalGiftController) UpdateRecipient(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	gift, ok := ctl.load(c, false)
	if !ok {
		return
	}
	var req struct {
		Action  string `json:"action"`
		Version int    `json:"version"`
	}
	if c.ShouldBindJSON(&req) != nil {
		Fail(c, 400, "操作无效")
		return
	}
	if req.Action != "sent" && req.Action != "unsent" && req.Action != "disable" && req.Action != "enable" && req.Action != "reset" {
		Fail(c, 400, "操作无效")
		return
	}
	if req.Action == "reset" && gift.Mode != "individual" {
		Fail(c, 400, "统一密码请在礼物内容中修改，不能为单个收件人重置")
		return
	}
	var row database.DigitalGiftRecipient
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockVoyageBatchWrites(tx); err != nil {
			return err
		}
		if err := tx.First(&row, "id = ? AND gift_id = ?", c.Param("recipient"), gift.ID).Error; err != nil {
			return err
		}
		if !row.RosterActive && req.Action != "disable" {
			return errGiftConflict
		}
		if row.Version != req.Version {
			return errGiftConflict
		}
		switch req.Action {
		case "sent":
			if e := syncManuallyNotifiedMessages(tx, "gift", gift.ID, row.UID); e != nil {
				return e
			}
			now := time.Now()
			row.SentAt = &now
		case "unsent":
			row.SentAt = nil
		case "disable":
			row.Disabled = true
			row.AccessVersion++
		case "enable":
			row.Disabled = false
			row.AccessVersion++
		case "reset":
			password, err := secret.NewGiftPassword()
			if err != nil {
				return err
			}
			row.PasswordEncrypted, err = secret.SealGiftValue(giftScope(gift.ID, row.UID), password)
			if err != nil {
				return err
			}
			row.AccessVersion++
			row.SentAt = nil
		}
		row.Version++
		res := tx.Model(&database.DigitalGiftRecipient{}).Where("id = ? AND version = ?", row.ID, req.Version).Select("Version", "AccessVersion", "PasswordEncrypted", "Disabled", "SentAt").Updates(&row)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errGiftConflict
		}
		return nil
	})
	if err != nil {
		if err == errMessageManualSentBlocked {
			Fail(c, 409, err.Error())
			return
		}
		giftError(c, err)
		return
	}
	Success(c, row)
}
