package controller

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"errors"
	"mofufans/internal/avatar"
	"mofufans/internal/bilibili"
	"mofufans/internal/database"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const signatureRequestSessionKey = "signature_request_id"
const signatureSecretSessionKey = "signature_request_secret"
const signatureTextSessionKey = "signature_verification_text"
const signatureBrandMaxRunes = 20

var signatureMarker = regexp.MustCompile(`MFV-[A-Z2-7]{26}`)
var signatureShortSuffix = regexp.MustCompile(`验证[a-z2-7]{8}$`)

type SignatureReader interface {
	PublicSignature(context.Context, int64) (bilibili.Profile, error)
}

func signatureAudience(c *gin.Context) string { return strings.ToLower(c.Request.Host) }

// Theme titles are display data, not a source of delimiters or unbounded cookie data.
func cleanSignatureBrand(value string) string {
	var out []rune
	for _, r := range strings.Join(strings.Fields(value), " ") {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || strings.ContainsRune(" ._·-", r) {
			out = append(out, r)
			if len(out) == signatureBrandMaxRunes {
				break
			}
		}
	}
	return strings.TrimSpace(string(out))
}

func (ctl *AccountController) signatureBrand() string {
	if ctl.SignatureBrand == nil {
		return "MofuFun"
	}
	if brand := cleanSignatureBrand(ctl.SignatureBrand()); brand != "" {
		return brand
	}
	return "本站"
}

func signatureTextHash(text string) string {
	// Domain separation keeps the old long-marker hashes independently verifiable.
	return hashVerificationToken("signature-short-v1|" + text)
}

func signatureSessionText(c *gin.Context, request database.SignatureChallenge) string {
	text, ok := sessions.Default(c).Get(signatureTextSessionKey).(string)
	if !ok || len(text) > 128 || !utf8.ValidString(text) || !signatureShortSuffix.MatchString(text) {
		return ""
	}
	brand := strings.TrimSuffix(text[:len(text)-8], "验证")
	if brand == "" || cleanSignatureBrand(brand) != brand || subtle.ConstantTimeCompare([]byte(signatureTextHash(text)), []byte(request.CodeHash)) != 1 {
		return ""
	}
	return text
}

