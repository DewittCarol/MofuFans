package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"mofufans/internal/bilibili"
	"mofufans/internal/database"
	"mofufans/internal/secret"
)

type fakeMessageSender struct {
	calls  []string
	bodies []string
	err    error
}

func (f *fakeMessageSender) SendMessage(_ context.Context, _ string, uid, body string) (string, error) {
	f.calls = append(f.calls, uid)
	f.bodies = append(f.bodies, body)
	return "1234567890123456789", f.err
}

type testQueueView struct {
	Queue database.BilibiliMessageQueue `json:"queue"`
	Items []struct {
		database.BilibiliMessageItem
		Text string `json:"text"`
	} `json:"items"`
	TestText string `json:"test_text"`
}

func messageSetup(t *testing.T) (*gin.Engine, *BilibiliMessagesController, *fakeMessageSender, database.DigitalGift) {
	r, _ := giftTestRouter(t)
	viper.Set("bilibili.test_recipient_uid", "123456")
	if err := database.InitializeBilibiliMessages(); err != nil {
		t.Fatal(err)
	}
	gift := giftCreate(t, r, "shared", true)
	ctl := NewBilibiliMessagesController(NewVoyageBilibiliController())
	fake := &fakeMessageSender{}
	ctl.sender = func(uint) (directMessageClient, string, error) { return fake, "99999", nil }
	r.Use(func(c *gin.Context) {
		var admin database.Admin
		admin.ID = 1
		if c.GetHeader("Other") != "" {
			admin.ID = 2
		}
		c.Set("user", admin)
	})
	r.GET("/messages", ctl.List)
	r.POST("/messages", ctl.Prepare)
	r.POST("/api/bilibili/messages", ctl.Prepare)
	r.POST("/messages/:queue/test", ctl.Test)
	r.POST("/messages/:queue/action", ctl.Action)
	r.POST("/messages/:queue/retry", ctl.Retry)
	return r, ctl, fake, gift
}
func messageCall(t *testing.T, r *gin.Engine, path string, body any, want int, other bool) testQueueView {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", path, strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})
	if other {
		req.Header.Set("Other", "yes")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var result struct {
		Code    int           `json:"code"`
		Message string        `json:"message"`
		Data    testQueueView `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Code != want {
		t.Fatalf("%s got %d want %d: %s", path, result.Code, want, result.Message)
	}
	if strings.Contains(w.Body.String(), "fingerprint") || strings.Contains(w.Body.String(), "credentials") {
		t.Fatal("private fields exposed")
	}
	return result.Data
}
func prepareTestQueue(t *testing.T, r *gin.Engine, gift database.DigitalGift) testQueueView {
	return messageCall(t, r, "/messages", gin.H{"scope": "gift", "scope_id": gift.ID, "version": gift.Version, "interval": 30, "items": []gin.H{{"uid": "610010", "version": 0}, {"uid": "610011", "version": 0}}}, 200, false)
}

func TestDirectMessagesSkipConnectedAccountWithoutBlockingBatch(t *testing.T) {
	r, _, _, gift := messageSetup(t)
	view := messageCall(t, r, "/messages", gin.H{"scope": "gift", "scope_id": gift.ID, "version": gift.Version, "interval": 30, "items": []gin.H{{"uid": "99999", "version": 0}, {"uid": "610010", "version": 0}}}, 200, false)
	if len(view.Items) != 1 || view.Items[0].UID != "610010" || !strings.Contains(view.Queue.Note, "99999") {
		t.Fatalf("connected account was not skipped cleanly: %+v", view)
	}
	var selfItems int64
	database.DB.Model(&database.BilibiliMessageItem{}).Where("queue_id = ? AND uid = ?", view.Queue.ID, "99999").Count(&selfItems)
	if selfItems != 0 {
		t.Fatal("connected account was persisted as a message target")
	}
}

func TestDirectMessagesSenderOnlyCreatesNoQueue(t *testing.T) {
	r, _, _, gift := messageSetup(t)
	messageCall(t, r, "/messages", gin.H{"scope": "gift", "scope_id": gift.ID, "version": gift.Version, "interval": 30, "items": []gin.H{{"uid": "99999", "version": 0}}}, 400, false)
	var queues int64
	database.DB.Model(&database.BilibiliMessageQueue{}).Count(&queues)
	if queues != 0 {
		t.Fatal("sender-only request created an empty queue")
	}
}

func clearMessageClock(t *testing.T) {
	t.Helper()
	if err := database.DB.Model(&database.BilibiliMessageClock{}).Where("id = 1").Update("next_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
}
func TestDirectMessagesIntervalRange(t *testing.T) {
	for _, tc := range []struct{ interval, code, stored int }{{0, 200, 5}, {-1, 400, 0}, {4, 400, 0}, {5, 200, 5}, {10, 200, 10}, {300, 200, 300}, {301, 400, 0}} {
		t.Run(fmt.Sprint(tc.interval), func(t *testing.T) {
			r, _, _, gift := messageSetup(t)
			view := messageCall(t, r, "/messages", gin.H{"scope": "gift", "scope_id": gift.ID, "version": gift.Version, "interval": tc.interval, "items": []gin.H{{"uid": "610010", "version": 0}}}, tc.code, false)
			if tc.code == 200 && view.Queue.Interval != tc.stored {
				t.Fatalf("stored interval %d want %d", view.Queue.Interval, tc.stored)
			}
		})
	}
}
func TestDirectMessagesChangeIntervalWhilePausedKeepsConfirmationAndCooldown(t *testing.T) {
	r, ctl, fake, gift := messageSetup(t)
	q := prepareTestQueue(t, r, gift).Queue
	now := time.Now()
	database.DB.Model(&q).Updates(map[string]any{"status": "paused", "test_status": "sent", "test_at": now, "confirmed_at": now})
	path := fmt.Sprintf("/messages/%d/action", q.ID)
	for _, invalid := range []int{0, 4, 301} {
		messageCall(t, r, path, gin.H{"action": "set_interval", "interval": invalid}, 409, false)
	}
	messageCall(t, r, path, gin.H{"action": "set_interval", "interval": 5}, 404, true)
	view := messageCall(t, r, path, gin.H{"action": "set_interval", "interval": 5}, 200, false)
	if view.Queue.Interval != 5 || view.Queue.TestStatus != "sent" || view.Queue.TestAt == nil || view.Queue.Status != "paused" || len(fake.calls) != 0 {
		t.Fatal("interval change altered queue or sent messages")
	}
	ctl.workers[q.ID] = true
	messageCall(t, r, path, gin.H{"action": "confirm", "confirmed": true}, 200, false)
	messageCall(t, r, path, gin.H{"action": "set_interval", "interval": 10}, 409, false)
	if !ctl.tick(q.ID) || len(fake.calls) != 1 {
		t.Fatal("resumed queue did not send first item")
	}
	var clock database.BilibiliMessageClock
	database.DB.First(&clock, 1)
	if wait := time.Until(clock.NextAt); wait <= 0 || wait > 6*time.Second {
		t.Fatalf("unexpected 5-second cooldown %v", wait)
	}
	ctl.tick(q.ID)
	if len(fake.calls) != 1 {
		t.Fatal("5-second interval was not enforced")
	}
	messageCall(t, r, path, gin.H{"action": "pause"}, 200, false)
	messageCall(t, r, path, gin.H{"action": "set_interval", "interval": 10}, 200, false)
	messageCall(t, r, path, gin.H{"action": "confirm", "confirmed": true}, 409, false)
	clearMessageClock(t)
	messageCall(t, r, path, gin.H{"action": "confirm", "confirmed": true}, 200, false)
	if !ctl.tick(q.ID) || len(fake.calls) != 2 {
		t.Fatal("second item not sent after resume")
	}
	database.DB.First(&clock, 1)
	if wait := time.Until(clock.NextAt); wait <= 0 || wait > 11*time.Second {
		t.Fatalf("unexpected 10-second cooldown %v", wait)
	}
	database.DB.Model(&q).Update("status", "testing")
	messageCall(t, r, path, gin.H{"action": "set_interval", "interval": 5}, 409, false)
}
func TestDirectMessagesRequireTestConfirmationAndPreserveInterval(t *testing.T) {
	r, ctl, fake, gift := messageSetup(t)
	view := prepareTestQueue(t, r, gift)
	q := view.Queue
	if len(fake.calls) != 0 {
		t.Fatal("preparation sent a message")
	}
	path := fmt.Sprintf("/messages/%d", q.ID)
	messageCall(t, r, path+"/action", gin.H{"action": "confirm", "confirmed": true}, 409, false)
	messageCall(t, r, path+"/test", gin.H{}, 404, true)
	view = messageCall(t, r, path+"/test", gin.H{}, 200, false)
	if view.Queue.TestStatus != "sent" || len(fake.calls) != 1 || fake.calls[0] != q.TestUID || q.TestUID != "123456" {
		t.Fatal("test did not target authorized recipient")
	}
	var stored database.DigitalGift
	database.DB.First(&stored, gift.ID)
	password, _ := secret.OpenGiftValue(giftScope(gift.ID, ""), stored.PasswordEncrypted)
	if strings.Contains(fake.bodies[0], password) || !strings.Contains(fake.bodies[0], "测试密码") {
		t.Fatal("test leaked real password")
	}
	messageCall(t, r, path+"/test", gin.H{}, 409, false)
	messageCall(t, r, path+"/action", gin.H{"action": "confirm", "confirmed": false}, 409, false)
	messageCall(t, r, path+"/action", gin.H{"action": "confirm", "confirmed": true}, 409, false) // interval still in force
	clearMessageClock(t)
	// Avoid background timers in a unit test; advance the worker explicitly.
	ctl.workers[q.ID] = true
	messageCall(t, r, path+"/action", gin.H{"action": "confirm", "confirmed": true}, 200, false)
	if !ctl.tick(q.ID) || len(fake.calls) != 2 || fake.calls[1] != "610010" {
		t.Fatal("first recipient not sent")
	}
	ctl.tick(q.ID)
	if len(fake.calls) != 2 {
		t.Fatal("interval ignored")
	}
	messageCall(t, r, path+"/action", gin.H{"action": "pause"}, 200, false)
	clearMessageClock(t)
	if ctl.tick(q.ID) || len(fake.calls) != 2 {
		t.Fatal("paused queue sent")
	}
	messageCall(t, r, path+"/action", gin.H{"action": "confirm", "confirmed": true}, 200, false)
	ctl.tick(q.ID)
	ctl.tick(q.ID)
	if len(fake.calls) != 3 || fake.calls[2] != "610011" {
		t.Fatal("resume duplicated or missed recipient")
	}
	database.DB.First(&q, q.ID)
	if q.Status != "completed" {
		t.Fatal("queue not completed")
	}
	var items []database.BilibiliMessageItem
	database.DB.Where("queue_id = ?", q.ID).Find(&items)
	for _, i := range items {
		if strings.Contains(i.Body, password) || strings.Contains(i.Body, "领取") {
			t.Fatal("plaintext persisted")
		}
	}
}
func TestDirectMessagesRetainUnknownAndRecoveryNeverRetries(t *testing.T) {
	r, ctl, fake, gift := messageSetup(t)
	q := prepareTestQueue(t, r, gift).Queue
	now := time.Now()
	database.DB.Model(&q).Updates(map[string]any{"status": "running", "test_status": "sent", "test_at": now, "confirmed_at": now})
	fake.err = &bilibili.MessageError{Note: "response missing", Uncertain: true}
	if !ctl.tick(q.ID) {
		t.Fatal("unknown result blocked the remaining recipients")
	}
	var items []database.BilibiliMessageItem
	database.DB.Where("queue_id = ?", q.ID).Order("id").Find(&items)
	if len(fake.calls) != 1 || items[0].Status != "unknown" || items[1].Status != "pending" {
		t.Fatal("unknown state not retained")
	}
	ctl.tick(q.ID)
	if len(fake.calls) != 1 {
		t.Fatal("automatic retry")
	}
	path := fmt.Sprintf("/messages/%d", q.ID)
	messageCall(t, r, path+"/action", gin.H{"action": "confirm", "confirmed": true}, 409, false)
	messageCall(t, r, path+"/action", gin.H{"action": "retry", "item_id": items[0].ID}, 409, false)
	messageCall(t, r, path+"/action", gin.H{"action": "cancel"}, 200, false)
	messageCall(t, r, "/messages", gin.H{"scope": "gift", "scope_id": gift.ID, "version": gift.Version, "items": []gin.H{{"uid": "610010", "version": 0}}}, 409, false)
	messageCall(t, r, path+"/action", gin.H{"action": "resolve_sent", "item_id": items[0].ID, "confirmed": true}, 200, false)
	// Interrupted sends and interrupted tests survive restart as unknown, not pending.
	database.DB.Model(&q).Updates(map[string]any{"status": "testing", "test_status": "sending"})
	database.DB.Model(&items[1]).Update("status", "sending")
	if err := ctl.Recover(); err != nil {
		t.Fatal(err)
	}
	database.DB.First(&q, q.ID)
	database.DB.First(&items[1], items[1].ID)
	if q.Status != "paused" || q.TestStatus != "unknown" || items[1].Status != "unknown" {
		t.Fatal("restart made an uncertain send retryable")
	}
}
func TestDirectMessagesRevalidateChangedGiftAndVoyageCredentials(t *testing.T) {
	r, ctl, fake, gift := messageSetup(t)
	q := prepareTestQueue(t, r, gift).Queue
	database.DB.Model(&q).Update("status", "running")
	database.DB.Model(&gift).Update("version", gift.Version+1)
	if ctl.tick(q.ID) || len(fake.calls) != 0 {
		t.Fatal("changed gift was sent")
	}
	// Different source, same worker: voyage codes and membership must also be checked.
	database.DB.AutoMigrate(&database.VoyageDispatchConfig{}, &database.VoyageRecipient{})
	issue := database.VoyageIssue{Kind: "batch", Title: "9 月地址收集", Published: true, Version: 1}
	database.DB.Create(&issue)
	database.DB.Create(&database.VoyageBatch{ID: issue.ID, Title: issue.Title, Published: true, Version: 5})
	enc, hash, _ := secret.NewVoyageCode(recipientScope(issue.ID, "610020"))
	row := database.VoyageRecipient{IssueID: issue.ID, UID: "610020", CodeEncrypted: enc, CodeHash: hash, Version: 1}
	database.DB.Create(&row)
	q = database.BilibiliMessageQueue{AdminID: 1, Scope: "voyage", ScopeID: issue.ID, SenderUID: "99999", Status: "running", Interval: 30}
	database.DB.Create(&q)
	source, err := readMessageSource(database.DB, q, row.UID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := secret.SealBilibiliValue(queueScope(q, row.UID), "认证码："+source.Password)
	item := database.BilibiliMessageItem{QueueID: q.ID, UID: row.UID, Body: body, Fingerprint: source.Fingerprint, Status: "pending"}
	database.DB.Create(&item)
	ctl.tick(q.ID)
	database.DB.First(&row, row.ID)
	if len(fake.calls) != 1 || row.SentAt == nil {
		t.Fatal("voyage successful send not marked")
	}
	// A queued old password cannot mark a newly reset credential as sent.
	database.DB.Model(&row).Updates(map[string]any{"sent_at": nil, "version": row.Version + 1})
	if err := ctl.recordSent(q, item, "confirmed"); err != nil {
		t.Fatal(err)
	}
	database.DB.First(&row, row.ID)
	if row.SentAt != nil {
		t.Fatal("reset credential incorrectly marked sent")
	}
}
func TestPersistentBiliLoginSurvivesNewSessionButNotOtherAdmin(t *testing.T) {
	r, ctl, _ := setupBiliController(t, &testGuardImporter{})
	if err := database.InitializeBilibiliMessages(); err != nil {
		t.Fatal(err)
	}
	raw := `{"cookies":{"SESSDATA":"secret-cookie","bili_jct":"csrf","DedeUserID":"42"},"device":"stable-device"}`
	sealed, err := secret.SealBilibiliValue(loginScope(1), raw)
	if err != nil {
		t.Fatal(err)
	}
	database.DB.Create(&database.BilibiliLogin{AdminID: 1, UID: "42", Name: "主播", Credentials: sealed})
	_, view := biliCall(t, r, "GET", "/session", "", "new-mofu-login", nil, 200)
	if view.Status != "connected" || view.Account.UID != "42" {
		t.Fatal("saved account not restored")
	}
	_, view = biliCall(t, r, "GET", "/session", "", "other-login", map[string]string{"X-Test-Admin": "other"}, 200)
	if view.Account != nil {
		t.Fatal("cross-admin login exposed")
	}
	if _, err := secret.OpenBilibiliValue(loginScope(2), sealed); err == nil {
		t.Fatal("owner binding missing")
	}
	if err := ctl.forget(1); err != nil {
		t.Fatal(err)
	}
	_, view = biliCall(t, r, "GET", "/session", "", "new-mofu-login", nil, 200)
	if view.Status != "disconnected" {
		t.Fatal("disconnect restored old credentials")
	}
	var count int64
	database.DB.Model(&database.BilibiliLogin{}).Count(&count)
	if count != 0 {
		t.Fatal("credentials not deleted")
	}
}

func TestMessagePreparationSupportsLargeBatchAndRejectsStaleVersion(t *testing.T) {
	r, _, _, gift := messageSetup(t)
	password := giftPassword(t, r, gift.ID, "")
	body := strings.Repeat("说明", 170) + password
	entries := []gin.H{}
	for n := 0; n < 80; n++ {
		entries = append(entries, gin.H{"uid": fmt.Sprint(700000 + n), "version": 0, "body": body})
	}
	view := messageCall(t, r, "/api/bilibili/messages", gin.H{"scope": "gift", "scope_id": gift.ID, "version": gift.Version, "items": entries}, 200, false)
	if len(view.Items) != 80 {
		t.Fatal("large request truncated")
	}
	messageCall(t, r, fmt.Sprintf("/messages/%d/action", view.Queue.ID), gin.H{"action": "cancel"}, 200, false)
	issue := database.VoyageIssue{Kind: "batch", Title: "9月地址收集", Published: true, Version: 2}
	database.DB.Create(&issue)
	database.DB.Create(&database.VoyageBatch{ID: issue.ID, Title: issue.Title, Published: true, Version: 7})
	database.DB.AutoMigrate(&database.VoyageDispatchConfig{}, &database.VoyageRecipient{})
	enc, hash, _ := secret.NewVoyageCode(recipientScope(issue.ID, "610015"))
	code, _ := secret.OpenVoyageCode(enc, recipientScope(issue.ID, "610015"))
	row := database.VoyageRecipient{IssueID: issue.ID, UID: "610015", CodeEncrypted: enc, CodeHash: hash, Version: 1}
	database.DB.Create(&row)
	req := gin.H{"scope": "voyage", "scope_id": issue.ID, "version": 2, "items": []gin.H{{"uid": row.UID, "version": 1, "body": "UID：610015 认证码：" + code}}}
	messageCall(t, r, "/messages", req, 409, false)
	req["version"] = 7
	messageCall(t, r, "/messages", req, 200, false)
}
