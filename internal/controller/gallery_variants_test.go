package controller

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"mofufans/internal/database"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func TestGalleryVariantsRevalidatePublicationAndPreserveOriginal(t *testing.T) {
	libraryTestDB(t)
	ctl := &GalleryController{Directory: t.TempDir()}
	name := "0123456789abcdef0123456789abcdef.png"
	im := image.NewNRGBA(image.Rect(0, 0, 1000, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 1000; x++ {
			im.Set(x, y, color.NRGBA{uint8(x), uint8(y), uint8(x * y), 255})
		}
	}
	var original bytes.Buffer
	_ = png.Encode(&original, im)
	if err := os.WriteFile(filepath.Join(ctl.directory(), name), original.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	work := database.GalleryWork{Source: "fan", Status: "pending", Version: 1, Images: []database.GalleryImage{{Filename: name, Width: 1000, Height: 600}}}
	if err := database.DB.Create(&work).Error; err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(sessions.Sessions("session", cookie.NewStore([]byte("offline-variant-test-session-key-long"))))
	r.GET("/api/gallery/assets/:filename", ctl.File)
	r.DELETE("/gallery/:id", ctl.Delete)
	asset := galleryPrefix + name
	get := func(url, etag string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", url, nil)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	for _, suffix := range []string{"", "?variant=small-v1", "?variant=thumb-v1", "?variant=preview-v1"} {
		if get(asset+suffix, "").Code != 404 {
			t.Fatal("pending image variant exposed")
		}
	}
	database.DB.Model(&work).Update("status", "published")
	for _, variant := range []string{"small-v1", "thumb-v1", "preview-v1"} {
		url := asset + "?variant=" + variant
		w := get(url, "")
		if w.Code != 200 || w.Header().Get("Cache-Control") != "private, no-cache" {
			t.Fatalf("variant failed: %d %s", w.Code, w.Body.String())
		}
		if _, _, err := image.DecodeConfig(bytes.NewReader(w.Body.Bytes())); err != nil {
			t.Fatal(err)
		}
		etag := w.Header().Get("ETag")
		if etag == "" || get(url, etag).Code != 304 {
			t.Fatal("browser cache not revalidated")
		}
		database.DB.Model(&work).Update("status", "rejected")
		if get(url, etag).Code != 404 {
			t.Fatal("cached hidden image still accessible")
		}
		database.DB.Model(&work).Update("status", "published")
	}
	if !bytes.Equal(get(asset, "").Body.Bytes(), original.Bytes()) {
		t.Fatal("original download changed")
	}
	if get(asset+"?variant=99999", "").Code != 400 {
		t.Fatal("arbitrary size accepted")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("DELETE", "/gallery/1", nil))
	if w.Code != 200 {
		t.Fatal("delete failed")
	}
	if get(asset+"?variant=thumb-v1", "").Code != 404 {
		t.Fatal("deleted thumbnail still accessible")
	}
	files, _ := filepath.Glob(filepath.Join(ctl.directory(), ".previews", "v1", "*"))
	if len(files) != 0 {
		t.Fatal("orphaned display copies")
	}
}
