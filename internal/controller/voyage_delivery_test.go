package controller

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"mofufans/internal/database"
	"mofufans/internal/secret"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestVoyageClaimAndShippingRemainScopedAndPrivate(t *testing.T) {
	appearanceDB(t)
	if err := database.DB.AutoMigrate(&database.VoyageIssue{}, &database.VoyageRecipient{}, &database.VoyageDelivery{}); err != nil {
		t.Fatal(err)
	}
	viper.Set("security.bilibili_cookie_key", "offline-address-tests-key-at-least-32-bytes")
	t.Cleanup(func() { viper.Set("security.bilibili_cookie_key", "") })
	issue := database.VoyageIssue{Title: "离线测试", Published: true, Gifts: []database.VoyageGift{}}
	database.DB.Create(&issue)
	encrypted, hash, err := secret.NewVoyageCode(recipientScope(issue.ID, "1000001"))
	if err != nil {
		t.Fatal(err)
	}
	code, _ := secret.OpenVoyageCode(encrypted, recipientScope(issue.ID, "1000001"))
	row := database.VoyageRecipient{IssueID: issue.ID, UID: "1000001", Tier: "captain", CodeEncrypted: encrypted, CodeHash: hash, Version: 1}
	database.DB.Create(&row)
	ctl := &VoyageDeliveryController{}
	r := gin.New()
	r.POST("/issues/:id/claim", ctl.Claim)
	r.GET("/issues/:id/shipments", ctl.List)
	r.PUT("/issues/:id/recipients/:recipient/shipment", ctl.Ship)
	r.GET("/issues/:id/recipients", (&VoyageRecipientsController{}).List)
	path := fmt.Sprintf("/issues/%d/claim", issue.ID)
	post := func(path, method string, body any) testAPIResponse {
		raw, _ := json.Marshal(body)
		_, result := performJSONRequest(r, method, path, string(raw), nil)
		return result
	}
	auth := map[string]any{"uid": row.UID, "code": code, "action": "view"}
	if result := post(path, "POST", auth); result.Code != 200 || !strings.Contains(string(result.Data), `"address":null`) {
		t.Fatalf("initial view: %+v", result)
	}
	wrong := map[string]any{"uid": "999999", "code": code, "action": "view"}
	if post(path, "POST", wrong).Code != 401 {
		t.Fatal("wrong UID exposed data")
	}
	if post("/issues/999/claim", "POST", auth).Code != 401 {
		t.Fatal("wrong issue exposed data")
	}
	auth["code"] = "0000-0000-0000-0000"
	if post(path, "POST", auth).Code != 401 {
		t.Fatal("wrong code accepted")
	}
	auth["code"] = code
	auth["action"] = "save"
	auth["version"] = 0
	auth["address"] = VoyageAddress{Name: "离线收件人", Phone: "13800000000", Address: "离线测试省测试市测试区一号", Note: "测试备注"}
	result := post(path, "POST", auth)
	if result.Code != 200 {
		t.Fatalf("save: %+v", result)
	}
	var saved database.VoyageDelivery
	database.DB.First(&saved, "recipient_id = ?", row.ID)
	if saved.Version != 1 || saved.AddressEncrypted == "" || strings.Contains(saved.AddressEncrypted, "13800000000") {
		t.Fatal("address not encrypted")
	}
	if _, err := secret.OpenVoyageAddress("wrong:scope", saved.AddressEncrypted); err == nil {
		t.Fatal("ciphertext usable for wrong recipient")
	}
	if post(path, "POST", auth).Code != 409 {
		t.Fatal("stale address version overwrote data")
	}
	database.DB.Model(&row).Update("sent_at", time.Now())
	shipping := fmt.Sprintf("/issues/%d/recipients/%d/shipment", issue.ID, row.ID)
	result = post(shipping, "PUT", map[string]any{"courier": "测试快递", "tracking": "TEST123456", "version": 1})
	if result.Code != 200 {
		t.Fatalf("shipping: %+v", result)
	}
	auth["version"] = 2
	if post(path, "POST", auth).Code != 409 {
		t.Fatal("shipped address modified")
	}
	auth["action"] = "view"
	result = post(path, "POST", auth)
	if result.Code != 200 || !strings.Contains(string(result.Data), "TEST123456") {
		t.Fatalf("tracking not visible: %+v", result)
	}
	_, regular := performJSONRequest(r, "GET", fmt.Sprintf("/issues/%d/recipients", issue.ID), "", nil)
	for _, secretText := range []string{"13800000000", "离线收件人", saved.AddressEncrypted, code, "TEST123456"} {
		if strings.Contains(string(regular.Data), secretText) {
			t.Fatal("private data leaked into regular roster")
		}
	}
	database.DB.Model(&row).Update("disabled", true)
	if post(path, "POST", auth).Code != 401 {
		t.Fatal("revoked recipient retained access")
	}
	database.DB.Model(&row).Updates(map[string]any{"disabled": false, "code_hash": strings.Repeat("0", 64)})
	if post(path, "POST", auth).Code != 401 {
		t.Fatal("old code retained access")
	}
	req := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
	req.Header.Set("Origin", "https://other.invalid")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	var cross testAPIResponse
	_ = json.Unmarshal(response.Body.Bytes(), &cross)
	if cross.Code != 403 {
		t.Fatal("cross-origin claim allowed")
	}
}

func TestClaimLimiterBoundsAttempts(t *testing.T) {
	ctl := &VoyageDeliveryController{}
	for i := 0; i < 12; i++ {
		if !ctl.allow("test-ip") {
			t.Fatal("initial attempts rejected")
		}
	}
	if ctl.allow("test-ip") {
		t.Fatal("unbounded claim attempts")
	}
	if !ctl.allow("another-ip") {
		t.Fatal("one IP blocked another")
	}
}
