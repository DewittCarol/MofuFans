package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"mofufans/internal/controller"
	"mofufans/internal/database"
)

func TestPetEntrySettingsRequireAdminAndPreserveOmittedValues(t *testing.T) {
	previous := database.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "pet-settings.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	database.DB = db
	t.Cleanup(func() { sqlDB, _ := db.DB(); sqlDB.Close(); database.DB = previous })
	if err := db.AutoMigrate(&database.Admin{}, &database.Config{}); err != nil {
		t.Fatal(err)
	}
	config := database.Config{RequireVerifiedUserToPost: true, AllowRepeatedReactions: true, AIKeyCiphertext: "private-fixture"}
	if err := db.Create(&config).Error; err != nil {
		t.Fatal(err)
	}
	admin := database.Admin{Username: "pet-owner", Password: "0123456789abcdef0123456789abcdef", IsSuperAdmin: true}
	db.Create(&admin)
	r := gin.New()
	r.Use(sessions.Sessions("session", cookie.NewStore([]byte(strings.Repeat("p", 32)))))
	users := &controller.UserController{}
	ctl := &controller.ConfigController{}
	r.POST("/login", users.Login)
	r.GET("/config", ctl.Get)
	r.GET("/settings", authMiddleware, ctl.GetSettings)
	r.PUT("/settings", authMiddleware, ctl.PutSettings)
	call := func(method, path, body string, login *http.Cookie) map[string]interface{} {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if login != nil {
			req.AddCookie(login)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var result map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if call("PUT", "/settings", `{"pet_default_expanded":true}`, nil)["code"] != float64(408) {
		t.Fatal("anonymous settings write allowed")
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/login", strings.NewReader(`{"username":"pet-owner","password":"0123456789abcdef0123456789abcdef"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if len(w.Result().Cookies()) == 0 {
		t.Fatal("login failed")
	}
	session := w.Result().Cookies()[0]
	for _, body := range []string{`{"require_verified_user_to_post":true,"pet_default_expanded":true}`, `{"require_verified_user_to_post":true}`} {
		res := call("PUT", "/settings", body, session)
		if res["code"] != float64(200) || res["data"].(map[string]interface{})["pet_default_expanded"] != true {
			t.Fatal("explicit enable or omitted value was not preserved")
		}
	}
	if call("PUT", "/settings", `{"pet_default_expanded":"false"}`, session)["code"] != float64(400) {
		t.Fatal("invalid type accepted")
	}
	public := call("GET", "/config", "", nil)["data"].(map[string]interface{})
	if public["pet_default_expanded"] != true || public["ai_key_ciphertext"] != nil {
		t.Fatal("public default missing or private field exposed")
	}
	res := call("PUT", "/settings", `{"require_verified_user_to_post":true,"pet_default_expanded":false}`, session)
	if res["code"] != float64(200) || res["data"].(map[string]interface{})["pet_default_expanded"] != false {
		t.Fatal("explicit false was lost")
	}
	db.First(&config, config.ID)
	if config.PetDefaultExpanded || !config.RequireVerifiedUserToPost || !config.AllowRepeatedReactions || config.AIKeyCiphertext != "private-fixture" {
		t.Fatal("unrelated settings changed")
	}
}
