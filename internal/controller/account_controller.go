package controller

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"mofufans/internal/adminauth"
	"mofufans/internal/database"
	"mofufans/internal/guard"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	memberSessionKey              = "member_bilibili_uid"
	memberAuthSessionKey          = "member_auth_v2"
	verificationRequestSessionKey = "bilibili_verification_request_id"
	verificationTokenSessionKey   = "bilibili_verification_token"
)

type AvatarStorage interface {
	Save(context.Context, int64, string) (string, error)
	Delete(string) error
}

type AccountController struct {
	AvatarStorage      AvatarStorage
	SignatureReader    SignatureReader
	LoginProfileReader SignatureReader
	SignatureBrand     func() string
	Now                func() time.Time
	Limits             *MemberRateLimits
	limitOnce          sync.Once
}

// Member access is separate from administrator authentication.
func MemberAccountsEnabled(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var config database.Config
	if err := database.DB.First(&config).Error; err != nil {
		Fail(c, 503, "暂时无法读取账户设置，请稍后再试")
		c.Abort()
		return
	}
	if config.DisableMemberAccounts {
		Fail(c, 403, "登录注册暂未开放")
		c.Abort()
		return
	}
	c.Next()
}

func normalizeBilibiliUID(value string) (int64, string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return 0, "", errors.New("B 站 UID 无效")
	}
	uid, err := strconv.ParseInt(value, 10, 64)
	if err != nil || uid <= 0 {
		return 0, "", errors.New("B 站 UID 无效")
	}
	return uid, strconv.FormatInt(uid, 10), nil
}

func newVerificationToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(raw)
	return token, hashVerificationToken(token), nil
}

func hashVerificationToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func publicUser(user database.User) gin.H {
	return gin.H{"username": user.Username, "bilibili_uid": strconv.FormatInt(user.BilibiliUID, 10), "bilibili_name": user.BilibiliName, "bilibili_avatar": user.BilibiliAvatar, "verified_at": user.VerifiedAt, "signature_verified": user.SignatureVerifiedAt != nil, "signature_verified_at": user.SignatureVerifiedAt, "is_disabled": user.IsDisabled, "created_at": user.CreatedAt, "updated_at": user.UpdatedAt}
}

// Retired routes must not consume an already verified follow request either.
func (*AccountController) StartVerification(c *gin.Context) {
	Fail(c, 410, "关注验证已停用，请刷新页面后使用签名验证")
}
func (*AccountController) VerificationStatus(c *gin.Context) {
	Fail(c, 410, "关注验证已停用，请刷新页面后使用签名验证")
}
func (*AccountController) Register(c *gin.Context) {
	Fail(c, 410, "请刷新页面，通过签名验证完成注册")
}

func memberStamp(user database.User) string {
	return hashVerificationToken("member-v2|" + strconv.FormatInt(user.BilibiliUID, 10) + "|" + user.PasswordHash + "|" + user.AuthVersion)
}

func establishMemberSession(c *gin.Context, user database.User) error {
	s := sessions.Default(c)
	s.Set(memberSessionKey, user.BilibiliUID)
	s.Set(memberAuthSessionKey, memberStamp(user))
	s.Delete(signatureRequestSessionKey)
	s.Delete(signatureSecretSessionKey)
	s.Delete(signatureTextSessionKey)
	s.Delete(verificationRequestSessionKey)
	s.Delete(verificationTokenSessionKey)
	// Preserve administrator fields and their existing twelve-hour cookie lifetime.
	return s.Save()
}

func clearMemberSession(c *gin.Context) error {
	s := sessions.Default(c)
	s.Delete(memberSessionKey)
	s.Delete(memberAuthSessionKey)
	s.Delete(verificationRequestSessionKey)
	s.Delete(verificationTokenSessionKey)
	s.Delete(signatureRequestSessionKey)
	s.Delete(signatureSecretSessionKey)
	s.Delete(signatureTextSessionKey)
	return s.Save()
}

