package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mofufans/internal/bilibili"
	"mofufans/internal/database"
	"mofufans/internal/secret"
	"mofufans/internal/siteconfig"
)

type directMessageClient interface {
	SendMessage(context.Context, string, string, string) (string, error)
}
type BilibiliMessagesController struct {
	login           *VoyageBilibiliController
	mu              sync.Mutex
	sendMu          sync.Mutex
	workers         map[uint]bool
	resumeAfterStop map[uint]bool
	// Protected by sendMu; a successful send resets consecutive errors for this sender.
	failureStreak map[string]int
	// Tests substitute a sender; production only uses an explicitly QR-connected account.
	sender func(uint) (directMessageClient, string, error)
}

func NewBilibiliMessagesController(login *VoyageBilibiliController) *BilibiliMessagesController {
	ctl := &BilibiliMessagesController{login: login, workers: map[uint]bool{}, resumeAfterStop: map[uint]bool{}, failureStreak: map[string]int{}}
	ctl.sender = ctl.connected
	return ctl
}
func (ctl *BilibiliMessagesController) Recover() error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&database.BilibiliMessageItem{}).Where("status = ?", "sending").Updates(map[string]any{"status": "unknown", "note": "服务重启前正在发送，请先到 B 站核对"}).Error; err != nil {
			return err
		}
		if err := tx.Model(&database.BilibiliMessageQueue{}).Where("test_status = ?", "sending").Updates(map[string]any{"test_status": "unknown"}).Error; err != nil {
			return err
		}
		return tx.Model(&database.BilibiliMessageQueue{}).Where("status IN ?", []string{"running", "testing"}).Updates(map[string]any{"status": "paused", "note": "服务已重启，队列暂停，请核对后继续"}).Error
	})
}
func (ctl *BilibiliMessagesController) connected(admin uint) (directMessageClient, string, error) {
	s, err := ctl.login.savedSession(admin)
	if err != nil {
		return nil, "", errors.New("B 站登录无法读取，请断开后重新登录")
	}
	if s == nil {
		return nil, "", errors.New("请先扫码登录 B 站")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.view.Status != "connected" || s.view.Account == nil {
		return nil, "", errors.New("请重新扫码登录 B 站")
	}
	if s.view.Job != nil && s.view.Job.Status == "running" {
		return nil, "", errors.New("正在从 B 站获取名单，请完成后再发送私信")
	}
	client, ok := s.client.(directMessageClient)
	if !ok {
		return nil, "", errors.New("当前登录不支持直接私信，请重新扫码")
	}
	return client, s.view.Account.UID, nil
}
func queueScope(q database.BilibiliMessageQueue, uid string) string {
	return fmt.Sprintf("messages:%d:%d:%s", q.AdminID, q.ID, uid)
}
func queueOwner(c *gin.Context) uint {
	if _, ok := voyageBiliOwner(c); !ok {
		return 0
	}
	c.Header("Cache-Control", "private, no-store")
	return biliAdmin(c)
}
func messageFail(c *gin.Context, err error) { Fail(c, 409, err.Error()) }

var errMessageChanged = errors.New("名单、礼物或私信设置已有变化，请取消当前队列，重新预览待发私信")

type messageSource struct {
	Fingerprint, Password, Name, Title, Mode string
	RowID                                    uint
	Version                                  int
}

func readMessageSource(tx *gorm.DB, q database.BilibiliMessageQueue, uid string) (messageSource, error) {
	var s messageSource
	var state any
	if q.Scope == "voyage" {
		var issue database.VoyageIssue
		var row database.VoyageRecipient
		var config database.VoyageDispatchConfig
		var batch database.VoyageBatch
		if tx.First(&issue, q.ScopeID).Error != nil || !issue.Published || issue.Kind != "batch" {
			return s, errors.New("请先发布地址收集批次")
		}
		if tx.First(&batch, q.ScopeID).Error != nil || !batch.Published {
			return s, errors.New("地址收集批次不存在或未发布")
		}
		if tx.Where("issue_id = ? AND uid = ?", q.ScopeID, uid).First(&row).Error != nil || row.Disabled {
			return s, errors.New("收件人已不在收集范围中")
		}
		if row.SentAt != nil {
			return s, errMessageAlreadySent
		}
		if err := tx.Where("issue_id = ?", q.ScopeID).Limit(1).Find(&config).Error; err != nil {
			return s, err
		}
		password, err := secret.OpenVoyageCode(row.CodeEncrypted, recipientScope(q.ScopeID, uid))
		if err != nil {
			return s, errors.New("无法读取认证码")
		}
		s = messageSource{Password: password, Name: row.Name, Title: issue.Title, RowID: row.ID, Version: row.Version}
		state = []any{batch.Version, issue.Version, row.Version, row.CodeHash, config.Version, config.Template, config.EntryURL}
		if tx.Migrator().HasTable(&database.VoyageLongTermAddress{}) {
			d, err := readDelivery(tx, row)
			if err != nil {
				return s, err
			}
			// Never send an outdated reuse notice after the recipient changes or
			// confirms their address. The queue must be previewed again instead.
			state = append(state.([]any), d.Version, d.AddressSource, d.LongTermChanged)
		}
	} else if q.Scope == "gift" {
		var gift database.DigitalGift
		var row database.DigitalGiftRecipient
		if tx.First(&gift, q.ScopeID).Error != nil || !gift.Published {
			return s, errors.New("请先上架电子礼物")
		}
		result := tx.Where("gift_id = ? AND uid = ?", q.ScopeID, uid).Limit(1).Find(&row)
		if result.Error != nil {
			return s, result.Error
		}
		if row.ID != 0 && (row.Disabled || !row.RosterActive) && gift.Mode == "shared" {
			return s, errors.New("收件人已停用通知或移出关联名单")
		}
		if (gift.Mode == "individual" && row.ID == 0) || (row.ID != 0 && (row.Disabled || !row.RosterActive)) {
			return s, errors.New("收件人没有这份礼物的领取资格")
		}
		if row.SentAt != nil {
			return s, errMessageAlreadySent
		}
		s = messageSource{Name: row.Name, Title: gift.Title, Mode: gift.Mode, RowID: row.ID, Version: row.Version}
		if gift.Mode != "open" {
			encrypted, key := gift.PasswordEncrypted, ""
			if gift.Mode == "individual" {
				encrypted, key = row.PasswordEncrypted, uid
			}
			password, err := secret.OpenGiftValue(giftScope(gift.ID, key), encrypted)
			if err != nil {
				return s, errors.New("无法读取礼物密码")
			}
			s.Password = password
		}
		state = []any{gift.Version, gift.AccessVersion, row.Version, row.AccessVersion, gift.PasswordEncrypted, row.PasswordEncrypted}
	} else {
		return s, errors.New("私信来源无效")
	}
	raw, _ := json.Marshal(state)
	hash := sha256.Sum256(raw)
	s.Fingerprint = hex.EncodeToString(hash[:])
	return s, nil
}
func defaultGiftMessage(s messageSource, uid string) string {
	body := "「" + s.Title + "」已开放领取。\n请打开 本站，在「电子礼物」中找到这份礼物。\n"
	if s.Mode == "open" {
		return body + "无需密码，可以直接查看和下载。"
	}
	if s.Mode == "individual" {
		body += "UID：" + uid + "\n"
	}
	body += "礼物密码：" + s.Password + "\n输入"
	if s.Mode == "individual" {
		body += " UID 和"
	}
	return body + "密码即可查看和下载。密码区分大小写，请妥善保管。"
}

type prepareMessageRequest struct {
	Scope         string `json:"scope"`
	ScopeID       uint   `json:"scope_id"`
	Version       int    `json:"version"`
	ConfigVersion int    `json:"config_version"`
	Interval      int    `json:"interval"`
	Items         []struct {
		UID     string `json:"uid"`
		Body    string `json:"body"`
		Version int    `json:"version"`
	} `json:"items"`
}

func (ctl *BilibiliMessagesController) Prepare(c *gin.Context) {
	admin := queueOwner(c)
	if admin == 0 || !deliveryRequest(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
	var req prepareMessageRequest
	if !strings.HasPrefix(c.GetHeader("Content-Type"), "application/json") || c.ShouldBindJSON(&req) != nil || req.ScopeID == 0 || len(req.Items) < 1 || len(req.Items) > 500 || (req.Scope != "voyage" && req.Scope != "gift") {
		Fail(c, 400, "请选择 1 至 500 位收件人")
		return
	}
	if req.Interval == 0 {
		req.Interval = 5
	}
	if req.Interval < 5 || req.Interval > 300 {
		Fail(c, 400, "发送间隔需在 5 至 300 秒之间")
		return
	}
	_, sender, err := ctl.sender(admin)
	if err != nil {
		messageFail(c, err)
		return
	}
	items := make([]struct {
		UID     string `json:"uid"`
		Body    string `json:"body"`
		Version int    `json:"version"`
	}, 0, len(req.Items))
	skippedSender := false
	for _, item := range req.Items {
		if item.UID == sender {
			skippedSender = true
			continue
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		Fail(c, 400, "名单中只有当前发送账号，无需给自己发送私信")
		return
	}
	req.Items = items
	ctl.mu.Lock()
	defer ctl.mu.Unlock()
	testUID := siteconfig.Active().TestUID
	if !claimUID.MatchString(testUID) || testUID == sender {
		messageFail(c, errors.New("请在后台「部署与集成」设置测试 UID，使用你能查看私信且不同于发送账号的 B 站账号"))
		return
	}
	q := database.BilibiliMessageQueue{TestUID: testUID, AdminID: admin, Scope: req.Scope, ScopeID: req.ScopeID, SenderUID: sender, Status: "prepared", TestStatus: "pending", Interval: req.Interval}
	if skippedSender {
		q.Note = fmt.Sprintf("已自动排除当前发送账号 UID %s；不会给自己发送私信。", sender)
	}
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockVoyageBatchWrites(tx); err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&q).Where("admin_id = ? AND status NOT IN ?", admin, []string{"completed", "cancelled"}).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("已有待处理的私信队列，请先完成或取消它；地址收集与电子礼物共用发送队列")
		}
		if err := tx.Table("bilibili_message_items AS i").Joins("JOIN bilibili_message_queues AS q ON q.id = i.queue_id").Where("q.admin_id = ? AND i.status IN ?", admin, []string{"sending", "unknown"}).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("之前的队列还有发送结果待确认，请先核对处理后再准备新的私信")
		}

		if req.Scope == "gift" {
			var g database.DigitalGift
			if tx.First(&g, req.ScopeID).Error != nil || g.Version != req.Version {
				return errMessageChanged
			}
		} else {
			var batch database.VoyageBatch
			var config database.VoyageDispatchConfig
			if tx.First(&batch, req.ScopeID).Error != nil || batch.Version != req.Version {
				return errMessageChanged
			}
			if err := tx.Where("issue_id = ?", req.ScopeID).Limit(1).Find(&config).Error; err != nil {
				return err
			}
			if config.Version != req.ConfigVersion {
				return errMessageChanged
			}
		}
		if err := tx.Create(&q).Error; err != nil {
			return err
		}
		seen := map[string]bool{}
		test := ""
		for _, input := range req.Items {
			if !claimUID.MatchString(input.UID) || seen[input.UID] {
				return errors.New("收件人 UID 无效或重复")
			}
			seen[input.UID] = true
			var uncertain int64
			if err := tx.Table("bilibili_message_items AS i").Joins("JOIN bilibili_message_queues AS q ON q.id = i.queue_id").Where("q.scope = ? AND q.scope_id = ? AND i.uid = ? AND i.status IN ?", q.Scope, q.ScopeID, input.UID, []string{"sending", "unknown"}).Count(&uncertain).Error; err != nil {
				return err
			}
			if uncertain > 0 {
				return errors.New("这位用户仍有结果待确认的私信，请先在之前的队列中核对处理")
			}
			s, err := readMessageSource(tx, q, input.UID)
			if err != nil {
				return fmt.Errorf("UID %s：%s", input.UID, err)
			}
			if input.Version != s.Version {
				return errMessageChanged
			}
			body := strings.TrimSpace(input.Body)
			if body == "" && q.Scope == "gift" {
				body = defaultGiftMessage(s, input.UID)
			}
			if body == "" || utf8.RuneCountInString(body) > 500 || (s.Password != "" && !strings.Contains(body, s.Password)) {
				return errors.New("私信需保留当前认证码或礼物密码，且不超过 500 字")
			}
			if test == "" {
				test = body
				if s.Password != "" {
					test = strings.ReplaceAll(test, s.Password, "测试密码")
				}
				test = strings.ReplaceAll(test, "UID："+input.UID, "UID："+q.TestUID)
			}
			sealed, err := secret.SealBilibiliValue(queueScope(q, input.UID), body)
			if err != nil {
				return err
			}
			item := database.BilibiliMessageItem{QueueID: q.ID, UID: input.UID, Body: sealed, Fingerprint: s.Fingerprint, Status: "pending"}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
		}
		sealed, err := secret.SealBilibiliValue(queueScope(q, "test"), test)
		if err != nil {
			return err
		}
		q.TestBody = sealed
		return tx.Model(&q).Update("test_body", sealed).Error
	})
	if err != nil {
		messageFail(c, err)
		return
	}
	ctl.reply(c, q)
}
func (ctl *BilibiliMessagesController) reply(c *gin.Context, q database.BilibiliMessageQueue) {
	if q.ID == 0 {
		Success(c, nil)
		return
	}
	var items []database.BilibiliMessageItem
	if err := database.DB.Where("queue_id = ?", q.ID).Order("id").Find(&items).Error; err != nil {
		Fail(c, 500, "私信队列读取失败")
		return
	}
	type itemView struct {
		database.BilibiliMessageItem
		Text string `json:"text"`
	}
	view := []itemView{}
	for _, item := range items {
		body, err := secret.OpenBilibiliValue(queueScope(q, item.UID), item.Body)
		if err != nil {
			Fail(c, 500, "私信内容无法读取")
			return
		}
		view = append(view, itemView{item, body})
	}
	test, err := secret.OpenBilibiliValue(queueScope(q, "test"), q.TestBody)
	if err != nil {
		Fail(c, 500, "测试内容无法读取")
		return
	}
	var clock database.BilibiliMessageClock
	database.DB.Where("id = 1").Limit(1).Find(&clock)
	Success(c, gin.H{"queue": q, "items": view, "test_text": test, "next_at": clock.NextAt})
}
func (ctl *BilibiliMessagesController) List(c *gin.Context) {
	admin := queueOwner(c)
	if admin == 0 {
		return
	}
	var q database.BilibiliMessageQueue
	// Return the current queue even when it belongs to the other module.
	result := database.DB.Where("admin_id = ? AND status NOT IN ?", admin, []string{"completed", "cancelled"}).Order("id desc").Limit(1).Find(&q)
	if result.Error != nil {
		Fail(c, 500, "私信队列读取失败")
		return
	}
	if q.ID == 0 {
		database.DB.Table("bilibili_message_queues AS q").Select("q.*").Where("q.admin_id = ? AND EXISTS (SELECT 1 FROM bilibili_message_items i WHERE i.queue_id = q.id AND i.status IN ?)", admin, []string{"sending", "unknown"}).Order("q.id desc").Limit(1).Find(&q)
	}
	if q.ID == 0 {
		database.DB.Where("admin_id = ? AND scope = ? AND scope_id = ?", admin, c.Query("scope"), c.Query("scope_id")).Order("id desc").Limit(1).Find(&q)
	}
	ctl.reply(c, q)
}
func (ctl *BilibiliMessagesController) load(c *gin.Context) (database.BilibiliMessageQueue, bool) {
	var q database.BilibiliMessageQueue
	admin := queueOwner(c)
	if admin == 0 || !deliveryRequest(c) {
		return q, false
	}
	if database.DB.Where("id = ? AND admin_id = ?", c.Param("queue"), admin).First(&q).Error != nil {
		Fail(c, 404, "私信队列不存在")
		return q, false
	}
	return q, true
}
func (ctl *BilibiliMessagesController) Action(c *gin.Context) {
	ctl.mu.Lock()
	defer ctl.mu.Unlock()
	q, ok := ctl.load(c)
	if !ok {
		return
	}
	var req struct {
		Action    string `json:"action"`
		Confirmed bool   `json:"confirmed"`
		ItemID    uint   `json:"item_id"`
		Interval  int    `json:"interval"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	if !strings.HasPrefix(c.GetHeader("Content-Type"), "application/json") || c.ShouldBindJSON(&req) != nil {
		Fail(c, 400, "操作无效")
		return
	}
	if (q.Status == "completed" || q.Status == "cancelled") && req.Action != "resolve_sent" && req.Action != "skip" {
		messageFail(c, errors.New("队列已结束，请刷新查看"))
		return
	}
	var err error
	switch req.Action {
	case "set_interval":
		if q.Status == "running" || q.Status == "testing" {
			err = errors.New("请先暂停队列，再修改发送间隔")
			break
		}
		if req.Interval < 5 || req.Interval > 300 {
			err = errors.New("发送间隔需在 5 至 300 秒之间")
			break
		}
		err = database.DB.Model(&q).Update("interval", req.Interval).Error
	case "pause":
		err = database.DB.Model(&q).Updates(map[string]any{"status": "paused", "note": "已暂停。正在发送的一条仍可能完成"}).Error
	case "cancel":
		err = database.DB.Transaction(func(tx *gorm.DB) error {
			if e := tx.Model(&database.BilibiliMessageItem{}).Where("queue_id = ? AND status IN ?", q.ID, []string{"pending", "failed"}).Updates(map[string]any{"status": "skipped", "note": "已取消"}).Error; e != nil {
				return e
			}
			return tx.Model(&q).Updates(map[string]any{"status": "cancelled", "note": "已取消。已发出的私信保留"}).Error
		})
	case "confirm":
		if !req.Confirmed || q.TestStatus != "sent" || q.TestAt == nil || time.Since(*q.TestAt) > 24*time.Hour {
			err = errors.New("请先发测试私信，确认收到且内容无误后再继续")
			break
		}
		if q.Status == "running" || q.Status == "testing" {
			err = errors.New("队列正在处理，请勿重复操作")
			break
		}
		_, sender, e := ctl.sender(q.AdminID)
		if e != nil {
			err = e
			break
		}
		if sender != q.SenderUID {
			err = errors.New("登录账号已更换，请取消队列并重新测试")
			break
		}
		var items []database.BilibiliMessageItem
		if e = database.DB.Where("queue_id = ? AND status <> ?", q.ID, "sent").Find(&items).Error; e != nil {
			err = e
			break
		}
		for _, item := range items {
			if item.Status == "sending" {
				err = errors.New("有私信正在发送，请稍候再继续")
				break
			}
			if item.Status == "pending" {
				var s messageSource
				var skipped bool
				e := database.DB.Transaction(func(tx *gorm.DB) error {
					if e := lockVoyageBatchWrites(tx); e != nil {
						return e
					}
					var e error
					s, skipped, e = queuedMessageSource(tx, q, item)
					return e
				})
				if skipped {
					continue
				}
				if e != nil || s.Fingerprint != item.Fingerprint {
					err = errMessageChanged
					break
				}
			}
		}
		if err != nil {
			break
		}
		if e = ctl.cooldown(); e != nil {
			err = e
			break
		}
		now := time.Now()
		err = database.DB.Model(&q).Updates(map[string]any{"status": "running", "confirmed_at": now, "note": ""}).Error
		if err == nil {
			ctl.launch(q.ID)
		}
	case "resolve_sent", "skip", "retry":
		if q.Status == "running" || q.Status == "testing" {
			err = errors.New("请先暂停队列")
			break
		}
		var item database.BilibiliMessageItem
		if database.DB.First(&item, "id = ? AND queue_id = ?", req.ItemID, q.ID).Error != nil {
			err = errors.New("这条私信不存在")
			break
		}
		if item.Status == "sending" || item.Status == "sent" || item.Status == "skipped" {
			err = errors.New("这条私信不能执行该操作")
			break
		}
		if (req.Action == "retry" || req.Action == "resolve_sent") && !req.Confirmed {
			err = errors.New("请先到 B 站核对是否已经发送")
			break
		}
		if req.Action == "skip" && item.Status == "unknown" && !req.Confirmed {
			err = errors.New("请先核对这条私信的发送结果")
			break
		}
		if req.Action == "resolve_sent" {
			err = ctl.recordSent(q, item, "人工确认")
		} else {
			status, note := "skipped", "已跳过"
			if req.Action == "retry" {
				status, note = "pending", "已确认未发出，等待继续"
			}
			err = database.DB.Model(&item).Updates(map[string]any{"status": status, "note": note}).Error
		}
	default:
		err = errors.New("操作无效")
	}
	if err != nil {
		messageFail(c, err)
		return
	}
	database.DB.First(&q, q.ID)
	ctl.reply(c, q)
}
func (ctl *BilibiliMessagesController) cooldown() error {
	var clock database.BilibiliMessageClock
	if err := database.DB.Where("id = 1").Limit(1).Find(&clock).Error; err != nil {
		return err
	}
	if time.Now().Before(clock.NextAt) {
		return fmt.Errorf("请等到 %s 后继续发送", clock.NextAt.In(time.FixedZone("CST", 8*3600)).Format("01-02 15:04:05"))
	}
	return nil
}
func (ctl *BilibiliMessagesController) reserve(interval int) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		return reserveMessageInterval(tx, interval)
	})
}
func reserveMessageInterval(tx *gorm.DB, interval int) error {
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&database.BilibiliMessageClock{ID: 1}).Error; err != nil {
		return err
	}
	now := time.Now()
	result := tx.Model(&database.BilibiliMessageClock{}).Where("id = 1 AND next_at <= ?", now).Update("next_at", now.Add(time.Duration(interval)*time.Second))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("请等待上一条私信的发送间隔结束")
	}
	return nil
}
func (ctl *BilibiliMessagesController) Test(c *gin.Context) {
	ctl.mu.Lock()
	q, ok := ctl.load(c)
	if !ok {
		ctl.mu.Unlock()
		return
	}
	var req struct {
		RetryConfirmed bool `json:"retry_confirmed"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	if !strings.HasPrefix(c.GetHeader("Content-Type"), "application/json") || c.ShouldBindJSON(&req) != nil {
		ctl.mu.Unlock()
		Fail(c, 400, "操作无效")
		return
	}
	if q.Status == "running" || q.Status == "testing" || q.Status == "completed" || q.Status == "cancelled" || q.TestStatus == "sent" {
		ctl.mu.Unlock()
		messageFail(c, errors.New("当前队列不能重复测试"))
		return
	}
	if q.TestStatus == "unknown" && !req.RetryConfirmed {
		ctl.mu.Unlock()
		messageFail(c, errors.New("上次测试结果不明，请先到 B 站确认未发出再重试"))
		return
	}
	client, sender, err := ctl.sender(q.AdminID)
	if err == nil && sender != q.SenderUID {
		err = errors.New("登录账号已改变，请重新准备队列")
	}
	if err == nil && (!claimUID.MatchString(q.TestUID) || sender == q.TestUID) {
		err = errors.New("测试收件人无效或与发送账号相同，请重新准备队列")
	}
	if err != nil {
		ctl.mu.Unlock()
		messageFail(c, err)
		return
	}
	body, err := secret.OpenBilibiliValue(queueScope(q, "test"), q.TestBody)
	if err != nil {
		ctl.mu.Unlock()
		Fail(c, 500, "测试私信无法读取")
		return
	}
	ctl.sendMu.Lock()
	if err = ctl.reserve(q.Interval); err != nil {
		ctl.sendMu.Unlock()
		ctl.mu.Unlock()
		messageFail(c, err)
		return
	}
	if err = database.DB.Model(&q).Updates(map[string]any{"status": "testing", "test_status": "sending", "note": "正在向 UID " + q.TestUID + " 发送测试"}).Error; err != nil {
		ctl.sendMu.Unlock()
		ctl.mu.Unlock()
		Fail(c, 500, "测试状态保存失败")
		return
	}
	ctl.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	_, err = client.SendMessage(ctx, sender, q.TestUID, body)
	ctl.extendInterval(q.Interval)
	ctl.persistCookies(q.AdminID, q.SenderUID, client)
	updates := map[string]any{"status": "awaiting_confirmation", "test_status": "sent", "test_at": time.Now(), "note": "测试已提交 B 站。请确认 UID " + q.TestUID + " 收到且内容无误"}
	if err == nil {
		delete(ctl.failureStreak, q.SenderUID)
	}
	if err != nil {
		status := "failed"
		var detail *bilibili.MessageError
		if errors.As(err, &detail) && detail.Uncertain {
			status = "unknown"
		}
		updates = map[string]any{"status": "paused", "test_status": status, "note": err.Error()}
		ctl.restrict(q, err)
	}
	// Cancellation while a request is in flight must not revive the queue.
	result := database.DB.Model(&q).Where("status = ?", "testing").Updates(updates)
	if result.Error == nil && result.RowsAffected == 0 {
		result = database.DB.Model(&q).Updates(map[string]any{"test_status": updates["test_status"], "test_at": updates["test_at"]})
	}
	ctl.sendMu.Unlock()
	if result.Error != nil {
		Fail(c, 500, "测试结果未能保存，请先到 B 站核对")
		return
	}
	database.DB.First(&q, q.ID)
	ctl.reply(c, q)
}
func (ctl *BilibiliMessagesController) restrict(q database.BilibiliMessageQueue, err error) string {
	var detail *bilibili.MessageError
	if errors.Is(err, bilibili.ErrGuardLogin) || (errors.As(err, &detail) && detail.Login) {
		_ = ctl.login.forget(q.AdminID)
		return "B 站登录已失效，请重新扫码"
	}
	if ctl.failureStreak == nil {
		ctl.failureStreak = map[string]int{}
	}
	ctl.failureStreak[q.SenderUID]++
	until := time.Time{}
	reason := ""
	if errors.As(err, &detail) && !detail.RetryAt.IsZero() {
		until = detail.RetryAt
		reason = "B 站明确限制了请求，等待冷却后自动继续"
	} else if ctl.failureStreak[q.SenderUID] >= 3 {
		until = time.Now().Add(10 * time.Minute)
		reason = "连续 3 次发送报错，保守冷却 10 分钟后自动继续"
	}
	if !until.IsZero() {
		if e := database.DB.Model(&database.BilibiliMessageClock{}).Where("id = 1 AND next_at < ?", until).Update("next_at", until).Error; e != nil {
			ctl.pause(q, "冷却状态保存失败，请刷新核对后继续")
			return "冷却状态保存失败，队列已暂停"
		}
		delete(ctl.failureStreak, q.SenderUID)
	}
	return reason
}
func (ctl *BilibiliMessagesController) launch(id uint) {
	if ctl.workers[id] {
		ctl.resumeAfterStop[id] = true
		return
	}
	ctl.workers[id] = true
	go func() {
		defer func() {
			ctl.mu.Lock()
			delete(ctl.workers, id)
			resume := ctl.resumeAfterStop[id]
			delete(ctl.resumeAfterStop, id)
			if resume {
				var current database.BilibiliMessageQueue
				if database.DB.First(&current, id).Error == nil && current.Status == "running" {
					ctl.launch(id)
				}
			}
			ctl.mu.Unlock()
		}()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			if !ctl.tick(id) {
				return
			}
			<-ticker.C
		}
	}()
}
func (ctl *BilibiliMessagesController) tick(id uint) bool {
	ctl.sendMu.Lock()
	defer ctl.sendMu.Unlock()
	var q database.BilibiliMessageQueue
	if database.DB.First(&q, id).Error != nil || q.Status != "running" {
		return false
	}
	var unresolved int64
	if err := database.DB.Model(&database.BilibiliMessageItem{}).Where("queue_id = ? AND status = ?", id, "sending").Count(&unresolved).Error; err != nil {
		ctl.pause(q, "发送状态读取失败，请检查后继续")
		return false
	}
	if unresolved > 0 {
		ctl.pause(q, "有私信正在发送，请核对后继续")
		return false
	}
	var item database.BilibiliMessageItem
	result := database.DB.Where("queue_id = ? AND status = ?", id, "pending").Order("id").Limit(1).Find(&item)
	if result.Error != nil {
		ctl.pause(q, "队列读取失败，请稍后继续")
		return false
	}
	if item.ID == 0 {
		var review int64
		if database.DB.Model(&database.BilibiliMessageItem{}).Where("queue_id=? AND status IN ?", id, []string{"failed", "unknown"}).Count(&review).Error != nil {
			ctl.pause(q, "发送结果读取失败，请刷新后继续")
			return false
		}
		note := "本轮待发私信已处理完"
		if review > 0 {
			note = fmt.Sprintf("本轮待发私信已处理完，仍有 %d 条待复核，请查看发送失败和结果待确认列表", review)
		}
		database.DB.Model(&q).Where("status = ?", "running").Updates(map[string]any{"status": "completed", "note": note})
		return false
	}
	if ctl.cooldown() != nil {
		return true
	}
	client, sender, err := ctl.sender(q.AdminID)
	if err != nil {
		ctl.pause(q, err.Error())
		return false
	}
	if sender != q.SenderUID {
		ctl.pause(q, "登录账号已改变，请重新准备队列")
		return false
	}
	var body string
	claimed := false
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		if e := lockVoyageBatchWrites(tx); e != nil {
			return e
		}
		var current database.BilibiliMessageQueue
		if e := tx.First(&current, q.ID).Error; e != nil {
			return e
		}
		if current.Status != "running" {
			return nil
		}
		if e := tx.First(&item, item.ID).Error; e != nil {
			return e
		}
		if item.Status != "pending" {
			return nil
		}
		source, skipped, e := queuedMessageSource(tx, current, item)
		if skipped {
			return nil
		}
		if e != nil || source.Fingerprint != item.Fingerprint {
			return errMessageChanged
		}
		body, e = secret.OpenBilibiliValue(queueScope(current, item.UID), item.Body)
		if e != nil {
			return errors.New("私信内容无法读取")
		}
		if e = reserveMessageInterval(tx, current.Interval); e != nil {
			return e
		}
		result := tx.Model(&item).Where("status = ? AND EXISTS (SELECT 1 FROM bilibili_message_queues WHERE id = ? AND status = ?)", "pending", q.ID, "running").Update("status", "sending")
		if result.Error != nil {
			return result.Error
		}
		claimed = result.RowsAffected == 1
		return nil
	})
	if err != nil {
		if ctl.cooldown() != nil {
			return true
		}
		ctl.pause(q, err.Error())
		return false
	}
	if !claimed {
		return true
	}
	return ctl.deliverMessage(q, item, client, sender, body, false)
}

