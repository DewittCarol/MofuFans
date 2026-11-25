package controller

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"mofufans/internal/bilibili"
	"mofufans/internal/database"
	"mofufans/internal/secret"
	"strings"
	"testing"
	"time"
)

func TestMessageConsecutiveErrorsCoolOnlyAfterThreeAndResetAfterSuccess(t *testing.T) {
	_, ctl, fake, q, _ := voyageMessageFixture(t)
	for _, uid := range []string{"610022", "610023"} {
		enc, hash, e := secret.NewVoyageCode(recipientScope(q.ScopeID, uid))
		if e != nil {
			t.Fatal(e)
		}
		row := database.VoyageRecipient{IssueID: q.ScopeID, UID: uid, CodeEncrypted: enc, CodeHash: hash, Version: 1}
		database.DB.Create(&row)
		s, e := readMessageSource(database.DB, q, uid)
		if e != nil {
			t.Fatal(e)
		}
		body, e := secret.SealBilibiliValue(queueScope(q, uid), "认证码："+s.Password)
		if e != nil {
			t.Fatal(e)
		}
		database.DB.Create(&database.BilibiliMessageItem{QueueID: q.ID, UID: uid, Body: body, Fingerprint: s.Fingerprint, Status: "pending"})
	}
	fake.err = &bilibili.MessageError{Note: "25003"}
	var clock database.BilibiliMessageClock
	for i := 0; i < 3; i++ {
		if i > 0 {
			clearMessageClock(t)
		}
		if !ctl.tick(q.ID) {
			t.Fatal("worker stopped instead of retaining failure")
		}
		database.DB.First(&clock, 1)
		if i < 2 && time.Until(clock.NextAt) > 6*time.Second {
			t.Fatal("premature cooldown")
		}
	}
	if time.Until(clock.NextAt) < 9*time.Minute {
		t.Fatal("third consecutive error did not cool")
	}
	database.DB.First(&q, q.ID)
	if !strings.Contains(q.Note, "连续 3 次") {
		t.Fatal("cooldown cause missing")
	}
	if !ctl.tick(q.ID) || len(fake.calls) != 3 {
		t.Fatal("fourth send bypassed cooldown")
	}
	clearMessageClock(t)
	fake.err = nil
	if !ctl.tick(q.ID) || len(fake.calls) != 4 {
		t.Fatal("next recipient not resumed after cooldown")
	}
}

func TestMessageSingleOrdinaryFailureContinuesWithoutCooldown(t *testing.T) {
	_, ctl, fake, q, _ := voyageMessageFixture(t)
	fake.err = &bilibili.MessageError{Note: "25003"}
	if !ctl.tick(q.ID) {
		t.Fatal("ordinary rejection stopped worker")
	}
	var clock database.BilibiliMessageClock
	database.DB.First(&clock, 1)
	if time.Until(clock.NextAt) > 6*time.Second {
		t.Fatal("single error introduced long cooldown")
	}
	fake.err = nil
	clearMessageClock(t)
	if !ctl.tick(q.ID) || len(fake.calls) != 2 || fake.calls[0] == fake.calls[1] {
		t.Fatal("next recipient not processed exactly once")
	}
	if ctl.failureStreak[q.SenderUID] != 0 {
		t.Fatal("successful send did not reset consecutive errors")
	}
}

func retryFixture(t *testing.T) (*gin.Engine, *BilibiliMessagesController, *fakeMessageSender, database.BilibiliMessageQueue, []database.VoyageRecipient, database.BilibiliMessageItem) {
	t.Helper()
	r, ctl, fake, q, rows := voyageMessageFixture(t)
	database.DB.Model(&q).Update("status", "completed")
	var item database.BilibiliMessageItem
	database.DB.Where("queue_id=? AND uid=?", q.ID, rows[0].UID).First(&item)
	database.DB.Model(&item).Updates(map[string]any{"status": "failed", "note": "25003"})
	database.DB.Create(&database.BilibiliMessageClock{ID: 1})
	return r, ctl, fake, q, rows, item
}

