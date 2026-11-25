package router

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mofufans/internal/adminauth"
	"mofufans/internal/controller"
	"mofufans/internal/database"
	"mofufans/internal/guard"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func transport(value string) string { sum := md5.Sum([]byte(value)); return hex.EncodeToString(sum[:]) }

type guardRig struct {
	t        *testing.T
	r        *gin.Engine
	g        *guard.Manager
	password string
}

func newGuardRig(t *testing.T) *guardRig {
	t.Helper()
	previous := database.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "guard.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	database.DB = db
	t.Cleanup(func() { sql, _ := db.DB(); _ = sql.Close(); database.DB = previous })
	if err = db.AutoMigrate(&database.Admin{}); err != nil {
		t.Fatal(err)
	}
	password := "Integration-credential-2026"
	hash, err := adminauth.HashNewPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	db.Create(&database.Admin{Username: "owner", Password: hash, IsSuperAdmin: true})
	db.Create(&database.Admin{Username: "moderator", Password: hash})
	g, err := guard.New(guard.Options{Directory: t.TempDir(), Secret: strings.Repeat("g", 32)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.Close)
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(g.Middleware(nil))
	r.Use(sessions.Sessions("session", cookie.NewStore([]byte(strings.Repeat("s", 32)))))
	ctl := &controller.UserController{}
	security := &controller.SecurityController{Protection: g}
	r.POST("/api/login", ctl.Login)
	r.GET("/api/info", authMiddleware, ctl.Info)
	r.POST("/api/admin/reauth", authMiddleware, ctl.Reauthenticate)
	r.GET("/api/admin/security", authMiddleware, security.View)
	r.POST("/api/admin/security/bans", authMiddleware, controller.RequireRecentAdmin, security.Ban)
	r.DELETE("/api/admin/security/bans/:id", authMiddleware, controller.RequireRecentAdmin, security.Unban)
	r.PUT("/api/user/:id", authMiddleware, controller.RequireRecentAdmin, ctl.Put)
	r.POST("/test/expire-proof", authMiddleware, func(c *gin.Context) {
		s := sessions.Default(c)
		s.Set("reauth_until", time.Now().Add(-time.Minute).Unix())
		_ = s.Save()
		c.Status(200)
	})
	return &guardRig{t, r, g, password}
}
func (s *guardRig) call(method, path, ip string, body any, session *http.Cookie) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, "http://example.com"+path, strings.NewReader(string(raw)))
	req.RemoteAddr = ip + ":1234"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MofuFans-Request", "settings")
	if session != nil {
		req.AddCookie(session)
	}
	w := httptest.NewRecorder()
	s.r.ServeHTTP(w, req)
	return w
}
func responseCode(w *httptest.ResponseRecorder) int {
	var b struct{ Code int }
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	return b.Code
}
func (s *guardRig) login(name, ip string) *http.Cookie {
	w := s.call("POST", "/api/login", ip, map[string]string{"username": name, "password": transport(s.password)}, nil)
	if responseCode(w) != 200 {
		s.t.Fatal(w.Code, w.Body.String())
	}
	return w.Result().Cookies()[0]
}
func TestGuardAdminPublicAccessAndModeratorIsolation(t *testing.T) {
	s := newGuardRig(t)
	if responseCode(s.call("GET", "/api/admin/security", "192.0.2.1", nil, nil)) == 200 {
		t.Fatal("public security access")
	}
	member := s.login("moderator", "192.0.2.2")
	if responseCode(s.call("GET", "/api/admin/security", "192.0.2.2", nil, member)) != 403 {
		t.Fatal("moderator can read security data")
	}
	if responseCode(s.call("POST", "/api/admin/security/bans", "192.0.2.2", map[string]any{"ip": "198.51.100.1", "minutes": 60}, member)) != 403 {
		t.Fatal("moderator can ban")
	}
	if len(s.g.View().Bans) != 0 {
		t.Fatal("forbidden action changed state")
	}
}
func TestGuardAdminAttackCannotUseValidAccountToEraseOtherFailures(t *testing.T) {
	s := newGuardRig(t)
	for i := 0; i < 8; i++ {
		w := s.call("POST", "/api/login", "192.0.2.3", map[string]string{"username": fmt.Sprint("unknown-", i), "password": transport("invalid-credential")}, nil)
		if responseCode(w) != 401 {
			t.Fatal(i, w.Body.String())
		}
	}
	w := s.call("POST", "/api/login", "192.0.2.3", map[string]string{"username": "owner", "password": transport(s.password)}, nil)
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("no real HTTP cooldown", w.Code)
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("blocked request received a session")
	}
}
func TestGuardRecentAuthAndPasswordChangeRevokeOldGrant(t *testing.T) {
	s := newGuardRig(t)
	session := s.login("owner", "192.0.2.10")
	// A current login is already a fresh proof; the first sensitive action does not require a second password prompt.
	w := s.call("POST", "/api/admin/security/bans", "192.0.2.10", map[string]any{"ip": "198.51.100.1", "minutes": 15}, session)
	if responseCode(w) != 200 {
		t.Fatal(w.Body.String())
	}
	var added struct{ Data struct{ ID string } }
	_ = json.Unmarshal(w.Body.Bytes(), &added)
	if responseCode(s.call("DELETE", "/api/admin/security/bans/"+added.Data.ID, "192.0.2.10", nil, session)) != 200 {
		t.Fatal("manual unban failed")
	}
	if responseCode(s.call("PUT", "/api/user/1", "192.0.2.10", map[string]string{"new_password": "short"}, session)) != 400 {
		t.Fatal("weak password accepted")
	}
	if responseCode(s.call("PUT", "/api/user/1", "192.0.2.10", map[string]string{"new_password": "Replacement-credential-2026"}, session)) != 200 {
		t.Fatal("strong password rejected")
	}
	if responseCode(s.call("GET", "/api/info", "192.0.2.10", nil, session)) != 408 {
		t.Fatal("old session survived")
	}
	w = s.call("POST", "/api/admin/security/bans", "192.0.2.10", map[string]any{"ip": "198.51.100.2", "minutes": 15}, session)
	if responseCode(w) == 200 {
		t.Fatal("old recent-auth grant survived password rotation")
	}
}
func TestGuardOriginRejectionPrecedesCredentialMutation(t *testing.T) {
	s := newGuardRig(t)
	session := s.login("owner", "192.0.2.11")
	req := httptest.NewRequest("PUT", "http://example.com/api/user/1", strings.NewReader(`{"new_password":"Bad-cross-site-credential"}`))
	req.RemoteAddr = "192.0.2.11:1234"
	req.AddCookie(session)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://sibling.example.com")
	req.Header.Set("Sec-Fetch-Site", "same-site")
	w := httptest.NewRecorder()
	s.r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("same-site sibling admitted")
	}
	if responseCode(s.call("GET", "/api/info", "192.0.2.11", nil, session)) != 200 {
		t.Fatal("cross-site attempt changed credentials")
	}
}
func TestGuardExpiredProofRequiresPasswordAndCannotBeReplayed(t *testing.T) {
	s := newGuardRig(t)
	session := s.login("owner", "192.0.2.12")
	w := s.call("POST", "/test/expire-proof", "192.0.2.12", nil, session)
	expired := w.Result().Cookies()[0]
	w = s.call("POST", "/api/admin/security/bans", "192.0.2.12", map[string]any{"ip": "198.51.100.3", "minutes": 15}, expired)
	if w.Code != 428 || len(s.g.View().Bans) != 0 {
		t.Fatal("expired proof admitted")
	}
	w = s.call("POST", "/api/admin/reauth", "192.0.2.12", map[string]string{"password": transport("wrong-credential")}, expired)
	if responseCode(w) != 401 {
		t.Fatal("wrong reauthentication password accepted")
	}
	w = s.call("POST", "/api/admin/reauth", "192.0.2.12", map[string]string{"password": transport(s.password)}, expired)
	if responseCode(w) != 200 {
		t.Fatal(w.Body.String())
	}
	fresh := w.Result().Cookies()[0]
	w = s.call("POST", "/api/admin/security/bans", "192.0.2.12", map[string]any{"ip": "198.51.100.3", "minutes": 15}, fresh)
	if responseCode(w) != 200 {
		t.Fatal("fresh proof rejected", w.Body.String())
	}
}
func TestGuardSessionExpiryIsEnforcedByServer(t *testing.T) {
	r := gin.New()
	r.Use(sessions.Sessions("session", newSessionStore(strings.Repeat("s", 32), 1)))
	r.POST("/login", func(c *gin.Context) { s := sessions.Default(c); s.Set("signed_in", true); _ = s.Save(); c.Status(200) })
	r.GET("/info", func(c *gin.Context) { c.JSON(200, gin.H{"signed_in": sessions.Default(c).Get("signed_in") == true}) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/login", nil))
	old := w.Result().Cookies()[0]
	call := func() bool {
		req := httptest.NewRequest("GET", "/info", nil)
		req.AddCookie(old)
		out := httptest.NewRecorder()
		r.ServeHTTP(out, req)
		var value map[string]bool
		_ = json.Unmarshal(out.Body.Bytes(), &value)
		return value["signed_in"]
	}
	if !call() {
		t.Fatal("fresh session refused")
	}
	time.Sleep(2100 * time.Millisecond)
	if call() {
		t.Fatal("expired cookie replay accepted by server")
	}
}
