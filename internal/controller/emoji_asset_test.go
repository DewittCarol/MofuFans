package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"mofufans/internal/database"
)

func emojiUpload(t *testing.T, router *gin.Engine, kind, filename string, data []byte, collectionIDs ...uint) testAPIResponse {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("kind", kind)
	if len(collectionIDs) > 0 {
		_ = writer.WriteField("collection_id", fmt.Sprint(collectionIDs[0]))
	}
	file, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write(data)
	_ = writer.Close()
	req := httptest.NewRequest("POST", "/emojis", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	var response testAPIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func emojiTestRouter(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	appearanceDB(t)
	directory := t.TempDir()
	controller := &EmojiAssetController{Directory: directory}
	router := gin.New()
	router.POST("/emojis", controller.Upload)
	router.POST("/emojis/unicode", controller.AddUnicode)
	router.PUT("/emojis/:id/name", controller.Rename)
	router.GET("/emojis", controller.List)
	router.PUT("/emojis/status", controller.SetStatus)
	router.GET("/emoji-catalog", controller.Catalog)
	router.POST("/emoji-collections", controller.SaveCollection)
	router.PUT("/emoji-collections/:id", controller.SaveCollection)
	router.DELETE("/emoji-collections/:id", controller.DeleteCollection)
	router.PUT("/emojis/order", controller.Reorder)
	router.GET("/emoji-assets/:filename", controller.File)
	return router, directory
}

func TestEmojiRenameKeepsHistoricalIdentifiers(t *testing.T) {
	router, _ := emojiTestRouter(t)
	for _, kind := range []string{"input", "reaction"} {
		var asset database.EmojiAsset
		if err := database.DB.Where("kind = ?", kind).First(&asset).Error; err != nil {
			t.Fatal(err)
		}
		_, result := performJSONRequest(router, "PUT", fmt.Sprintf("/emojis/%d/name", asset.ID), `{"name":"  小狗的新名字  "}`, nil)
		if result.Code != 200 {
			t.Fatalf("rename failed: %+v", result)
		}
		if err := database.InitializeEmojiCatalog(); err != nil {
			t.Fatal(err)
		}
		var saved database.EmojiAsset
		database.DB.First(&saved, asset.ID)
		if saved.Name != "小狗的新名字" || saved.Value != asset.Value || saved.URL != asset.URL || saved.Kind != asset.Kind || saved.Builtin != asset.Builtin {
			t.Fatalf("rename changed identity or startup reset name: %+v", saved)
		}
		for _, invalid := range []string{"", "   ", "a\nb", strings.Repeat("绒", 33)} {
			body, _ := json.Marshal(map[string]string{"name": invalid})
			_, result = performJSONRequest(router, "PUT", fmt.Sprintf("/emojis/%d/name", asset.ID), string(body), nil)
			if result.Code != 400 {
				t.Fatalf("invalid name accepted: %q", invalid)
			}
		}
	}
	_, missing := performJSONRequest(router, "PUT", "/emojis/999999/name", `{"name":"小狗"}`, nil)
	if missing.Code != 404 {
		t.Fatal("missing emoji did not return 404")
	}
}

func TestUnicodeEmojiCreationValidationAndReaction(t *testing.T) {
	router, directory := emojiTestRouter(t)
	for _, value := range []string{"🐶", "👍🏽", "👨‍👩‍👧‍👦", "🇨🇳", "1️⃣", "❤"} {
		body, _ := json.Marshal(map[string]string{"value": value, "name": "测试名称"})
		_, result := performJSONRequest(router, "POST", "/emojis/unicode", string(body), nil)
		if result.Code != 200 {
			t.Fatalf("valid emoji %s rejected: %+v", value, result)
		}
		var saved database.EmojiAsset
		json.Unmarshal(result.Data, &saved)
		if saved.URL != "" || saved.Kind != "reaction" || saved.Name != "测试名称" {
			t.Fatalf("not a native emoji: %+v", saved)
		}
	}
	_, duplicate := performJSONRequest(router, "POST", "/emojis/unicode", `{"value":"❤️"}`, nil)
	if duplicate.Code != 409 {
		t.Fatal("variation selector bypassed duplicate check")
	}
	for _, value := range []string{"", "hello", "1", "🐶🐶", "🐶abc", "<script>", "🐶\n😄", "\u200d", "🏽"} {
		body, _ := json.Marshal(map[string]string{"value": value})
		_, result := performJSONRequest(router, "POST", "/emojis/unicode", string(body), nil)
		if result.Code != 400 {
			t.Fatalf("invalid emoji %q accepted", value)
		}
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 0 {
		t.Fatal("native emoji creation wrote image files")
	}
	q := database.Question{Content: "unicode reaction", IsPublish: true}
	database.DB.Create(&q)
	controller := &QuestionController{eventChan: make(chan SSEvent, 2)}
	router.POST("/question/:id/emoji", controller.Emoji)
	req := httptest.NewRequest("POST", fmt.Sprintf("/question/%d/emoji", q.ID), strings.NewReader("emoji=%F0%9F%90%B6"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	var result testAPIResponse
	json.Unmarshal(recorder.Body.Bytes(), &result)
	if result.Code != 200 || len(controller.eventChan) != 1 {
		t.Fatalf("new native emoji could not be reacted: %+v", result)
	}
	database.DB.First(&q, q.ID)
	if !strings.Contains(q.Emojis, `"value":"🐶","count":1`) {
		t.Fatal("new emoji reaction not persisted")
	}
}

func TestUnicodeEmojiRestoresDeletedImageWithoutSplittingCounts(t *testing.T) {
	router, _ := emojiTestRouter(t)
	var original database.EmojiAsset
	database.DB.Where("kind = ? AND value = ?", "reaction", "👍").First(&original)
	database.DB.Model(&original).Update("is_deleted", true)
	_, result := performJSONRequest(router, "POST", "/emojis/unicode", `{"value":"👍","name":"点赞"}`, nil)
	if result.Code != 200 {
		t.Fatalf("could not re-add removed emoji: %+v", result)
	}
	if err := database.InitializeEmojiCatalog(); err != nil {
		t.Fatal(err)
	}
	var saved database.EmojiAsset
	database.DB.First(&saved, original.ID)
	if saved.Value != original.Value || saved.URL != "" || saved.IsDeleted || saved.Name != "点赞" {
		t.Fatalf("restore lost identifier or did not use native emoji: %+v", saved)
	}
	var count int64
	database.DB.Model(&database.EmojiAsset{}).Where("kind = ? AND value = ?", "reaction", "👍").Count(&count)
	if count != 1 {
		t.Fatal("restore created a second counter identity")
	}
}

func TestEmojiUploadPersistenceRemovalAndRestore(t *testing.T) {
	router, directory := emojiTestRouter(t)
	var imageBytes bytes.Buffer
	_ = png.Encode(&imageBytes, image.NewNRGBA(image.Rect(0, 0, 32, 32)))
	response := emojiUpload(t, router, "input", "示例.png", imageBytes.Bytes())
	if response.Code != 200 {
		t.Fatalf("upload failed: %+v", response)
	}
	var asset database.EmojiAsset
	_ = json.Unmarshal(response.Data, &asset)
	if asset.Name != "示例" || asset.Value != "[示例]" || asset.Kind != "input" {
		t.Fatalf("bad metadata: %+v", asset)
	}
	stored, err := os.ReadFile(filepath.Join(directory, filepath.Base(asset.URL)))
	if err != nil || !bytes.Equal(stored, imageBytes.Bytes()) {
		t.Fatal("file was not saved locally without modification")
	}
	for _, deleted := range []bool{true, false} {
		_, status := performJSONRequest(router, "PUT", "/emojis/status", fmt.Sprintf(`{"ids":[%d],"is_deleted":%v}`, asset.ID, deleted), nil)
		if status.Code != 200 {
			t.Fatalf("status failed: %+v", status)
		}
		_, list := performJSONRequest(router, "GET", "/emojis", "", nil)
		var all []database.EmojiAsset
		_ = json.Unmarshal(list.Data, &all)
		found := false
		for _, item := range all {
			if item.ID == asset.ID {
				found = item.IsDeleted == deleted
			}
		}
		if !found {
			t.Fatal("historical metadata or current status missing")
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest("GET", "/emoji-assets/"+filepath.Base(asset.URL), nil))
		if recorder.Code != 200 || !bytes.Equal(recorder.Body.Bytes(), imageBytes.Bytes()) {
			t.Fatal("historical asset unavailable")
		}
		if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("missing nosniff header")
		}
	}
	duplicate := emojiUpload(t, router, "input", "示例.png", imageBytes.Bytes())
	var other database.EmojiAsset
	_ = json.Unmarshal(duplicate.Data, &other)
	if duplicate.Code != 200 || other.Value == asset.Value || other.URL != asset.URL {
		t.Fatal("names must stay distinct while identical bytes share local storage")
	}
}

func TestEmojiRejectsUnsupportedOversizeAndPathTraversal(t *testing.T) {
	router, directory := emojiTestRouter(t)
	for _, data := range [][]byte{[]byte(`<svg onload="alert(1)"></svg>`), []byte("not an image"), make([]byte, maxEmojiBytes+1)} {
		if response := emojiUpload(t, router, "input", "fake.png", data); response.Code != 400 {
			t.Fatalf("invalid image accepted: %+v", response)
		}
	}
	var huge bytes.Buffer
	_ = png.Encode(&huge, image.NewNRGBA(image.Rect(0, 0, 4097, 1)))
	if response := emojiUpload(t, router, "input", "huge.png", huge.Bytes()); response.Code != 400 {
		t.Fatal("oversized dimensions accepted")
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 0 {
		t.Fatal("invalid uploads wrote files")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("GET", "/emoji-assets/secret.json", nil))
	if recorder.Code != 404 {
		t.Fatal("uncontrolled filename was served")
	}
}

func TestEmojiPreservesAnimatedGIFAndWebP(t *testing.T) {
	router, directory := emojiTestRouter(t)
	frame := image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.Black, color.White})
	var animation bytes.Buffer
	_ = gif.EncodeAll(&animation, &gif.GIF{Image: []*image.Paletted{frame, frame}, Delay: []int{10, 20}, LoopCount: 0})
	webp, err := os.ReadFile("testdata/animated.webp")
	if err != nil {
		t.Fatal(err)
	}
	for filename, data := range map[string][]byte{"动图.gif": animation.Bytes(), "表情.webp": webp} {
		response := emojiUpload(t, router, "reaction", filename, data)
		if response.Code != 200 {
			t.Fatalf("%s failed: %+v", filename, response)
		}
		var asset database.EmojiAsset
		_ = json.Unmarshal(response.Data, &asset)
		saved, err := os.ReadFile(filepath.Join(directory, filepath.Base(asset.URL)))
		if err != nil || !bytes.Equal(saved, data) {
			t.Fatalf("%s animation was modified", filename)
		}
	}
}

func TestRemovedEmojiCannotBeReactedAndDefaultsStayDeleted(t *testing.T) {
	_, _ = emojiTestRouter(t)
	var asset database.EmojiAsset
	database.DB.Where("kind = ? AND value = ?", "reaction", "👍").First(&asset)
	database.DB.Model(&asset).Update("is_deleted", true)
	if err := database.InitializeEmojiCatalog(); err != nil {
		t.Fatal(err)
	}
	var saved database.EmojiAsset
	database.DB.First(&saved, asset.ID)
	if !saved.IsDeleted {
		t.Fatal("startup restored a removed default")
	}
	controller := &QuestionController{eventChan: make(chan SSEvent, 2)}
	router := gin.New()
	router.POST("/question/:id/emoji", controller.Emoji)
	req := httptest.NewRequest("POST", "/question/1/emoji", bytes.NewBufferString("emoji=%F0%9F%91%8D"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	var response testAPIResponse
	_ = json.Unmarshal(recorder.Body.Bytes(), &response)
	if response.Code != 400 {
		t.Fatalf("removed emoji still accepted: %+v", response)
	}
	if err := database.DB.Model(&database.EmojiAsset{}).Where("id > ?", 0).Update("is_deleted", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.InitializeEmojiCatalog(); err != nil {
		t.Fatal(err)
	}
	var active int64
	database.DB.Model(&database.EmojiAsset{}).Where("is_deleted = ?", false).Count(&active)
	if active != 0 {
		t.Fatal("an intentionally empty catalog was repopulated")
	}
}
