package controller

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"mofufans/internal/ai"
	"mofufans/internal/database"
	"net/http/httptest"
	"strings"
	"testing"
)

func settingsRequest(t *testing.T, r *gin.Engine, method, path string, body any) (int, string) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MofuFans-Request", "settings")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	var envelope struct {
		Code int `json:"code"`
	}
	if json.Unmarshal(response.Body.Bytes(), &envelope) != nil {
		t.Fatal("invalid envelope")
	}
	return envelope.Code, response.Body.String()
}
func TestAISettingsEncryptionOwnershipAndMigration(t *testing.T) {
	appearanceDB(t)
	viper.Set("security.bilibili_cookie_key", "test-ai-encryption-key-at-least-32-characters")
	t.Cleanup(func() { viper.Set("security.bilibili_cookie_key", "") })
	database.DB.Create(&database.Config{SpamPrompt: "keep rules", DeepSeekAPIKey: "legacy-secret", BackgroundURL: "keep-background"})
	if err := database.InitializeAI(); err != nil {
		t.Fatal(err)
	}
	var config database.Config
	database.DB.First(&config)
	if config.DeepSeekAPIKey != "" || config.AIKeyCiphertext == "" || strings.Contains(config.AIKeyCiphertext, "legacy-secret") {
		t.Fatal("legacy key not migrated")
	}
	key, err := config.AIKey()
	if err != nil || key != "legacy-secret" {
		t.Fatal("migration lost key")
	}
	originalCipher := config.AIKeyCiphertext
	if err := database.InitializeAI(); err != nil {
		t.Fatal(err)
	}
	database.DB.First(&config)
	if config.AIKeyCiphertext != originalCipher {
		t.Fatal("migration repeated")
	}
	options, _ := config.AIOptions()
	if !options.Enabled || options.Model != "deepseek-v4-flash" {
		t.Fatal("legacy configuration changed")
	}
	r := gin.New()
	owner := true
	r.Use(func(c *gin.Context) { c.Set("user", database.Admin{IsSuperAdmin: owner}) })
	ctl := &AIController{}
	cfg := &ConfigController{}
	r.GET("/ai", ctl.Get)
	r.PUT("/ai", ctl.Put)
	r.POST("/models", ctl.Models)
	r.POST("/test", ctl.Test)
	r.GET("/config", cfg.Get)
	r.GET("/settings", cfg.GetSettings)
	r.PUT("/settings", cfg.PutSettings)
	r.PUT("/config", cfg.Put)
	for _, path := range []string{"/ai", "/config", "/settings"} {
		code, body := settingsRequest(t, r, "GET", path, nil)
		if code != 200 || strings.Contains(body, "legacy-secret") || strings.Contains(body, originalCipher) {
			t.Fatalf("private configuration exposed on %s", path)
		}
	}
	owner = false
	for _, methodPath := range [][2]string{{"GET", "/ai"}, {"PUT", "/ai"}, {"POST", "/models"}, {"POST", "/test"}} {
		if code, _ := settingsRequest(t, r, methodPath[0], methodPath[1], nil); code != 403 {
			t.Fatalf("non-owner access: %v", methodPath)
		}
	}
	owner = true
	update := AIRequest{Options: options, SpamPrompt: "new rules"}
	if code, body := settingsRequest(t, r, "PUT", "/ai", update); code != 200 {
		t.Fatal(body)
	}
	database.DB.First(&config)
	key, _ = config.AIKey()
	if key != "legacy-secret" || config.SpamPrompt != "new rules" {
		t.Fatal("blank key did not preserve stored value")
	}
	update.BaseURL = "https://new-provider.example.com/v1"
	if code, _ := settingsRequest(t, r, "PUT", "/ai", update); code != 400 {
		t.Fatal("stored key forwarded to new endpoint")
	}
	update.APIKey = "replacement-secret"
	if code, body := settingsRequest(t, r, "PUT", "/ai", update); code != 200 || strings.Contains(body, update.APIKey) {
		t.Fatalf("key replacement failed: %s", body)
	}
	for _, path := range []string{"/settings", "/config"} {
		if code, _ := settingsRequest(t, r, "PUT", path, map[string]any{"announcement": "new", "spam_prompt": "must not change AI rules", "deepseek_api_key": "must not save", "allow_repeated_reactions": true}); code != 200 {
			t.Fatal("ordinary settings save failed")
		}
	}
	database.DB.First(&config)
	key, _ = config.AIKey()
	if key != "replacement-secret" || config.SpamPrompt != "new rules" || config.BackgroundURL != "keep-background" || config.DeepSeekAPIKey != "" {
		t.Fatal("unrelated settings overwritten")
	}
	update.ClearKey = true
	update.Enabled = false
	update.APIKey = ""
	if code, body := settingsRequest(t, r, "PUT", "/ai", update); code != 200 {
		t.Fatal(body)
	}
	database.DB.First(&config)
	options, _ = config.AIOptions()
	if config.AIKeyCiphertext != "" || options.Enabled {
		t.Fatal("clear key did not disable AI")
	}
}
func TestNewInstallAIDisabled(t *testing.T) {
	config := database.Config{}
	options, err := config.AIOptions()
	if err != nil || options.Enabled || options.Protocol != ai.Default().Protocol {
		t.Fatal("AI should be optional")
	}
}