func TestMessageExplicitRetrySendsOneFromCompletedAndRetainsReceipt(t *testing.T) {
	r, _, fake, q, rows, item := retryFixture(t)
	path := fmt.Sprintf("/messages/%d/retry", q.ID)
	messageCall(t, r, path, gin.H{"item_id": item.ID}, 404, true)
	messageCall(t, r, path, gin.H{"item_id": item.ID}, 200, false)
	database.DB.First(&item, item.ID)
	database.DB.First(&q, q.ID)
	database.DB.First(&rows[0], rows[0].ID)
	if len(fake.calls) != 1 || fake.calls[0] != rows[0].UID || item.Status != "sent" || item.MessageKey != "1234567890123456789" || rows[0].SentAt == nil || q.Status != "completed" {
		t.Fatal("retry did not preserve receipt/queue semantics")
	}
	clearMessageClock(t)
	messageCall(t, r, path, gin.H{"item_id": item.ID}, 409, false)
	if len(fake.calls) != 1 {
		t.Fatal("duplicate click resent successful item")
	}
	var other database.BilibiliMessageItem
	database.DB.Where("queue_id=? AND uid=?", q.ID, rows[1].UID).First(&other)
	if other.Status != "pending" {
		t.Fatal("single retry started rest of queue")
	}
}

func TestMessageExplicitRetryFailureIsRetainedWithoutAutomaticReplay(t *testing.T) {
	r, ctl, fake, q, _, item := retryFixture(t)
	fake.err = &bilibili.MessageError{Note: "25003"}
	messageCall(t, r, fmt.Sprintf("/messages/%d/retry", q.ID), gin.H{"item_id": item.ID}, 200, false)
	database.DB.First(&item, item.ID)
	if item.Status != "failed" || !strings.HasPrefix(item.Note, "手动重试仍未成功") || len(fake.calls) != 1 {
		t.Fatal("failed retry not retained for user review")
	}
	if ctl.tick(q.ID) || len(fake.calls) != 1 {
		t.Fatal("failed manual retry started automatic resend")
	}
}

func TestMessageExplicitRetryUnknownRequiresConfirmationAndRejectsChangedSource(t *testing.T) {
	r, _, fake, q, rows, item := retryFixture(t)
	path := fmt.Sprintf("/messages/%d/retry", q.ID)
	database.DB.Model(&item).Update("status", "unknown")
	messageCall(t, r, path, gin.H{"item_id": item.ID}, 409, false)
	if len(fake.calls) != 0 {
		t.Fatal("uncertain message resent without confirmation")
	}
	messageCall(t, r, path, gin.H{"item_id": item.ID, "confirmed": true}, 200, false)
	if len(fake.calls) != 1 {
		t.Fatal("confirmed explicit retry did not send")
	}
	// A second recipient's changed credential must not reuse the old body.
	var other database.BilibiliMessageItem
	database.DB.Where("queue_id=? AND uid=?", q.ID, rows[1].UID).First(&other)
	database.DB.Model(&other).Update("status", "failed")
	database.DB.Model(&rows[1]).Update("version", 2)
	clearMessageClock(t)
	messageCall(t, r, path, gin.H{"item_id": other.ID}, 409, false)
	if len(fake.calls) != 1 {
		t.Fatal("changed source was retried")
	}
}

func TestMessageExplicitRetryHonorsClockAndConcurrentSend(t *testing.T) {
	r, ctl, fake, q, _, item := retryFixture(t)
	path := fmt.Sprintf("/messages/%d/retry", q.ID)
	database.DB.Model(&database.BilibiliMessageClock{}).Where("id=1").Update("next_at", time.Now().Add(time.Minute))
	messageCall(t, r, path, gin.H{"item_id": item.ID}, 409, false)
	clearMessageClock(t)
	ctl.sendMu.Lock()
	messageCall(t, r, path, gin.H{"item_id": item.ID}, 409, false)
	ctl.sendMu.Unlock()
	if len(fake.calls) != 0 {
		t.Fatal("retry bypassed cooldown or send lock")
	}
}
