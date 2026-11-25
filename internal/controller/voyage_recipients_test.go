package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"mofufans/internal/bilibili"
	"mofufans/internal/database"
	"mofufans/internal/secret"
)

func TestVoyageImportParser(t *testing.T) {
	for _, text := range []string{"UID,档位\n1000001,舰长\n1000001,舰长", "\ufeff档位\t昵称\tUID\n3\t测试\t01000001"} {
		rows, err := parseVoyageImport(text)
		if err != nil || len(rows) == 0 || rows[0].UID != "1000001" || rows[0].Tier != "captain" {
			t.Fatalf("parse failed: %+v %v", rows, err)
		}
	}
	for _, text := range []string{"1000001,舰长\n1000001,总督", "1000001,未知", "1.000001E6,舰长", "UID,档位", "0,舰长", "9223372036854775808,舰长"} {
		if _, err := parseVoyageImport(text); err == nil {
			t.Fatalf("accepted invalid import %q", text)
		}
	}
}

type voyageProfileFake struct{ fail bool }

func (fake *voyageProfileFake) PublicCard(_ context.Context, uid int64) (bilibili.Profile, error) {
	if fake.fail {
		return bilibili.Profile{}, errors.New("upstream down")
	}
	return bilibili.Profile{MID: uid, Name: "测试用户", Face: "https://i1.hdslb.com/avatar.png"}, nil
}

type voyageAvatarFake struct{}

func (voyageAvatarFake) Save(context.Context, int64, string) (string, error) {
	return "/upload-img/test-avatar.png", nil
}
func (voyageAvatarFake) Delete(string) error { return nil }

