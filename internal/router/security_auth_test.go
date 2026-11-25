package router

import (
	"encoding/json"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"mofufans/internal/controller"
	"mofufans/internal/database"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdminLoginAndPasswordChangeRevokeSession(t *testing.T) {
	previous := database.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "auth.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	database.DB = db
	t.Cleanup(func() { sqlDB, _ := db.DB(); sqlDB.Close(); database.DB = previous })
	if err := db.AutoMigrate(&database.Admin{}); err != nil {
		t.Fatal(err)
	}
	admin := database.Admin{Username: "owner", Password: "0123456789abcdef0123456789abcdef", IsSuperAdmin: true}
	db.Create(&admin)
	r := gin.New()
	r.Use(sessions.Sessions("session", cookie.NewStore([]byte(strings.Repeat("x", 32)))))
	ctl := &controller.UserController{}
	r.POST("/login", ctl.Login)
	r.PUT("/user/:id", authMiddleware, ctl.Put)
	r.GET("/protected", authMiddleware, ctl.Info)
	call := func(method, path, body string, login *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if login != nil {
			req.AddCookie(login)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	code := func(w *httptest.ResponseRecorder) int {
		var b struct{ Code int }
		json.Unmarshal(w.Body.Bytes(), &b)
		return b.Code
	}
	unknown := call("POST", "/login", `{"username":"missing","password":"0123456789abcdef0123456789abcdef"}`, nil)
	wrong := call("POST", "/login", `{"username":"owner","password":"abcdef0123456789abcdef0123456789"}`, nil)
	if unknown.Body.String() != wrong.Body.String() {
		t.Fatal("login exposes account existence")
	}
	if code(call("GET", "/protected", "", nil)) != 408 {
		t.Fatal("anonymous management access")
	}
	login := call("POST", "/login", `{"username":"owner","password":"0123456789abcdef0123456789abcdef"}`, nil)
	if code(login) != 200 {
		t.Fatal(login.Body.String())
	}
	session := login.Result().Cookies()[0]
	db.First(&admin, admin.ID)
	if !strings.HasPrefix(admin.Password, "$2") {
		t.Fatal("legacy credential not upgraded")
	}
	if code(call("GET", "/protected", "", session)) != 200 {
		t.Fatal("valid session rejected")
	}
	if code(call("PUT", "/user/1", `{"new_password":"Replacement-credential-2026"}`, session)) != 200 {
		t.Fatal("password change failed")
	}
	if code(call("GET", "/protected", "", session)) != 408 {
		t.Fatal("old session survived password change")
	}
}
