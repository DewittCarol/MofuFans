package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mofufans/internal/siteconfig"
	"mofufans/internal/storage"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mofufans/internal/database"
	"mofufans/internal/guard"
	"mofufans/internal/secret"
)

const giftMaxFile = 25 << 20

type DigitalGiftController struct {
	Directory string
	Files     *storage.Assets
	limit     VoyageDeliveryController
	uploadMu  sync.Mutex
}

func (ctl *DigitalGiftController) directory() string {
	if ctl.Directory != "" {
		return ctl.Directory
	}
	if dir := viper.GetString("digital_gift_directory"); dir != "" {
		return dir
	}
	// Keep uploads with the configured database, outside web roots and release directories.
	if db := viper.GetString("sqlite"); db != "" {
		return filepath.Join(filepath.Dir(strings.Split(db, "?")[0]), "digital-gifts")
	}
	return "data/digital-gifts"
}

func giftScope(id uint, uid string) string { return fmt.Sprintf("gift:%d:password:%s", id, uid) }
func giftCookie(id uint) string            { return fmt.Sprintf("mofu_gift_%d", id) }
func giftPath(id uint) string              { return fmt.Sprintf("/api/gifts/%d", id) }
func giftID(c *gin.Context) uint           { n, _ := strconv.ParseUint(c.Param("id"), 10, 32); return uint(n) }
func giftError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, 404, "礼物不存在或已下架")
		return
	}
	if errors.Is(err, errGiftConflict) {
		Fail(c, 409, err.Error())
		return
	}
	Fail(c, 500, "操作失败，请稍后重试")
}

var errGiftConflict = errors.New("内容已更新，请刷新后重试")

func giftRequest(c *gin.Context, limit int64) bool {
	body := c.Request.Body
	if !deliveryRequest(c) {
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, body, limit)
	return true
}

func (ctl *DigitalGiftController) load(c *gin.Context, files bool) (database.DigitalGift, bool) {
	c.Header("Cache-Control", "private, no-store")
	var gift database.DigitalGift
	q := database.DB
	if files {
		q = q.Preload("Files", func(db *gorm.DB) *gorm.DB { return db.Order("id asc") })
	}
	if !c.GetBool("authed") {
		q = q.Where("published = ?", true)
	}
	if giftID(c) == 0 {
		Fail(c, 404, "礼物不存在或已下架")
		return gift, false
	}
	if err := q.First(&gift, giftID(c)).Error; err != nil {
		giftError(c, err)
		return gift, false
	}
	if gift.Files == nil {
		gift.Files = []database.DigitalGiftFile{}
	}
	return gift, true
}

func (ctl *DigitalGiftController) List(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	rows := []database.DigitalGift{}
	q := database.DB.Order("id desc")
	if !c.GetBool("authed") {
		q = q.Select("id", "title", "intro", "mode", "created_at", "updated_at").Where("published = ?", true)
	}
	if err := q.Find(&rows).Error; err != nil {
		giftError(c, err)
		return
	}
	if c.GetBool("authed") {
		Success(c, rows)
		return
	}
	// A separate projection prevents future model fields from leaking to the locked list.
	result := make([]gin.H, 0, len(rows))
	for _, g := range rows {
		result = append(result, gin.H{"id": g.ID, "title": g.Title, "intro": g.Intro, "mode": g.Mode})
	}
	Success(c, result)
}