func memberInDB(c *gin.Context, db *gorm.DB) (database.User, bool) {
	s := sessions.Default(c)
	var uid int64
	switch value := s.Get(memberSessionKey).(type) {
	case int64:
		uid = value
	case int:
		uid = int64(value)
	case string:
		uid, _, _ = normalizeBilibiliUID(value)
	}
	stamp, ok := s.Get(memberAuthSessionKey).(string)
	if !ok || uid <= 0 || len(stamp) != 64 {
		return database.User{}, false
	}
	var user database.User
	if db.Where("bilibili_uid = ?", uid).First(&user).Error != nil || user.IsDisabled || user.AuthVersion == "" {
		return database.User{}, false
	}
	if subtle.ConstantTimeCompare([]byte(stamp), []byte(memberStamp(user))) != 1 {
		return database.User{}, false
	}
	return user, true
}

func currentMember(c *gin.Context) (database.User, bool)     { return memberInDB(c, database.DB) }
func MemberCurrentUser(c *gin.Context) (database.User, bool) { return currentMember(c) }
func MemberStrongUser(c *gin.Context) (database.User, bool) {
	return MemberStrongUserInDB(c, database.DB)
}
func MemberStrongUserInDB(c *gin.Context, db *gorm.DB) (database.User, bool) {
	u, ok := memberInDB(c, db)
	return u, ok && u.SignatureVerifiedAt != nil
}

func validMemberPassword(password string) bool {
	return utf8.RuneCountInString(password) >= 8 && len(password) <= 72
}

func (ctl *AccountController) Login(c *gin.Context) {
	if !memberWriteAllowed(c) {
		return
	}
	var body struct {
		Username    string `json:"username"`
		BilibiliUID string `json:"bilibili_uid"`
		Password    string `json:"password"`
	}
	if !memberBind(c, &body) {
		return
	}
	login := strings.TrimSpace(body.Username)
	if body.BilibiliUID != "" {
		login = strings.TrimSpace(body.BilibiliUID)
	}
	if login == "" || len(login) > 128 || len(body.Password) > 72 {
		Fail(c, 401, "账号或密码错误")
		return
	}
	if !ctl.allow(c, "login-ip", c.ClientIP(), 30, 10*time.Minute) {
		return
	}
	var user database.User
	var err error
	rateIdentity := "name:" + login
	if body.BilibiliUID != "" {
		uid, canonical, e := normalizeBilibiliUID(body.BilibiliUID)
		if e != nil {
			Fail(c, 401, "账号或密码错误")
			return
		}
		rateIdentity = "uid:" + canonical
		err = database.DB.Where("bilibili_uid = ?", uid).First(&user).Error
	} else {
		// Legacy login names stay exact. The frontend sends bilibili_uid for UID login.
		err = database.DB.Where("username = ?", login).First(&user).Error
	}
	if err == nil {
		rateIdentity = "uid:" + strconv.FormatInt(user.BilibiliUID, 10)
	}
	if !ctl.allow(c, "login-account", rateIdentity, 12, 10*time.Minute) {
		return
	}
	attempt, allowed := guard.BeginAuth(c, "member-login", rateIdentity)
	if !allowed {
		return
	}
	defer attempt.Cancel()
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.Password)) != nil {
		if err != nil {
			adminauth.DummyCompare()
		}
		attempt.Failure()
		Fail(c, 401, "账号或密码错误")
		return
	}
	if user.IsDisabled {
		Fail(c, 403, "账号已被禁用")
		return
	}
	attempt.Success()
	if user.AuthVersion == "" {
		version, _, e := newVerificationToken()
		if e != nil {
			Fail(c, 500, "登录失败，请重试")
			return
		}
		update := database.DB.Model(&database.User{}).Where("bilibili_uid = ? AND password_hash = ? AND is_disabled = ? AND (auth_version IS NULL OR auth_version = '')", user.BilibiliUID, user.PasswordHash, false).Update("auth_version", version)
		if update.Error != nil {
			Fail(c, 500, "登录失败，请重试")
			return
		}
		if update.RowsAffected == 1 {
			user.AuthVersion = version
		} else {
			Fail(c, 401, "账号状态已变化，请重新登录")
			return
		}
	}
	ctl.refreshLoginProfile(c.Request.Context(), user)
	// Never turn a password verified before the network request into a session
	// for credentials that were changed while the request was in flight.
	var confirmed database.User
	err = database.DB.Where("bilibili_uid = ? AND password_hash = ? AND auth_version = ? AND is_disabled = ?", user.BilibiliUID, user.PasswordHash, user.AuthVersion, false).First(&confirmed).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, 401, "账号状态已变化，请重新登录")
		return
	}
	if err != nil {
		Fail(c, 503, "暂时无法确认账号状态，请重试")
		return
	}
	user = confirmed
	if establishMemberSession(c, user) != nil {
		Fail(c, 500, "登录失败，请重试")
		return
	}
	Success(c, publicUser(user))
}