func TestVoyageRecipientsKeepCodesScopedAndImportsAtomic(t *testing.T) {
	appearanceDB(t)
	if err := database.DB.AutoMigrate(&database.VoyageIssue{}, &database.VoyageRecipient{}, &database.VoyageDispatchConfig{}); err != nil {
		t.Fatal(err)
	}
	viper.Set("security.bilibili_cookie_key", "test-voyage-key-at-least-thirty-two-bytes")
	t.Cleanup(func() { viper.Set("security.bilibili_cookie_key", "") })
	issue := database.VoyageIssue{Title: "测试一期", Gifts: []database.VoyageGift{}}
	second := database.VoyageIssue{Title: "测试二期", Gifts: []database.VoyageGift{}}
	database.DB.Create(&issue)
	database.DB.Create(&second)
	fake := &voyageProfileFake{}
	ctl := &VoyageRecipientsController{Client: fake, AvatarStorage: voyageAvatarFake{}}
	r := gin.New()
	r.GET("/issues/:id/recipients", ctl.List)
	r.POST("/issues/:id/recipients/preview", ctl.Import)
	r.POST("/issues/:id/recipients/import", ctl.Import)
	r.PUT("/issues/:id/recipients/:recipient", ctl.Update)
	r.POST("/issues/:id/recipients/:recipient/profile", ctl.Profile)
	r.GET("/issues/:id/dispatch", ctl.Dispatch)
	r.PUT("/issues/:id/dispatch", ctl.SaveDispatch)
	v := &VoyageController{}
	r.GET("/public", v.List)
	r.DELETE("/issues/:id", v.Delete)
	call := func(method, path string, body any) testAPIResponse {
		raw, _ := json.Marshal(body)
		_, result := performJSONRequest(r, method, path, string(raw), nil)
		return result
	}
	base := fmt.Sprintf("/issues/%d", issue.ID)
	if got := call("POST", base+"/recipients/preview", gin.H{"text": "UID,档位\n1000001,舰长"}); got.Code != 200 {
		t.Fatal(got)
	}
	var count int64
	database.DB.Model(&database.VoyageRecipient{}).Count(&count)
	if count != 0 {
		t.Fatal("preview inserted data")
	}
	text := "UID,档位\n1000001,舰长\n1000002,总督\n1000001,舰长"
	if got := call("POST", base+"/recipients/import", gin.H{"text": text}); got.Code != 200 {
		t.Fatal(got)
	}
	var row database.VoyageRecipient
	database.DB.Where("issue_id = ? AND uid = ?", issue.ID, "1000001").First(&row)
	originalHash := row.CodeHash
	originalEncrypted := row.CodeEncrypted
	code, err := secret.OpenVoyageCode(row.CodeEncrypted, recipientScope(issue.ID, row.UID))
	if err != nil || len(code) != 6 {
		t.Fatal("invalid generated credential", err)
	}
	if _, err := secret.OpenVoyageCode(row.CodeEncrypted, recipientScope(second.ID, row.UID)); err == nil {
		t.Fatal("credential moved across issues")
	}
	call("POST", base+"/recipients/import", gin.H{"text": text})
	database.DB.First(&row, row.ID)
	if row.CodeHash != originalHash || row.CodeEncrypted != originalEncrypted {
		t.Fatal("repeat import rotated credentials")
	}
	if got := call("POST", base+"/recipients/import", gin.H{"text": "1000003,舰长\n1000001,总督"}); got.Code != 409 {
		t.Fatal("tier conflict accepted")
	}
	database.DB.Model(&database.VoyageRecipient{}).Where("issue_id = ?", issue.ID).Count(&count)
	if count != 2 {
		t.Fatal("partial import committed")
	}
	if got := call("GET", base+"/recipients", nil); strings.Contains(string(got.Data), "code") || strings.Contains(string(got.Data), originalHash) {
		t.Fatal("regular roster leaked credential")
	}
	if got := call("GET", "/public", nil); strings.Contains(string(got.Data), row.UID) {
		t.Fatal("public issue leaked roster")
	}
	if got := call("DELETE", base, nil); got.Code != 409 {
		t.Fatal("issue with recipients deleted")
	}
	rowPath := fmt.Sprintf("%s/recipients/%d", base, row.ID)
	if got := call("PUT", fmt.Sprintf("/issues/%d/recipients/%d", second.ID, row.ID), gin.H{"action": "sent", "version": row.Version}); got.Code != 404 {
		t.Fatal("cross-issue recipient changed")
	}
	if got := call("POST", rowPath+"/profile", gin.H{}); got.Code != 200 || !strings.Contains(string(got.Data), "测试用户") {
		t.Fatal("profile not enriched")
	}
	if got := call("PUT", rowPath, gin.H{"action": "prepared", "version": row.Version}); got.Code != 200 {
		t.Fatal(got)
	}
	database.DB.First(&row, row.ID)
	if row.PreparedAt == nil || row.SentAt != nil {
		t.Fatal("preparing incorrectly marked sent")
	}
	if got := call("PUT", rowPath, gin.H{"action": "sent", "version": row.Version - 1}); got.Code != 409 {
		t.Fatal("stale mutation accepted")
	}
	call("PUT", rowPath, gin.H{"action": "sent", "version": row.Version})
	database.DB.First(&row, row.ID)
	if row.SentAt == nil {
		t.Fatal("sent state not persisted")
	}
	call("PUT", rowPath, gin.H{"action": "tier", "tier": "admiral", "version": row.Version})
	database.DB.First(&row, row.ID)
	if row.CodeHash != originalHash || row.SentAt != nil {
		t.Fatal("tier correction rotated code or kept stale notification")
	}
	call("PUT", rowPath, gin.H{"action": "disable", "version": row.Version})
	database.DB.First(&row, row.ID)
	if got := call("GET", base+"/dispatch", nil); strings.Contains(string(got.Data), code) {
		t.Fatal("disabled code exposed")
	}
	if got := call("PUT", rowPath, gin.H{"action": "prepared", "version": row.Version}); got.Code != 400 {
		t.Fatal("disabled recipient prepared")
	}
	call("PUT", rowPath, gin.H{"action": "enable", "version": row.Version})
	database.DB.First(&row, row.ID)
	if got := call("PUT", rowPath, gin.H{"action": "rotate", "version": row.Version}); got.Code != 400 {
		t.Fatal("retired rotation accepted")
	}
	database.DB.First(&row, row.ID)
	if row.CodeHash != originalHash {
		t.Fatal("retired rotation changed original code")
	}
	if got := call("PUT", base+"/dispatch", gin.H{"template": "{UID} {领取码}", "entry_url": "javascript:alert(1)", "version": 0}); got.Code != 400 {
		t.Fatal("unsafe URL accepted")
	}
	if got := call("PUT", base+"/dispatch", gin.H{"template": "{UID} {领取码}", "entry_url": "", "version": 0}); got.Code != 200 {
		t.Fatal("template save failed")
	}
	if got := call("PUT", base+"/dispatch", gin.H{"template": "{UID} {领取码}", "version": 0}); got.Code != 409 {
		t.Fatal("stale template replaced saved content")
	}
}

