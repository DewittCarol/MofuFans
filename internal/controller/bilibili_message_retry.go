package controller

import (
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mofufans/internal/database"
	"mofufans/internal/secret"
	"net/http"
	"strings"
)

// Retry sends exactly one explicitly selected failed item. It never changes a
// completed/paused queue back to running and never queues an automatic replay.
func (ctl *BilibiliMessagesController) Retry(c *gin.Context) {
	q, ok := ctl.load(c)
	if !ok {
		return
	}
	var req struct {
		ItemID    uint `json:"item_id"`
		Confirmed bool `json:"confirmed"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	if !strings.HasPrefix(c.GetHeader("Content-Type"), "application/json") || c.ShouldBindJSON(&req) != nil || req.ItemID == 0 {
		Fail(c, 400, "请选择需要重试的失败私信")
		return
	}
	if !ctl.sendMu.TryLock() {
		Fail(c, 409, "正在发送其他私信，请稍候再重试")
		return
	}
	defer ctl.sendMu.Unlock()
	if database.DB.First(&q, q.ID).Error != nil {
		Fail(c, 404, "私信队列不存在")
		return
	}
	if q.Status == "testing" {
		Fail(c, 409, "正在发送测试私信，请稍候再重试")
		return
	}
	if err := ctl.cooldown(); err != nil {
		messageFail(c, err)
		return
	}
	client, sender, err := ctl.sender(q.AdminID)
	if err != nil {
		messageFail(c, err)
		return
	}
	if sender != q.SenderUID {
		Fail(c, 409, "登录账号已更换，请使用原发送账号重试")
		return
	}
	var item database.BilibiliMessageItem
	var body string
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		if e := lockVoyageBatchWrites(tx); e != nil {
			return e
		}
		if e := tx.First(&item, "id=? AND queue_id=?", req.ItemID, q.ID).Error; e != nil {
			return errors.New("这条私信不存在")
		}
		if item.Status != "failed" && item.Status != "unknown" {
			return errors.New("只能重试发送失败或结果待确认的私信，请刷新列表")
		}
		if item.Status == "unknown" && !req.Confirmed {
			return errors.New("这条私信结果不明，请先到 B 站确认未发出，再重试")
		}
		var unresolved int64
		if e := tx.Model(&database.BilibiliMessageItem{}).Where("status='sending'").Count(&unresolved).Error; e != nil {
			return e
		}
		if unresolved > 0 {
			return errors.New("有私信正在发送，请稍候再重试")
		}
		source, e := readMessageSource(tx, q, item.UID)
		if e != nil {
			return e
		}
		if source.Fingerprint != item.Fingerprint {
			return errors.New("该用户的名单或地址状态已有变化，请重新预览这条私信")
		}
		body, e = secret.OpenBilibiliValue(queueScope(q, item.UID), item.Body)
		if e != nil {
			return errors.New("私信内容无法读取")
		}
		if e = reserveMessageInterval(tx, q.Interval); e != nil {
			return e
		}
		result := tx.Model(&item).Where("status=?", item.Status).Updates(map[string]any{"status": "sending", "note": "正在手动重试"})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("该记录已被其他页面处理，请刷新")
		}
		return nil
	})
	if err != nil {
		messageFail(c, err)
		return
	}
	ctl.deliverMessage(q, item, client, sender, body, true)
	if database.DB.First(&item, item.ID).Error != nil {
		Fail(c, 500, "重试结果暂时无法读取，请刷新后核对")
		return
	}
	Success(c, gin.H{"id": item.ID, "uid": item.UID, "status": item.Status, "note": item.Note})
}
