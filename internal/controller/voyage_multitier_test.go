package controller

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"mofufans/internal/database"
	"strings"
	"testing"
)

func TestVoyageMultiTierRosterAndSeparateShipments(t *testing.T) {
	r := batchTestSetup(t)
	r.PUT("/rosters/:roster/members/:member", (&VoyageRostersController{}).Update)
	roster := database.VoyageRoster{Title: "8月", Category: "monthly", Month: "2026-08", Version: 1}
	database.DB.Create(&roster)
	path := fmt.Sprintf("/rosters/%d/members/import", roster.ID)
	res := batchCall(t, r, "POST", path, gin.H{"text": "12345 舰长\n12345 提督\n12345 总督\n12345 舰长", "version": 1}, 200)
	var result struct {
		Added   int
		Skipped int
	}
	json.Unmarshal(res.Data, &result)
	if result.Added != 3 || result.Skipped != 1 {
		t.Fatalf("multi-tier import: %+v", result)
	}
	res = batchCall(t, r, "POST", path, gin.H{"text": "12345 舰长\n12345 提督\n12345 总督", "version": 2}, 200)
	json.Unmarshal(res.Data, &result)
	if result.Added != 0 || result.Skipped != 3 {
		t.Fatal("same tier import was not idempotent")
	}
	first := newTestBatch(t, r)
	p := syncTestBatch(t, r, &first, []database.VoyageBatchSource{{RosterID: roster.ID, Tiers: []string{"captain", "admiral"}}})
	if p.Total != 1 || len(p.Rows) != 1 || len(p.Rows[0].Items) != 2 {
		t.Fatal("same UID should get one address with two monthly tier labels")
	}
	var recipient database.VoyageRecipient
	database.DB.Where("issue_id = ?", first.ID).First(&recipient)
	if !strings.Contains(batchDescription(recipient), "舰长礼物") || !strings.Contains(batchDescription(recipient), "提督礼物") || strings.Contains(batchDescription(recipient), "总督礼物") {
		t.Fatal("wrong first batch labels")
	}
	second := newTestBatch(t, r)
	q := syncTestBatch(t, r, &second, []database.VoyageBatchSource{{RosterID: roster.ID, Tiers: []string{"governor"}}})
	if q.Total != 1 || len(q.Rows[0].Items) != 1 || !strings.Contains(q.Rows[0].Items[0].Label, "总督礼物") {
		t.Fatal("governor entitlement cannot be selected independently")
	}
	duplicate := newTestBatch(t, r)
	q = syncTestBatch(t, r, &duplicate, []database.VoyageBatchSource{{RosterID: roster.ID, Tiers: []string{"captain", "admiral", "governor"}}})
	if q.Total != 0 || len(q.Warnings) != 3 {
		t.Fatal("a tier entitlement was allocated twice")
	}
	var member database.VoyageRosterMember
	database.DB.Where("roster_id = ? AND tier = ?", roster.ID, "captain").First(&member)
	batchCall(t, r, "PUT", fmt.Sprintf("/rosters/%d/members/%d", roster.ID, member.ID), gin.H{"name": "同一用户", "tier": "admiral", "version": member.Version}, 409)
	var after database.VoyageRosterMember
	database.DB.First(&after, member.ID)
	if after.Tier != "captain" {
		t.Fatal("duplicate edit overwrote an entitlement")
	}
}

func TestVoyageMultiTierMigrationPreservesMemberIdentity(t *testing.T) {
	batchTestSetup(t)
	roster := database.VoyageRoster{Title: "8月", Category: "monthly", Month: "2026-08", Version: 1}
	database.DB.Create(&roster)
	member := database.VoyageRosterMember{RosterID: roster.ID, UID: "12345", Tier: "captain", Version: 1}
	database.DB.Create(&member)
	if e := database.DB.Exec("CREATE UNIQUE INDEX roster_uid ON voyage_roster_members(roster_id, uid)").Error; e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if e := database.InitializeVoyageBatches(); e != nil {
			t.Fatal(e)
		}
	}
	if database.DB.Migrator().HasIndex(&database.VoyageRosterMember{}, "roster_uid") {
		t.Fatal("old uniqueness restriction survived migration")
	}
	var preserved database.VoyageRosterMember
	database.DB.First(&preserved, member.ID)
	if preserved.UID != member.UID || preserved.Tier != member.Tier || !preserved.CreatedAt.Equal(member.CreatedAt) {
		t.Fatal("migration changed original membership")
	}
	if e := database.DB.Create(&database.VoyageRosterMember{RosterID: roster.ID, UID: member.UID, Tier: "admiral", Version: 1}).Error; e != nil {
		t.Fatal("migration did not allow a second tier", e)
	}
	if e := database.DB.Create(&database.VoyageRosterMember{RosterID: roster.ID, UID: member.UID, Tier: "captain", Version: 1}).Error; e == nil {
		t.Fatal("duplicate tier uniqueness lost")
	}
}