func (ctl *DigitalGiftController) Save(c *gin.Context) {
	if !giftRequest(c, 32<<10) {
		return
	}
	var req struct {
		Title     string `json:"title"`
		Intro     string `json:"intro"`
		Content   string `json:"content"`
		Mode      string `json:"mode"`
		Published bool   `json:"published"`
		Version   int    `json:"version"`
		Password  string `json:"password"`
	}
	if c.ShouldBindJSON(&req) != nil {
		Fail(c, 400, "请检查礼物内容")
		return
	}
	req.Title, req.Intro, req.Content, req.Password = strings.TrimSpace(req.Title), strings.TrimSpace(req.Intro), strings.TrimSpace(req.Content), strings.TrimSpace(req.Password)
	if req.Title == "" || utf8.RuneCountInString(req.Title) > 120 || utf8.RuneCountInString(req.Intro) > 500 || utf8.RuneCountInString(req.Content) > 6000 || (req.Mode != "shared" && req.Mode != "individual" && req.Mode != "open") {
		Fail(c, 400, "请填写名称，并选择领取方式；名称最多 120 字，简介 500 字，正文 6000 字")
		return
	}
	if req.Password != "" && (utf8.RuneCountInString(req.Password) < 6 || utf8.RuneCountInString(req.Password) > 64 || strings.IndexFunc(req.Password, unicode.IsSpace) >= 0 || strings.IndexFunc(req.Password, unicode.IsControl) >= 0) {
		Fail(c, 400, "密码须为 6–64 个字符，不能包含空格或换行")
		return
	}
	if req.Mode != "shared" && req.Password != "" {
		Fail(c, 400, "只有统一密码模式可以填写密码")
		return
	}
	var gift database.DigitalGift
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockVoyageBatchWrites(tx); err != nil {
			return err
		}
		if c.Param("id") != "" {
			if giftID(c) == 0 {
				return gorm.ErrRecordNotFound
			}
			if err := tx.First(&gift, giftID(c)).Error; err != nil {
				return err
			}
			if gift.Version != req.Version || gift.Mode != req.Mode {
				return errGiftConflict
			}
		} else {
			gift = database.DigitalGift{Mode: req.Mode, Version: 1, AccessVersion: 1, RosterManaged: true}
			if err := tx.Create(&gift).Error; err != nil {
				return err
			}
		}
		if gift.Published != req.Published {
			gift.AccessVersion++
		}
		if gift.Mode == "shared" && (req.Password != "" || gift.PasswordEncrypted == "") {
			password := req.Password
			var err error
			if password == "" {
				password, err = secret.NewGiftPassword()
				if err != nil {
					return err
				}
			}
			changed := gift.PasswordEncrypted == ""
			if !changed {
				previous, err := secret.OpenGiftValue(giftScope(gift.ID, ""), gift.PasswordEncrypted)
				if err != nil {
					return err
				}
				changed = previous != password
			}
			gift.PasswordEncrypted, err = secret.SealGiftValue(giftScope(gift.ID, ""), password)
			if err != nil {
				return err
			}
			gift.AccessVersion++
			if changed {
				// A new shared password must also reach previously notified members.
				if err := tx.Model(&database.DigitalGiftRecipient{}).Where("gift_id = ? AND sent_at IS NOT NULL", gift.ID).
					Updates(map[string]any{"sent_at": nil, "version": gorm.Expr("version + 1")}).Error; err != nil {
					return err
				}
			}
		}
		version := gift.Version
		gift.Title, gift.Intro, gift.Content, gift.Published = req.Title, req.Intro, req.Content, req.Published
		gift.Version++
		res := tx.Model(&database.DigitalGift{}).Where("id = ? AND version = ?", gift.ID, version).Select("Title", "Intro", "Content", "Published", "Version", "AccessVersion", "PasswordEncrypted").Updates(&gift)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errGiftConflict
		}
		return nil
	})
	if err != nil {
		giftError(c, err)
		return
	}
	gift.Files = []database.DigitalGiftFile{}
	if err = database.DB.Where("gift_id = ?", gift.ID).Order("id asc").Find(&gift.Files).Error; err != nil {
		giftError(c, err)
		return
	}
	Success(c, gift)
}

type giftGrant struct {
	Expires          int64  `json:"expires"`
	Version          int    `json:"version"`
	UID              string `json:"uid"`
	RecipientVersion int    `json:"recipient_version"`
}

