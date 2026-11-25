package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"mofufans/internal/database"
	"mofufans/internal/secret"
	"strings"
	"sync"
	"testing"
	"time"
)

func batchTestSetup(t *testing.T) *gin.Engine {
	t.Helper()
	appearanceDB(t)
	if e := database.DB.AutoMigrate(&database.VoyageIssue{}, &database.VoyageRecipient{}, &database.VoyageDelivery{}, &database.VoyageDispatchConfig{}); e != nil {
		t.Fatal(e)
	}
	if e := database.InitializeVoyageBatches(); e != nil {
		t.Fatal(e)
	}
	viper.Set("security.bilibili_cookie_key", "offline-batch-tests-key-at-least-32-bytes")
	t.Cleanup(func() { viper.Set("security.bilibili_cookie_key", "") })
	r := gin.New()
	b := &VoyageBatchesController{}
	d := &VoyageDeliveryController{}
	roster := &VoyageRostersController{}
	r.POST("/batches", b.Save)
	r.PUT("/batches/:id", b.Save)
	r.POST("/batches/:id/sync", b.Sync)
	r.GET("/public", b.Public)
	r.POST("/issues/:id/claim", d.Claim)
	r.POST("/issues/:id/shipments/batch", d.Batch)
	r.POST("/issues/:id/shipments/export", d.Export)
	r.GET("/issues/:id/dispatch", (&VoyageRecipientsController{}).Dispatch)
	r.POST("/rosters", roster.Save)
	r.POST("/rosters/:roster/members/preview", roster.Import)
	r.POST("/rosters/:roster/members/import", roster.Import)
	return r
}
func batchCall(t *testing.T, r *gin.Engine, method, path string, body any, want int) testAPIResponse {
	t.Helper()
	raw, _ := json.Marshal(body)
	_, res := performJSONRequest(r, method, path, string(raw), nil)
	if res.Code != want {
		t.Fatalf("%s %s: got %d want %d: %+v", method, path, res.Code, want, res)
	}
	return res
}
func newTestBatch(t *testing.T, r *gin.Engine) database.VoyageBatch {
	res := batchCall(t, r, "POST", "/batches", gin.H{"title": "9月集中寄送"}, 200)
	var b database.VoyageBatch
	json.Unmarshal(res.Data, &b)
	return b
}
func syncTestBatch(t *testing.T, r *gin.Engine, b *database.VoyageBatch, sources []database.VoyageBatchSource) batchPreview {
	t.Helper()
	path := fmt.Sprintf("/batches/%d/sync", b.ID)
	body := gin.H{"version": b.Version, "sources": sources}
	res := batchCall(t, r, "POST", path, body, 200)
	var p batchPreview
	json.Unmarshal(res.Data, &p)
	body["confirm"] = true
	body["token"] = p.Token
	batchCall(t, r, "POST", path, body, 200)
	database.DB.First(b, b.ID)
	return p
}
func TestVoyageBatchPartialMonthsAndExactTiers(t *testing.T) {
	r := batchTestSetup(t)
	sources := []database.VoyageBatchSource{}
	add := func(month, category string, rows map[string]string, tiers []string) {
		list := database.VoyageRoster{Title: month + "名单", Month: month, Category: category, Version: 1}
		if category == "event" {
			list.Title = "8月转盘抽奖礼物"
			list.Month = "2026-08"
		}
		database.DB.Create(&list)
		for uid, tier := range rows {
			database.DB.Create(&database.VoyageRosterMember{RosterID: list.ID, UID: uid, Tier: tier, Version: 1})
		}
		sources = append(sources, database.VoyageBatchSource{RosterID: list.ID, Tiers: tiers})
	}
	all := []string{"captain", "admiral", "governor"}
	add("2026-05", "monthly", map[string]string{"10001": "captain", "10002": "captain", "10003": "captain"}, all)
	add("2026-06", "monthly", map[string]string{"10001": "captain", "10002": "governor", "10003": "admiral"}, []string{"captain", "admiral"})
	add("2026-07", "monthly", map[string]string{"10001": "captain", "10002": "governor"}, []string{"governor"})
	add("2026-08", "monthly", map[string]string{"10001": "captain", "10002": "captain", "10004": ""}, all)
	add("", "event", map[string]string{"10002": "", "10005": ""}, nil)
	b := newTestBatch(t, r)
	p := syncTestBatch(t, r, &b, sources)
	if p.Total != 4 || len(p.Warnings) != 1 {
		t.Fatalf("wrong totals: %+v", p)
	}
	var members []database.VoyageRecipient
	database.DB.Where("issue_id = ?", b.ID).Find(&members)
	expected := map[string]int{"10001": 3, "10002": 4, "10003": 2, "10005": 1}
	for _, m := range members {
		if len(m.BatchItems) != expected[m.UID] {
			t.Fatalf("UID %s wrong scope %+v", m.UID, m.BatchItems)
		}
		if m.UID == "10002" && strings.Contains(batchDescription(m), "6月") {
			t.Fatal("governor included in captain/admiral filter")
		}
	}
	// Hidden delivery scopes never become gift catalog entries.
	var catalog int64
	database.DB.Model(&database.VoyageIssue{}).Where("kind <> ?", "batch").Count(&catalog)
	if catalog != 0 {
		t.Fatal("batch leaked into gift catalog")
	}
	// A different batch may use the remaining tier in the same month.
	other := newTestBatch(t, r)
	q := syncTestBatch(t, r, &other, []database.VoyageBatchSource{{RosterID: sources[1].RosterID, Tiers: []string{"governor"}}})
	if q.Total != 1 {
		t.Fatal("separate governor batch unavailable")
	}
	// Same eligibility cannot be allocated twice, even before shipping.
	third := newTestBatch(t, r)
	q = syncTestBatch(t, r, &third, []database.VoyageBatchSource{sources[0]})
	if q.Total != 0 || len(q.Warnings) != 3 {
		t.Fatal("duplicate allocation not prevented")
	}
}
func TestVoyageBatchClaimsPrivacyFreezeAndResync(t *testing.T) {
	r := batchTestSetup(t)
	list := database.VoyageRoster{Title: "5月", Month: "2026-05", Category: "monthly", Version: 1}
	database.DB.Create(&list)
	m := database.VoyageRosterMember{RosterID: list.ID, UID: "12345", Tier: "captain", Version: 1}
	database.DB.Create(&m)
	b := newTestBatch(t, r)
	sources := []database.VoyageBatchSource{{RosterID: list.ID, Tiers: []string{"captain", "admiral"}}}
	syncTestBatch(t, r, &b, sources)
	var row database.VoyageRecipient
	database.DB.Where("issue_id = ?", b.ID).First(&row)
	code, _ := secret.OpenVoyageCode(row.CodeEncrypted, recipientScope(b.ID, row.UID))
	claim := fmt.Sprintf("/issues/%d/claim", b.ID)
	payload := gin.H{"uid": row.UID, "code": code, "action": "view"}
	batchCall(t, r, "POST", claim, payload, 401)
	batchCall(t, r, "PUT", fmt.Sprintf("/batches/%d", b.ID), gin.H{"title": b.Title, "published": true, "version": b.Version}, 200)
	database.DB.First(&b, b.ID)
	res := batchCall(t, r, "GET", "/public", nil, 200)
	if bytes.Contains(res.Data, []byte(row.UID)) || bytes.Contains(res.Data, []byte(code)) || bytes.Contains(res.Data, []byte("captain")) {
		t.Fatal("public list leaked membership")
	}
	payload["code"] = "wrong"
	batchCall(t, r, "POST", claim, payload, 401)
	payload["code"] = code
	payload["uid"] = "54321"
	batchCall(t, r, "POST", claim, payload, 401)
	payload["uid"] = row.UID
	res = batchCall(t, r, "POST", claim, payload, 200)
	if !bytes.Contains(res.Data, []byte("舰长礼物")) {
		t.Fatal("personal labels absent")
	}
	payload["action"] = "save"
	payload["version"] = 0
	payload["address"] = VoyageAddress{Name: "离线测试", Phone: "13800000000", Address: "测试省测试市测试区一号"}
	batchCall(t, r, "POST", claim, payload, 200)
	var original database.VoyageDelivery
	database.DB.First(&original, "recipient_id = ?", row.ID)
	// Preview cannot be committed after the source was edited.
	path := fmt.Sprintf("/batches/%d/sync", b.ID)
	res = batchCall(t, r, "POST", path, gin.H{"sources": sources, "version": b.Version}, 200)
	var old batchPreview
	json.Unmarshal(res.Data, &old)
	database.DB.Model(&m).Updates(map[string]any{"tier": "admiral", "version": 2})
	database.DB.Model(&list).Update("version", 2)
	batchCall(t, r, "POST", path, gin.H{"sources": sources, "version": b.Version, "confirm": true, "token": old.Token}, 409)
	p := syncTestBatch(t, r, &b, sources)
	if p.Changed != 1 {
		t.Fatal("tier change not surfaced")
	}
	database.DB.First(&row, row.ID)
	if row.SentAt != nil || !strings.Contains(batchDescription(row), "提督") {
		t.Fatal("resync state invalid")
	}
	after, _ := secret.OpenVoyageCode(row.CodeEncrypted, recipientScope(b.ID, row.UID))
	if after != code {
		t.Fatal("resync replaced credential")
	}
	var stored database.VoyageDelivery
	database.DB.First(&stored, "recipient_id = ?", row.ID)
	if stored.AddressEncrypted != original.AddressEncrypted {
		t.Fatal("resync changed address")
	}
	// Closed collections allow reads, not address edits.
	batchCall(t, r, "PUT", fmt.Sprintf("/batches/%d", b.ID), gin.H{"title": b.Title, "published": true, "closed": true, "version": b.Version}, 200)
	payload["version"] = 1
	batchCall(t, r, "POST", claim, payload, 409)
	payload["action"] = "view"
	batchCall(t, r, "POST", claim, payload, 200)
	// Shipping freezes the roster and each person's selected monthly scope.
	database.DB.Model(&row).Update("sent_at", time.Now())
	batchCall(t, r, "POST", fmt.Sprintf("/issues/%d/shipments/batch", b.ID), gin.H{"action": "ship", "items": []gin.H{{"id": row.ID, "version": 1, "recipient_version": row.Version, "courier": "测试快递", "tracking": "TEST12345"}}}, 200)
	database.DB.First(&b, b.ID)
	batchCall(t, r, "POST", path, gin.H{"sources": sources, "version": b.Version}, 409)
}
func TestVoyageRosterImportFormatsAndConflicts(t *testing.T) {
	for _, text := range []string{"UID,档位,昵称\n12345,舰长,测试", "\ufeff舰长,12345,测试,2026-05-01T12:00:00", "12345 舰长"} {
		rows, e := parseRosterImport(text, "monthly")
		if e != nil || len(rows) != 1 || rows[0].UID != "12345" || rows[0].Tier != "captain" {
			t.Fatalf("parse %q: %+v %v", text, rows, e)
		}
	}
	if rows, e := parseRosterImport("12345 舰长\n12345 总督\n12345 舰长", "monthly"); e != nil || len(rows) != 3 || rows[1].Status != "new" || rows[2].Status != "duplicate" {
		t.Fatal("different tiers must coexist; only identical tier repeats are duplicates")
	}
	rows, e := parseRosterImport("UID,昵称\n12345,中奖用户", "event")
	if e != nil || rows[0].Name != "中奖用户" || rows[0].Tier != "" {
		t.Fatal("event parser invalid")
	}
	r := batchTestSetup(t)
	res := batchCall(t, r, "POST", "/rosters", gin.H{"title": "5月", "category": "monthly", "month": "2026-05"}, 200)
	var roster database.VoyageRoster
	json.Unmarshal(res.Data, &roster)
	batchCall(t, r, "POST", fmt.Sprintf("/rosters/%d/members/import", roster.ID), gin.H{"text": "12345 舰长", "version": 1}, 200)
	batchCall(t, r, "POST", fmt.Sprintf("/rosters/%d/members/import", roster.ID), gin.H{"text": "12345 总督", "version": 2}, 200)
	// No network, account or real business database is used by these tests.
}

