package controller

import (
	"mime"
	"mofufans/internal/guard"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

type memberRateWindow struct {
	Count int
	Until time.Time
}

// Limits are per application controller, bounded, and never store raw login names.
type MemberRateLimits struct {
	mu      sync.Mutex
	windows map[string]memberRateWindow
	network chan struct{}
}

func NewMemberRateLimits() *MemberRateLimits {
	return &MemberRateLimits{windows: map[string]memberRateWindow{}, network: make(chan struct{}, 2)}
}

func (l *MemberRateLimits) take(scope, key string, limit int, span time.Duration, now time.Time) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.windows == nil {
		l.windows = map[string]memberRateWindow{}
	}
	if l.network == nil {
		l.network = make(chan struct{}, 2)
	}
	for key, v := range l.windows {
		if !now.Before(v.Until) {
			delete(l.windows, key)
		}
	}
	k := scope + ":" + hashVerificationToken(key)
	w, exists := l.windows[k]
	if !exists {
		if len(l.windows) >= 8192 {
			return false, 60
		}
		w = memberRateWindow{Until: now.Add(span)}
	}
	if w.Count >= limit {
		return false, int(w.Until.Sub(now).Seconds()) + 1
	}
	w.Count++
	l.windows[k] = w
	return true, 0
}

func (ctl *AccountController) clock() time.Time {
	if ctl.Now != nil {
		return ctl.Now().UTC()
	}
	return time.Now().UTC()
}
func (ctl *AccountController) limits() *MemberRateLimits {
	ctl.limitOnce.Do(func() {
		if ctl.Limits == nil {
			ctl.Limits = NewMemberRateLimits()
		}
	})
	return ctl.Limits
}
func (ctl *AccountController) allow(c *gin.Context, scope, key string, limit int, span time.Duration) bool {
	ok, after := ctl.limits().take(scope, key, limit, span, ctl.clock())
	if !ok {
		guard.Reject(c, after)
	}
	return ok
}

// The existing cookie is SameSite Strict. This also rejects same-site sibling
// origins and non-JSON browser form requests, without affecting admin sessions.
func memberWriteAllowed(c *gin.Context) bool {
	c.Header("Cache-Control", "no-store")
	if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
		return true
	}
	if site := c.GetHeader("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		Fail(c, 403, "请从本站页面提交")
		return false
	}
	if origin := c.GetHeader("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || (viper.GetBool("security.secure_cookie") && u.Scheme != "https") || !strings.EqualFold(u.Host, c.Request.Host) || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			Fail(c, 403, "请从本站页面提交")
			return false
		}
	}
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		Fail(c, 415, "请使用页面提供的操作")
		return false
	}
	return true
}

func MemberRequestSecurity(c *gin.Context) {
	if !memberWriteAllowed(c) {
		c.Abort()
		return
	}
	c.Next()
}

func memberBind(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if c.ShouldBindJSON(target) != nil {
		Fail(c, 400, "请求无效")
		return false
	}
	return true
}