func (ctl *DigitalGiftController) authorized(c *gin.Context, gift database.DigitalGift) bool {
	if c.GetBool("authed") || gift.Mode == "open" {
		return true
	}
	cookie, err := c.Cookie(giftCookie(gift.ID))
	if err != nil {
		return false
	}
	raw, err := secret.OpenGiftValue(fmt.Sprintf("gift:%d:grant", gift.ID), cookie)
	if err != nil {
		return false
	}
	var grant giftGrant
	if json.Unmarshal([]byte(raw), &grant) != nil || grant.Expires <= time.Now().Unix() || grant.Version != gift.AccessVersion {
		return false
	}
	if gift.Mode == "individual" {
		var row database.DigitalGiftRecipient
		return database.DB.First(&row, "gift_id = ? AND uid = ? AND disabled = ? AND roster_active = ?", gift.ID, grant.UID, false, true).Error == nil && grant.RecipientVersion == row.AccessVersion
	}
	return grant.UID == ""
}

func (ctl *DigitalGiftController) Detail(c *gin.Context) {
	gift, ok := ctl.load(c, true)
	if !ok {
		return
	}
	if !ctl.authorized(c, gift) {
		Fail(c, 401, "请先输入密码解锁这份礼物")
		return
	}
	Success(c, gift)
}

func (ctl *DigitalGiftController) Unlock(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	if !ctl.limit.allow(c.ClientIP()) {
		Fail(c, 429, "尝试太频繁，请稍后再试")
		return
	}
	var req struct {
		UID      string `json:"uid"`
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&req) != nil || len(req.Password) > 256 {
		Fail(c, 400, "请检查密码")
		return
	}
	gift, ok := ctl.load(c, true)
	if !ok {
		return
	}
	if gift.Mode == "open" {
		Success(c, gift)
		return
	}
	uid := strings.TrimSpace(req.UID)
	targetUID := uid
	if gift.Mode != "individual" {
		targetUID = ""
	}
	attempt, allowed := guard.BeginAuth(c, "claim", fmt.Sprintf("gift:%d:%s", gift.ID, targetUID))
	if !allowed {
		return
	}
	defer attempt.Cancel()
	encrypted := gift.PasswordEncrypted
	grant := giftGrant{Expires: time.Now().Add(12 * time.Hour).Unix(), Version: gift.AccessVersion}
	if gift.Mode == "individual" {
		var row database.DigitalGiftRecipient
		if !claimUID.MatchString(uid) || database.DB.First(&row, "gift_id = ? AND uid = ? AND disabled = ? AND roster_active = ?", gift.ID, uid, false, true).Error != nil {
			attempt.Failure()
			Fail(c, 403, "UID 或密码不正确，或领取资格已停用")
			return
		}
		encrypted = row.PasswordEncrypted
		grant.UID = uid
		grant.RecipientVersion = row.AccessVersion
	} else {
		uid = ""
	}
	if !secret.CheckGiftPassword(giftScope(gift.ID, uid), encrypted, strings.TrimSpace(req.Password)) {
		attempt.Failure()
		Fail(c, 403, "UID 或密码不正确，或领取资格已停用")
		return
	}
	attempt.Success()
	raw, _ := json.Marshal(grant)
	token, err := secret.SealGiftValue(fmt.Sprintf("gift:%d:grant", gift.ID), string(raw))
	if err != nil {
		giftError(c, err)
		return
	}
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(giftCookie(gift.ID), token, 12*3600, giftPath(gift.ID), "", siteconfig.Active().SecureCookie, true)
	Success(c, gift)
}

func (ctl *DigitalGiftController) Lock(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	id := giftID(c)
	if id == 0 {
		Fail(c, 400, "礼物编号无效")
		return
	}
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(giftCookie(id), "", -1, giftPath(id), "", siteconfig.Active().SecureCookie, true)
	Success(c, nil)
}

