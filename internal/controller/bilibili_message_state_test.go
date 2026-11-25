package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"mofufans/internal/bilibili"
	"mofufans/internal/database"
	"mofufans/internal/secret"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func voyageMessageFixture(t *testing.T) (*gin.Engine, *BilibiliMessagesController, *fakeMessageSender, database.BilibiliMessageQueue, []database.VoyageRecipient) {
	t.Helper()
	r, ctl, fake, _ := messageSetup(t)
	if e := database.DB.AutoMigrate(&database.VoyageDispatchConfig{}, &database.VoyageRecipient{}); e != nil {
		t.Fatal(e)
	}
	issue := database.VoyageIssue{Kind: "batch", Title: "通知测试批次", Published: true, Version: 1}
	database.DB.Create(&issue)
	database.DB.Create(&database.VoyageBatch{ID: issue.ID, Title: issue.Title, Published: true, Version: 1})
	now := time.Now()
	q := database.BilibiliMessageQueue{AdminID: 1, Scope: "voyage", ScopeID: issue.ID, SenderUID: "99999", Status: "running", TestStatus: "sent", TestAt: &now, Interval: 5}
	database.DB.Create(&q)
	encrypted, e := secret.SealBilibiliValue(queueScope(q, "test"), "测试消息")
	if e != nil {
		t.Fatal(e)
	}
	database.DB.Model(&q).Update("test_body", encrypted)
	rows := []database.VoyageRecipient{}
	for _, uid := range []string{"610020", "610021"} {
		encrypted, hash, e := secret.NewVoyageCode(recipientScope(issue.ID, uid))
		if e != nil {
			t.Fatal(e)
		}
		row := database.VoyageRecipient{IssueID: issue.ID, UID: uid, Name: "测试用户" + uid, CodeEncrypted: encrypted, CodeHash: hash, Version: 1}
		database.DB.Create(&row)
		source, e := readMessageSource(database.DB, q, uid)
		if e != nil {
			t.Fatal(e)
		}
		body, e := secret.SealBilibiliValue(queueScope(q, uid), "认证码："+source.Password)
		if e != nil {
			t.Fatal(e)
		}
		database.DB.Create(&database.BilibiliMessageItem{QueueID: q.ID, UID: uid, Body: body, Fingerprint: source.Fingerprint, Status: "pending"})
		rows = append(rows, row)
	}
	recipients := &VoyageRecipientsController{}
	r.PUT("/issues/:id/recipients/:recipient", recipients.Update)
	r.GET("/messages/monitor", ctl.Monitor)
	return r, ctl, fake, q, rows
}
func manualVoyageState(t *testing.T, r *gin.Engine, row database.VoyageRecipient, action string, version, want int) {
	t.Helper()
	giftJSON(t, r, "PUT", fmt.Sprintf("/issues/%d/recipients/%d", row.IssueID, row.ID), gin.H{"action": action, "version": version}, nil, want)
}

func TestManualSentVoyageSkipsQueuedItemsAtomically(t *testing.T) {
	r, ctl, fake, q, rows := voyageMessageFixture(t)
	other := database.BilibiliMessageQueue{AdminID: 2, Scope: "gift", ScopeID: q.ScopeID, Status: "prepared"}
	database.DB.Create(&other)
	untouched := database.BilibiliMessageItem{QueueID: other.ID, UID: rows[0].UID, Status: "pending"}
	database.DB.Create(&untouched)
	manualVoyageState(t, r, rows[0], "sent", rows[0].Version-1, 409)
	var item database.BilibiliMessageItem
	database.DB.Where("queue_id=? AND uid=?", q.ID, rows[0].UID).First(&item)
	if item.Status != "pending" {
		t.Fatal("stale manual edit changed queue")
	}
	manualVoyageState(t, r, rows[0], "sent", rows[0].Version, 200)
	database.DB.First(&item, item.ID)
	database.DB.First(&rows[0], rows[0].ID)
	database.DB.First(&untouched, untouched.ID)
	if item.Status != "sent" || item.MessageKey != "人工确认" || item.SentAt == nil || !strings.Contains(item.Note, "标记为已发") || rows[0].SentAt == nil || untouched.Status != "pending" {
		t.Fatal("manual mark did not preserve source and scope semantics")
	}
	if !ctl.tick(q.ID) || len(fake.calls) != 1 || fake.calls[0] != rows[1].UID {
		t.Fatal("manual mark blocked or duplicated queued delivery")
	}
	manualVoyageState(t, r, rows[0], "unsent", rows[0].Version, 200)
	database.DB.First(&item, item.ID)
	if item.Status != "sent" {
		t.Fatal("undo silently requeued a notification")
	}
}

