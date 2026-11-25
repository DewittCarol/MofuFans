package main

import (
	"encoding/json"
	"github.com/spf13/viper"
	"mofufans/internal/siteconfig"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigEnvironmentOverrideAndFailClosed(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	dir := t.TempDir()
	t.Setenv("MOFUFANS_STATE_DIRECTORY", dir)
	file := filepath.Join(dir, "config.json")
	config := map[string]any{"db_type": "sqlite", "sqlite": filepath.Join(dir, "data", "site.db"), "storage_type": "local", "security": map[string]any{"session_secret": strings.Repeat("s", 48), "bilibili_cookie_key": strings.Repeat("k", 48)}}
	raw, _ := json.Marshal(config)
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MOFUFANS_CONFIG", file)
	t.Setenv("MOFUFANS_SECURITY_SESSION_SECRET", strings.Repeat("e", 48))
	if err := loadConfig(); err != nil {
		t.Fatal(err)
	}
	if viper.GetString("security.session_secret") != strings.Repeat("e", 48) {
		t.Fatal("environment override ignored")
	}
	t.Setenv("MOFUFANS_SECURITY_SESSION_SECRET", "short")
	if loadConfig() == nil {
		t.Fatal("short session secret accepted")
	}
	t.Setenv("MOFUFANS_SECURITY_SESSION_SECRET", strings.Repeat("k", 48))
	if loadConfig() == nil {
		t.Fatal("reused encryption/session key accepted")
	}
}

func TestNoConfigGeneratesPersistentIndependentKeys(t *testing.T) {
	viper.Reset()
	t.Cleanup(func() { viper.Reset(); siteconfig.Use(nil) })
	t.Chdir(t.TempDir())
	t.Setenv("MOFUFANS_CONFIG", "")
	t.Setenv("MOFUFANS_SECURITY_SESSION_SECRET", "")
	t.Setenv("MOFUFANS_SECURITY_BILIBILI_COOKIE_KEY", "")
	if err := loadConfig(); err != nil {
		t.Fatal(err)
	}
	a, b := viper.GetString("security.session_secret"), viper.GetString("security.bilibili_cookie_key")
	if len(a) != 64 || len(b) != 64 || a == b {
		t.Fatal("keys not independently generated")
	}
	if siteconfig.Active().StorageType != "local" || viper.GetString("bootstrap_admin.username") != "" {
		t.Fatal("unexpected startup defaults")
	}
	viper.Reset()
	if err := loadConfig(); err != nil {
		t.Fatal(err)
	}
	if a != viper.GetString("security.session_secret") || b != viper.GetString("security.bilibili_cookie_key") {
		t.Fatal("keys changed on restart")
	}
	if err := os.Remove("data/instance.json"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("data/mofufans.db", []byte("existing database fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	viper.Reset()
	if loadConfig() == nil {
		t.Fatal("lost keys on an existing installation silently replaced")
	}
}
