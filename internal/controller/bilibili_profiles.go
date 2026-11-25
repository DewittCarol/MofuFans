package controller

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
	"mofufans/internal/database"
)

type BilibiliProfilesController struct {
	Client        VoyageProfileClient
	AvatarStorage AvatarStorage
	mu            sync.Mutex
	networkMu     sync.Mutex
	limiter       *rate.Limiter
	cooldown      time.Time
}

type bilibiliProfileView struct {
	database.BilibiliProfile
	PauseUntil *time.Time `json:"pause_until,omitempty"`
}

func (ctl *BilibiliProfilesController) Get(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	uid := c.Param("uid")
	id, err := strconv.ParseInt(uid, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != uid {
		Fail(c, 400, "请输入有效的 B 站 UID")
		return
	}
	var input struct {
		Refresh bool `json:"refresh"`
	}
	if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		Fail(c, 400, "同步参数无效")
		return
	}
	profile, err := ctl.lookup(c.Request.Context(), uid, id, input.Refresh)
	if err != nil {
		Fail(c, 503, "用户资料暂不可用，请稍后重试")
		return
	}
	c.Header("Cache-Control", "no-store")
	Success(c, profile)
}

func (ctl *BilibiliProfilesController) lookup(ctx context.Context, uid string, id int64, refresh bool) (bilibiliProfileView, error) {
	if err := ctx.Err(); err != nil {
		return bilibiliProfileView{}, err
	}
	p := database.BilibiliProfile{UID: uid}
	err := database.DB.Where("uid = ?", uid).First(&p).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return bilibiliProfileView{}, err
	}
	view := func() bilibiliProfileView {
		ctl.mu.Lock()
		defer ctl.mu.Unlock()
		v := bilibiliProfileView{BilibiliProfile: p}
		if ctl.cooldown.After(time.Now()) {
			pause := ctl.cooldown
			v.PauseUntil = &pause
		}
		return v
	}
	cacheFor := 7 * 24 * time.Hour
	if refresh {
		// Share very recent results across tabs without bypassing failure cooldowns.
		cacheFor = time.Minute
	}
	fresh := func() bool {
		now := time.Now()
		return (p.RetryAt != nil && p.RetryAt.After(now)) || (p.Error == "" && p.Name != "" && p.Avatar != "" && now.Sub(p.CheckedAt) < cacheFor)
	}
	if fresh() {
		return view(), nil
	}
	// A slow upstream lookup must not hold up reads of valid persisted caches.
	ctl.networkMu.Lock()
	defer ctl.networkMu.Unlock()
	if err := ctx.Err(); err != nil {
		return bilibiliProfileView{}, err
	}
	p = database.BilibiliProfile{UID: uid}
	if err = database.DB.Where("uid = ?", uid).First(&p).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return bilibiliProfileView{}, err
	}
	if fresh() {
		return view(), nil
	}
	ctl.mu.Lock()
	cooldown := ctl.cooldown
	ctl.mu.Unlock()
	if cooldown.After(time.Now()) {
		p.Error = "B 站资料暂缓获取，稍后会继续"
		retry := cooldown
		p.RetryAt = &retry
		return view(), nil
	}
	if ctl.Client == nil || ctl.AvatarStorage == nil {
		return bilibiliProfileView{}, errors.New("profile service unavailable")
	}
	if ctl.limiter == nil {
		ctl.limiter = rate.NewLimiter(rate.Every(time.Second), 1)
	}
	if err = ctl.limiter.Wait(ctx); err != nil {
		return bilibiliProfileView{}, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	card, err := ctl.Client.PublicCard(queryCtx, id)
	if ctx.Err() != nil {
		return bilibiliProfileView{}, ctx.Err()
	}
	if err != nil || card.MID != id || card.Name == "" {
		p.Error = "头像和昵称暂未获取成功，可稍后重试"
		ctl.mu.Lock()
		ctl.cooldown = time.Now().Add(time.Minute)
		ctl.mu.Unlock()
	} else {
		p.Name = card.Name
		avatar := p.Avatar
		var saveErr error
		check, canCheck := ctl.AvatarStorage.(interface{ Available(string) bool })
		if p.AvatarSourceURL != card.Face || !canCheck || !check.Available(p.Avatar) {
			avatar, saveErr = ctl.AvatarStorage.Save(queryCtx, id, card.Face)
		}
		if ctx.Err() != nil {
			return bilibiliProfileView{}, ctx.Err()
		}
		if saveErr != nil {
			p.Error = "昵称已获取，头像可稍后重试"
		} else {
			p.Avatar = avatar
			p.AvatarSourceURL = card.Face
			p.Error = ""
		}
	}
	p.CheckedAt = time.Now()
	p.RetryAt = nil
	if p.Error != "" {
		retry := p.CheckedAt.Add(5 * time.Minute)
		p.RetryAt = &retry
	}
	if err = database.DB.Save(&p).Error; err != nil {
		return bilibiliProfileView{}, err
	}
	return view(), nil
}

// Bulk cache reads never contact Bilibili. The browser can display old values
// immediately, then pass only stale or missing profiles to its limited queue.
func (*BilibiliProfilesController) Cached(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	var input struct {
		UIDs []string `json:"uids"`
	}
	if c.ShouldBindJSON(&input) != nil || len(input.UIDs) > 500 {
		Fail(c, 400, "每次最多读取500个用户")
		return
	}
	for _, uid := range input.UIDs {
		if !validBookUID(uid) {
			Fail(c, 400, "请输入有效的 B 站 UID")
			return
		}
	}
	rows := []database.BilibiliProfile{}
	if len(input.UIDs) > 0 {
		if err := database.DB.Where("uid IN ?", input.UIDs).Find(&rows).Error; err != nil {
			Fail(c, 500, "缓存资料暂时无法读取")
			return
		}
	}
	Success(c, rows)
}