// The caller has claimed this item and holds sendMu. Neither queue processing
// nor an explicit retry can overlap another send or repeat an uncertain result.
func (ctl *BilibiliMessagesController) deliverMessage(q database.BilibiliMessageQueue, item database.BilibiliMessageItem, client directMessageClient, sender, body string, manual bool) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	key, err := client.SendMessage(ctx, sender, item.UID, body)
	ctl.extendInterval(q.Interval)
	ctl.persistCookies(q.AdminID, q.SenderUID, client)
	if err != nil {
		status := "failed"
		var detail *bilibili.MessageError
		if errors.As(err, &detail) && detail.Uncertain {
			status = "unknown"
		}
		note := err.Error()
		if manual {
			note = "手动重试仍未成功：" + note
		}
		if e := database.DB.Model(&item).Updates(map[string]any{"status": status, "note": note}).Error; e != nil {
			ctl.pause(q, "发送结果保存失败，请先核对后继续")
			return false
		}
		reason := ctl.restrict(q, err)
		if errors.Is(err, bilibili.ErrGuardLogin) || (errors.As(err, &detail) && detail.Login) {
			ctl.pause(q, err.Error())
			return false
		}
		if reason == "" {
			reason = "个别私信未能发出，已保留待复核记录，继续发送其他用户"
		}
		database.DB.Model(&q).Where("status = ?", "running").Update("note", reason)
		return true
	}
	if err = ctl.recordSent(q, item, key); err != nil {
		ctl.pause(q, "私信已提交，但结果保存失败。请到 B 站核对这条私信后继续")
		database.DB.Model(&item).Updates(map[string]any{"status": "unknown", "note": "发送结果未能保存，请到 B 站核对"})
		return false
	}
	delete(ctl.failureStreak, q.SenderUID)
	database.DB.Model(&q).Where("status = ?", "running").Update("note", "")
	return true
}
func (ctl *BilibiliMessagesController) pause(q database.BilibiliMessageQueue, note string) {
	database.DB.Model(&q).Where("status <> ?", "cancelled").Updates(map[string]any{"status": "paused", "note": note})
}
func (ctl *BilibiliMessagesController) recordSent(q database.BilibiliMessageQueue, item database.BilibiliMessageItem, key string) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockVoyageBatchWrites(tx); err != nil {
			return err
		}
		now := time.Now()
		if err := tx.Model(&item).Updates(map[string]any{"status": "sent", "sent_at": now, "message_key": key, "note": ""}).Error; err != nil {
			return err
		}
		// Only mark the credential that was actually sent; a concurrent reset must stay pending.
		s, err := readMessageSource(tx, q, item.UID)
		if err != nil || s.Fingerprint != item.Fingerprint {
			return nil
		}
		updates := map[string]any{"sent_at": now, "version": gorm.Expr("version + 1")}
		if q.Scope == "voyage" {
			return tx.Model(&database.VoyageRecipient{}).Where("id = ? AND version = ?", s.RowID, s.Version).Updates(updates).Error
		}
		if s.RowID != 0 {
			return tx.Model(&database.DigitalGiftRecipient{}).Where("id = ? AND version = ?", s.RowID, s.Version).Updates(updates).Error
		}
		return nil
	})
}

func (ctl *BilibiliMessagesController) extendInterval(seconds int) {
	until := time.Now().Add(time.Duration(seconds) * time.Second)
	database.DB.Model(&database.BilibiliMessageClock{}).Where("id = 1 AND next_at < ?", until).Update("next_at", until)
}

func (ctl *BilibiliMessagesController) persistCookies(admin uint, uid string, client directMessageClient) {
	if login, ok := client.(interface{ ExportLogin() (string, error) }); ok {
		raw, err := login.ExportLogin()
		if err != nil {
			return
		}
		sealed, err := secret.SealBilibiliValue(loginScope(admin), raw)
		if err != nil {
			return
		}
		// Never recreate a login that was disconnected during the request.
		database.DB.Model(&database.BilibiliLogin{}).Where("admin_id = ? AND uid = ?", admin, uid).Update("credentials", sealed)
	}
}