func signatureShortEnd(text string, end int) bool {
	if end == len(text) {
		return true
	}
	next := text[end]
	return !((next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') || (next >= '0' && next <= '9') || next == '_' || next == '-')
}

func signatureMatches(c *gin.Context, request database.SignatureChallenge, signature string) bool {
	if text := signatureSessionText(c, request); text != "" {
		for offset := 0; offset < len(signature); {
			index := strings.Index(signature[offset:], text)
			if index < 0 {
				break
			}
			start := offset + index
			end := start + len(text)
			// The complete branded text is hashed. It can directly follow an old
			// signature, but the eight-character code must not prefix a longer token.
			if signatureShortEnd(signature, end) {
				return true
			}
			offset = end
		}
		return false
	}
	// In-flight challenges from the previous release did not have session text.
	for _, index := range signatureMarker.FindAllStringIndex(signature, -1) {
		marker := signature[index[0]:index[1]]
		if subtle.ConstantTimeCompare([]byte(hashVerificationToken(marker)), []byte(request.CodeHash)) == 1 {
			return true
		}
	}
	return false
}

func (ctl *AccountController) StartSignature(c *gin.Context) {
	if !memberWriteAllowed(c) {
		return
	}
	var body struct {
		BilibiliUID string `json:"bilibili_uid"`
		Purpose     string `json:"purpose"`
	}
	if !memberBind(c, &body) {
		return
	}
	uid, uidText, err := normalizeBilibiliUID(body.BilibiliUID)
	if err != nil {
		Fail(c, 400, err.Error())
		return
	}
	if body.Purpose != "register" && body.Purpose != "recover" && body.Purpose != "upgrade" {
		Fail(c, 400, "验证用途无效")
		return
	}
	if !ctl.allow(c, "start-ip", c.ClientIP(), 12, 10*time.Minute) || !ctl.allow(c, "start-uid", uidText, 5, 10*time.Minute) || !ctl.allow(c, "start-global", "all", 300, 10*time.Minute) {
		return
	}
	var user database.User
	found := database.DB.Where("bilibili_uid = ?", uid).First(&user).Error
	if found != nil && !errors.Is(found, gorm.ErrRecordNotFound) {
		Fail(c, 503, "暂时无法创建验证，请重试")
		return
	}
	if found == nil && user.IsDisabled {
		Fail(c, 403, "账号已被禁用，请联系管理员")
		return
	}
	if body.Purpose == "register" && found == nil {
		Fail(c, 409, "该 UID 已注册，请直接登录或找回密码")
		return
	}
	if body.Purpose != "register" && found != nil {
		Fail(c, 404, "该 UID 尚未注册，请先注册")
		return
	}
	if body.Purpose == "upgrade" {
		current, ok := currentMember(c)
		if !ok {
			Fail(c, 408, "请先登录")
			return
		}
		if current.BilibiliUID != uid {
			Fail(c, 403, "只能验证当前登录账号")
			return
		}
		if current.SignatureVerifiedAt != nil {
			Fail(c, 409, "当前账号已完成签名验证")
			return
		}
	}
	secret, secretHash, err := newVerificationToken()
	if err != nil {
		Fail(c, 500, "创建验证失败，请重试")
		return
	}
	codeBytes := make([]byte, 5)
	if _, err = rand.Read(codeBytes); err != nil {
		Fail(c, 500, "创建验证失败，请重试")
		return
	}
	code := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(codeBytes))
	verificationText := ctl.signatureBrand() + "验证" + code
	now := ctl.clock()
	request := database.SignatureChallenge{BilibiliUID: uid, Purpose: body.Purpose, Audience: signatureAudience(c), SecretHash: secretHash, CodeHash: signatureTextHash(verificationText), Status: database.VerificationPending, RequestedAt: now, ExpiresAt: now.Add(10 * time.Minute)}
	if found == nil {
		request.AccountStamp = memberStamp(user)
	}
	if len(request.Audience) > 255 || request.Audience == "" {
		Fail(c, 400, "站点地址无效")
		return
	}
	// Old records contain only hashes and metadata; bound their retention.
	if err = database.DB.Where("expires_at < ?", now.Add(-24*time.Hour)).Delete(&database.SignatureChallenge{}).Error; err != nil {
		Fail(c, 503, "暂时无法创建验证，请重试")
		return
	}
	if err = database.DB.Create(&request).Error; err != nil {
		Fail(c, 503, "暂时无法创建验证，请重试")
		return
	}
	s := sessions.Default(c)
	// Supersede only this browser's old challenge; another browser cannot cancel it.
	if previous, e := loadSignature(c, database.DB); e == nil {
		database.DB.Model(&database.SignatureChallenge{}).Where("id = ? AND status IN ?", previous.ID, []string{database.VerificationPending, database.VerificationVerified}).Update("status", database.VerificationExpired)
	}
	s.Set(signatureRequestSessionKey, request.ID)
	s.Set(signatureSecretSessionKey, secret)
	s.Set(signatureTextSessionKey, verificationText)
	s.Delete(verificationRequestSessionKey)
	s.Delete(verificationTokenSessionKey)
	if s.Save() != nil {
		database.DB.Delete(&request)
		Fail(c, 500, "创建验证失败，请重试")
		return
	}
	Success(c, gin.H{"challenge_id": strconv.FormatUint(uint64(request.ID), 10), "purpose": request.Purpose, "bilibili_uid": uidText, "verification_text": verificationText, "expires_at": request.ExpiresAt, "status": request.Status})
}

func loadSignature(c *gin.Context, db *gorm.DB) (database.SignatureChallenge, error) {
	s := sessions.Default(c)
	id := s.Get(signatureRequestSessionKey)
	secret, ok := s.Get(signatureSecretSessionKey).(string)
	if id == nil || !ok || len(secret) != 64 {
		return database.SignatureChallenge{}, errors.New("missing challenge")
	}
	var request database.SignatureChallenge
	// Only numeric IDs created by us may reach the database condition.
	var requestID uint
	switch value := id.(type) {
	case uint:
		requestID = value
	case int:
		if value > 0 {
			requestID = uint(value)
		}
	case uint64:
		requestID = uint(value)
	}
	if requestID == 0 {
		return request, errors.New("invalid challenge")
	}
	if err := db.Where("id = ?", requestID).First(&request).Error; err != nil {
		return request, err
	}
	if request.Audience != signatureAudience(c) || subtle.ConstantTimeCompare([]byte(request.SecretHash), []byte(hashVerificationToken(secret))) != 1 {
		return request, errors.New("challenge owner mismatch")
	}
	return request, nil
}

func signatureView(request database.SignatureChallenge, now time.Time) gin.H {
	status := request.Status
	if !now.Before(request.ExpiresAt) || (status == database.VerificationVerified && (request.ConfirmationUntil == nil || !now.Before(*request.ConfirmationUntil))) {
		status = database.VerificationExpired
	}
	data := gin.H{"challenge_id": strconv.FormatUint(uint64(request.ID), 10), "status": status, "purpose": request.Purpose, "bilibili_uid": strconv.FormatInt(request.BilibiliUID, 10), "expires_at": request.ExpiresAt}
	if status == database.VerificationVerified {
		data["bilibili_name"] = request.BilibiliName
		data["confirmation_until"] = request.ConfirmationUntil
	}
	return data
}

func (ctl *AccountController) SignatureStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	request, err := loadSignature(c, database.DB)
	if err != nil {
		Fail(c, 404, "没有进行中的签名验证")
		return
	}
	data := signatureView(request, ctl.clock())
	if data["status"] == database.VerificationPending || data["status"] == database.VerificationVerified {
		if !signatureAccountValid(c, database.DB, request) {
			Fail(c, 409, "账号状态已变化，请重新开始验证")
			return
		}
		if text := signatureSessionText(c, request); text != "" {
			data["verification_text"] = text
		}
	}
	Success(c, data)
}