func (ctl *DigitalGiftController) Password(c *gin.Context) {
	gift, ok := ctl.load(c, false)
	if !ok {
		return
	}
	if gift.Mode == "open" {
		Fail(c, 400, "这份礼物无需密码")
		return
	}
	uid, encrypted := "", gift.PasswordEncrypted
	if gift.Mode == "individual" {
		uid = c.Query("uid")
		var row database.DigitalGiftRecipient
		if !claimUID.MatchString(uid) || database.DB.First(&row, "gift_id = ? AND uid = ? AND disabled = ? AND roster_active = ?", gift.ID, uid, false, true).Error != nil {
			Fail(c, 404, "名单中没有这位用户，或已停用")
			return
		}
		encrypted = row.PasswordEncrypted
	}
	password, err := secret.OpenGiftValue(giftScope(gift.ID, uid), encrypted)
	if err != nil {
		giftError(c, err)
		return
	}
	Success(c, gin.H{"password": password})
}

func (ctl *DigitalGiftController) Upload(c *gin.Context) {
	ctl.uploadMu.Lock()
	defer ctl.uploadMu.Unlock()
	if !giftRequest(c, giftMaxFile+(1<<20)) {
		return
	}
	gift, ok := ctl.load(c, false)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, giftMaxFile+(1<<20))
	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		Fail(c, 413, "文件过大或格式无效，单个文件最多 25 MB")
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	header, err := c.FormFile("file")
	if err != nil || header.Size <= 0 || header.Size > giftMaxFile {
		Fail(c, 400, "请选择不超过 25 MB 的文件")
		return
	}
	name := filepath.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
	if name == "." || len(name) > 255 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		Fail(c, 400, "文件名无效或过长")
		return
	}
	src, err := header.Open()
	if err != nil {
		giftError(c, err)
		return
	}
	defer src.Close()
	if err = os.MkdirAll(ctl.directory(), 0700); err != nil {
		giftError(c, err)
		return
	}
	file, err := os.CreateTemp(ctl.directory(), "gift-*")
	if err != nil {
		giftError(c, err)
		return
	}
	keep := false
	defer func() {
		file.Close()
		if !keep {
			os.Remove(file.Name())
			_ = ctl.assets().Delete(c.Request.Context(), filepath.Base(file.Name()))
		}
	}()
	n, err := io.Copy(file, io.LimitReader(src, giftMaxFile+1))
	if err != nil || n != header.Size || n > giftMaxFile {
		Fail(c, 400, "上传不完整，请重试")
		return
	}
	if _, err = file.Seek(0, 0); err != nil {
		giftError(c, err)
		return
	}
	headerBytes := make([]byte, 512)
	read, _ := file.Read(headerBytes)
	row := database.DigitalGiftFile{GiftID: gift.ID, Name: name, StorageKey: filepath.Base(file.Name()), Size: n, MediaType: http.DetectContentType(headerBytes[:read])}
	if err = file.Close(); err != nil {
		giftError(c, err)
		return
	}
	if err = ctl.assets().PutFile(c.Request.Context(), row.StorageKey, file.Name()); err != nil {
		giftError(c, err)
		return
	}
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		var current database.DigitalGift
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, gift.ID).Error; err != nil {
			return err
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		giftError(c, err)
		return
	}
	keep = true
	ctl.assets().ReleaseStaging(file.Name())
	Success(c, row)
}

