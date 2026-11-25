package controller

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"image"
	"image/png"
	"mofufans/internal/database"
	"net/http/httptest"
	"testing"
)

func TestBackgroundUploadSaveClearAndOpacity(t *testing.T) {
	appearanceDB(t)
	database.DB.Create(&database.Config{SpamPrompt: "keep", DeepSeekAPIKey: "private"})
	b := &BackgroundController{Directory: t.TempDir()}
	r := gin.New()
	r.POST("/emojis", b.Upload)
	r.PUT("/background", b.Put)
	r.GET("/api/background-assets/:filename", b.File)
	var data bytes.Buffer
	png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 32, 24)))
	upload := emojiUpload(t, r, "", "test.png", data.Bytes())
	if upload.Code != 200 {
		t.Fatalf("upload: %+v", upload)
	}
	var asset struct {
		URL string `json:"url"`
	}
	json.Unmarshal(upload.Data, &asset)
	file := httptest.NewRecorder()
	r.ServeHTTP(file, httptest.NewRequest("GET", asset.URL, nil))
	if file.Code != 200 || !bytes.Equal(file.Body.Bytes(), data.Bytes()) {
		t.Fatal("uploaded image not served intact")
	}
	for _, opacity := range []int{0, 25, 100} {
		body, _ := json.Marshal(map[string]any{"url": asset.URL, "opacity": opacity})
		_, saved := performJSONRequest(r, "PUT", "/background", string(body), nil)
		var config database.Config
		database.DB.First(&config)
		if saved.Code != 200 || config.BackgroundOpacity != opacity || config.BackgroundURL != asset.URL || config.DeepSeekAPIKey != "private" {
			t.Fatal("background save corrupted configuration")
		}
	}
	for _, body := range []string{`{"url":"","opacity":101}`, `{"url":"","opacity":-1}`, `{"url":"https://example.com/a.png","opacity":25}`, `{"url":"/api/background-assets/../a.png","opacity":25}`, `{"url":""}`} {
		_, res := performJSONRequest(r, "PUT", "/background", body, nil)
		if res.Code != 400 {
			t.Fatalf("invalid setting accepted: %s", body)
		}
	}
	_, clear := performJSONRequest(r, "PUT", "/background", `{"url":"","opacity":25}`, nil)
	if clear.Code != 200 {
		t.Fatal("default background restoration failed")
	}
	if emojiUpload(t, r, "", "fake.png", []byte("not an image")).Code != 400 {
		t.Fatal("invalid upload accepted")
	}
}