func (*AccountController) Info(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	user, ok := currentMember(c)
	if !ok {
		Fail(c, 408, "请先登录")
		return
	}
	Success(c, publicUser(user))
}
func (*AccountController) Logout(c *gin.Context) {
	if !memberWriteAllowed(c) {
		return
	}
	var body struct {
		UID string `json:"uid"`
	}
	if !memberBind(c, &body) {
		return
	}
	if user, ok := currentMember(c); ok && !expectedMemberUID(c, body.UID, user.BilibiliUID) {
		return
	}
	if clearMemberSession(c) != nil {
		Fail(c, 500, "退出失败，请重试")
		return
	}
	Success(c, nil)
}

func expectedMemberUID(c *gin.Context, value string, uid int64) bool {
	requested, _, err := normalizeBilibiliUID(value)
	if err != nil || requested != uid {
		Fail(c, 403, "登录账号已变化，请刷新页面后重试")
		return false
	}
	return true
}

func (ctl *AccountController) ChangePassword(c *gin.Context) { ctl.rotateMember(c, true) }
func (ctl *AccountController) LogoutOthers(c *gin.Context)   { ctl.rotateMember(c, false) }

func (ctl *AccountController) rotateMember(c *gin.Context, changePassword bool) {
	if !memberWriteAllowed(c) {
		return
	}
	user, ok := currentMember(c)
	if !ok {
		Fail(c, 408, "请先登录")
		return
	}
	var body struct {
		CurrentPassword string `json:"current_password"`
		Password        string `json:"password"`
		UID             string `json:"uid"`
	}
	if !memberBind(c, &body) {
		return
	}
	if !expectedMemberUID(c, body.UID, user.BilibiliUID) {
		return
	}
	if !ctl.allow(c, "password-ip", c.ClientIP(), 20, 10*time.Minute) || !ctl.allow(c, "password-uid", strconv.FormatInt(user.BilibiliUID, 10), 10, 10*time.Minute) {
		return
	}
	old := body.Password
	if changePassword {
		old = body.CurrentPassword
	}
	attempt, allowed := guard.BeginAuth(c, "member-login", "uid:"+strconv.FormatInt(user.BilibiliUID, 10))
	if !allowed {
		return
	}
	defer attempt.Cancel()
	if len(old) > 72 || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(old)) != nil {
		attempt.Failure()
		Fail(c, 401, "当前密码不正确")
		return
	}
	attempt.Success()
	version, _, err := newVerificationToken()
	if err != nil {
		Fail(c, 500, "保存失败，请重试")
		return
	}
	newHash := user.PasswordHash
	if changePassword {
		if !validMemberPassword(body.Password) {
			Fail(c, 400, "密码至少 8 个字符，中文等多字节字符合计不超过 72 字节")
			return
		}
		hash, e := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if e != nil {
			Fail(c, 500, "保存失败，请重试")
			return
		}
		newHash = string(hash)
	}
	result := database.DB.Model(&database.User{}).Where("bilibili_uid = ? AND password_hash = ? AND auth_version = ? AND is_disabled = ?", user.BilibiliUID, user.PasswordHash, user.AuthVersion, false).Updates(map[string]any{"password_hash": newHash, "auth_version": version})
	if result.Error != nil || result.RowsAffected != 1 {
		Fail(c, 409, "账号状态已变化，请重新登录")
		return
	}
	user.PasswordHash = newHash
	user.AuthVersion = version
	if establishMemberSession(c, user) != nil {
		Fail(c, 500, "已更新，请重新登录")
		return
	}
	Success(c, publicUser(user))
}
