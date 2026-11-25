package siteconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func settingsFixture(t *testing.T) *Store {
	t.Helper()
	viper.Reset()
	t.Cleanup(func() { viper.Reset(); Use(nil) })
	viper.Set("storage_type", "local")
	viper.Set("oss.endpoint", "https://oss-cn-hangzhou.aliyuncs.com")
	viper.Set("oss.region", "cn-hangzhou")
	viper.Set("oss.prefix", "mofufans")
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestSavedSettingsRedactedAndStorageAppliedAfterRestart(t *testing.T) {
	s := settingsFixture(t)
	o := s.Saved()
	o.StorageType = "oss"
	o.OSS.Bucket = "fixture-bucket"
	o.OSS.AccessKey = "fixture-access"
	o.OSS.SecretKey = "fixture-secret"
	o.TestUID = "123456"
	o.SecureCookie = true
	if err := Validate(o); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(o, false); err != nil {
		t.Fatal(err)
	}
	if s.Active().StorageType != "local" || s.Active().TestUID != "123456" || !s.Active().SecureCookie {
		t.Fatal("incorrect live activation")
	}
	raw, _ := json.Marshal(s.View())
	if strings.Contains(string(raw), "fixture-access") || strings.Contains(string(raw), "fixture-secret") {
		t.Fatal("credentials exposed in view")
	}
	if s.View()["restart_required"] != true {
		t.Fatal("restart not reported")
	}
	o.OSS.AccessKey = ""
	o.OSS.SecretKey = ""
	kept, err := s.Propose(Update{Options: o})
	if err != nil || kept.OSS.SecretKey != "fixture-secret" {
		t.Fatal("empty secret did not preserve credential")
	}
	restarted, err := Open(s.directory)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Active().StorageType != "oss" || restarted.View()["restart_required"] != false {
		t.Fatal("restart did not activate storage")
	}
	t.Setenv("MOFUFANS_BILIBILI_TEST_RECIPIENT_UID", "654321")
	proposed, err := restarted.Propose(Update{Options: o})
	if err != nil {
		t.Fatal(err)
	}
	if proposed.TestUID != "654321" {
		t.Fatal("environment precedence lost")
	}
}
func TestRejectedSettingsAndFailedSaveKeepActiveSnapshot(t *testing.T) {
	s := settingsFixture(t)
	o := s.Saved()
	o.TestUID = "001"
	if _, err := s.Propose(Update{Options: o}); err == nil {
		t.Fatal("invalid UID accepted")
	}
	o.TestUID = ""
	o.StorageType = "oss"
	o.OSS.Endpoint = "http://127.0.0.1"
	if Validate(o) == nil {
		t.Fatal("local endpoint accepted")
	}
	before := s.Active()
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	s.directory = blocker
	o = before
	o.TestUID = "123"
	if s.Save(o, true) == nil || s.Active() != before {
		t.Fatal("failed save changed active settings")
	}
}
func TestTokenSurvivesRestartAndIsRemoved(t *testing.T) {
	s := settingsFixture(t)
	a, err := s.SetupToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.SetupToken()
	if err != nil || a != b || len(a) != 64 {
		t.Fatal("unstable token")
	}
	s.RemoveSetupToken()
	if _, err = os.Stat(filepath.Join(s.directory, "setup-token")); !os.IsNotExist(err) {
		t.Fatal("token not removed")
	}
}
