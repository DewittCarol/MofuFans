package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mofufans/internal/database"
)

func rosterSyncSetup(t *testing.T) *gin.Engine {
	r := fulfillmentSetup(t)
	ctl := &VoyageRostersController{}
	r.PUT("/rosters/:roster/members/:member", ctl.Update)
	r.POST("/rosters/:roster/members/delete", ctl.DeleteMembers)
	return r
}
func rosterTestMember(t *testing.T, roster database.VoyageRoster, uid, tier string) database.VoyageRosterMember {
	var m database.VoyageRosterMember
	if err := database.DB.Where("roster_id = ? AND uid = ? AND tier = ?", roster.ID, uid, tier).First(&m).Error; err != nil {
		t.Fatal(err)
	}
	return m
}
func updateRosterTestMember(t *testing.T, r *gin.Engine, m *database.VoyageRosterMember, want int) {
	batchCall(t, r, "PUT", fmt.Sprintf("/rosters/%d/members/%d", m.RosterID, m.ID), m, want)
	database.DB.First(m, m.ID)
}
func deleteRosterTestMembers(t *testing.T, r *gin.Engine, roster database.VoyageRoster, members []database.VoyageRosterMember, want int) {
	database.DB.First(&roster, roster.ID)
	items := []gin.H{}
	for _, m := range members {
		items = append(items, gin.H{"id": m.ID, "version": m.Version})
	}
	batchCall(t, r, "POST", fmt.Sprintf("/rosters/%d/members/delete", roster.ID), gin.H{"version": roster.Version, "items": items}, want)
}

func TestRosterSyncDisableRestoreImportAndMixedParcel(t *testing.T) {
	r := rosterSyncSetup(t)
	roster := fulfillmentRoster(t, "monthly", database.VoyageRosterMember{UID: "10001", Tier: "captain"}, database.VoyageRosterMember{UID: "10001", Tier: "governor"})
	b := newTestBatch(t, r)
	syncTestBatch(t, r, &b, []database.VoyageBatchSource{{RosterID: roster.ID, Tiers: []string{"captain", "governor"}}})
	person := fulfillmentPerson(t, b, "10001")
	fulfillmentAddress(t, person, "测试省测试市测试区一号")
	_, parcels := fulfillmentPlan(t, r, shippingSelection{b.ID, roster.ID, "captain"}, shippingSelection{b.ID, roster.ID, "governor"})
	before := parcels[0]
	m := rosterTestMember(t, roster, "10001", "captain")
	m.Disabled = true
	updateRosterTestMember(t, r, &m, 200)
	updated := fulfillmentPerson(t, b, "10001")
	if updated.Disabled || len(updated.BatchItems) != 1 || updated.BatchItems[0].Tier != "governor" || updated.CodeEncrypted != person.CodeEncrypted {
		t.Fatalf("wrong live scope: %+v", updated)
	}
	var parcel database.VoyageParcel
	database.DB.First(&parcel, before.ID)
	if len(parcel.Items) != 1 || parcel.Items[0].Tier != "governor" || parcel.CancelledAt != nil || parcel.AddressEncrypted != before.AddressEncrypted || parcel.Version <= before.Version {
		t.Fatal("mixed parcel not reconciled safely")
	}
	fulfillmentChange(t, r, &before, "ship", gin.H{"courier": "顺丰", "tracking": "SF123456"}, 409)
	m.Disabled = false
	m.Name = "更正昵称"
	updateRosterTestMember(t, r, &m, 200)
	updated = fulfillmentPerson(t, b, "10001")
	if updated.Disabled || len(updated.BatchItems) != 2 || updated.Name != "更正昵称" || updated.CodeEncrypted != person.CodeEncrypted {
		t.Fatal("restore must reuse scope and credentials")
	}
	database.DB.First(&parcel, parcel.ID)
	if parcel.Name != "更正昵称" {
		t.Fatal("parcel display name did not follow correction")
	}
	preview, err := shippingPreview(database.DB, []shippingSelection{{b.ID, roster.ID, "captain"}})
	if err != nil || preview.Total != 1 {
		t.Fatalf("restored item absent from pending: %v", err)
	}
	database.DB.First(&roster, roster.ID)
	batchCall(t, r, "POST", fmt.Sprintf("/rosters/%d/members/import", roster.ID), gin.H{"version": roster.Version, "text": "10002 舰长"}, 200)
	if got := fulfillmentPerson(t, b, "10002"); len(got.BatchItems) != 1 || got.CodeEncrypted == "" {
		t.Fatal("import did not automatically join linked collection")
	}
}

