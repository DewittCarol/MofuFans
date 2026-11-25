package moderation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mofufans/internal/ai"
	"mofufans/internal/database"
	"time"
)

type Worker struct {
	DB     *gorm.DB
	Client *ai.Client
}

// A bounded, durable queue. No provider calls occur in the submission request.
func Queue(db *gorm.DB, id uint, manualRetry bool) (bool, error) {
	queued := false
	err := db.Transaction(func(tx *gorm.DB) error {
		// Acquire the SQLite write lock before reading a row that another worker may update.
		if err := tx.Model(&database.Question{}).Where("id = ?", id).UpdateColumn("id", id).Error; err != nil {
			return err
		}
		var q database.Question
		if err := tx.First(&q, id).Error; err != nil {
			return err
		}
		var prior database.QuestionModeration
		err := tx.First(&prior, "question_id = ?", id).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if prior.Status == "pending" || prior.Status == "running" {
			return nil
		}
		if !manualRetry && prior.QuestionID != 0 {
			return nil
		}
		var count int64
		if err := tx.Model(&database.QuestionModeration{}).Where("status IN ?", []string{"pending", "running"}).Count(&count).Error; err != nil {
			return err
		}
		token := make([]byte, 16)
		if _, err := rand.Read(token); err != nil {
			return err
		}
		row := database.QuestionModeration{QuestionID: id, Status: "pending", ImageStatus: "pending", Token: hex.EncodeToString(token), Attempts: prior.Attempts, DecisionLocked: prior.DecisionLocked || q.IsPublish || q.IsArchive || (q.IsSpam && prior.QuestionID == 0)}
		var config database.Config
		if err := tx.First(&config).Error; err != nil {
			return err
		}
		options, err := config.AIOptions()
		if err != nil {
			return err
		}
		if !options.Enabled {
			row.Status = "skipped"
			row.Error = "AI 审核未开启，请人工审核"
			row.ImageStatus = "not_reviewed"
		}
		if count >= 1000 {
			row.Status = "failed"
			row.Error = "审核队列已满，投稿已保留；请稍后手动重试"
			row.ImageStatus = "not_reviewed"
		}
		if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error; err != nil {
			return err
		}
		queued = row.Status == "pending"
		return nil
	})
	return queued, err
}

// The administrator's decision wins even if an old model request finishes later.
func MarkManual(tx *gorm.DB, id uint) error {
	row := database.QuestionModeration{QuestionID: id, Status: "manual", DecisionLocked: true, ImageStatus: "not_reviewed"}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "question_id"}}, DoUpdates: clause.Assignments(map[string]any{"status": "manual", "decision_locked": true, "token": "", "error": "", "updated_at": time.Now().UTC()})}).Create(&row).Error
}

func (w *Worker) Start(ctx context.Context) {
	// Running calls are never presumed successful after a restart; pending calls resume.
	if err := w.DB.Model(&database.QuestionModeration{}).Where("status = ?", "running").Updates(map[string]any{"status": "failed", "error": "服务重启导致审核中断，请重试或人工审核", "image_status": "not_reviewed"}).Error; err != nil {
		log.Warn("AI review queue recovery will retry")
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.RecoverExpired(time.Now().UTC())
			}
		}
	}()
	for i := 0; i < 2; i++ {
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				if w.ProcessOne(ctx) {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	}
}

// Also recover jobs whose final database write failed, without calling the model again.
func (w *Worker) RecoverExpired(now time.Time) {
	if err := w.DB.Model(&database.QuestionModeration{}).Where("status = ? AND updated_at < ?", "running", now.Add(-ai.RequestTimeout-90*time.Second)).Updates(map[string]any{"status": "failed", "error": "审核超时或结果未能保存，请重试或人工审核", "image_status": "not_reviewed", "updated_at": now}).Error; err != nil {
		log.Warn("AI review queue recovery unavailable")
	}
}