func TestManualSentBlocksInFlightAndUncertainResults(t *testing.T) {
	for _, status := range []string{"sending", "unknown"} {
		t.Run(status, func(t *testing.T) {
			r, _, _, q, rows := voyageMessageFixture(t)
			database.DB.Model(&database.BilibiliMessageItem{}).Where("queue_id=? AND uid=?", q.ID, rows[0].UID).Update("status", status)
			manualVoyageState(t, r, rows[0], "sent", rows[0].Version, 409)
			database.DB.First(&rows[0], rows[0].ID)
			if rows[0].SentAt != nil || rows[0].Version != 1 {
				t.Fatal("uncertain manual mark partially committed")
			}
		})
	}
}

func TestOldManualSentIsSkippedAtConfirmAndAtClaim(t *testing.T) {
	for _, stage := range []string{"confirm", "claim"} {
		t.Run(stage, func(t *testing.T) {
			r, ctl, fake, q, rows := voyageMessageFixture(t)
			if stage == "confirm" {
				database.DB.Model(&q).Update("status", "paused")
				database.DB.Model(&rows[0]).Updates(map[string]any{"sent_at": time.Now(), "version": 2})
				ctl.workers[q.ID] = true
				messageCall(t, r, fmt.Sprintf("/messages/%d/action", q.ID), gin.H{"action": "confirm", "confirmed": true}, 200, false)
			} else {
				ctl.sender = func(uint) (directMessageClient, string, error) {
					manualVoyageState(t, r, rows[0], "sent", 1, 200)
					return fake, "99999", nil
				}
				if !ctl.tick(q.ID) || len(fake.calls) != 0 {
					t.Fatal("manual mark during sender lookup was sent")
				}
				ctl.sender = func(uint) (directMessageClient, string, error) { return fake, "99999", nil }
			}
			var item database.BilibiliMessageItem
			database.DB.Where("queue_id=? AND uid=?", q.ID, rows[0].UID).First(&item)
			if item.Status != "sent" {
				t.Fatal("existing manual mark was not reconciled")
			}
			if !ctl.tick(q.ID) || len(fake.calls) != 1 || fake.calls[0] != rows[1].UID {
				t.Fatal("remaining recipient was blocked")
			}
		})
	}
}

func TestManualSentGiftAlsoSkipsPending(t *testing.T) {
	r, _, _, gift := messageSetup(t)
	row := database.DigitalGiftRecipient{GiftID: gift.ID, UID: "610010", RosterActive: true, Version: 1, AccessVersion: 1}
	database.DB.Create(&row)
	q := messageCall(t, r, "/messages", gin.H{"scope": "gift", "scope_id": gift.ID, "version": gift.Version, "items": []gin.H{{"uid": row.UID, "version": row.Version}}}, 200, false).Queue
	path := fmt.Sprintf("/api/admin/gifts/%d/recipients/%d", gift.ID, row.ID)
	giftJSON(t, r, "PUT", path, gin.H{"action": "sent", "version": row.Version}, nil, 200)
	var item database.BilibiliMessageItem
	database.DB.Where("queue_id=?", q.ID).First(&item)
	if item.Status != "sent" || item.MessageKey != "人工确认" {
		t.Fatal("gift manual mark did not reconcile queue")
	}
}

func TestRecipientFailureRetainedWithoutBlockingNextOrAutomaticRetry(t *testing.T) {
	_, ctl, fake, q, _ := voyageMessageFixture(t)
	fake.err = &bilibili.MessageError{Note: "recipient rejected 21047", Recipient: true}
	if !ctl.tick(q.ID) {
		t.Fatal("recipient rejection stopped worker")
	}
	var items []database.BilibiliMessageItem
	database.DB.Where("queue_id=?", q.ID).Order("id").Find(&items)
	var clock database.BilibiliMessageClock
	database.DB.First(&clock, 1)
	if items[0].Status != "failed" || items[0].SentAt != nil || items[1].Status != "pending" || time.Until(clock.NextAt) > 6*time.Second {
		t.Fatal("recipient failure lost or introduced account cooldown")
	}
	fake.err = nil
	clearMessageClock(t)
	if !ctl.tick(q.ID) || len(fake.calls) != 2 || fake.calls[0] == fake.calls[1] {
		t.Fatal("next recipient was blocked or failed message replayed")
	}
	if ctl.tick(q.ID) {
		t.Fatal("completed queue kept running")
	}
	database.DB.First(&q, q.ID)
	if q.Status != "completed" || !strings.Contains(q.Note, "1 条待复核") {
		t.Fatal("completion hid failure")
	}
	states, e := latestMessageStates(database.DB, q.Scope, q.ScopeID)
	if e != nil || states[items[0].UID].Status != "failed" {
		t.Fatal("persistent recipient failure missing")
	}
}

