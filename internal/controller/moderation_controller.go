package controller

import (
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mofufans/internal/database"
	"mofufans/internal/moderation"
)

type ModerationController struct{}

func (*ModerationController) Status(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var cfg database.Config
	if database.DB.First(&cfg).Error != nil {
		Fail(c, 500, "审核设置读取失败")
		return
	}
	o, e := cfg.AIOptions()
	if e != nil {
		Fail(c, 500, e.Error())
		return
	}
	Success(c, gin.H{"enabled": o.Enabled, "review_images": o.ReviewImages})
}
func (*ModerationController) Queue(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req struct {
		IDs   []uint `json:"ids"`
		TagID uint   `json:"tag_id"`
		Scope string `json:"scope"`
	}
	if !aiBody(c, &req) {
		return
	}
	if len(req.IDs) > 100 || (len(req.IDs) > 0 && (req.TagID > 0 || req.Scope != "")) {
		Fail(c, 400, "每批最多 100 条，请选择一种补审范围")
		return
	}
	var cfg database.Config
	if database.DB.First(&cfg).Error != nil {
		Fail(c, 500, "审核设置读取失败")
		return
	}
	o, e := cfg.AIOptions()
	if e != nil || !o.Enabled {
		Fail(c, 400, "请先在系统设置中开启 AI 辅助审核")
		return
	}
	if key, err := cfg.AIKey(); err != nil || key == "" {
		Fail(c, 400, "请先配置有效的 AI API Key")
		return
	}
	ids := req.IDs
	remaining := false
	if len(ids) == 0 {
		if req.TagID == 0 || req.Scope != "unreviewed" {
			Fail(c, 400, "请选择投稿或指定要补审的话题")
			return
		}
		err := database.DB.Model(&database.Question{}).Where("tag_id = ? AND is_publish = ? AND is_archive = ? AND is_spam = ?", req.TagID, false, false, false).
			Where("NOT EXISTS (SELECT 1 FROM question_moderations m WHERE m.question_id = questions.id AND (m.decision_locked = ? OR m.status IN ?))", true, []string{"pending", "running", "passed", "blocked", "manual"}).
			Order("id ASC").Limit(101).Pluck("id", &ids).Error
		if err != nil {
			Fail(c, 500, "补审范围读取失败")
			return
		}
		if len(ids) > 100 {
			ids = ids[:100]
			remaining = true
		}
	}
	queued, skipped := 0, 0
	failed := []uint{}
	seen := map[uint]bool{}
	for _, id := range ids {
		if id == 0 || seen[id] {
			skipped++
			continue
		}
		seen[id] = true
		ok, err := moderation.Queue(database.DB, id, true)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				skipped++
			} else {
				failed = append(failed, id)
			}
		} else if ok {
			queued++
		} else {
			skipped++
		}
	}
	Success(c, gin.H{"queued": queued, "skipped": skipped, "failed_ids": failed, "has_more": remaining})
}

func attachModeration(db *gorm.DB, questions []database.Question) {
	if len(questions) == 0 {
		return
	}
	ids := make([]uint, 0, len(questions))
	for _, q := range questions {
		ids = append(ids, q.ID)
	}
	var reviews []database.QuestionModeration
	if db.Where("question_id IN ?", ids).Find(&reviews).Error != nil {
		return
	}
	rows := map[uint]*database.QuestionModeration{}
	for i := range reviews {
		rows[reviews[i].QuestionID] = &reviews[i]
	}
	for i := range questions {
		questions[i].Moderation = rows[questions[i].ID]
	}
}
