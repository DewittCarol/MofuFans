package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
	"mofufans/internal/bilibili"
	"mofufans/internal/database"
	"mofufans/internal/secret"
)

type VoyageProfileClient interface {
	PublicCard(context.Context, int64) (bilibili.Profile, error)
}
type VoyageRecipientsController struct {
	Client        VoyageProfileClient
	AvatarStorage AvatarStorage
	profileMu     sync.Mutex
	limiter       *rate.Limiter
}

const defaultVoyageTemplate = "你好，{昵称}！感谢你对{主播}的支持！\n这是「{期次}」的专属认证信息：\nB站 UID：{UID}\n本期档位：{档位}\n认证码：{认证码}\n{登记地址}\n请保存好这条私信，后续登记地址、查询物流会用到 UID 和认证码。认证码仅供本人使用，请勿转发。"

func normalizeVoyageTemplate(value string) string {
	value = strings.ReplaceAll(value, "领取码", "认证码")
	return value
}

func recipientScope(issue uint, uid string) string { return fmt.Sprintf("%d:%s", issue, uid) }
func voyageIssueForRequest(c *gin.Context) (database.VoyageIssue, bool) {
	c.Header("Cache-Control", "no-store")
	var issue database.VoyageIssue
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		Fail(c, 400, "期次无效")
		return issue, false
	}
	if err = database.DB.First(&issue, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, 404, "本期不存在，请重新选择")
		} else {
			Fail(c, 500, "期次读取失败，请重试")
		}
		return issue, false
	}
	return issue, true
}
func voyageRecipientForRequest(c *gin.Context, issue uint) (database.VoyageRecipient, bool) {
	var row database.VoyageRecipient
	id, err := strconv.ParseUint(c.Param("recipient"), 10, 32)
	if err != nil || id == 0 {
		Fail(c, 400, "名单用户无效")
		return row, false
	}
	if err = database.DB.Where("issue_id = ? AND id = ?", issue, id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, 404, "用户不在本期名单中")
		} else {
			Fail(c, 500, "用户读取失败，请重试")
		}
		return row, false
	}
	return row, true
}

func (*VoyageRecipientsController) List(c *gin.Context) {
	issue, ok := voyageIssueForRequest(c)
	if !ok {
		return
	}
	rows := []database.VoyageRecipient{}
	if err := database.DB.Where("issue_id = ?", issue.ID).Order("id asc").Find(&rows).Error; err != nil {
		Fail(c, 500, "名单读取失败，请重试")
		return
	}
	Success(c, rows)
}

func (ctl *VoyageRecipientsController) Import(c *gin.Context) {
	if database.DB.Migrator().HasTable(&database.VoyageBatch{}) {
		Fail(c, 409, "请在独立名单管理导入，再在收集批次预览确认")
		return
	}
	issue, ok := voyageIssueForRequest(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var request struct {
		Text string `json:"text"`
	}
	if c.ShouldBindJSON(&request) != nil {
		Fail(c, 400, "名单格式无效或文件超过 1 MB")
		return
	}
	rows, err := parseVoyageImport(request.Text)
	if err != nil {
		Fail(c, 400, err.Error())
		return
	}
	var current []database.VoyageRecipient
	if err = database.DB.Where("issue_id = ?", issue.ID).Find(&current).Error; err != nil {
		Fail(c, 500, "现有名单读取失败")
		return
	}
	existing := map[string]database.VoyageRecipient{}
	for _, row := range current {
		existing[row.UID] = row
	}
	added, skipped, conflicts := 0, 0, 0
	for i := range rows {
		row := &rows[i]
		if row.Status == "duplicate" {
			skipped++
			continue
		}
		if old, exists := existing[row.UID]; exists {
			if row.Tier != "" && old.Tier != row.Tier {
				row.Status = "conflict"
				row.PreviousTier = old.Tier
				conflicts++
			} else {
				row.Status = "existing"
				skipped++
			}
		} else {
			added++
		}
	}
	if c.FullPath() != "" && strings.HasSuffix(c.FullPath(), "/preview") {
		Success(c, gin.H{"rows": rows, "added": added, "skipped": skipped, "conflicts": conflicts})
		return
	}
	if conflicts > 0 {
		Fail(c, 409, "有用户与现有名单档位不同，请先在名单中修正档位，再导入其余用户；原认证码会保留")
		return
	}
	if len(current)+added > 10000 {
		Fail(c, 400, "每期最多 10000 人")
		return
	}
	inserted := 0
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		// Recheck inside the transaction; repeating an import never replaces credentials.
		var lockedIssue database.VoyageIssue
		if err := tx.First(&lockedIssue, issue.ID).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if row.Status == "duplicate" {
				continue
			}
			var old database.VoyageRecipient
			err := tx.Where("issue_id = ? AND uid = ?", issue.ID, row.UID).First(&old).Error
			if err == nil {
				if row.Tier != "" && old.Tier != row.Tier {
					return errors.New("名单已改变，请重新预览")
				}
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			encrypted, hash, err := secret.NewVoyageCode(recipientScope(issue.ID, row.UID))
			if err != nil {
				return err
			}
			entry := database.VoyageRecipient{IssueID: issue.ID, UID: row.UID, Tier: row.Tier, CodeEncrypted: encrypted, CodeHash: hash, Version: 1}
			if err = tx.Create(&entry).Error; err != nil {
				return err
			}
			inserted++
		}
		return nil
	})
	if err != nil {
		Fail(c, 409, "导入未保存，请检查服务器加密配置或重新预览名单后再试")
		return
	}
	Success(c, gin.H{"added": inserted, "skipped": len(rows) - inserted})
}