func TestUnknownContinuesOtherRecipientsAfterCooldownWithoutReplay(t *testing.T) {
	_, ctl, fake, q, _ := voyageMessageFixture(t)
	until := time.Now().Add(20 * time.Minute)
	fake.err = &bilibili.MessageError{Note: "response missing", Uncertain: true, RetryAt: until}
	if !ctl.tick(q.ID) {
		t.Fatal("uncertain recipient blocked entire queue")
	}
	if !ctl.tick(q.ID) || len(fake.calls) != 1 {
		t.Fatal("cooldown or uncertain replay protection lost")
	}
	var clock database.BilibiliMessageClock
	database.DB.First(&clock, 1)
	if clock.NextAt.Before(until) {
		t.Fatal("Retry-After was shortened")
	}
	fake.err = nil
	clearMessageClock(t)
	ctl.tick(q.ID)
	if len(fake.calls) != 2 || fake.calls[0] == fake.calls[1] {
		t.Fatal("other recipient not sent safely")
	}
}

func TestExpiredLoginStillRequiresUserReauthentication(t *testing.T) {
	_, ctl, fake, q, _ := voyageMessageFixture(t)
	fake.err = &bilibili.MessageError{Note: "login expired", Login: true}
	if ctl.tick(q.ID) {
		t.Fatal("expired account continued failing remaining recipients")
	}
	database.DB.First(&q, q.ID)
	if q.Status != "paused" || len(fake.calls) != 1 {
		t.Fatal("login recovery state missing")
	}
}

type blockingMessageSender struct {
	entered chan struct{}
	release chan struct{}
}

func (s *blockingMessageSender) SendMessage(context.Context, string, string, string) (string, error) {
	close(s.entered)
	<-s.release
	return "1234567", nil
}
func TestManualSentWhileHTTPRequestInFlightDoesNotOverwriteReceipt(t *testing.T) {
	r, ctl, _, q, rows := voyageMessageFixture(t)
	sender := &blockingMessageSender{make(chan struct{}), make(chan struct{})}
	ctl.sender = func(uint) (directMessageClient, string, error) { return sender, "99999", nil }
	done := make(chan bool, 1)
	go func() { done <- ctl.tick(q.ID) }()
	select {
	case <-sender.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("send did not start")
	}
	manualVoyageState(t, r, rows[0], "sent", 1, 409)
	close(sender.release)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("send did not complete")
	}
	var item database.BilibiliMessageItem
	database.DB.Where("queue_id=? AND uid=?", q.ID, rows[0].UID).First(&item)
	if item.Status != "sent" || item.MessageKey != "1234567" {
		t.Fatal("manual mark replaced real receipt")
	}
}

func TestMessageMonitorCrossAdminReadOnlyScopeAndProgress(t *testing.T) {
	r, _, _, q, rows := voyageMessageFixture(t)
	if e := database.DB.AutoMigrate(&database.Admin{}); e != nil {
		t.Fatal(e)
	}
	database.DB.Create(&database.Admin{BaseModel: database.BaseModel{ID: 1}, Username: "mofu", Password: "DO_NOT_EXPOSE_ADMIN_SECRET"})
	database.DB.Create(&database.Admin{BaseModel: database.BaseModel{ID: 2}, Username: "admin"})
	database.DB.Model(&database.BilibiliMessageItem{}).Where("queue_id=? AND uid=?", q.ID, rows[0].UID).Updates(map[string]any{"status": "failed", "note": "21047"})
	req := httptest.NewRequest("GET", fmt.Sprintf("/messages/monitor?scope=voyage&scope_id=%d&status=failed", q.ScopeID), nil)
	req.Header.Set("Other", "yes")
	req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var res struct {
		Code int `json:"code"`
		Data struct {
			Selected messageQueueSummary `json:"selected"`
			Total    int64               `json:"total"`
			Items    []struct {
				UID    string `json:"uid"`
				Status string `json:"status"`
			} `json:"items"`
		} `json:"data"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &res); e != nil {
		t.Fatal(e)
	}
	if res.Code != 200 || res.Data.Selected.Owner != "mofu" || res.Data.Selected.CanManage || res.Data.Selected.Total != 2 || res.Data.Selected.Counts["failed"] != 1 || res.Data.Total != 1 || len(res.Data.Items) != 1 || res.Data.Items[0].UID != rows[0].UID {
		t.Fatalf("cross-admin progress incorrect: %s", w.Body.String())
	}
	for _, word := range []string{"DO_NOT_EXPOSE_ADMIN_SECRET", "test_body", "fingerprint", "\"body\"", "\"text\"", "code_encrypted"} {
		if strings.Contains(w.Body.String(), word) {
			t.Fatal("monitor leaked private message data")
		}
	}
	messageCall(t, r, fmt.Sprintf("/messages/%d/action", q.ID), gin.H{"action": "pause"}, 404, true)
}