func fingerprint(c database.Config) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(c.AIConfig+"\x00"+c.AIKeyCiphertext+"\x00"+c.SpamPrompt)))
}
func (w *Worker) ProcessOne(ctx context.Context) bool {
	var job database.QuestionModeration
	if w.DB.Where("status = ?", "pending").Order("updated_at ASC, question_id ASC").Limit(1).Find(&job).Error != nil || job.QuestionID == 0 {
		return false
	}
	claim := w.DB.Model(&database.QuestionModeration{}).Where("question_id = ? AND token = ? AND status = ?", job.QuestionID, job.Token, "pending").Updates(map[string]any{"status": "running", "attempts": gorm.Expr("attempts + 1"), "updated_at": time.Now().UTC()})
	if claim.Error != nil || claim.RowsAffected != 1 {
		return false
	}
	w.run(ctx, job)
	return true
}
func (w *Worker) run(parent context.Context, job database.QuestionModeration) {
	defer func() {
		if recover() != nil {
			w.finish(job, database.Question{}, "", nil, errors.New("审核任务异常，投稿已保留，请重试"), "not_reviewed", 0)
		}
	}()
	var config database.Config
	var q database.Question
	if w.DB.First(&config).Error != nil || w.DB.Preload("Tag").First(&q, job.QuestionID).Error != nil {
		w.finish(job, q, "", nil, errors.New("审核资料读取失败，投稿已保留"), "not_reviewed", 0)
		return
	}
	sig := fingerprint(config)
	options, err := config.AIOptions()
	if err == nil && !options.Enabled {
		err = errors.New("AI 审核已关闭，投稿保留供人工审核")
	}
	var key string
	if err == nil {
		key, err = config.AIKey()
	}
	if err == nil && key == "" {
		err = errors.New("AI API Key 未配置，请先完成设置")
	}
	ctx, cancel := context.WithTimeout(parent, ai.RequestTimeout)
	defer cancel()
	imageStatus := "none"
	var images []ai.Image
	if q.ImagesNum > 0 {
		imageStatus = "disabled"
		if options.ReviewImages {
			imageStatus = "failed"
			if err == nil {
				images, err = LoadImages(q.Images, q.ImagesNum)
				if err == nil {
					imageStatus = "complete"
				}
			}
		}
	}
	var result *ai.Review
	if err == nil {
		client := w.Client
		if client == nil {
			client = ai.DefaultClient
		}
		var r ai.Review
		r, err = client.Review(ctx, options, key, config.SpamPrompt, q.Content, q.Tag.TagName+"\n"+q.Tag.Description, q.ImagesNum, images)
		if err == nil {
			result = &r
		}
	}
	job.Model = options.Model
	w.finish(job, q, sig, result, err, imageStatus, len(images))
}
func (w *Worker) finish(job database.QuestionModeration, original database.Question, sig string, result *ai.Review, reviewErr error, imageStatus string, images int) {
	err := w.DB.Transaction(func(tx *gorm.DB) error {
		// This conditional write serializes completion with manual edits and retry requests.
		lock := tx.Model(&database.QuestionModeration{}).Where("question_id = ? AND token = ? AND status = ?", job.QuestionID, job.Token, "running").UpdateColumn("updated_at", time.Now().UTC())
		if lock.Error != nil {
			return lock.Error
		}
		if lock.RowsAffected != 1 {
			return nil
		}
		var live database.QuestionModeration
		var q database.Question
		var config database.Config
		if err := tx.First(&live, "question_id = ?", job.QuestionID).Error; err != nil {
			return err
		}
		if err := tx.First(&q, job.QuestionID).Error; err != nil {
			return tx.Where("question_id = ?", job.QuestionID).Delete(&database.QuestionModeration{}).Error
		}
		if err := tx.First(&config).Error; err != nil {
			return err
		}
		if sig != "" && fingerprint(config) != sig {
			reviewErr = errors.New("审核期间设置已变更，请按新设置重新审核")
			result = nil
		}
		if !original.UpdatedAt.IsZero() && !q.UpdatedAt.Equal(original.UpdatedAt) {
			reviewErr = errors.New("审核期间投稿已被修改，保留当前处理结果")
			result = nil
		}
		now := time.Now().UTC()
		values := map[string]any{"status": "failed", "error": "", "reason": "", "model": job.Model, "image_status": imageStatus, "images_checked": images, "checked_at": now, "updated_at": now}
		if reviewErr != nil {
			values["error"] = reviewErr.Error()
		} else if result != nil {
			values["reason"] = result.Reason
			values["status"] = "passed"
			if result.IsSpam {
				values["status"] = "blocked"
			}
			if !live.DecisionLocked && !q.IsPublish && !q.IsArchive {
				if err := tx.Model(&database.Question{}).Where("id = ?", q.ID).Update("is_spam", result.IsSpam).Error; err != nil {
					return err
				}
			}
		}
		return tx.Model(&database.QuestionModeration{}).Where("question_id = ?", job.QuestionID).Updates(values).Error
	})
	if err != nil {
		log.Warn("Could not persist AI review; question remains available for manual review")
	}
}
