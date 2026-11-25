package controller

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"fmt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
	"mofufans/internal/bilibili"
	"mofufans/internal/database"
	"mofufans/internal/secret"
)

const voyageBiliCookie = "mofu_bili_import"
const voyageBiliPath = "/api/voyage/bilibili"

type guardImportClient interface {
	QR(context.Context) (bilibili.GuardQR, error)
	Poll(context.Context, string) (string, bilibili.GuardAccount, error)
	Month(context.Context, string, time.Time, func(bilibili.GuardProgress)) (bilibili.GuardResult, error)
}

type voyageBiliJob struct {
	RosterID uint                   `json:"roster_id"`
	Month    string                 `json:"month"`
	Status   string                 `json:"status"`
	Error    string                 `json:"error,omitempty"`
	Progress bilibili.GuardProgress `json:"progress"`
	Result   *bilibili.GuardResult  `json:"result,omitempty"`
}
type voyageBiliView struct {
	Status  string                 `json:"status"`
	QRURL   string                 `json:"qr_url,omitempty"`
	Expires time.Time              `json:"expires_at"`
	Account *bilibili.GuardAccount `json:"account,omitempty"`
	Job     *voyageBiliJob         `json:"job,omitempty"`
}
type voyageBiliSession struct {
	mu        sync.Mutex
	adminID   uint
	owner     [32]byte
	created   time.Time
	expires   time.Time
	qrExpires time.Time
	nextPoll  time.Time
	polling   bool
	closed    bool
	key       string
	client    guardImportClient
	cancel    context.CancelFunc
	view      voyageBiliView
}
type VoyageBilibiliController struct {
	mu        sync.Mutex
	sessions  map[string]*voyageBiliSession
	saved     map[uint]*voyageBiliSession
	newClient func() guardImportClient
}

func NewVoyageBilibiliController() *VoyageBilibiliController {
	return &VoyageBilibiliController{sessions: map[string]*voyageBiliSession{}, saved: map[uint]*voyageBiliSession{}, newClient: func() guardImportClient { return bilibili.NewGuardImporter() }}
}

func voyageBiliOwner(c *gin.Context) ([32]byte, bool) {
	value, ok := c.Get("user")
	admin, valid := value.(database.Admin)
	session, err := c.Cookie("session")
	if !ok || !valid || admin.ID == 0 || err != nil || session == "" {
		Fail(c, 401, "请重新登录网站后台")
		return [32]byte{}, false
	}
	// Include the exact Mofu session, so a later login cannot reuse this connection.
	return sha256.Sum256([]byte(session + "/" + strconv.FormatUint(uint64(admin.ID), 10))), true
}

func voyageBiliMutation(c *gin.Context) bool {
	if !deliveryRequest(c) {
		return false
	}
	var body struct{}
	if !strings.HasPrefix(c.GetHeader("Content-Type"), "application/json") || c.ShouldBindJSON(&body) != nil {
		Fail(c, 400, "请求无效，请刷新页面重试")
		return false
	}
	return true
}

func voyageBiliSetCookie(c *gin.Context, token string, age int) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(voyageBiliCookie, token, age, voyageBiliPath, "", true, true)
}

func (s *voyageBiliSession) closeLocked() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.closed = true
	s.client = nil
	s.key = ""
	s.view = voyageBiliView{Status: "disconnected"}
}

