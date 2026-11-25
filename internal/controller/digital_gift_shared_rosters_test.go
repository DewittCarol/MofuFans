package controller

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mofufans/internal/database"
)

func sharedGiftRoster(t *testing.T, r *gin.Engine, gift *database.DigitalGift) database.VoyageRoster {
	t.Helper()
	roster := database.VoyageRoster{Title: "统一密码通知", Category: "event"}
	if err := database.DB.Create(&roster).Error; err != nil {
		t.Fatal(err)
	}
	for _, member := range []database.VoyageRosterMember{
		{RosterID: roster.ID, UID: "610010", Tier: "captain"},
		{RosterID: roster.ID, UID: "610010", Tier: "admiral"},
		{RosterID: roster.ID, UID: "610011", Tier: "captain"},
	} {
		if err := database.DB.Create(&member).Error; err != nil {
			t.Fatal(err)
		}
	}
	giftJSON(t, r, "PUT", fmt.Sprintf("/api/admin/gifts/%d/rosters", gift.ID), gin.H{"roster_ids": []uint{roster.ID}, "version": gift.Version}, nil, 200)
	if err := database.DB.First(gift, gift.ID).Error; err != nil {
		t.Fatal(err)
	}
	return roster
}

func TestSharedRosterTracksNotificationsWithoutRestrictingUnlock(t *testing.T) {
	r, _ := giftTestRouter(t)
	gift := giftCreate(t, r, "shared", true)
	roster := sharedGiftRoster(t, r, &gift)
	admin := fmt.Sprintf("/api/admin/gifts/%d", gift.ID)
	base := fmt.Sprintf("/api/gifts/%d", gift.ID)
	password := giftPassword(t, r, gift.ID, "")
	_, data := giftJSON(t, r, "GET", admin+"/recipients", nil, nil, 200)
	var rows []database.DigitalGiftRecipient
	if err := json.Unmarshal(data, &rows); err != nil || len(rows) != 2 {
		t.Fatalf("UID dedupe failed: %s", data)
	}
	for _, row := range rows {
		var stored database.DigitalGiftRecipient
		database.DB.First(&stored, row.ID)
		if stored.PasswordEncrypted != "" {
			t.Fatal("shared roster generated an individual password")
		}
	}
	row := rows[0]
	path := fmt.Sprintf("%s/recipients/%d", admin, row.ID)
	w, _ := giftJSON(t, r, "POST", base+"/unlock", gin.H{"password": password}, nil, 200)
	cookies := w.Result().Cookies()
	giftJSON(t, r, "PUT", path, gin.H{"action": "reset", "version": row.Version}, nil, 400)
	giftJSON(t, r, "PUT", path, gin.H{"action": "disable", "version": row.Version}, nil, 200)
	giftJSON(t, r, "GET", base, nil, cookies, 200)
	giftJSON(t, r, "POST", base+"/unlock", gin.H{"password": password}, nil, 200)
	if _, err := readMessageSource(database.DB, database.BilibiliMessageQueue{Scope: "gift", ScopeID: gift.ID}, row.UID); err == nil {
		t.Fatal("disabled notification was eligible for sending")
	}
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("roster_id = ?", roster.ID).Delete(&database.VoyageRosterMember{}).Error; err != nil {
			return err
		}
		return syncDigitalGiftsForRoster(tx, roster.ID)
	}); err != nil {
		t.Fatal(err)
	}
	giftJSON(t, r, "GET", base, nil, cookies, 200)
	if _, err := readMessageSource(database.DB, database.BilibiliMessageQueue{Scope: "gift", ScopeID: gift.ID}, rows[1].UID); err == nil {
		t.Fatal("removed member remained eligible for notifications")
	}
	source, err := readMessageSource(database.DB, database.BilibiliMessageQueue{Scope: "gift", ScopeID: gift.ID}, "610012")
	if err != nil || source.Password != password || source.RowID != 0 {
		t.Fatal("manual recipient outside the roster no longer works", err)
	}
}

func TestSharedRosterBatchSendsSharedPasswordAndResetsAfterRotation(t *testing.T) {
	r, ctl, fake, gift := messageSetup(t)
	sharedGiftRoster(t, r, &gift)
	password := giftPassword(t, r, gift.ID, "")
	var rows []database.DigitalGiftRecipient
	database.DB.Where("gift_id = ?", gift.ID).Order("id").Find(&rows)
	items := []gin.H{}
	for _, row := range rows {
		items = append(items, gin.H{"uid": row.UID, "version": row.Version})
	}
	view := messageCall(t, r, "/messages", gin.H{"scope": "gift", "scope_id": gift.ID, "version": gift.Version, "items": items}, 200, false)
	if len(view.Items) != 2 || len(fake.calls) != 0 {
		t.Fatal("preview must include both members without sending")
	}
	for _, item := range view.Items {
		if !strings.Contains(item.Text, password) || strings.Contains(item.Text, "UID：") {
			t.Fatal("wrong shared message")
		}
	}
	path := fmt.Sprintf("/messages/%d", view.Queue.ID)
	messageCall(t, r, path+"/test", gin.H{}, 200, false)
	clearMessageClock(t)
	ctl.workers[view.Queue.ID] = true
	messageCall(t, r, path+"/action", gin.H{"action": "confirm", "confirmed": true}, 200, false)
	for range rows {
		ctl.tick(view.Queue.ID)
		clearMessageClock(t)
	}
	ctl.tick(view.Queue.ID)
	if len(fake.calls) != 3 {
		t.Fatalf("expected one test and two notifications, got %d", len(fake.calls))
	}
	for _, body := range fake.bodies[1:] {
		if !strings.Contains(body, password) {
			t.Fatal("batch password mismatch")
		}
	}
	database.DB.Where("gift_id = ?", gift.ID).Find(&rows)
	for _, row := range rows {
		if row.SentAt == nil || row.Version != 2 {
			t.Fatal("successful send not recorded")
		}
		if _, err := readMessageSource(database.DB, view.Queue, row.UID); err == nil {
			t.Fatal("already sent member is still eligible")
		}
	}
	admin := fmt.Sprintf("/api/admin/gifts/%d", gift.ID)
	// Saving other content (or re-entering the same password) preserves sent state.
	for _, samePassword := range []string{"", password} {
		_, data := giftJSON(t, r, "PUT", admin, gin.H{"title": "更新介绍", "mode": "shared", "published": true, "version": gift.Version, "password": samePassword}, nil, 200)
		_ = json.Unmarshal(data, &gift)
		database.DB.Where("gift_id = ?", gift.ID).Find(&rows)
		for _, row := range rows {
			if row.SentAt == nil {
				t.Fatal("ordinary save cleared sent state")
			}
		}
	}
	giftJSON(t, r, "PUT", admin, gin.H{"title": gift.Title, "mode": "shared", "published": true, "version": gift.Version, "password": "NewShared123"}, nil, 200)
	database.DB.Where("gift_id = ?", gift.ID).Find(&rows)
	for _, row := range rows {
		if row.SentAt != nil || row.Version != 3 {
			t.Fatal("new shared password did not return member to pending")
		}
		source, err := readMessageSource(database.DB, view.Queue, row.UID)
		if err != nil || source.Password != "NewShared123" {
			t.Fatal("recipient did not use rotated shared password", err)
		}
	}
}
