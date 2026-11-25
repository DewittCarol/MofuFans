package controller

import (
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mofufans/internal/database"
	"mofufans/internal/secret"
	"sort"
)

type giftRosterMember struct {
	database.VoyageRosterMember
	RosterTitle string `json:"roster_title"`
}

func giftSourceMembers(tx *gorm.DB, giftID uint) ([]giftRosterMember, error) {
	rows := []giftRosterMember{}
	err := tx.Table("voyage_roster_members AS m").Select("m.*, r.title AS roster_title").Joins("JOIN digital_gift_rosters AS b ON b.roster_id = m.roster_id").Joins("JOIN voyage_rosters AS r ON r.id = m.roster_id").Where("b.gift_id = ?", giftID).Order("m.roster_id, m.id").Scan(&rows).Error
	return rows, err
}
func syncDigitalGiftRecipients(tx *gorm.DB, giftID uint) error {
	var gift database.DigitalGift
	if err := tx.First(&gift, giftID).Error; err != nil {
		return err
	}
	members, err := giftSourceMembers(tx, giftID)
	if err != nil {
		return err
	}
	active := map[string]string{}
	for _, member := range members {
		if member.Disabled {
			continue
		}
		name, found := active[member.UID]
		if !found || name == "" {
			active[member.UID] = member.Name
		}
	}
	var rows []database.DigitalGiftRecipient
	if err = tx.Where("gift_id = ?", giftID).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		name, present := active[row.UID]
		if row.RosterActive != present || (present && row.Name != name) {
			if row.RosterActive != present {
				row.AccessVersion++
			}
			row.RosterActive = present
			if present {
				row.Name = name
			}
			row.Version++
			if err = tx.Model(&row).Select("Name", "RosterActive", "Version", "AccessVersion").Updates(&row).Error; err != nil {
				return err
			}
		}
		delete(active, row.UID)
	}
	// Stable order also makes multiple rows for the same UID share one password.
	uids := make([]string, 0, len(active))
	for uid := range active {
		uids = append(uids, uid)
	}
	sort.Strings(uids)
	for _, uid := range uids {
		var encrypted string
		if gift.Mode == "individual" {
			password, err := secret.NewGiftPassword()
			if err != nil {
				return err
			}
			encrypted, err = secret.SealGiftValue(giftScope(giftID, uid), password)
			if err != nil {
				return err
			}
		}
		row := database.DigitalGiftRecipient{GiftID: giftID, UID: uid, Name: active[uid], PasswordEncrypted: encrypted, RosterActive: true, Version: 1, AccessVersion: 1}
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}
func syncDigitalGiftsForRoster(tx *gorm.DB, rosterID uint) error {
	if !tx.Migrator().HasTable(&database.DigitalGiftRoster{}) {
		return nil
	}
	var bindings []database.DigitalGiftRoster
	if err := tx.Where("roster_id = ?", rosterID).Find(&bindings).Error; err != nil {
		return err
	}
	for _, binding := range bindings {
		if err := syncDigitalGiftRecipients(tx, binding.GiftID); err != nil {
			return err
		}
	}
	return nil
}
func (ctl *DigitalGiftController) Rosters(c *gin.Context) {
	gift, ok := ctl.load(c, false)
	if !ok {
		return
	}
	if gift.Mode != "individual" && gift.Mode != "shared" {
		Fail(c, 400, "当前领取方式不支持关联名单")
		return
	}
	var bindings []database.DigitalGiftRoster
	if err := database.DB.Where("gift_id = ?", gift.ID).Order("roster_id").Find(&bindings).Error; err != nil {
		giftError(c, err)
		return
	}
	ids := []uint{}
	for _, binding := range bindings {
		ids = append(ids, binding.RosterID)
	}
	Success(c, gin.H{"roster_ids": ids, "version": gift.Version})
}
func (ctl *DigitalGiftController) SaveRosters(c *gin.Context) {
	if !giftRequest(c, 16<<10) {
		return
	}
	var req struct {
		RosterIDs []uint `json:"roster_ids"`
		Version   int    `json:"version"`
	}
	if c.ShouldBindJSON(&req) != nil || len(req.RosterIDs) > 200 {
		Fail(c, 400, "请检查选择的名单，最多关联 200 张")
		return
	}
	seen := map[uint]bool{}
	for _, id := range req.RosterIDs {
		if id == 0 || seen[id] {
			Fail(c, 400, "名单选择无效或重复")
			return
		}
		seen[id] = true
	}
	var gift database.DigitalGift
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockVoyageBatchWrites(tx); err != nil {
			return err
		}
		if giftID(c) == 0 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.First(&gift, giftID(c)).Error; err != nil {
			return err
		}
		if gift.Mode != "individual" && gift.Mode != "shared" {
			return errors.New("当前领取方式不支持关联名单")
		}
		if gift.Version != req.Version {
			return errGiftConflict
		}
		var count int64
		if len(req.RosterIDs) > 0 {
			if err := tx.Model(&database.VoyageRoster{}).Where("id IN ?", req.RosterIDs).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(req.RosterIDs)) {
				return gorm.ErrRecordNotFound
			}
		}
		if err := tx.Where("gift_id = ?", gift.ID).Delete(&database.DigitalGiftRoster{}).Error; err != nil {
			return err
		}
		for _, id := range req.RosterIDs {
			if err := tx.Create(&database.DigitalGiftRoster{GiftID: gift.ID, RosterID: id}).Error; err != nil {
				return err
			}
		}
		if err := syncDigitalGiftRecipients(tx, gift.ID); err != nil {
			return err
		}
		gift.Version++
		gift.RosterManaged = true
		result := tx.Model(&gift).Where("version = ?", req.Version).Select("Version", "RosterManaged").Updates(&gift)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errGiftConflict
		}
		return nil
	})
	if err != nil {
		giftError(c, err)
		return
	}
	Success(c, gin.H{"roster_ids": req.RosterIDs, "version": gift.Version})
}