func TestRosterSyncTierMovesAcrossEarlierCollection(t *testing.T) {
	r := rosterSyncSetup(t)
	roster := fulfillmentRoster(t, "monthly", database.VoyageRosterMember{UID: "10001", Tier: "captain"})
	admirals := newTestBatch(t, r)
	syncTestBatch(t, r, &admirals, []database.VoyageBatchSource{{RosterID: roster.ID, Tiers: []string{"admiral"}}})
	captains := newTestBatch(t, r)
	syncTestBatch(t, r, &captains, []database.VoyageBatchSource{{RosterID: roster.ID, Tiers: []string{"captain"}}})
	m := rosterTestMember(t, roster, "10001", "captain")
	m.Tier = "admiral"
	updateRosterTestMember(t, r, &m, 200)
	old := fulfillmentPerson(t, captains, "10001")
	next := fulfillmentPerson(t, admirals, "10001")
	if !old.Disabled || len(old.BatchItems) != 0 || next.Disabled || len(next.BatchItems) != 1 || next.BatchItems[0].Tier != "admiral" {
		t.Fatal("stale reservation blocked tier transfer")
	}
}

func TestRosterDeleteKeepsShippedHistoryAndCancelsEmptyUnshipped(t *testing.T) {
	r := rosterSyncSetup(t)
	roster := fulfillmentRoster(t, "monthly", database.VoyageRosterMember{UID: "10001", Tier: "captain"}, database.VoyageRosterMember{UID: "10002", Tier: "captain"})
	b := newTestBatch(t, r)
	sources := []database.VoyageBatchSource{{RosterID: roster.ID, Tiers: []string{"captain"}}}
	syncTestBatch(t, r, &b, sources)
	person := fulfillmentPerson(t, b, "10001")
	fulfillmentAddress(t, person, "测试省测试市已寄地址")
	_, parcels := fulfillmentPlan(t, r, shippingSelection{b.ID, roster.ID, "captain"})
	shipped := parcels[0]
	pending := parcels[1]
	fulfillmentChange(t, r, &shipped, "ship", gin.H{"courier": "顺丰", "tracking": "SF123456"}, 200)
	var members []database.VoyageRosterMember
	database.DB.Where("roster_id = ?", roster.ID).Find(&members)
	deleteRosterTestMembers(t, r, roster, members, 200)
	var count int64
	database.DB.Model(&database.VoyageRosterMember{}).Where("roster_id = ?", roster.ID).Count(&count)
	if count != 0 {
		t.Fatal("records were disabled instead of deleted")
	}
	var kept database.VoyageParcel
	database.DB.First(&kept, shipped.ID)
	if kept.CancelledAt != nil || kept.ShippedAt == nil || kept.Version != shipped.Version || kept.AddressEncrypted != shipped.AddressEncrypted || kept.Tracking != shipped.Tracking {
		t.Fatal("shipped history changed")
	}
	database.DB.First(&pending, pending.ID)
	if pending.CancelledAt == nil {
		t.Fatal("empty unshipped parcel not cancelled")
	}
	if got := fulfillmentPerson(t, b, "10002"); !got.Disabled || len(got.BatchItems) != 0 {
		t.Fatal("deleted user still eligible")
	}
	if got := fulfillmentPerson(t, b, "10001"); got.Disabled || len(got.BatchItems) != 1 || got.CodeEncrypted != person.CodeEncrypted {
		t.Fatal("shipped tracking access lost")
	}
	// Deleting then reimporting the same UID/tier must not issue a second gift.
	database.DB.First(&roster, roster.ID)
	batchCall(t, r, "POST", fmt.Sprintf("/rosters/%d/members/import", roster.ID), gin.H{"version": roster.Version, "text": "10001 舰长"}, 200)
	if got := fulfillmentPerson(t, b, "10001"); len(got.BatchItems) != 1 || got.BatchItems[0].Key != shipped.Items[0].Key {
		t.Fatal("reimport duplicated shipped entitlement")
	}
	if _, err := shippingPreview(database.DB, []shippingSelection{{b.ID, roster.ID, "captain"}}); err == nil {
		t.Fatal("shipped gift became pending again")
	}
	other := newTestBatch(t, r)
	p := syncTestBatch(t, r, &other, sources)
	if p.Total != 0 {
		t.Fatal("reimport allowed duplicate in another collection")
	}
	database.DB.Model(&database.VoyageRecipient{}).Where("id = ?", person.ID).Update("disabled", true)
	p = syncTestBatch(t, r, &other, sources)
	if p.Total != 0 {
		t.Fatal("disabled recipient hid a shipped reservation")
	}
}

