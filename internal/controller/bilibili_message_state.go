package controller

import (
	"errors"
	"gorm.io/gorm"
	"mofufans/internal/database"
	"time"
)

var errMessageAlreadySent = errors.New("这位用户已经标记为已发私信")
var errMessageManualSentBlocked = errors.New("该用户有正在发送或结果待确认的私信，请先暂停队列，并在队列中核对发送结果")
var errMessageRecipientChanged = errors.New("此用户已被其他页面修改，请刷新名单后重试")

// The caller holds the same business write lock as the send claim. A manual
// notification is explicitly recorded as manual, never as a Bilibili receipt.
func syncManuallyNotifiedMessages(tx *gorm.DB, scope string, scopeID uint, uid string) error {
	if !tx.Migrator().HasTable(&database.BilibiliMessageItem{}) {
		return nil
	}
	queues := tx.Model(&database.BilibiliMessageQueue{}).Select("id").Where("scope = ? AND scope_id = ?", scope, scopeID)
	var count int64
	if e := tx.Model(&database.BilibiliMessageItem{}).Where("queue_id IN (?) AND uid = ? AND status IN ?", queues, uid, []string{"sending", "unknown"}).Count(&count).Error; e != nil {
		return e
	}
	if count > 0 {
		return errMessageManualSentBlocked
	}
	return tx.Model(&database.BilibiliMessageItem{}).Where("queue_id IN (?) AND uid = ? AND status IN ?", queues, uid, []string{"pending", "failed"}).Updates(manualMessageUpdates()).Error
}

func manualMessageUpdates() map[string]any {
	return map[string]any{"status": "sent", "sent_at": time.Now(), "message_key": "人工确认", "note": "已在名单中手动标记为已发，非 B 站接口回执"}
}

func queuedMessageSource(tx *gorm.DB, q database.BilibiliMessageQueue, item database.BilibiliMessageItem) (messageSource, bool, error) {
	source, err := readMessageSource(tx, q, item.UID)
	if !errors.Is(err, errMessageAlreadySent) {
		return source, false, err
	}
	result := tx.Model(&database.BilibiliMessageItem{}).Where("id = ? AND queue_id = ? AND status IN ?", item.ID, q.ID, []string{"pending", "failed"}).Updates(manualMessageUpdates())
	return source, result.Error == nil, result.Error
}

type messageRecipientState struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

func latestMessageStates(tx *gorm.DB, scope string, scopeID uint) (map[string]messageRecipientState, error) {
	result := map[string]messageRecipientState{}
	if !tx.Migrator().HasTable(&database.BilibiliMessageItem{}) {
		return result, nil
	}
	var rows []struct {
		UID    string
		Status string
		Note   string
	}
	latest := tx.Table("bilibili_message_items AS latest_i").Select("MAX(latest_i.id)").Joins("JOIN bilibili_message_queues AS latest_q ON latest_q.id = latest_i.queue_id").Where("latest_q.scope = ? AND latest_q.scope_id = ?", scope, scopeID).Group("latest_i.uid")
	if e := tx.Model(&database.BilibiliMessageItem{}).Select("uid,status,note").Where("id IN (?)", latest).Find(&rows).Error; e != nil {
		return nil, e
	}
	for _, row := range rows {
		result[row.UID] = messageRecipientState{Status: row.Status, Note: row.Note}
	}
	return result, nil
}
