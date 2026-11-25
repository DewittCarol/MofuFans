package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"golang.org/x/time/rate"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"mofufans/internal/database"
	"mofufans/internal/siteconfig"
)

func installationFixture(t *testing.T) (*InstallationController, *gin.Engine) {
	t.Helper()
	viper.Reset()
	siteconfig.Use(nil)
	t.Cleanup(func() { viper.Reset(); siteconfig.Use(nil) })
	viper.Set("storage_type", "local")
	viper.Set("db_type", "sqlite")
	dir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	old := database.DB
	database.DB = db
	t.Cleanup(func() { sql, _ := db.DB(); sql.Close(); database.DB = old })
	if err = db.AutoMigrate(&database.Admin{}); err != nil {
		t.Fatal(err)
	}
	if err = database.InitializeTheme(); err != nil {
		t.Fatal(err)
	}
	s, err := siteconfig.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	siteconfig.Use(s)
	ctl, err := NewInstallationController(s)
	if err != nil {
		t.Fatal(err)
	}
	ctl.limiter = rate.NewLimiter(rate.Inf, 1)
	ctl.CheckStorage = func(context.Context, siteconfig.Options) error { return nil }
	r := gin.New()
	r.Use(ctl.Gate)
	r.GET("/api/setup/status", ctl.Status)
	r.POST("/api/setup/verify", ctl.Verify)
	r.POST("/api/setup/complete", ctl.Complete)
	r.GET("/api/business", func(c *gin.Context) { Success(c, true) })
	r.Use(func(c *gin.Context) { c.Set("user", database.Admin{IsSuperAdmin: c.GetHeader("Owner") == "yes"}) })
	r.GET("/api/settings", ctl.GetSettings)
	r.PUT("/api/settings", ctl.PutSettings)
	return ctl, r
}
func installationCall(t *testing.T, r *gin.Engine, method, path, token string, body any, owner bool) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MofuFans-Request", "settings")
	req.Header.Set("X-Setup-Token", token)
	if owner {
		req.Header.Set("Owner", "yes")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func setupRequest() installationRequest {
	return installationRequest{Username: "owner", Password: "fixture-password-123", SiteName: "Fixture fans", StreamerName: "Fixture streamer", Settings: siteconfig.Update{Options: siteconfig.Options{StorageType: "local"}}}
}
func TestFirstRunGateClaimAndReplayProtection(t *testing.T) {
	ctl, r := installationFixture(t)
	call := func(method, path, token string, body any) map[string]any {
		return installationCall(t, r, method, path, token, body, false)
	}
	if call("GET", "/api/business", "", nil)["code"] != float64(503) {
		t.Fatal("uninitialized business route open")
	}
	if call("POST", "/api/setup/complete", "wrong", setupRequest())["code"] != float64(403) {
		t.Fatal("invalid token accepted")
	}
	status, _ := json.Marshal(call("GET", "/api/setup/status", "", nil))
	if strings.Contains(string(status), ctl.Token) {
		t.Fatal("status exposes token")
	}
	if call("POST", "/api/setup/complete", ctl.Token, setupRequest())["code"] != float64(200) {
		t.Fatal("claim failed")
	}
	if ctl.Required() || call("GET", "/api/business", "", nil)["code"] != float64(200) {
		t.Fatal("gate did not open")
	}
	if call("POST", "/api/setup/complete", ctl.Token, setupRequest())["code"] != float64(409) {
		t.Fatal("claim replay accepted")
	}
	var admins []database.Admin
	database.DB.Find(&admins)
	if len(admins) != 1 || !admins[0].IsSuperAdmin || admins[0].Password == setupRequest().Password {
		t.Fatal("incorrect admin creation")
	}
	var theme database.SiteTheme
	database.DB.First(&theme, 1)
	if !strings.Contains(theme.Config, "Fixture fans") {
		t.Fatal("brand not configured")
	}
	again, err := NewInstallationController(ctl.Store)
	if err != nil || again.Required() {
		t.Fatal("restart reopens installation")
	}
	if installationCall(t, r, "GET", "/api/settings", "", nil, false)["code"] != float64(403) {
		t.Fatal("non-owner settings exposed")
	}
	update := siteconfig.Update{Options: siteconfig.Options{StorageType: "local", TestUID: "123456", SecureCookie: true}}
	if installationCall(t, r, "PUT", "/api/settings", "", update, true)["code"] != float64(200) || siteconfig.Active().TestUID != "123456" {
		t.Fatal("live settings not applied")
	}
}
func TestFailedOSSCheckDoesNotCreateAdmin(t *testing.T) {
	ctl, r := installationFixture(t)
	ctl.CheckStorage = func(context.Context, siteconfig.Options) error { return errors.New("connection failed") }
	request := setupRequest()
	request.Settings.StorageType = "oss"
	request.Settings.OSS = siteconfig.OSS{Endpoint: "https://oss-cn-hangzhou.aliyuncs.com", Region: "cn-hangzhou", Bucket: "fixture", AccessKey: "fixture-id", SecretKey: "fixture-secret"}
	result := installationCall(t, r, "POST", "/api/setup/complete", ctl.Token, request, false)
	if result["code"] != float64(400) || !ctl.Required() {
		t.Fatal("failed OSS check allowed setup")
	}
	var count int64
	database.DB.Model(&database.Admin{}).Count(&count)
	if count != 0 || ctl.Store.Active().StorageType != "local" {
		t.Fatal("failed setup mutated installation")
	}
}
