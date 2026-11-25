package controller

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"io"
	"mofufans/internal/database"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const giftChunkSize = 8 << 20

func (ctl *DigitalGiftController) uploadPath(upload database.DigitalGiftUpload) (string, error) {
	if filepath.Base(upload.StorageKey) != upload.StorageKey || !strings.HasPrefix(upload.StorageKey, "gift-upload-") {
		return "", errors.New("invalid upload path")
	}
	return filepath.Join(ctl.directory(), upload.StorageKey), nil
}
func (ctl *DigitalGiftController) cleanupUploads() {
	var rows []database.DigitalGiftUpload
	if database.DB.Where("updated_at < ?", time.Now().Add(-24*time.Hour)).Limit(50).Find(&rows).Error != nil {
		return
	}
	for _, row := range rows {
		if database.DB.Delete(&row).Error == nil && row.FileID == 0 {
			if path, err := ctl.uploadPath(row); err == nil {
				_ = os.Remove(path)
			}
		}
	}
}
func (ctl *DigitalGiftController) StartUpload(c *gin.Context) {
	if !giftRequest(c, 2048) {
		return
	}
	gift, ok := ctl.load(c, false)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	if c.ShouldBindJSON(&req) != nil || req.Size < 0 {
		Fail(c, 400, "文件信息无效")
		return
	}
	name := filepath.Base(strings.ReplaceAll(req.Name, "\\", "/"))
	if name == "" || name == "." || len(name) > 255 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		Fail(c, 400, "文件名无效或过长")
		return
	}
	ctl.uploadMu.Lock()
	defer ctl.uploadMu.Unlock()
	ctl.cleanupUploads()
	if err := database.DB.First(&gift, gift.ID).Error; err != nil {
		giftError(c, err)
		return
	}
	if err := os.MkdirAll(ctl.directory(), 0700); err != nil {
		giftError(c, err)
		return
	}
	file, err := os.CreateTemp(ctl.directory(), "gift-upload-*")
	if err != nil {
		giftError(c, err)
		return
	}
	file.Close()
	token := make([]byte, 16)
	if _, err = rand.Read(token); err != nil {
		os.Remove(file.Name())
		giftError(c, err)
		return
	}
	row := database.DigitalGiftUpload{Token: hex.EncodeToString(token), GiftID: gift.ID, Name: name, Size: req.Size, StorageKey: filepath.Base(file.Name()), UpdatedAt: time.Now()}
	if err = database.DB.Create(&row).Error; err != nil {
		os.Remove(file.Name())
		giftError(c, err)
		return
	}
	Success(c, row)
}
func (ctl *DigitalGiftController) uploadRecord(c *gin.Context) (database.DigitalGiftUpload, bool) {
	var row database.DigitalGiftUpload
	if giftID(c) == 0 || len(c.Param("upload")) != 32 {
		Fail(c, 404, "上传记录不存在，请重新选择文件")
		return row, false
	}
	if err := database.DB.First(&row, "token = ? AND gift_id = ?", c.Param("upload"), giftID(c)).Error; err != nil {
		giftError(c, err)
		return row, false
	}
	if row.UpdatedAt.Before(time.Now().Add(-24 * time.Hour)) {
		Fail(c, 410, "上传已过期，请重新选择文件")
		return row, false
	}
	return row, true
}
func (ctl *DigitalGiftController) UploadStatus(c *gin.Context) {
	if _, ok := ctl.load(c, false); !ok {
		return
	}
	row, ok := ctl.uploadRecord(c)
	if ok {
		Success(c, row)
	}
}
func (ctl *DigitalGiftController) UploadChunk(c *gin.Context) {
	if !giftRequest(c, giftChunkSize) {
		return
	}
	offset, err := strconv.ParseInt(c.GetHeader("Upload-Offset"), 10, 64)
	if err != nil || offset < 0 {
		Fail(c, 400, "上传进度无效")
		return
	}
	bytes, err := io.ReadAll(c.Request.Body)
	if err != nil || len(bytes) == 0 {
		Fail(c, 400, "文件片段不完整，请重试")
		return
	}
	ctl.uploadMu.Lock()
	defer ctl.uploadMu.Unlock()
	if _, ok := ctl.load(c, false); !ok {
		return
	}
	row, ok := ctl.uploadRecord(c)
	if !ok {
		return
	}
	if row.FileID != 0 || row.Received != offset {
		Fail(c, 409, "上传进度已变化，请重新读取进度")
		return
	}
	if int64(len(bytes)) > row.Size-row.Received {
		Fail(c, 400, "文件内容超过声明的大小")
		return
	}
	path, err := ctl.uploadPath(row)
	if err != nil {
		giftError(c, err)
		return
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		giftError(c, err)
		return
	}
	defer file.Close()
	// Discard an unacknowledged tail left by a process interruption.
	if err = file.Truncate(row.Received); err != nil {
		giftError(c, err)
		return
	}
	n, err := file.WriteAt(bytes, row.Received)
	if err == nil && n != len(bytes) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = file.Sync()
	}
	if err != nil {
		_ = file.Truncate(row.Received)
		giftError(c, err)
		return
	}
	before := row.Received
	row.Received += int64(n)
	row.UpdatedAt = time.Now()
	result := database.DB.Model(&row).Where("received = ? AND file_id = ?", before, 0).Select("Received", "UpdatedAt").Updates(&row)
	if result.Error != nil || result.RowsAffected != 1 {
		_ = file.Truncate(before)
		if result.Error != nil {
			giftError(c, result.Error)
		} else {
			giftError(c, errGiftConflict)
		}
		return
	}
	Success(c, row)
}
func (ctl *DigitalGiftController) CompleteUpload(c *gin.Context) {
	if !giftRequest(c, 1024) {
		return
	}
	ctl.uploadMu.Lock()
	defer ctl.uploadMu.Unlock()
	gift, ok := ctl.load(c, false)
	if !ok {
		return
	}
	upload, ok := ctl.uploadRecord(c)
	if !ok {
		return
	}
	if upload.FileID != 0 {
		var existing database.DigitalGiftFile
		if err := database.DB.First(&existing, "id = ? AND gift_id = ?", upload.FileID, gift.ID).Error; err != nil {
			giftError(c, err)
			return
		}
		Success(c, existing)
		return
	}
	if upload.Received != upload.Size {
		Fail(c, 409, "文件还未上传完成")
		return
	}
	path, err := ctl.uploadPath(upload)
	if err != nil {
		giftError(c, err)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		giftError(c, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != upload.Size {
		Fail(c, 409, "上传文件不完整，请重新上传")
		return
	}
	head := make([]byte, 512)
	n, _ := file.Read(head)
	row := database.DigitalGiftFile{GiftID: gift.ID, Name: upload.Name, StorageKey: upload.StorageKey, Size: upload.Size, MediaType: http.DetectContentType(head[:n])}
	if err = file.Close(); err != nil {
		giftError(c, err)
		return
	}
	if err = ctl.assets().PutFile(c.Request.Context(), row.StorageKey, path); err != nil {
		giftError(c, err)
		return
	}
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		var current database.DigitalGift
		if err := tx.First(&current, gift.ID).Error; err != nil {
			return err
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return tx.Model(&upload).Updates(map[string]any{"file_id": row.ID, "updated_at": time.Now()}).Error
	})
	if err != nil {
		_ = ctl.assets().RollbackFile(c.Request.Context(), row.StorageKey, path)
		giftError(c, err)
		return
	}
	ctl.assets().ReleaseStaging(path)
	Success(c, row)
}
func (ctl *DigitalGiftController) CancelUpload(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	ctl.uploadMu.Lock()
	defer ctl.uploadMu.Unlock()
	var upload database.DigitalGiftUpload
	err := database.DB.First(&upload, "token = ? AND gift_id = ?", c.Param("upload"), giftID(c)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Success(c, nil)
		return
	}
	if err != nil {
		giftError(c, err)
		return
	}
	if err = database.DB.Delete(&upload).Error; err != nil {
		giftError(c, err)
		return
	}
	if upload.FileID == 0 {
		if path, err := ctl.uploadPath(upload); err == nil {
			_ = os.Remove(path)
		}
	}
	Success(c, nil)
}
