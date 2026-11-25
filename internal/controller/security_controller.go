package controller

import (
	"fmt"
	"mofufans/internal/adminauth"
	"mofufans/internal/database"
	"mofufans/internal/guard"
	"net/http"
	"net/netip"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

type SecurityController struct{ Protection *guard.Manager }

func RequireRecentAdmin(c *gin.Context) {
	u, ok := adminauth.Current(c)
	session := sessions.Default(c)
	until, _ := session.Get("reauth_until").(int64)
	stamp, _ := session.Get("reauth_stamp").(string)
	if !ok || stamp != adminauth.Stamp(u) || until <= time.Now().Unix() || until > time.Now().Add(guard.ReauthLifetime+time.Minute).Unix() {
		c.AbortWithStatusJSON(http.StatusPreconditionRequired, gin.H{"code": 428, "message": "请再次验证当前管理员密码"})
		return
	}
	c.Next()
}
func (*UserController) Reauthenticate(c *gin.Context) {
	u, ok := adminauth.Current(c)
	if !ok {
		Fail(c, 408, "请先登录")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if !installationBody(c, &body) {
		return
	}
	a, ok := guard.BeginAuth(c, "admin-login", fmt.Sprintf("admin:%d", u.ID))
	if !ok {
		return
	}
	defer a.Cancel()
	valid, _ := adminauth.VerifyPassword(u.Password, body.Password)
	if !valid {
		a.Failure()
		Fail(c, 401, "密码不正确")
		return
	}
	s := sessions.Default(c)
	s.Set("reauth_until", time.Now().Add(guard.ReauthLifetime).Unix())
	s.Set("reauth_stamp", adminauth.Stamp(u))
	if s.Save() != nil {
		Fail(c, 500, "验证暂时不可用")
		return
	}
	a.Success()
	Success(c, gin.H{"valid_seconds": int(guard.ReauthLifetime.Seconds())})
}
func securityOwner(c *gin.Context) bool {
	u, ok := c.Get("user")
	admin, typed := u.(database.Admin)
	if !ok || !typed || !admin.IsSuperAdmin {
		Fail(c, 403, "仅站点所有者可管理安全记录")
		return false
	}
	return true
}
func (ctl *SecurityController) View(c *gin.Context) {
	if !securityOwner(c) {
		return
	}
	c.Header("Cache-Control", "private, no-store")
	view := ctl.Protection.View()
	view.ClientIP = c.ClientIP()
	if ip, err := netip.ParseAddr(view.ClientIP); err == nil {
		view.PrivateClientIP = ip.IsPrivate() || ip.IsLoopback()
	}
	Success(c, view)
}
func (ctl *SecurityController) Ban(c *gin.Context) {
	if !securityOwner(c) {
		return
	}
	var body struct {
		IP      string `json:"ip"`
		Minutes int    `json:"minutes"`
	}
	if !installationBody(c, &body) {
		return
	}
	b, err := ctl.Protection.AddBan(body.IP, body.Minutes)
	if err != nil {
		Fail(c, 400, "请检查IP、时长与安全状态存储权限")
		return
	}
	Success(c, b)
}
func (ctl *SecurityController) Unban(c *gin.Context) {
	if !securityOwner(c) {
		return
	}
	if ctl.Protection.RemoveBan(c.Param("id")) != nil {
		Fail(c, 400, "封禁已过期或暂时无法解除")
		return
	}
	Success(c, nil)
}