func TestRosterBulkDeleteValidatesEveryRecordAndRollsBack(t *testing.T) {
	r := rosterSyncSetup(t)
	roster := fulfillmentRoster(t, "monthly", database.VoyageRosterMember{UID: "10001", Tier: "captain"}, database.VoyageRosterMember{UID: "10001", Tier: "governor"})
	other := fulfillmentRoster(t, "event", database.VoyageRosterMember{UID: "10002"})
	a := rosterTestMember(t, roster, "10001", "captain")
	b := rosterTestMember(t, roster, "10001", "governor")
	foreign := rosterTestMember(t, other, "10002", "")
	stale := b
	stale.Version++
	deleteRosterTestMembers(t, r, roster, []database.VoyageRosterMember{a, stale}, 409)
	deleteRosterTestMembers(t, r, roster, []database.VoyageRosterMember{a, foreign}, 409)
	deleteRosterTestMembers(t, r, roster, []database.VoyageRosterMember{a, a}, 409)
	var count int64
	database.DB.Model(&database.VoyageRosterMember{}).Where("roster_id = ?", roster.ID).Count(&count)
	if count != 2 {
		t.Fatal("failed delete partially applied")
	}
	batch := newTestBatch(t, r)
	syncTestBatch(t, r, &batch, []database.VoyageBatchSource{{RosterID: roster.ID, Tiers: []string{"captain", "governor"}}})
	callback := "test:fail-auto-sync"
	database.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "voyage_recipients" {
			tx.AddError(errors.New("injected sync failure"))
		}
	})
	deleteRosterTestMembers(t, r, roster, []database.VoyageRosterMember{a}, 409)
	database.DB.Callback().Update().Remove(callback)
	database.DB.Model(&database.VoyageRosterMember{}).Where("roster_id = ?", roster.ID).Count(&count)
	if count != 2 {
		t.Fatal("sync failure deleted source record")
	}
	if got := fulfillmentPerson(t, batch, "10001"); len(got.BatchItems) != 2 {
		t.Fatal("sync failure partially changed recipient")
	}
	deleteRosterTestMembers(t, r, roster, []database.VoyageRosterMember{a}, 200)
	if got := fulfillmentPerson(t, batch, "10001"); len(got.BatchItems) != 1 || got.BatchItems[0].Tier != "governor" {
		t.Fatal("delete affected other tier")
	}
}

func TestRosterBulkDeleteBeyondOnePageAndEmptyPublishedCollection(t *testing.T) {
	r := rosterSyncSetup(t)
	roster := fulfillmentRoster(t, "event")
	members := make([]database.VoyageRosterMember, 700)
	for i := range members {
		members[i] = database.VoyageRosterMember{RosterID: roster.ID, UID: fmt.Sprint(10000 + i), Version: 1}
	}
	if err := database.DB.CreateInBatches(&members, 100).Error; err != nil {
		t.Fatal(err)
	}
	b := newTestBatch(t, r)
	syncTestBatch(t, r, &b, []database.VoyageBatchSource{{RosterID: roster.ID}})
	database.DB.Model(&b).Update("published", true)
	deleteRosterTestMembers(t, r, roster, members, 200)
	var count int64
	database.DB.Model(&database.VoyageRecipient{}).Where("issue_id = ? AND disabled = ?", b.ID, false).Count(&count)
	if count != 0 {
		t.Fatal("published collection prevented automatic removal")
	}
	database.DB.First(&b, b.ID)
	batchCall(t, r, "PUT", fmt.Sprintf("/batches/%d", b.ID), gin.H{"version": b.Version, "title": "空名单仍可编辑", "published": true}, 200)
	// Cross-site destructive requests still fail before any database writes.
	req := httptest.NewRequest("POST", fmt.Sprintf("/rosters/%d/members/delete", roster.ID), strings.NewReader(`{"items":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://foreign.invalid")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	var res testAPIResponse
	json.Unmarshal(response.Body.Bytes(), &res)
	if res.Code != 403 {
		t.Fatal("cross-origin delete accepted")
	}
}

func TestRosterRemovingAddressSourceUsesRemainingCollection(t *testing.T) {
	r := rosterSyncSetup(t)
	a := fulfillmentRoster(t, "monthly", database.VoyageRosterMember{UID: "10001", Tier: "captain"})
	b := fulfillmentRoster(t, "event", database.VoyageRosterMember{UID: "10001"})
	one := newTestBatch(t, r)
	syncTestBatch(t, r, &one, []database.VoyageBatchSource{{RosterID: a.ID, Tiers: []string{"captain"}}})
	two := newTestBatch(t, r)
	syncTestBatch(t, r, &two, []database.VoyageBatchSource{{RosterID: b.ID}})
	p1 := fulfillmentPerson(t, one, "10001")
	p2 := fulfillmentPerson(t, two, "10001")
	fulfillmentAddress(t, p1, "测试省测试市第一地址")
	fulfillmentAddress(t, p2, "测试省测试市第二地址")
	_, parcels := fulfillmentPlan(t, r, shippingSelection{one.ID, a.ID, "captain"}, shippingSelection{two.ID, b.ID, ""})
	p := parcels[0]
	fulfillmentChange(t, r, &p, "address", gin.H{"address_recipient_id": p1.ID, "address_version": 1}, 200)
	now := time.Now()
	database.DB.Model(&p).Update("exported_at", &now)
	m := rosterTestMember(t, a, "10001", "captain")
	deleteRosterTestMembers(t, r, a, []database.VoyageRosterMember{m}, 200)
	database.DB.First(&p, p.ID)
	if len(p.Items) != 1 || p.Items[0].RosterID != b.ID || p.AddressRecipientID != p2.ID || p.ExportedAt != nil {
		t.Fatal("parcel kept removed address source")
	}
}
