package controller

import (
	"encoding/json"
	"mofufans/internal/database"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func appearanceDB(t *testing.T) {
	t.Helper()
	var err error
	database.DB, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "appearance.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.DB.AutoMigrate(&database.Config{}, &database.QuestionModeration{}, &database.Question{}, &database.LikeRecord{}, &database.EmojiAsset{}, &database.EmojiCollection{}); err != nil {
		t.Fatal(err)
	}
	if err := database.InitializeEmojiCatalog(); err != nil {
		t.Fatal(err)
	}
	db, _ := database.DB.DB()
	t.Cleanup(func() { db.Close() })
}

func TestCursorSettingPersistsAndRejectsUnknownTheme(t *testing.T) {
	appearanceDB(t)
	database.DB.Create(&database.Config{SpamPrompt: "keep spam rules", DeepSeekAPIKey: "private-key"})
	router := gin.New()
	c := &ConfigController{}
	router.GET("/config", c.Get)
	router.GET("/settings", c.GetSettings)
	router.PUT("/settings", c.PutSettings)
	_, public := performJSONRequest(router, "GET", "/config", "", nil)
	if strings.Contains(string(public.Data), "private-key") {
		t.Fatal("public config leaked private settings")
	}
	var initial map[string]any
	if err := json.Unmarshal(public.Data, &initial); err != nil {
		t.Fatal(err)
	}
	if initial["cursor_theme"] != "system" {
		t.Fatalf("wrong initial cursor: %v", initial)
	}
	for _, theme := range []string{"system"} {
		_, res := performJSONRequest(router, "PUT", "/settings", `{"spam_prompt":"keep spam rules","cursor_theme":"`+theme+`"}`, nil)
		if res.Code != 200 {
			t.Fatalf("save failed: %+v", res)
		}
		_, res = performJSONRequest(router, "GET", "/settings", "", nil)
		var data SettingsRequest
		if err := json.Unmarshal(res.Data, &data); err != nil {
			t.Fatal(err)
		}
		if data.CursorTheme == nil || *data.CursorTheme != theme || len(data.CursorThemes) != 1 {
			t.Fatalf("wrong saved theme: %+v", data)
		}
		_, public := performJSONRequest(router, "GET", "/config", "", nil)
		var publicConfig struct {
			CursorTheme string `json:"cursor_theme"`
		}
		if err := json.Unmarshal(public.Data, &publicConfig); err != nil || publicConfig.CursorTheme != theme {
			t.Fatalf("public cursor did not switch to %s", theme)
		}
	}
	_, invalid := performJSONRequest(router, "PUT", "/settings", `{"spam_prompt":"must not overwrite","cursor_theme":"unknown"}`, nil)
	if invalid.Code != 400 {
		t.Fatal("invalid theme accepted")
	}
	_, legacy := performJSONRequest(router, "PUT", "/settings", `{"spam_prompt":"keep spam rules"}`, nil)
	if legacy.Code != 200 {
		t.Fatal("old client rejected")
	}
	var config database.Config
	database.DB.First(&config)
	if config.CursorTheme != "system" || config.SpamPrompt != "keep spam rules" {
		t.Fatalf("setting corrupted: %+v", config)
	}
}