func TestVoyageImportOptionalTier(t *testing.T) {
	for _, text := range []string{"1000001", "UID\n1000001", "UID,档位\n1000001,", "UID,昵称\n1000001,测试", "昵称\tUID\n测试\t1000001"} {
		rows, err := parseVoyageImport(text)
		if err != nil || len(rows) != 1 || rows[0].UID != "1000001" || rows[0].Tier != "" {
			t.Fatalf("optional tier parse failed: %+v %v", rows, err)
		}
	}
	for _, text := range []string{"1000001\n1000001\t舰长", "1000001,舰长\n1000001", "UID,档位\n1000001,\n1000001,舰长"} {
		rows, err := parseVoyageImport(text)
		if err != nil || len(rows) != 2 || rows[0].Tier != "captain" || rows[1].Status != "duplicate" {
			t.Fatalf("optional duplicate tier failed: %+v %v", rows, err)
		}
	}
}

func TestVoyageImportHandTypedWhitespaceAndOptionalHeader(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "plain space without header", text: "1000001 舰长\n1000002 提督"},
		{name: "multiple spaces with header", text: "UID   档位\n1000001   舰长\n1000002   提督"},
		{name: "full width space without header", text: "1000001　舰长\n1000002　提督"},
		{name: "non breaking space without header", text: "1000001\u00a0舰长\n1000002\u00a0提督"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows, err := parseVoyageImport(test.text)
			if err != nil || len(rows) != 2 || rows[0].UID != "1000001" || rows[0].Tier != "captain" || rows[1].UID != "1000002" || rows[1].Tier != "admiral" {
				t.Fatalf("whitespace import failed: %+v %v", rows, err)
			}
		})
	}
}

func TestVoyageUIDOnlyImportKeepsExistingTierAndCredentials(t *testing.T) {
	appearanceDB(t)
	if err := database.DB.AutoMigrate(&database.VoyageIssue{}, &database.VoyageRecipient{}); err != nil {
		t.Fatal(err)
	}
	viper.Set("security.bilibili_cookie_key", "test-voyage-key-at-least-thirty-two-bytes")
	t.Cleanup(func() { viper.Set("security.bilibili_cookie_key", "") })
	issue := database.VoyageIssue{Title: "可选档位"}
	database.DB.Create(&issue)
	ctl := &VoyageRecipientsController{}
	r := gin.New()
	r.POST("/issues/:id/recipients/preview", ctl.Import)
	r.POST("/issues/:id/recipients/import", ctl.Import)
	r.PUT("/issues/:id/recipients/:recipient", ctl.Update)
	call := func(method, path string, body any) testAPIResponse {
		raw, _ := json.Marshal(body)
		_, result := performJSONRequest(r, method, path, string(raw), nil)
		return result
	}
	base := fmt.Sprintf("/issues/%d/recipients", issue.ID)
	if got := call("POST", base+"/preview", gin.H{"text": "UID\n1000001"}); got.Code != 200 {
		t.Fatal(got)
	}
	var count int64
	database.DB.Model(&database.VoyageRecipient{}).Count(&count)
	if count != 0 {
		t.Fatal("optional preview wrote data")
	}
	if got := call("POST", base+"/import", gin.H{"text": "UID,档位\n1000001,\n1000002,舰长"}); got.Code != 200 {
		t.Fatal(got)
	}
	var blank, existing database.VoyageRecipient
	database.DB.Where("uid = ?", "1000001").First(&blank)
	database.DB.Where("uid = ?", "1000002").First(&existing)
	if blank.Tier != "" || len(blank.CodeHash) == 0 {
		t.Fatal("missing tier prevented credential creation")
	}
	hash, encrypted := existing.CodeHash, existing.CodeEncrypted
	now := time.Now()
	database.DB.Model(&existing).Updates(map[string]any{"sent_at": now, "disabled": true})
	if got := call("POST", base+"/import", gin.H{"text": "UID\n1000001\n1000002"}); got.Code != 200 {
		t.Fatal(got)
	}
	database.DB.First(&existing, existing.ID)
	if existing.Tier != "captain" || existing.CodeHash != hash || existing.CodeEncrypted != encrypted || existing.SentAt == nil || !existing.Disabled {
		t.Fatal("UID-only reimport changed an existing recipient")
	}
	path := fmt.Sprintf("%s/%d", base, blank.ID)
	if got := call("PUT", path, gin.H{"action": "tier", "tier": "admiral", "version": blank.Version}); got.Code != 200 {
		t.Fatal(got)
	}
	database.DB.First(&blank, blank.ID)
	if got := call("PUT", path, gin.H{"action": "tier", "tier": "", "version": blank.Version}); got.Code != 200 {
		t.Fatal(got)
	}
	database.DB.First(&blank, blank.ID)
	if blank.Tier != "" {
		t.Fatal("tier could not be cleared")
	}
	if got := call("PUT", path, gin.H{"action": "tier", "tier": "unknown", "version": blank.Version}); got.Code != 400 {
		t.Fatal("unknown tier accepted")
	}
}