func (ctl *VoyageRecipientsController) Profile(c *gin.Context) {
	issue, ok := voyageIssueForRequest(c)
	if !ok {
		return
	}
	row, ok := voyageRecipientForRequest(c, issue.ID)
	if !ok {
		return
	}
	if ctl.Client == nil || ctl.AvatarStorage == nil {
		Fail(c, 503, "用户资料服务暂不可用")
		return
	}
	ctl.profileMu.Lock()
	defer ctl.profileMu.Unlock()
	// One request at a time and at most one start per second, across all admin tabs.
	if ctl.limiter == nil {
		ctl.limiter = rate.NewLimiter(rate.Every(time.Second), 1)
	}
	if err := ctl.limiter.Wait(c.Request.Context()); err != nil {
		return
	}
	if err := database.DB.First(&row, row.ID).Error; err != nil {
		Fail(c, 500, "用户读取失败")
		return
	}
	if row.ProfileCheckedAt != nil && time.Since(*row.ProfileCheckedAt) < time.Minute {
		Success(c, row)
		return
	}
	var cached database.VoyageRecipient
	if err := database.DB.Where("uid = ? AND name <> '' AND avatar <> '' AND profile_error = '' AND profile_checked_at > ?", row.UID, time.Now().Add(-24*time.Hour)).First(&cached).Error; err == nil {
		row.Name = cached.Name
		row.Avatar = cached.Avatar
		row.ProfileError = ""
	} else {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
		defer cancel()
		uid, _ := strconv.ParseInt(row.UID, 10, 64)
		profile, err := ctl.Client.PublicCard(ctx, uid)
		if err != nil || profile.MID != uid {
			row.ProfileError = "资料获取失败，可稍后重试；不影响名单和认证码"
		} else {
			row.Name = profile.Name
			stored, err := ctl.AvatarStorage.Save(ctx, uid, profile.Face)
			if err != nil {
				row.ProfileError = "昵称已获取，头像待重试"
			} else {
				row.Avatar = stored
				row.ProfileError = ""
			}
		}
	}
	now := time.Now()
	row.ProfileCheckedAt = &now
	if err := database.DB.Model(&database.VoyageRecipient{}).Where("id = ?", row.ID).Updates(map[string]any{"name": row.Name, "avatar": row.Avatar, "profile_error": row.ProfileError, "profile_checked_at": now}).Error; err != nil {
		Fail(c, 500, "资料保存失败，请重试")
		return
	}
	// Read again so a concurrent tier/notification edit cannot be reverted in the UI.
	if err := database.DB.First(&row, row.ID).Error; err != nil {
		Fail(c, 500, "资料读取失败")
		return
	}
	Success(c, row)
}

func (*VoyageRecipientsController) Update(c *gin.Context) {
	issue, ok := voyageIssueForRequest(c)
	if !ok {
		return
	}
	row, ok := voyageRecipientForRequest(c, issue.ID)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var request struct {
		Version int    `json:"version"`
		Action  string `json:"action"`
		Tier    string `json:"tier"`
	}
	if c.ShouldBindJSON(&request) != nil {
		Fail(c, 400, "操作无效")
		return
	}
	if database.DB.Migrator().HasTable(&database.VoyageBatch{}) && (request.Action == "tier" || request.Action == "enable" || request.Action == "disable") {
		Fail(c, 400, "请修改独立名单后，在批次中预览并同步领取范围")
		return
	}
	changes := map[string]any{"version": row.Version + 1}
	switch request.Action {
	case "tier":
		if strings.TrimSpace(request.Tier) != "" && voyageTier(request.Tier) == "" {
			Fail(c, 400, "请选择有效档位")
			return
		}
		changes["tier"] = voyageTier(request.Tier)
		changes["sent_at"] = nil
	case "disable":
		changes["disabled"] = true
	case "enable":
		changes["disabled"] = false
	case "sent":
		if row.Disabled {
			Fail(c, 400, "已停用的用户不能标记发送")
			return
		}
		changes["sent_at"] = time.Now()
	case "unsent":
		changes["sent_at"] = nil
	case "prepared":
		if row.Disabled {
			Fail(c, 400, "该领取资格已停用")
			return
		}
		changes["prepared_at"] = time.Now()
	default:
		Fail(c, 400, "操作无效")
		return
	}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if e := lockVoyageBatchWrites(tx); e != nil {
			return e
		}
		if request.Action == "sent" {
			if e := syncManuallyNotifiedMessages(tx, "voyage", issue.ID, row.UID); e != nil {
				return e
			}
		}
		result := tx.Model(&database.VoyageRecipient{}).Where("id = ? AND version = ?", row.ID, request.Version).Updates(changes)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errMessageRecipientChanged
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errMessageRecipientChanged) || errors.Is(err, errMessageManualSentBlocked) {
			Fail(c, 409, err.Error())
		} else {
			Fail(c, 500, "保存失败，请重试")
		}
		return
	}
	if err := database.DB.First(&row, row.ID).Error; err != nil {
		Fail(c, 500, "保存后读取失败，请刷新")
		return
	}
	Success(c, row)
}

