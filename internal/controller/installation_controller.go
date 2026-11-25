package controller

import (
	"context"
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mofufans/internal/adminauth"
	"mofufans/internal/database"
	"mofufans/internal/siteconfig"
	"mofufans/internal/storage"
	"mofufans/internal/theme"
	"net/http"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
)

type InstallationController struct {
	Store        *siteconfig.Store
	Token        string
	required     atomic.Bool
	mu           sync.Mutex
	limiter      *rate.Limiter
	CheckStorage func(context.Context, siteconfig.Options) error
}

func NewInstallationController(store *siteconfig.Store) (*InstallationController, error) {
	ctl := &InstallationController{Store: store, limiter: rate.NewLimiter(rate.Every(2*time.Second), 5), CheckStorage: checkOSS}
	var count int64
	if err := database.DB.Model(&database.Admin{}).Count(&count).Error; err != nil {
		return nil, err
	}
	ctl.required.Store(count == 0)
	if count == 0 {
		var err error
		ctl.Token, err = store.SetupToken()
		if err != nil {
			return nil, err
		}
	} else {
		store.RemoveSetupToken()
	}
	return ctl, nil
}
func (ctl *InstallationController) Required() bool { return ctl.required.Load() }
func (ctl *InstallationController) Status(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	Success(c, gin.H{"required": ctl.Required()})
}
func (ctl *InstallationController) Gate(c *gin.Context) {
	if ctl.Required() && !strings.HasPrefix(c.Request.URL.Path, "/api/setup/") && c.Request.URL.Path != "/api/theme" && c.Request.URL.Path != "/api/info" {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "站点尚未完成初始化", "setup_required": true})
		return
	}
	c.Next()
}
func (ctl *InstallationController) ownerToken(c *gin.Context) bool {
	c.Header("Cache-Control", "no-store")
	if !ctl.Required() {
		Fail(c, 409, "初始化已完成，请使用管理员账号登录")
		return false
	}
	if !ctl.limiter.Allow() {
		Fail(c, 429, "尝试过于频繁，请稍后重试")
		return false
	}
	if subtle.ConstantTimeCompare([]byte(c.GetHeader("X-Setup-Token")), []byte(ctl.Token)) != 1 {
		Fail(c, 403, "安装码不正确，请从服务日志或 data/setup-token 复制完整安装码")
		return false
	}
	return true
}
func installationBody(c *gin.Context, value any) bool {
	if c.GetHeader("X-MofuFans-Request") != "settings" || !strings.HasPrefix(c.GetHeader("Content-Type"), "application/json") {
		Fail(c, 400, "请求格式不正确，请刷新页面重试")
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	d := json.NewDecoder(c.Request.Body)
	d.DisallowUnknownFields()
	if d.Decode(value) != nil || d.Decode(new(any)) != io.EOF {
		Fail(c, 400, "设置格式不正确或内容过长")
		return false
	}
	return true
}
func (ctl *InstallationController) Verify(c *gin.Context) {
	if ctl.ownerToken(c) {
		Success(c, ctl.Store.View())
	}
}

type installationRequest struct {
	Username     string            `json:"username"`
	Password     string            `json:"password"`
	SiteName     string            `json:"site_name"`
	StreamerName string            `json:"streamer_name"`
	Settings     siteconfig.Update `json:"settings"`
}

func (ctl *InstallationController) Complete(c *gin.Context) {
	if !ctl.ownerToken(c) {
		return
	}
	var r installationRequest
	if !installationBody(c, &r) {
		return
	}
	r.Username = strings.TrimSpace(r.Username)
	r.SiteName = strings.TrimSpace(r.SiteName)
	r.StreamerName = strings.TrimSpace(r.StreamerName)
	if r.Username == "" || utf8.RuneCountInString(r.Username) > 64 {
		Fail(c, 400, "管理员用户名请填写 1–64 个字")
		return
	}
	if _, err := adminauth.HashNewPassword(r.Password); err != nil {
		Fail(c, 400, err.Error())
		return
	}
	cfg := theme.Default()
	cfg.Brand.SiteName = r.SiteName
	cfg.Brand.StreamerName = r.StreamerName
	cfg.Brand.TabTitle = r.SiteName
	cfg.Brand.AdminTitle = r.SiteName + " 管理"
	if r.SiteName == "" || r.StreamerName == "" || theme.Validate(cfg) != nil {
		Fail(c, 400, "请填写有效的站点名称和主播名称（最多 80 字）")
		return
	}
	opts, err := ctl.Store.Propose(r.Settings)
	if err != nil {
		Fail(c, 400, err.Error())
		return
	}
	ctl.mu.Lock()
	defer ctl.mu.Unlock()
	if !ctl.Required() {
		Fail(c, 409, "初始化已完成，请登录")
		return
	}
	if opts.StorageType == "oss" {
		if err = ctl.CheckStorage(c.Request.Context(), opts); err != nil {
			Fail(c, 400, err.Error())
			return
		}
	}
	password := md5.Sum([]byte(r.Password))
	passwordHash, err := adminauth.HashPassword(hex.EncodeToString(password[:]))
	if err != nil {
		Fail(c, 500, "密码保存失败，请重试")
		return
	}
	raw, _ := json.Marshal(cfg)
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		var count int64
		if e := tx.Model(&database.Admin{}).Count(&count).Error; e != nil {
			return e
		}
		if count != 0 {
			return errors.New("already initialized")
		}
		if e := tx.Create(&database.Admin{Username: r.Username, Password: passwordHash, IsSuperAdmin: true}).Error; e != nil {
			return e
		}
		if e := tx.Model(&database.SiteTheme{}).Where("id = ?", 1).Updates(map[string]any{"config": string(raw), "revision": gorm.Expr("revision + 1")}).Error; e != nil {
			return e
		}
		return ctl.Store.Save(opts, true)
	})
	if err != nil {
		Fail(c, 500, "初始化未完成，请检查数据目录权限后重试")
		return
	}
	ctl.required.Store(false)
	ctl.Store.RemoveSetupToken()
	Success(c, gin.H{"complete": true})
}
func setupSuperAdmin(c *gin.Context) bool {
	user, ok := c.Get("user")
	admin, valid := user.(database.Admin)
	if !ok || !valid || !admin.IsSuperAdmin {
		Fail(c, 403, "仅站点所有者可以修改部署与集成设置")
		return false
	}
	return true
}
func (ctl *InstallationController) GetSettings(c *gin.Context) {
	if setupSuperAdmin(c) {
		c.Header("Cache-Control", "no-store")
		Success(c, ctl.Store.View())
	}
}
func (ctl *InstallationController) PutSettings(c *gin.Context) {
	if !setupSuperAdmin(c) {
		return
	}
	var r siteconfig.Update
	if !installationBody(c, &r) {
		return
	}
	ctl.mu.Lock()
	defer ctl.mu.Unlock()
	o, err := ctl.Store.Propose(r)
	if err != nil {
		Fail(c, 400, err.Error())
		return
	}
	if siteconfig.StorageChanged(o, ctl.Store.Active()) && !r.ConfirmStorageChange {
		Fail(c, 409, "更换存储不会自动迁移现有文件，请先备份并确认文件迁移安排")
		return
	}
	if o.StorageType == "oss" {
		if err = ctl.CheckStorage(c.Request.Context(), o); err != nil {
			Fail(c, 400, err.Error())
			return
		}
	}
	if err = ctl.Store.Save(o, false); err != nil {
		Fail(c, 500, err.Error())
		return
	}
	Success(c, ctl.Store.View())
}
func (ctl *InstallationController) TestStorage(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/setup/") {
		if !ctl.ownerToken(c) {
			return
		}
	} else if !setupSuperAdmin(c) {
		return
	}
	var r siteconfig.Update
	if !installationBody(c, &r) {
		return
	}
	o, err := ctl.Store.Propose(r)
	if err == nil && o.StorageType == "oss" {
		err = ctl.CheckStorage(c.Request.Context(), o)
	}
	if err != nil {
		Fail(c, 400, err.Error())
		return
	}
	Success(c, gin.H{"message": "连接检查通过"})
}
func checkOSS(ctx context.Context, o siteconfig.Options) error {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	b, err := storage.Bucket(o)
	if err != nil {
		return errors.New("OSS 配置不完整，请检查地域和 Bucket")
	}
	id, err := siteconfig.Random()
	if err != nil {
		return errors.New("无法生成连接测试文件")
	}
	key := path.Join(o.OSS.Prefix, "connection-check", id+".txt")
	fail := errors.New("OSS 检查失败，请检查 Endpoint、地域、密钥，以及此前缀的读、写、列举、删除和 ACL 权限")
	if b.PutObject(key, strings.NewReader("MofuFans connection check"), oss.ObjectACL(oss.ACLPrivate), oss.WithContext(ctx)) != nil {
		return fail
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		_ = b.DeleteObject(key, oss.WithContext(cleanup))
	}()
	if _, err = b.ListObjects(oss.Prefix(path.Dir(key)+"/"), oss.MaxKeys(1), oss.WithContext(ctx)); err != nil {
		return fail
	}
	body, err := b.GetObject(key, oss.WithContext(ctx))
	if err != nil {
		return fail
	}
	_, err = io.Copy(io.Discard, io.LimitReader(body, 100))
	body.Close()
	if err != nil {
		return fail
	}
	if err = b.DeleteObject(key, oss.WithContext(ctx)); err != nil {
		return fail
	}
	return nil
}