func TestVoyageBatchTierCorrectionDoesNotCreateSecondEntitlement(t *testing.T) {
	for _, shipped := range []bool{false, true} {
		t.Run(fmt.Sprint(shipped), func(t *testing.T) {
			r := batchTestSetup(t)
			list := database.VoyageRoster{Title: "5月", Month: "2026-05", Category: "monthly", Version: 1}
			database.DB.Create(&list)
			m := database.VoyageRosterMember{RosterID: list.ID, UID: "12345", Tier: "captain", Version: 1}
			database.DB.Create(&m)
			a := newTestBatch(t, r)
			syncTestBatch(t, r, &a, []database.VoyageBatchSource{{RosterID: list.ID, Tiers: []string{"captain"}}})
			var row database.VoyageRecipient
			database.DB.Where("issue_id = ?", a.ID).First(&row)
			if shipped {
				now := time.Now()
				database.DB.Create(&database.VoyageDelivery{RecipientID: row.ID, ShippedAt: &now, Version: 1})
			}
			database.DB.Model(&m).Update("tier", "admiral")
			database.DB.Model(&list).Update("version", 2)
			b := newTestBatch(t, r)
			p := syncTestBatch(t, r, &b, []database.VoyageBatchSource{{RosterID: list.ID, Tiers: []string{"admiral"}}})
			if p.Total != 0 || len(p.Warnings) != 1 {
				t.Fatal("tier correction generated a second entitlement")
			}
			database.DB.First(&row, row.ID)
			if !strings.Contains(batchDescription(row), "舰长") {
				t.Fatal("old descriptive snapshot mutated")
			}
		})
	}
}

