package controller

import (
	"bytes"
	"encoding/json"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"image"
	"image/png"
	"mime/multipart"
	"mofufans/internal/database"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func libraryTestDB(t *testing.T) {
	t.Helper()
	old := database.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	database.DB = db
	t.Cleanup(func() { database.DB = old; raw, _ := db.DB(); _ = raw.Close() })
	if err = db.AutoMigrate(&database.SongTag{}, &database.Song{}, &database.SongTagLink{}, &database.GalleryWork{}, &database.GalleryImage{}, &database.Admin{}); err != nil {
		t.Fatal(err)
	}
}
func TestLibraryVideoIdentifiers(t *testing.T) {
	for _, raw := range []string{"BV17x411w7KC?p=17", "https://www.bilibili.com/video/BV17x411w7KC/?p=17&spm_id_from=ignored"} {
		value, err := normalizeSongVideo(raw)
		if err != nil || value != "BV17x411w7KC?p=17" {
			t.Fatal("multipart video lost its page", value, err)
		}
	}
	for _, raw := range []string{"BV17x411w7KC", " https://www.bilibili.com/video/BV17x411w7KC/?p=1 ", "bv17x411w7KC"} {
		value, err := normalizeSongVideo(raw)
		if err != nil || value != "BV17x411w7KC" {
			t.Fatalf("valid video rejected: %s", raw)
		}
	}
	for _, raw := range []string{"https://evil.invalid/video/BV17x411w7KC", "javascript:alert(1)", "BV123", "https://www.bilibili.com.evil.invalid/video/BV17x411w7KC"} {
		if _, err := normalizeSongVideo(raw); err == nil {
			t.Fatal("invalid video accepted")
		}
	}
}
func TestLibraryGalleryReviewGatesFiles(t *testing.T) {
	libraryTestDB(t)
	gin.SetMode(gin.TestMode)
	ctl := &GalleryController{Directory: t.TempDir()}
	r := gin.New()
	r.Use(sessions.Sessions("session", cookie.NewStore([]byte("offline-library-test-session-key-long"))))
	r.POST("/api/gallery", ctl.Create)
	r.GET("/api/gallery", ctl.List)
	r.GET("/api/gallery/assets/:filename", ctl.File)
	r.PUT("/api/admin/gallery/:id", func(c *gin.Context) { c.Set("authed", true) }, ctl.Update)
	var content bytes.Buffer
	writer := multipart.NewWriter(&content)
	_ = writer.WriteField("title", "")
	_ = writer.WriteField("status", "published")
	_ = writer.WriteField("source", "streamer")
	part, _ := writer.CreateFormFile("images", "sample.png")
	_ = png.Encode(part, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	_ = writer.Close()
	upload := httptest.NewRequest("POST", "/api/gallery", &content)
	upload.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	r.ServeHTTP(response, upload)
	var result struct{ Code int }
	_ = json.Unmarshal(response.Body.Bytes(), &result)
	if result.Code != 200 {
		t.Fatalf("upload failed: %s", response.Body.String())
	}
	var work database.GalleryWork
	if err := database.DB.Preload("Images").First(&work).Error; err != nil {
		t.Fatal(err)
	}
	if work.Source != "fan" || work.Status != "pending" || len(work.Images) != 1 {
		t.Fatal("public upload bypassed moderation")
	}
	asset := galleryPrefix + work.Images[0].Filename
	response = httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("GET", asset, nil))
	if response.Code != 404 {
		t.Fatal("pending image exposed")
	}
	response = httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("GET", "/api/gallery?status=pending", nil))
	if bytes.Contains(response.Body.Bytes(), []byte(work.Images[0].Filename)) {
		t.Fatal("pending work exposed in public list")
	}
	response = httptest.NewRecorder()
	request := httptest.NewRequest("PUT", "/api/admin/gallery/1", bytes.NewBufferString(`{"status":"published","version":1}`))
	request.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(response, request)
	_ = json.Unmarshal(response.Body.Bytes(), &result)
	if result.Code != 200 {
		t.Fatalf("approval failed: %s", response.Body.String())
	}
	response = httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("GET", asset, nil))
	if response.Code != 200 {
		t.Fatal("approved image not available")
	}
	database.DB.Model(&work).Update("status", "rejected")
	response = httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("GET", asset, nil))
	if response.Code != 404 {
		t.Fatal("hidden image still public")
	}
}
func TestLibrarySongCounts(t *testing.T) {
	libraryTestDB(t)
	database.DB.Create(&database.Song{Title: "歌曲一", Videos: []string{}})
	database.DB.Create(&database.Song{Title: "歌曲二", Performed: true, Videos: []string{"BV17x411w7KC"}})
	r := gin.New()
	r.GET("/api/songs", (&SongController{}).List)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("GET", "/api/songs?performed=true", nil))
	var result struct {
		Code int
		Data struct {
			Items  []database.Song
			Total  int
			Counts map[string]int
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Code != 200 || result.Data.Total != 1 || len(result.Data.Items) != 1 || result.Data.Counts["all"] != 2 || result.Data.Counts["performed"] != 1 {
		t.Fatalf("incorrect song counts: %s", response.Body.String())
	}
}
