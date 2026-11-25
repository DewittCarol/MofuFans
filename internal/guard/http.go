package guard

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const contextKey = "mofufans.guard"

func Current(c *gin.Context) *Manager {
	g, _ := c.Get(contextKey)
	manager, _ := g.(*Manager)
	return manager
}
func Reject(c *gin.Context, after int) {
	after = max(1, after)
	c.Header("Cache-Control", "private, no-store")
	c.Header("Retry-After", strconv.Itoa(after))
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"code": 429, "message": "操作较频繁，请稍后重试", "retry_after": after})
}
func BeginAuth(c *gin.Context, scope, identity string) (*Attempt, bool) {
	g := Current(c)
	if g == nil {
		return &Attempt{}, true
	}
	a, after := g.Begin(c.ClientIP(), scope, identity)
	if after > 0 {
		Reject(c, after)
		return nil, false
	}
	return a, true
}
func AcquireStream(c *gin.Context) (func(), bool) {
	g := Current(c)
	if g == nil {
		return func() {}, true
	}
	release, ok := g.Stream(c.ClientIP())
	if !ok {
		Reject(c, 30)
	}
	return release, ok
}

// OriginAllowed never trusts a forwarded Host or Proto header supplied by a visitor.
func OriginAllowed(r *http.Request, allowed []string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Sec-Fetch-Site") != "cross-site" && r.Header.Get("Sec-Fetch-Site") != "same-site"
	}
	if len(origin) > 2048 {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if r.TLS != nil && u.Scheme != "https" {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	for _, candidate := range allowed {
		if origin == candidate {
			return true
		}
	}
	return false
}
func Classify(method, path string) string {
	if path == "/api/ws" || path == "/api/sse" {
		return "stream"
	}
	if (method == http.MethodGet || method == http.MethodHead) && (strings.Contains(path, "/files/") || strings.Contains(path, "-assets/") || strings.HasPrefix(path, "/upload-img/")) {
		return "download"
	}
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return "read"
	}
	if path == "/api/login" || path == "/api/admin/reauth" {
		return "admin-login"
	}
	if path == "/api/account/login" || path == "/api/account/password" || path == "/api/account/logout-others" {
		return "member-login"
	}
	if strings.HasPrefix(path, "/api/account/signature/") || strings.HasPrefix(path, "/api/setup/") {
		return "signature"
	}
	if strings.HasSuffix(path, "/claim") || strings.HasSuffix(path, "/unlock") {
		return "claim"
	}
	if path == "/api/question" {
		return "post"
	}
	if strings.HasSuffix(path, "/emoji") {
		return "reaction"
	}
	if path == "/api/gallery" {
		return "post"
	}
	if strings.HasPrefix(path, "/api/bilibili/") || strings.HasPrefix(path, "/api/ai-settings/") || strings.HasPrefix(path, "/api/voyage/bilibili/") {
		return "upstream"
	}
	if strings.Contains(path, "/uploads/") || strings.HasSuffix(path, "/files") || strings.HasSuffix(path, "/uploads") || strings.Contains(path, "-assets") || path == "/api/emojis" || strings.HasPrefix(path, "/api/admin/gallery") {
		return "upload"
	}
	return "write"
}
func BodyLimit(path string) int64 {
	if path == "/api/login" || path == "/api/admin/reauth" || strings.HasPrefix(path, "/api/account/signature/") || path == "/api/account/login" {
		return 16 << 10
	}
	if path == "/api/website-assets" {
		return 520 << 20
	}
	if path == "/api/question" {
		return 32 << 20
	}
	if strings.HasSuffix(path, "/claim") || strings.HasSuffix(path, "/unlock") {
		return 16 << 10
	}
	if path == "/api/gallery" || strings.Contains(path, "-assets") || strings.HasPrefix(path, "/api/admin/gallery") || strings.Contains(path, "/files") || strings.Contains(path, "/uploads") || path == "/api/emojis" {
		return 40 << 20
	}
	return 16 << 20
}
func (g *Manager) Middleware(allowed []string, secure ...func() bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(contextKey, g)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Header("Cache-Control", "private, no-store")
		}
		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		} // Never persist query strings or arbitrary path payloads.
		scope := Classify(c.Request.Method, path)
		if (c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions) || scope == "stream" {
			origin := c.GetHeader("Origin")
			if !OriginAllowed(c.Request, allowed) || (len(secure) > 0 && secure[0]() && origin != "" && !strings.HasPrefix(origin, "https://")) {
				g.Note(c.ClientIP(), scope, "origin_denied")
				c.AbortWithStatusJSON(403, gin.H{"code": 403, "message": "请从本站页面操作"})
				return
			}
		}
		release, after := g.Enter(c.ClientIP(), scope)
		if release == nil {
			Reject(c, after)
			return
		}
		defer release()
		if c.Request.Body != nil {
			limit := BodyLimit(path)
			if c.Request.ContentLength > limit {
				g.Note(c.ClientIP(), scope, "request_rejected")
				c.AbortWithStatusJSON(413, gin.H{"code": 413, "message": "请求内容过大"})
				return
			}
			c.Request.Body = &progressBody{body: http.MaxBytesReader(c.Writer, c.Request.Body, limit), control: http.NewResponseController(unwrapWriter(c.Writer)), started: time.Now()}
		}
		c.Next()
	}
}

// An upload cannot retain a scarce write slot indefinitely by dripping bytes.
// Header and total request timeouts remain enforced by http.Server.
type progressBody struct {
	body    io.ReadCloser
	control *http.ResponseController
	started time.Time
	read    int64
}

func unwrapWriter(w http.ResponseWriter) http.ResponseWriter {
	for {
		inner, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return w
		}
		w = inner.Unwrap()
	}
}
func (b *progressBody) Read(p []byte) (int, error) {
	elapsed := time.Since(b.started)
	if elapsed > 15*time.Second && b.read < int64(elapsed/time.Second-15)*4096 {
		return 0, errors.New("request body too slow")
	}
	_ = b.control.SetReadDeadline(time.Now().Add(15 * time.Second))
	n, err := b.body.Read(p)
	b.read += int64(n)
	return n, err
}
func (b *progressBody) Close() error { return b.body.Close() }

// Reauthentication is a short, credential-bound signed session grant, never an IP allowlist.
const ReauthLifetime = 5 * time.Minute

func JSONRequest(c *gin.Context) bool {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	return err == nil && media == "application/json"
}