// Viewing a credential never changes notification, delivery or credential state.
func (*VoyageRecipientsController) Code(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	issue, ok := voyageIssueForRequest(c)
	if !ok {
		return
	}
	row, ok := voyageRecipientForRequest(c, issue.ID)
	if !ok {
		return
	}
	code, err := secret.OpenVoyageCode(row.CodeEncrypted, recipientScope(row.IssueID, row.UID))
	if err != nil {
		Fail(c, 500, "认证码无法读取，请检查服务器加密密钥")
		return
	}
	Success(c, gin.H{"code": code})
}

func (*VoyageRecipientsController) Dispatch(c *gin.Context) {
	issue, ok := voyageIssueForRequest(c)
	if !ok {
		return
	}
	var config database.VoyageDispatchConfig
	err := database.DB.First(&config, "issue_id = ?", issue.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		config = database.VoyageDispatchConfig{IssueID: issue.ID, Template: defaultVoyageTemplate}
	} else if err != nil {
		Fail(c, 500, "私信设置读取失败")
		return
	}
	if _, ok, e := batchFor(database.DB, issue.ID); e != nil {
		Fail(c, 500, "批次读取失败")
		return
	} else if ok {
		if config.Template == defaultVoyageTemplate {
			config.Template = "你好，{昵称}！本次登记地址涉及：{本次礼物}。\n请到「{批次名称}」填写一次收货地址，礼物可能分次寄出。\n\nB站 UID：{UID}\n认证码：{认证码}\n{登记地址}\n\n请保存认证信息，后续可用于查询物流。"
		}
	} else {
		config.Template = normalizeVoyageTemplate(config.Template)
	}

	rows, err := voyageRecipients(database.DB, issue.ID)
	if err != nil {
		Fail(c, 500, "名单读取失败")
		return
	}
	type item struct {
		database.VoyageRecipient
		Code      string                `json:"code"`
		CodeError bool                  `json:"code_error"`
		Message   messageRecipientState `json:"message"`
	}
	states, e := latestMessageStates(database.DB, "voyage", issue.ID)
	if e != nil {
		Fail(c, 500, "私信发送状态读取失败")
		return
	}
	items := make([]item, 0, len(rows))
	for _, row := range rows {
		entry := item{VoyageRecipient: row, Message: states[row.UID]}
		if !row.Disabled {
			entry.Code, err = secret.OpenVoyageCode(row.CodeEncrypted, recipientScope(row.IssueID, row.UID))
			entry.CodeError = err != nil
		}
		items = append(items, entry)
	}
	Success(c, gin.H{"config": config, "recipients": items})
}

func (*VoyageRecipientsController) SaveDispatch(c *gin.Context) {
	issue, ok := voyageIssueForRequest(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var request database.VoyageDispatchConfig
	if c.ShouldBindJSON(&request) != nil {
		Fail(c, 400, "私信设置格式无效")
		return
	}
	request.Template = normalizeVoyageTemplate(strings.TrimSpace(request.Template))
	request.EntryURL = strings.TrimSpace(request.EntryURL)
	if utf8.RuneCountInString(request.Template) > 2000 || !strings.Contains(request.Template, "{UID}") || !strings.Contains(request.Template, "{认证码}") {
		Fail(c, 400, "私信最多 2000 字，需保留 {UID} 和 {认证码}")
		return
	}
	if request.EntryURL != "" {
		u, err := url.Parse(request.EntryURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || len(request.EntryURL) > 2000 {
			Fail(c, 400, "登记入口需要完整的 http 或 https 网页地址")
			return
		}
	}
	version := request.Version
	request.IssueID = issue.ID
	request.Version++
	if version == 0 {
		if err := database.DB.Create(&request).Error; err != nil {
			Fail(c, 409, "设置已改变，请刷新后重试")
			return
		}
	} else {
		result := database.DB.Model(&database.VoyageDispatchConfig{}).Where("issue_id = ? AND version = ?", issue.ID, version).Updates(map[string]any{"template": request.Template, "entry_url": request.EntryURL, "version": request.Version})
		if result.Error != nil {
			Fail(c, 500, "私信设置保存失败")
			return
		}
		if result.RowsAffected != 1 {
			Fail(c, 409, "设置已被其他页面修改，请刷新后重试")
			return
		}
	}
	Success(c, request)
}