func signatureAccountValid(c *gin.Context, db *gorm.DB, request database.SignatureChallenge) bool {
	var user database.User
	err := db.Where("bilibili_uid = ?", request.BilibiliUID).First(&user).Error
	if request.Purpose == "register" {
		return errors.Is(err, gorm.ErrRecordNotFound)
	}
	if err != nil || user.IsDisabled || subtle.ConstantTimeCompare([]byte(request.AccountStamp), []byte(memberStamp(user))) != 1 {
		return false
	}
	if request.Purpose == "upgrade" {
		current, ok := memberInDB(c, db)
		return ok && current.BilibiliUID == request.BilibiliUID && current.SignatureVerifiedAt == nil
	}
	return request.Purpose == "recover"
}

func (ctl *AccountController) CheckSignature(c *gin.Context) {
	if !memberWriteAllowed(c) {
		return
	}
	var body struct {
		ChallengeID string `json:"challenge_id"`
	}
	if !memberBind(c, &body) {
		return
	}
	request, err := loadSignature(c, database.DB)
	if err != nil {
		Fail(c, 403, "请在发起验证的浏览器中继续")
		return
	}
	if body.ChallengeID != strconv.FormatUint(uint64(request.ID), 10) {
		Fail(c, 409, "验证页面已变化，请重新开始验证")
		return
	}
	now := ctl.clock()
	if !now.Before(request.ExpiresAt) {
		Fail(c, 410, "验证已过期，请重新开始")
		return
	}
	if !signatureAccountValid(c, database.DB, request) {
		Fail(c, 409, "账号状态已变化，请重新开始验证")
		return
	}
	if request.Status == database.VerificationVerified && request.ConfirmationUntil != nil && now.Before(*request.ConfirmationUntil) {
		Success(c, signatureView(request, now))
		return
	}
	if request.Status != database.VerificationPending {
		Fail(c, 410, "验证已失效，请重新开始")
		return
	}
	uidText := strconv.FormatInt(request.BilibiliUID, 10)
	if !ctl.allow(c, "check-ip", c.ClientIP(), 60, 10*time.Minute) || !ctl.allow(c, "check-uid", uidText, 30, 10*time.Minute) || !ctl.allow(c, "check-global", "all", 180, 10*time.Minute) {
		return
	}
	if request.CheckCount >= 30 {
		Fail(c, 429, "检查次数较多，请稍后重新开始验证")
		return
	}
	cutoff := now.Add(-5 * time.Second)
	result := database.DB.Model(&database.SignatureChallenge{}).Where("id = ? AND status = ? AND check_count < ? AND (last_checked_at IS NULL OR last_checked_at <= ?)", request.ID, database.VerificationPending, 30, cutoff).Updates(map[string]any{"last_checked_at": now, "check_count": gorm.Expr("check_count + 1")})
	if result.Error != nil {
		Fail(c, 503, "暂时无法检查，请重试")
		return
	}
	if result.RowsAffected != 1 {
		c.Header("Retry-After", "5")
		Fail(c, 429, "请稍等几秒再检查签名")
		return
	}
	limits := ctl.limits()
	select {
	case limits.network <- struct{}{}:
		defer func() { <-limits.network }()
	default:
		Fail(c, 503, "当前验证较多，请稍后重试")
		return
	}
	reader := ctl.SignatureReader
	if reader == nil {
		reader = bilibili.NewClient()
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	profile, err := reader.PublicSignature(ctx, request.BilibiliUID)
	if err != nil {
		Fail(c, 502, "暂时无法读取 B 站签名，请稍后重试")
		return
	}
	matched := profile.MID == request.BilibiliUID && signatureMatches(c, request, profile.Signature)
	if !matched {
		Fail(c, 422, "尚未找到本次验证文字。请确认已保存到该账号的个性签名，稍后再检查")
		return
	}
	now = ctl.clock()
	if !now.Before(request.ExpiresAt) {
		Fail(c, 410, "验证已过期，请重新开始")
		return
	}
	until := now.Add(5 * time.Minute)
	if request.ExpiresAt.Before(until) {
		until = request.ExpiresAt
	}
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		// Take a write lock before reading account state, including on SQLite.
		claim := tx.Model(&database.SignatureChallenge{}).Where("id = ? AND status = ? AND expires_at > ?", request.ID, database.VerificationPending, now).Updates(map[string]any{"status": database.VerificationVerified, "verified_at": now, "confirmation_until": until, "bilibili_name": profile.Name, "bilibili_avatar": profile.Face})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return errors.New("challenge unavailable")
		}
		if !signatureAccountValid(c, tx, request) {
			return errors.New("account changed")
		}
		return nil
	})
	if err != nil {
		Fail(c, 409, "验证状态已变化，请重新开始")
		return
	}
	request.Status = database.VerificationVerified
	request.VerifiedAt = &now
	request.ConfirmationUntil = &until
	request.BilibiliName = profile.Name
	Success(c, signatureView(request, now))
}