func TestVoyageBatchClearReleasesButPreservesPrivateRecords(t *testing.T) {
	r := batchTestSetup(t)
	list := database.VoyageRoster{Title: "抽奖", Category: "event", Version: 1}
	database.DB.Create(&list)
	database.DB.Create(&database.VoyageRosterMember{RosterID: list.ID, UID: "12345", Version: 1})
	a := newTestBatch(t, r)
	sources := []database.VoyageBatchSource{{RosterID: list.ID}}
	syncTestBatch(t, r, &a, sources)
	var row database.VoyageRecipient
	database.DB.Where("issue_id = ?", a.ID).First(&row)
	originalCode := row.CodeEncrypted
	private := database.VoyageDelivery{RecipientID: row.ID, AddressEncrypted: "opaque-address-ciphertext", Version: 1}
	database.DB.Create(&private)
	database.DB.Model(&a).Update("published", true)
	a.Published = true
	path := fmt.Sprintf("/batches/%d/sync", a.ID)
	res := batchCall(t, r, "POST", path, gin.H{"sources": []database.VoyageBatchSource{}, "version": a.Version}, 200)
	var preview batchPreview
	json.Unmarshal(res.Data, &preview)
	batchCall(t, r, "POST", path, gin.H{"sources": []database.VoyageBatchSource{}, "version": a.Version, "confirm": true, "token": preview.Token}, 409)
	database.DB.Model(&a).Update("published", false)
	a.Published = false
	p := syncTestBatch(t, r, &a, []database.VoyageBatchSource{})
	if p.Total != 0 || p.Removed != 1 {
		t.Fatal("clearing did not remove allocation")
	}
	database.DB.First(&row, row.ID)
	if !row.Disabled || row.CodeEncrypted != originalCode || len(row.BatchItems) != 0 {
		t.Fatal("clearing corrupted record")
	}
	var after database.VoyageDelivery
	database.DB.First(&after, "recipient_id = ?", private.RecipientID)
	if after.AddressEncrypted != private.AddressEncrypted {
		t.Fatal("clearing removed address")
	}
	b := newTestBatch(t, r)
	if syncTestBatch(t, r, &b, sources).Total != 1 {
		t.Fatal("cleared reservation not released")
	}
}

