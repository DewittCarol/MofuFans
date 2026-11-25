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

func TestAdminQuestionsNeverSilentlyFallBackToPublic(t *testing.T) {
	previous := database.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "questions.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	database.DB = db
	t.Cleanup(func() { sqlDB, _ := db.DB(); sqlDB.Close(); database.DB = previous })
	if err := db.AutoMigrate(&database.Admin{}, &database.Tag{}, &database.Question{}); err != nil {
		t.Fatal(err)
	}
	admin := database.Admin{Username: "owner", Password: "0123456789abcdef0123456789abcdef", IsSuperAdmin: true}
	db.Create(&admin)
	db.Create(&database.Tag{BaseModel: database.BaseModel{ID: 1}, TagName: "fixture"})
	db.Create(&database.Question{Content: "public fixture", TagID: 1, IsPublish: true})
	private := database.Question{Content: "private fixture", TagID: 1}
	db.Create(&private)
	r := gin.New()
	r.Use(sessions.Sessions("session", cookie.NewStore([]byte(strings.Repeat("x", 32)))))
	r.POST("/login", (&controller.UserController{}).Login)
	r.GET("/public", (&controller.QuestionController{}).Get)
	r.GET("/admin", authMiddleware, (&controller.QuestionController{}).Get)
	call := func(method, path, body string, session *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if session != nil {
			req.AddCookie(session)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	var out struct {
		Code int
		Data struct {
			Total     int
			Questions []database.Question
		}
	}
	if json.Unmarshal(call("GET", "/admin?publish=false", "", nil).Body.Bytes(), &out) != nil || out.Code != 408 {
		t.Fatal("anonymous admin list fell back to public data")
	}
	if json.Unmarshal(call("GET", "/public?publish=false", "", nil).Body.Bytes(), &out) != nil || out.Code != 200 || out.Data.Total != 1 || out.Data.Questions[0].Content != "public fixture" {
		t.Fatal("public endpoint exposed private question")
	}
	login := call("POST", "/login", `{"username":"owner","password":"0123456789abcdef0123456789abcdef"}`, nil)
	session := login.Result().Cookies()[0]
	if json.Unmarshal(call("GET", "/admin?publish=false", "", session).Body.Bytes(), &out) != nil || out.Code != 200 || out.Data.Total != 1 || out.Data.Questions[0].ID != private.ID {
		t.Fatal("admin cannot see unpublished questions")
	}
	db.Model(&admin).Update("password", "rotated credential")
	if json.Unmarshal(call("GET", "/admin?publish=false", "", session).Body.Bytes(), &out) != nil || out.Code != 408 {
		t.Fatal("revoked session received a misleading public list")
	}
}
