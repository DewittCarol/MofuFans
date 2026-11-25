package controller

import (
	"encoding/json"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"mofufans/internal/adminauth"
	"mofufans/internal/ai"
	"mofufans/internal/database"
	"mofufans/internal/secret"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModerationBackfillAndPublicIsolation(t *testing.T) {
	appearanceDB(t)
	viper.Set("security.bilibili_cookie_key", "test-ai-key-with-more-than-32-characters")
	t.Cleanup(func() { viper.Set("security.bilibili_cookie_key", "") })
	key, e := secret.EncryptCookie("test-key")
	if e != nil {
		t.Fatal(e)
	}
	o := ai.Default()
	o.Enabled = true
	o.Model = "test-model"
	raw, _ := json.Marshal(o)
	database.DB.Create(&database.Config{AIConfig: string(raw), AIKeyCiphertext: key, SpamPrompt: "rules"})
	rows := []database.Question{
		{BaseModel: database.BaseModel{ID: 1}, Content: "unreviewed", TagID: 1},
		{BaseModel: database.BaseModel{ID: 2}, Content: "public", TagID: 1, IsPublish: true},
		{BaseModel: database.BaseModel{ID: 3}, Content: "manual", TagID: 1},
		{BaseModel: database.BaseModel{ID: 4}, Content: "failed", TagID: 1},
		{BaseModel: database.BaseModel{ID: 5}, Content: "archived", TagID: 1, IsArchive: true},
		{BaseModel: database.BaseModel{ID: 6}, Content: "other topic", TagID: 2},
	}
	if e := database.DB.Create(&rows).Error; e != nil {
		t.Fatal(e)
	}
	database.DB.Create(&[]database.QuestionModeration{
		{QuestionID: 2, Status: "passed", Reason: "private review reason", Token: "private-token"},
		{QuestionID: 3, Status: "manual", DecisionLocked: true},
		{QuestionID: 4, Status: "failed"},
	})
	database.DB.AutoMigrate(&database.Admin{})
	admin := database.Admin{Username: "fixture-owner", Password: "fixture-password"}
	database.DB.Create(&admin)
	r := gin.New()
	r.Use(sessions.Sessions("test", cookie.NewStore([]byte("review-test-cookie"))))
	r.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-Admin") == "yes" {
			sessions.Default(c).Set("authed", true)
			sessions.Default(c).Set("user", admin.ID)
			sessions.Default(c).Set("auth_stamp", adminauth.Stamp(admin))
		}
	})
	r.GET("/question", (&QuestionController{}).Get)
	r.POST("/queue", (&ModerationController{}).Queue)
	code, body := settingsRequest(t, r, "POST", "/queue", map[string]any{"tag_id": 1, "scope": "unreviewed"})
	if code != 200 || !strings.Contains(body, `"queued":2`) {
		t.Fatalf("backfill failed %s", body)
	}
	code, body = settingsRequest(t, r, "POST", "/queue", map[string]any{"tag_id": 1, "scope": "unreviewed"})
	if code != 200 || !strings.Contains(body, `"queued":0`) {
		t.Fatalf("duplicate backfill %s", body)
	}
	for _, admin := range []bool{false, true} {
		req := httptest.NewRequest("GET", "/question?page_size=100&moderation_status=passed", nil)
		if admin {
			req.Header.Set("X-Test-Admin", "yes")
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		body = rec.Body.String()
		if strings.Contains(body, "private-token") {
			t.Fatal("internal queue token exposed")
		}
		if admin {
			if !strings.Contains(body, "private review reason") || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("admin audit missing")
			}
		} else if strings.Contains(body, "moderation") || strings.Contains(body, "private review reason") {
			t.Fatal("public response disclosed audit")
		}
	}
	var manual database.QuestionModeration
	database.DB.First(&manual, "question_id = ?", 3)
	if manual.Status != "manual" {
		t.Fatal("backfill touched manual decision")
	}
}