func TestVoyageBatchConcurrentConfirmOnlyAllocatesOnce(t *testing.T) {
	r := batchTestSetup(t)
	list := database.VoyageRoster{Title: "抽奖", Category: "event", Version: 1}
	database.DB.Create(&list)
	database.DB.Create(&database.VoyageRosterMember{RosterID: list.ID, UID: "12345", Version: 1})
	batches := []database.VoyageBatch{newTestBatch(t, r), newTestBatch(t, r)}
	sources := []database.VoyageBatchSource{{RosterID: list.ID}}
	// Both confirmations start from previews produced before either was committed.
	bodies := make([]string, 2)
	for i, b := range batches {
		res := batchCall(t, r, "POST", fmt.Sprintf("/batches/%d/sync", b.ID), gin.H{"sources": sources, "version": b.Version}, 200)
		var p batchPreview
		json.Unmarshal(res.Data, &p)
		raw, _ := json.Marshal(gin.H{"sources": sources, "version": b.Version, "confirm": true, "token": p.Token})
		bodies[i] = string(raw)
	}
	start := make(chan struct{})
	codes := make(chan int, 2)
	var wait sync.WaitGroup
	for i, b := range batches {
		wait.Add(1)
		go func(i int, b database.VoyageBatch) {
			defer wait.Done()
			<-start
			_, res := performJSONRequest(r, "POST", fmt.Sprintf("/batches/%d/sync", b.ID), bodies[i], nil)
			codes <- res.Code
		}(i, b)
	}
	close(start)
	wait.Wait()
	close(codes)
	success := 0
	for code := range codes {
		if code == 200 {
			success++
		} else if code != 409 {
			t.Fatalf("unexpected concurrent response: %d", code)
		}
	}
	var count int64
	database.DB.Model(&database.VoyageRecipient{}).Where("disabled = ?", false).Count(&count)
	if success != 1 || count != 1 {
		t.Fatalf("duplicate allocation: successes=%d recipients=%d", success, count)
	}
}