func (ctl *AccountController) FinishSignature(c *gin.Context) {
	if !memberWriteAllowed(c) {
		return
	}
	var body struct {
		Password    string `json:"password"`
		ChallengeID string `json:"challenge_id"`
	}
	if !memberBind(c, &body) {
		return
	}
	request, err := loadSignature(c, database.DB)
	if err != nil {
		Fail(c, 403, "请先完成签名验证")
		return
	}
	if body.ChallengeID != strconv.FormatUint(uint64(request.ID), 10) {
		Fail(c, 409, "验证页面已变化，请重新开始验证")
		return
	}
	now := ctl.clock()
	if request.Status != database.VerificationVerified || request.VerifiedAt == nil || request.ConfirmationUntil == nil || !now.Before(*request.ConfirmationUntil) || !now.Before(request.ExpiresAt) {
		Fail(c, 410, "验证已失效，请重新开始")
		return
	}
	if !signatureAccountValid(c, database.DB, request) {
		Fail(c, 409, "账号状态已变化，请重新开始验证")
		return
	}
	if !ctl.allow(c, "finish-ip", c.ClientIP(), 20, 10*time.Minute) {
		return
	}
	passwordHash := ""
	if request.Purpose != "upgrade" {
		if !validMemberPassword(body.Password) {
			Fail(c, 400, "密码至少 8 个字符，中文等多字节字符合计不超过 72 字节")
			return
		}
		hash, e := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if e != nil {
			Fail(c, 500, "保存密码失败，请重试")
			return
		}
		passwordHash = string(hash)
	}
	version, _, err := newVerificationToken()
	if err != nil {
		Fail(c, 500, "完成验证失败，请重试")
		return
	}
	storedAvatar := ""
	if request.Purpose == "register" && request.BilibiliAvatar != "" {
		store := ctl.AvatarStorage
		if store == nil {
			store = avatar.NewStore()
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		storedAvatar, _ = store.Save(ctx, request.BilibiliUID, request.BilibiliAvatar)
		cancel()
		// Profile images are optional and must not block a successful identity proof.
	}
	var user database.User
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		now = ctl.clock()
		claim := tx.Model(&database.SignatureChallenge{}).Where("id = ? AND status = ? AND expires_at > ? AND confirmation_until > ?", request.ID, database.VerificationVerified, now, now).Updates(map[string]any{"status": database.VerificationConsumed, "consumed_at": now})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return errors.New("challenge already consumed")
		}
		if !signatureAccountValid(c, tx, request) {
			return errors.New("account changed")
		}
		if request.Purpose == "register" {
			username := strconv.FormatInt(request.BilibiliUID, 10)
			var count int64
			if err := tx.Model(&database.User{}).Where("username = ?", username).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				username = "uid_" + username + "_" + version[:6]
			}
			user = database.User{BilibiliUID: request.BilibiliUID, Username: username, PasswordHash: passwordHash, BilibiliName: request.BilibiliName, BilibiliAvatar: storedAvatar, VerifiedAt: *request.VerifiedAt, SignatureVerifiedAt: request.VerifiedAt, AuthVersion: version}
			return tx.Create(&user).Error
		}
		if err := tx.Where("bilibili_uid = ?", request.BilibiliUID).First(&user).Error; err != nil {
			return err
		}
		oldPasswordHash, oldAuthVersion := user.PasswordHash, user.AuthVersion
		updates := map[string]any{"signature_verified_at": request.VerifiedAt, "auth_version": version, "bilibili_name": request.BilibiliName}
		if request.Purpose == "recover" {
			updates["password_hash"] = passwordHash
			user.PasswordHash = passwordHash
		}
		changed := tx.Model(&database.User{}).Where("bilibili_uid = ? AND password_hash = ? AND COALESCE(auth_version, '') = ? AND is_disabled = ?", request.BilibiliUID, oldPasswordHash, oldAuthVersion, false).Updates(updates)
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return errors.New("account changed during confirmation")
		}
		user.AuthVersion = version
		user.SignatureVerifiedAt = request.VerifiedAt
		user.BilibiliName = request.BilibiliName
		return nil
	})
	if err != nil {
		Fail(c, 409, "验证已被使用或账号状态已变化，请重新开始")
		return
	}
	s := sessions.Default(c)
	s.Delete(signatureRequestSessionKey)
	s.Delete(signatureSecretSessionKey)
	s.Delete(signatureTextSessionKey)
	if establishMemberSession(c, user) != nil {
		Fail(c, 500, "账号已更新，请重新登录")
		return
	}
	Success(c, publicUser(user))
}