func (ctl *DigitalGiftController) File(c *gin.Context) {
	// File errors must use actual HTTP statuses, so browsers never save an error as a gift.
	c.Header("Cache-Control", "private, no-store")
	var gift database.DigitalGift
	query := database.DB
	if !c.GetBool("authed") {
		query = query.Where("published = ?", true)
	}
	if giftID(c) == 0 || query.First(&gift, giftID(c)).Error != nil {
		c.AbortWithStatusJSON(404, gin.H{"message": "礼物不存在或已下架"})
		return
	}
	if !ctl.authorized(c, gift) {
		c.AbortWithStatusJSON(401, gin.H{"message": "解锁已过期，请返回电子礼物重新输入密码"})
		return
	}
	var row database.DigitalGiftFile
	if database.DB.First(&row, "id = ? AND gift_id = ?", c.Param("file"), gift.ID).Error != nil {
		c.AbortWithStatus(404)
		return
	}
	if filepath.Base(row.StorageKey) != row.StorageKey || !strings.HasPrefix(row.StorageKey, "gift-") {
		c.AbortWithStatus(404)
		return
	}
	file, err := ctl.assets().Open(c.Request.Context(), row.StorageKey)
	if err != nil {
		c.AbortWithStatus(404)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		c.AbortWithStatus(404)
		return
	}
	disposition := "attachment"
	if c.Query("preview") == "1" && giftPreviewType(row.MediaType) {
		disposition = "inline"
	}
	c.Header("Content-Type", row.MediaType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": row.Name}))
	http.ServeContent(c.Writer, c.Request, row.Name, info.ModTime(), file)
}

func giftPreviewType(t string) bool {
	switch t {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "audio/mpeg", "audio/wave", "audio/ogg", "video/mp4", "video/webm":
		return true
	}
	return false
}

func (ctl *DigitalGiftController) DeleteFile(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	gift, ok := ctl.load(c, false)
	if !ok {
		return
	}
	var row database.DigitalGiftFile
	if err := database.DB.First(&row, "id = ? AND gift_id = ?", c.Param("file"), gift.ID).Error; err != nil {
		giftError(c, err)
		return
	}
	if err := database.DB.Delete(&row).Error; err != nil {
		giftError(c, err)
		return
	}
	if filepath.Base(row.StorageKey) == row.StorageKey && strings.HasPrefix(row.StorageKey, "gift-") {
		_ = ctl.assets().Delete(c.Request.Context(), row.StorageKey)
	}
	Success(c, nil)
}

func (ctl *DigitalGiftController) Delete(c *gin.Context) {
	ctl.uploadMu.Lock()
	defer ctl.uploadMu.Unlock()
	if !deliveryRequest(c) {
		return
	}
	var req struct {
		Version int `json:"version"`
	}
	if c.ShouldBindJSON(&req) != nil {
		Fail(c, 400, "请刷新后重试")
		return
	}
	gift, ok := ctl.load(c, true)
	if !ok {
		return
	}
	var uploads []database.DigitalGiftUpload
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockVoyageBatchWrites(tx); err != nil {
			return err
		}
		r := tx.Where("id = ? AND version = ?", gift.ID, req.Version).Delete(&database.DigitalGift{})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return errGiftConflict
		}
		if err := tx.Where("gift_id = ?", gift.ID).Delete(&database.DigitalGiftRecipient{}).Error; err != nil {
			return err
		}
		if err := tx.Where("gift_id = ?", gift.ID).Find(&uploads).Error; err != nil {
			return err
		}
		if err := tx.Where("gift_id = ?", gift.ID).Delete(&database.DigitalGiftUpload{}).Error; err != nil {
			return err
		}
		if err := tx.Where("gift_id = ?", gift.ID).Delete(&database.DigitalGiftRoster{}).Error; err != nil {
			return err
		}
		return tx.Where("gift_id = ?", gift.ID).Delete(&database.DigitalGiftFile{}).Error
	})
	if err != nil {
		giftError(c, err)
		return
	}
	for _, upload := range uploads {
		if upload.FileID == 0 {
			if path, err := ctl.uploadPath(upload); err == nil {
				_ = os.Remove(path)
			}
		}
	}
	for _, f := range gift.Files {
		if filepath.Base(f.StorageKey) == f.StorageKey && strings.HasPrefix(f.StorageKey, "gift-") {
			_ = ctl.assets().Delete(c.Request.Context(), f.StorageKey)
		}
	}
	Success(c, nil)
}

func (ctl *DigitalGiftController) assets() *storage.Assets {
	if ctl.Files != nil {
		return ctl.Files
	}
	return storage.NewAssets("digital-gifts", ctl.directory())
}