func (ctl *VoyageBilibiliController) lookup(c *gin.Context) (*voyageBiliSession, bool) {
	c.Header("Cache-Control", "private, no-store")
	owner, ok := voyageBiliOwner(c)
	if !ok {
		return nil, false
	}
	if saved, err := ctl.savedSession(biliAdmin(c)); err != nil {
		Fail(c, 500, "已保存的 B 站登录无法读取，请断开后重新登录")
		return nil, false
	} else if saved != nil {
		return saved, true
	}
	token, _ := c.Cookie(voyageBiliCookie)
	ctl.mu.Lock()
	s := ctl.sessions[token]
	ctl.mu.Unlock()
	if s == nil {
		return nil, true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.owner != owner || s.closed || !time.Now().Before(s.expires) {
		return nil, true
	}
	if (s.view.Status == "waiting" || s.view.Status == "scanned") && time.Now().After(s.qrExpires) {
		s.view.Status = "expired"
		s.view.QRURL = ""
		s.key = ""
	}
	return s, true
}

func (ctl *VoyageBilibiliController) Status(c *gin.Context) {
	s, ok := ctl.lookup(c)
	if !ok {
		return
	}
	if s == nil {
		Success(c, voyageBiliView{Status: "disconnected"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	Success(c, s.view)
}

func (ctl *VoyageBilibiliController) Connect(c *gin.Context) {
	if !voyageBiliMutation(c) {
		return
	}
	owner, ok := voyageBiliOwner(c)
	if !ok {
		return
	}
	if saved, _ := ctl.savedSession(biliAdmin(c)); saved != nil {
		Fail(c, 409, "已保存 B 站登录。如需更换账号，请先断开登录")
		return
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		Fail(c, 500, "暂时无法创建连接")
		return
	}
	token := hex.EncodeToString(random[:])
	now := time.Now()
	client := ctl.newClient()
	s := &voyageBiliSession{adminID: biliAdmin(c), owner: owner, created: now, expires: now.Add(2 * time.Hour), client: client}
	s.view = voyageBiliView{Status: "waiting", Expires: s.expires}
	ctl.mu.Lock()
	for oldToken, old := range ctl.sessions {
		if old.owner == owner {
			old.mu.Lock()
			if now.Sub(old.created) < 5*time.Second || (old.view.Job != nil && old.view.Job.Status == "running") {
				old.mu.Unlock()
				ctl.mu.Unlock()
				Fail(c, 409, "操作正在进行，请稍后重试")
				return
			}
			old.closeLocked()
			old.mu.Unlock()
			delete(ctl.sessions, oldToken)
		}
	}
	if len(ctl.sessions) >= 64 {
		ctl.mu.Unlock()
		Fail(c, 429, "当前连接较多，请稍后重试")
		return
	}
	ctl.sessions[token] = s
	ctl.mu.Unlock()
	// Remove credentials even when the browser is closed and never polls again.
	time.AfterFunc(2*time.Hour, func() {
		ctl.mu.Lock()
		defer ctl.mu.Unlock()
		s.mu.Lock()
		if s.view.Status != "connected" {
			s.closeLocked()
		}
		s.mu.Unlock()
		delete(ctl.sessions, token)
	})
	qr, err := client.QR(c.Request.Context())
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.closeLocked()
		Fail(c, 502, err.Error())
		return
	}
	if s.closed {
		Fail(c, 409, "连接已取消，请重试")
		return
	}
	s.key = qr.Key
	s.view.QRURL = qr.URL
	s.qrExpires = time.Now().Add(3 * time.Minute)
	voyageBiliSetCookie(c, token, 7200)
	Success(c, s.view)
}

func (ctl *VoyageBilibiliController) Poll(c *gin.Context) {
	if !voyageBiliMutation(c) {
		return
	}
	s, ok := ctl.lookup(c)
	if !ok {
		return
	}
	if s == nil {
		Fail(c, 401, "连接已过期，请重新扫码")
		return
	}
	s.mu.Lock()
	if s.closed || (s.view.Status != "waiting" && s.view.Status != "scanned") || s.polling || time.Now().Before(s.nextPoll) {
		Success(c, s.view)
		s.mu.Unlock()
		return
	}
	s.polling = true
	s.nextPoll = time.Now().Add(2 * time.Second)
	client, key := s.client, s.key
	s.mu.Unlock()
	status, account, err := client.Poll(c.Request.Context(), key)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.polling = false
	if s.closed {
		Fail(c, 401, "连接已取消，请重新扫码")
		return
	}
	if err != nil {
		Fail(c, 502, err.Error())
		return
	}
	s.view.Status = status
	if status == "connected" {
		if persistent, ok := client.(interface{ ExportLogin() (string, error) }); ok {
			raw, e := persistent.ExportLogin()
			if e == nil {
				raw, e = secret.SealBilibiliValue(loginScope(s.adminID), raw)
			}
			if e == nil {
				e = database.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&database.BilibiliLogin{AdminID: s.adminID, UID: account.UID, Name: account.Name, Credentials: raw}).Error
			}
			if e != nil {
				s.closeLocked()
				Fail(c, 500, "登录未能加密保存，请稍后重新扫码")
				return
			}
			s.expires = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
			s.view.Expires = time.Time{}
		}
		s.view.Account = &account
		s.view.QRURL = ""
		s.key = ""
	}
	if status == "expired" {
		s.view.QRURL = ""
		s.key = ""
	}
	Success(c, s.view)
}

func (ctl *VoyageBilibiliController) Disconnect(c *gin.Context) {
	if !voyageBiliMutation(c) {
		return
	}
	if _, ok := voyageBiliOwner(c); !ok {
		return
	}
	if err := ctl.forget(biliAdmin(c)); err != nil {
		Fail(c, 500, "断开登录失败，请重试")
		return
	}
	voyageBiliSetCookie(c, "", -1)
	Success(c, voyageBiliView{Status: "disconnected"})
}

func (ctl *VoyageBilibiliController) Fetch(c *gin.Context) {
	if !voyageBiliMutation(c) {
		return
	}
	roster, ok := rosterRequest(c)
	if !ok {
		return
	}
	if roster.Category != "monthly" {
		Fail(c, 400, "只有月度大航海名单可以从 B 站获取")
		return
	}
	now := time.Now()
	if _, err := bilibili.GuardMonthDays(roster.Month, now); err != nil {
		Fail(c, 400, err.Error())
		return
	}
	s, ok := ctl.lookup(c)
	if !ok {
		return
	}
	if s == nil {
		Fail(c, 401, "请先扫码登录主播账号")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.view.Status != "connected" {
		Fail(c, 401, "请先扫码登录主播账号")
		return
	}
	if s.view.Job != nil && s.view.Job.Status == "running" {
		Fail(c, 409, "正在获取名单，请等待完成或先取消")
		return
	}
	job := &voyageBiliJob{RosterID: roster.ID, Month: roster.Month, Status: "running"}
	s.view.Job = job
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	s.cancel = cancel
	client := s.client
	go func() {
		defer cancel()
		result, err := client.Month(ctx, roster.Month, now, func(p bilibili.GuardProgress) {
			s.mu.Lock()
			defer s.mu.Unlock()
			if !s.closed && s.view.Job == job && job.Status == "running" {
				job.Progress = p
			}
		})
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed || s.view.Job != job || job.Status != "running" {
			return
		}
		s.cancel = nil
		if err != nil {
			job.Status = "error"
			job.Error = err.Error()
			if errors.Is(err, context.DeadlineExceeded) {
				job.Error = "获取超时，未导入任何记录。请稍后重试或导入文件。"
			}
			if errors.Is(err, bilibili.ErrGuardLogin) {
				s.view.Status = "expired"
				s.view.Account = nil
				s.client = nil
				if s.adminID != 0 && database.DB.Migrator().HasTable(&database.BilibiliLogin{}) {
					database.DB.Delete(&database.BilibiliLogin{}, "admin_id = ?", s.adminID)
				}
			}
			return
		}
		job.Status = "done"
		job.Result = &result
	}()
	Success(c, s.view)
}

func (ctl *VoyageBilibiliController) Cancel(c *gin.Context) {
	if !voyageBiliMutation(c) {
		return
	}
	s, ok := ctl.lookup(c)
	if !ok {
		return
	}
	if s == nil {
		Success(c, voyageBiliView{Status: "disconnected"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.view.Job = nil
	Success(c, s.view)
}

func biliAdmin(c *gin.Context) uint {
	value, _ := c.Get("user")
	admin, _ := value.(database.Admin)
	return admin.ID
}
func loginScope(id uint) string { return fmt.Sprintf("login:%d", id) }

func (ctl *VoyageBilibiliController) savedSession(adminID uint) (*voyageBiliSession, error) {
	if adminID == 0 || database.DB == nil || !database.DB.Migrator().HasTable(&database.BilibiliLogin{}) {
		return nil, nil
	}
	ctl.mu.Lock()
	defer ctl.mu.Unlock()
	if s := ctl.saved[adminID]; s != nil {
		s.mu.Lock()
		live := !s.closed && s.view.Status == "connected"
		s.mu.Unlock()
		if live {
			return s, nil
		}
		delete(ctl.saved, adminID)
	}
	var record database.BilibiliLogin
	result := database.DB.Where("admin_id = ?", adminID).Limit(1).Find(&record)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	for _, s := range ctl.sessions {
		if s.adminID == adminID {
			s.mu.Lock()
			live := !s.closed && s.view.Status == "connected"
			s.mu.Unlock()
			if live {
				ctl.saved[adminID] = s
				return s, nil
			}
		}
	}
	raw, err := secret.OpenBilibiliValue(loginScope(adminID), record.Credentials)
	if err != nil {
		return nil, err
	}
	client, err := bilibili.RestoreGuardImporter(raw)
	if err != nil {
		return nil, err
	}
	s := &voyageBiliSession{adminID: adminID, client: client, expires: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), view: voyageBiliView{Status: "connected", Account: &bilibili.GuardAccount{UID: record.UID, Name: record.Name}}}
	ctl.saved[adminID] = s
	return s, nil
}
func (ctl *VoyageBilibiliController) forget(adminID uint) error {
	ctl.mu.Lock()
	defer ctl.mu.Unlock()
	if database.DB.Migrator().HasTable(&database.BilibiliLogin{}) {
		if err := database.DB.Delete(&database.BilibiliLogin{}, "admin_id = ?", adminID).Error; err != nil {
			return err
		}
	}
	if s := ctl.saved[adminID]; s != nil {
		s.mu.Lock()
		s.closeLocked()
		s.mu.Unlock()
		delete(ctl.saved, adminID)
	}
	for token, s := range ctl.sessions {
		if s.adminID == adminID {
			s.mu.Lock()
			s.closeLocked()
			s.mu.Unlock()
			delete(ctl.sessions, token)
		}
	}
	if database.DB.Migrator().HasTable(&database.BilibiliMessageQueue{}) {
		return database.DB.Model(&database.BilibiliMessageQueue{}).Where("admin_id = ? AND status = ?", adminID, "running").Updates(map[string]any{"status": "paused", "note": "B 站已断开登录，请重新登录并测试后继续"}).Error
	}
	return nil
}