func TestEmojiReturnsCommittedSnapshotAndBroadcastsOnce(t *testing.T) {
	appearanceDB(t)
	q := database.Question{Content: "reaction test", IsPublish: true}
	if err := database.DB.Create(&q).Error; err != nil {
		t.Fatal(err)
	}
	controller := &QuestionController{eventChan: make(chan SSEvent, 4)}
	router := gin.New()
	router.POST("/question/:id/emoji", controller.Emoji)
	send := func(value string) testAPIResponse {
		req := httptest.NewRequest("POST", "/question/1/emoji", strings.NewReader(url.Values{"emoji": {value}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		var result testAPIResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	result := send("💖")
	if result.Code != 200 {
		t.Fatalf("reaction failed: %+v", result)
	}
	var snapshot EmojiRecords
	if err := json.Unmarshal(result.Data, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.CardID != 1 || len(snapshot.Emojis) != 1 || snapshot.Emojis[0].Value != "💖" || snapshot.Emojis[0].Count != 1 {
		t.Fatalf("wrong snapshot: %+v", snapshot)
	}
	database.DB.First(&q)
	if q.Emojis != `[{"value":"💖","count":1}]` {
		t.Fatalf("response wasn't committed: %s", q.Emojis)
	}
	if len(controller.eventChan) != 1 {
		t.Fatal("missing broadcast")
	}
	event := <-controller.eventChan
	if event.Type != SSEventEmoji {
		t.Fatal("wrong event type")
	}
	eventData, _ := json.Marshal(event.Data)
	if string(eventData) != string(result.Data) {
		t.Fatal("HTTP and WebSocket snapshots differ")
	}
	if duplicate := send("👍"); duplicate.Code != 400 {
		t.Fatal("duplicate reaction accepted")
	}
	if invalid := send("invalid"); invalid.Code != 400 {
		t.Fatal("invalid reaction accepted")
	}
	if len(controller.eventChan) != 0 {
		t.Fatal("failed reactions must not broadcast")
	}
}

func TestRepeatedReactionSettingAndRestoredLimit(t *testing.T) {
	appearanceDB(t)
	config := database.Config{SpamPrompt: "keep rules"}
	database.DB.Create(&config)
	database.DB.Create(&database.Question{Content: "repeat test", IsPublish: true})
	controller := &QuestionController{eventChan: make(chan SSEvent, 8)}
	settings := &ConfigController{}
	router := gin.New()
	router.PUT("/settings", settings.PutSettings)
	router.GET("/config", settings.Get)
	router.POST("/question/:id/emoji", controller.Emoji)
	setPolicy := func(body string) {
		t.Helper()
		_, response := performJSONRequest(router, "PUT", "/settings", body, nil)
		if response.Code != 200 {
			t.Fatalf("could not save policy: %+v", response)
		}
	}
	send := func(value string) testAPIResponse {
		t.Helper()
		req := httptest.NewRequest("POST", "/question/1/emoji", strings.NewReader(url.Values{"emoji": {value}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		var result testAPIResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	setPolicy(`{"spam_prompt":"keep rules","allow_repeated_reactions":true}`)
	// A client that predates this field must not accidentally disable it.
	setPolicy(`{"spam_prompt":"keep rules"}`)
	_, public := performJSONRequest(router, "GET", "/config", "", nil)
	var publicConfig database.Config
	json.Unmarshal(public.Data, &publicConfig)
	if !publicConfig.AllowRepeatedReactions {
		t.Fatal("repeat policy not exposed or not preserved")
	}
	for _, value := range []string{"😭", "😭", "👍"} {
		if response := send(value); response.Code != 200 {
			t.Fatalf("repeat rejected: %+v", response)
		}
	}
	var q database.Question
	database.DB.First(&q)
	if q.Emojis != `[{"value":"😭","count":2},{"value":"👍","count":1}]` {
		t.Fatalf("wrong repeat counts: %s", q.Emojis)
	}
	var records int64
	database.DB.Model(&database.LikeRecord{}).Count(&records)
	if records != 1 || len(controller.eventChan) != 3 {
		t.Fatal("repeat history or broadcasts incorrect")
	}
	setPolicy(`{"spam_prompt":"keep rules","allow_repeated_reactions":false}`)
	if response := send("❓"); response.Code != 400 {
		t.Fatal("disabled repeat mode failed to enforce the old limit")
	}
	setPolicy(`{"spam_prompt":"keep rules","allow_repeated_reactions":true}`)
	if response := send("❓"); response.Code != 200 {
		t.Fatal("re-enabled repeat mode rejected existing participant")
	}
	if response := send("not-a-real-emoji"); response.Code != 400 {
		t.Fatal("repeat mode allowed unknown emoji")
	}
}

func TestVoyagePublicSettingDefaultsClosedAndPreservesOmittedValue(t *testing.T) {
	appearanceDB(t)
	database.DB.Create(&database.Config{SpamPrompt: "rules"})
	r := gin.New()
	ctl := &ConfigController{}
	r.GET("/config", ctl.Get)
	r.GET("/settings", ctl.GetSettings)
	r.PUT("/settings", ctl.PutSettings)
	check := func(want bool) {
		t.Helper()
		for _, path := range []string{"/config", "/settings"} {
			_, res := performJSONRequest(r, "GET", path, "", nil)
			var data struct {
				Enabled bool `json:"voyage_public_enabled"`
			}
			if res.Code != 200 || json.Unmarshal(res.Data, &data) != nil || data.Enabled != want {
				t.Fatalf("wrong visibility at %s", path)
			}
		}
	}
	check(false)
	for _, body := range []string{`{"spam_prompt":"rules","voyage_public_enabled":true}`, `{"spam_prompt":"rules"}`} {
		_, res := performJSONRequest(r, "PUT", "/settings", body, nil)
		if res.Code != 200 {
			t.Fatal("save failed")
		}
		check(true)
	}
	_, res := performJSONRequest(r, "PUT", "/settings", `{"spam_prompt":"rules","voyage_public_enabled":false}`, nil)
	if res.Code != 200 {
		t.Fatal("disable failed")
	}
	check(false)
}
