package router

import (
	"encoding/json"
	"mofufans/internal/adminauth"
	"mofufans/internal/controller"
	"mofufans/internal/database"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestVoyageVisibilityAndPrivateRoutes(t *testing.T) {
	previous := database.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "voyage.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	database.DB = db
	t.Cleanup(func() { sqlDB, _ := db.DB(); sqlDB.Close(); database.DB = previous })
	if err = db.AutoMigrate(&database.Config{}, &database.Admin{}, &database.VoyageIssue{}); err != nil {
		t.Fatal(err)
	}
	config := database.Config{SpamPrompt: "rules"}
	db.Create(&config)
	admin := database.Admin{Username: "owner"}
	db.Create(&admin)
	dir := t.TempDir()
	publicName := strings.Repeat("a", 64) + ".png"
	draftName := strings.Repeat("b", 64) + ".png"
	unusedName := strings.Repeat("c", 64) + ".png"
	for _, name := range []string{publicName, draftName, unusedName} {
		if err = os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	db.Create(&database.VoyageIssue{Title: "published", Published: true, Images: []string{"/api/voyage-assets/" + publicName}})
	db.Create(&database.VoyageIssue{Title: "draft", Images: []string{"/api/voyage-assets/" + draftName}})
	r := gin.New()
	r.Use(sessions.Sessions("test", cookie.NewStore([]byte("0123456789abcdef0123456789abcdef"))))
	r.GET("/test-login", func(c *gin.Context) {
		s := sessions.Default(c)
		s.Set("authed", true)
		s.Set("user", admin.ID)
		s.Set("auth_stamp", adminauth.Stamp(admin))
		if e := s.Save(); e != nil {
			t.Fatal(e)
		}
		c.Status(200)
	})
	ctl := &controller.VoyageController{Directory: dir}
	r.GET("/voyage", voyageAccessMiddleware, ctl.List)
	r.GET("/api/voyage-assets/:filename", voyageAccessMiddleware, ctl.File)
	r.GET("/api/voyage-assets", authMiddleware, ctl.Assets)
	r.POST("/claim", voyageAccessMiddleware, (&controller.VoyageDeliveryController{}).Claim)
	r.GET("/manage", authMiddleware, func(c *gin.Context) { c.JSON(200, gin.H{"code": 200}) })
	book := &controller.VoyageAddressBookController{}
	r.GET("/book", authMiddleware, book.List)
	r.GET("/book/:uid", authMiddleware, book.Detail)
	r.POST("/book/:uid/action", authMiddleware, book.Action)
	r.POST("/profile-cache", authMiddleware, (&controller.BilibiliProfilesController{}).Cached)
	call := func(method, path string, login *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(`{"uid":"1000001","code":"bad","action":"view"}`))
		req.Header.Set("Content-Type", "application/json")
		if login != nil {
			req.AddCookie(login)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	expectCode := func(w *httptest.ResponseRecorder, want int) {
		t.Helper()
		var body struct{ Code int }
		if json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Code != want {
			t.Fatalf("expected %d, got %s", want, w.Body.String())
		}
	}
	expectCode(call("GET", "/voyage", nil), 403)
	for _, request := range [][2]string{{"GET", "/book"}, {"GET", "/book/12345"}, {"POST", "/book/12345/action"}, {"POST", "/profile-cache"}} {
		expectCode(call(request[0], request[1], nil), 408)
	}
	login := call("GET", "/test-login", nil).Result().Cookies()[0]
	expectCode(call("GET", "/voyage", login), 200)
	db.Model(&config).Update("voyage_public_enabled", true)
	public := call("GET", "/voyage", nil)
	expectCode(public, 200)
	if strings.Contains(public.Body.String(), `"draft"`) {
		t.Fatal("draft exposed")
	}
	if call("GET", "/api/voyage-assets/"+publicName, nil).Code != 200 {
		t.Fatal("published artwork unavailable")
	}
	if call("GET", "/api/voyage-assets/"+draftName, nil).Code != 404 {
		t.Fatal("draft artwork exposed")
	}
	if call("GET", "/api/voyage-assets/"+draftName, login).Code != 200 {
		t.Fatal("admin artwork preview unavailable")
	}
	expectCode(call("GET", "/manage", nil), 408)
	expectCode(call("GET", "/api/voyage-assets", nil), 408)
	library := call("GET", "/api/voyage-assets", login)
	expectCode(library, 200)
	for _, name := range []string{publicName, draftName, unusedName} {
		if !strings.Contains(library.Body.String(), name) {
			t.Fatal("admin cannot reuse retained upload")
		}
	}
	if library.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("library must not be cached publicly")
	}
	if call("GET", "/api/voyage-assets/"+unusedName, nil).Code != 404 {
		t.Fatal("unused artwork exposed")
	}

	expectCode(call("POST", "/claim", nil), 401)
	db.Model(&config).Update("voyage_public_enabled", false)
	expectCode(call("GET", "/voyage", nil), 403)
	expectCode(call("POST", "/claim", nil), 403)
	db.Delete(&admin)
	expectCode(call("GET", "/voyage", login), 403)
}
