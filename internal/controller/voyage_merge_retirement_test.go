package controller

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"mofufans/internal/database"
	"mofufans/internal/secret"
	"testing"
)

func TestVoyageRetiredMergeSchemaPreservesIndependentBatch(t *testing.T) {
	r := batchTestSetup(t)
	roster := database.VoyageRoster{Title: "8月", Month: "2026-08", Category: "monthly", Version: 1}
	database.DB.Create(&roster)
	member := database.VoyageRosterMember{RosterID: roster.ID, UID: "12345", Tier: "captain", Version: 1}
	database.DB.Create(&member)
	b := newTestBatch(t, r)
	syncTestBatch(t, r, &b, []database.VoyageBatchSource{{RosterID: roster.ID, Tiers: []string{"captain"}}})
	var row database.VoyageRecipient
	database.DB.Where("issue_id = ?", b.ID).First(&row)
	d := database.VoyageDelivery{RecipientID: row.ID, AddressEncrypted: "independent-address", Version: 1}
	database.DB.Create(&d)
	for _, sql := range []string{
		"ALTER TABLE voyage_issues ADD COLUMN shipping_target_id integer NOT NULL DEFAULT 0",
		"CREATE INDEX idx_voyage_issues_shipping_target_id ON voyage_issues(shipping_target_id)",
		"ALTER TABLE voyage_rosters ADD COLUMN legacy_issue_id integer",
		"CREATE UNIQUE INDEX idx_voyage_rosters_legacy_issue_id ON voyage_rosters(legacy_issue_id)",
		"ALTER TABLE voyage_batches ADD COLUMN legacy numeric NOT NULL DEFAULT 0",
	} {
		if e := database.DB.Exec(sql).Error; e != nil {
			t.Fatal(e)
		}
	}
	for i := 0; i < 2; i++ {
		if e := database.InitializeVoyageBatches(); e != nil {
			t.Fatal(e)
		}
	}
	for table, col := range map[string]string{"voyage_issues": "shipping_target_id", "voyage_rosters": "legacy_issue_id", "voyage_batches": "legacy"} {
		if database.DB.Migrator().HasColumn(table, col) {
			t.Fatal("obsolete merge column survived", table, col)
		}
	}
	var saved database.VoyageRecipient
	database.DB.First(&saved, row.ID)
	if saved.CodeHash != row.CodeHash || saved.CodeEncrypted != row.CodeEncrypted || saved.IssueID != b.ID || batchDescription(saved) != batchDescription(row) {
		t.Fatal("batch credential or entitlement changed")
	}
	var address database.VoyageDelivery
	database.DB.First(&address, "recipient_id = ?", row.ID)
	if address.AddressEncrypted != d.AddressEncrypted {
		t.Fatal("batch address changed")
	}
	var savedBatch database.VoyageBatch
	database.DB.First(&savedBatch, b.ID)
	if savedBatch.Version != b.Version || len(savedBatch.Sources) != 1 {
		t.Fatal("independent batch changed")
	}
}

func TestVoyageBatchCredentialAndShipmentStayInTheirOwnScope(t *testing.T) {
	r := batchTestSetup(t)
	roster := database.VoyageRoster{Title: "8月", Month: "2026-08", Category: "monthly", Version: 1}
	database.DB.Create(&roster)
	for _, tier := range []string{"captain", "governor"} {
		database.DB.Create(&database.VoyageRosterMember{RosterID: roster.ID, UID: "12345", Tier: tier, Version: 1})
	}
	a, b := newTestBatch(t, r), newTestBatch(t, r)
	syncTestBatch(t, r, &a, []database.VoyageBatchSource{{RosterID: roster.ID, Tiers: []string{"captain"}}})
	syncTestBatch(t, r, &b, []database.VoyageBatchSource{{RosterID: roster.ID, Tiers: []string{"governor"}}})
	batchCall(t, r, "PUT", fmt.Sprintf("/batches/%d", b.ID), gin.H{"title": b.Title, "published": true, "version": b.Version}, 200)
	var first, second database.VoyageRecipient
	database.DB.Where("issue_id = ?", a.ID).First(&first)
	database.DB.Where("issue_id = ?", b.ID).First(&second)
	codeA, _ := secret.OpenVoyageCode(first.CodeEncrypted, recipientScope(a.ID, first.UID))
	codeB, _ := secret.OpenVoyageCode(second.CodeEncrypted, recipientScope(b.ID, second.UID))
	path := fmt.Sprintf("/issues/%d/claim", b.ID)
	batchCall(t, r, "POST", path, gin.H{"uid": "12345", "code": codeA, "action": "view"}, 401)
	batchCall(t, r, "POST", path, gin.H{"uid": "12345", "code": codeB, "action": "view"}, 200)
	if _, e := voyageRecipient(database.DB, b.ID, first.ID); e == nil {
		t.Fatal("cross-batch recipient accepted")
	}
	if _, e := saveShipment(database.DB, b.ID, shipmentRequest{ID: first.ID, Action: "draft", Courier: "测试", Tracking: "TEST12345"}); e == nil {
		t.Fatal("cross-batch shipment accepted")
	}
}
