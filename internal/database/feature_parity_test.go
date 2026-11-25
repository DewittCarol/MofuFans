package database

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestFeatureParityPreservesLegacyCollectionAndIsIdempotent(t *testing.T) {
	db := catalogTestDB(t)
	previous := DB
	DB = db
	defer func() { DB = previous }()
	viper.Set("session_secret", "feature-parity-test-session-secret-32-bytes")
	defer viper.Reset()
	if err := db.AutoMigrate(&Config{}, &VoyageRecipient{}, &VoyageDispatchConfig{}, &VoyageDelivery{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Config{Announcement: "keep announcement", SpamPrompt: "keep prompt"}).Error; err != nil {
		t.Fatal(err)
	}
	issue := VoyageIssue{Kind: "catalog", Title: "旧舰礼", Month: "2026-09", Published: true, Version: 7, Gifts: []VoyageGift{{Name: "原礼物", Section: "guard", Tiers: []string{"captain"}}}}
	if err := db.Create(&issue).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	r := VoyageRecipient{IssueID: issue.ID, UID: "123456", Name: "旧成员", Tier: "captain", CodeEncrypted: "preserve-ciphertext", CodeHash: "preserve-hash", Version: 4, SentAt: &now}
	if err := db.Create(&r).Error; err != nil {
		t.Fatal(err)
	}
	d := VoyageDelivery{RecipientID: r.ID, AddressEncrypted: "preserve-address", Tracking: "keep-tracking", ShippedAt: &now, SubmittedAt: &now, Version: 2}
	if err := db.Create(&d).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateFeatureParity(); err != nil {
		t.Fatal(err)
	}
	var got VoyageRecipient
	db.First(&got, r.ID)
	if got.IssueID != r.IssueID || got.CodeEncrypted != r.CodeEncrypted || got.CodeHash != r.CodeHash || got.Version != r.Version || got.SentAt == nil || !got.SentAt.Equal(now) || len(got.BatchItems) != 1 {
		t.Fatalf("legacy identity or credentials changed: id=%d", got.ID)
	}
	var batch VoyageBatch
	if err := db.First(&batch, issue.ID).Error; err != nil || len(batch.Sources) != 1 {
		t.Fatal("legacy batch missing", err)
	}
	var parcel VoyageParcel
	if err := db.Where("legacy_recipient_id = ?", r.ID).First(&parcel).Error; err != nil || parcel.AddressEncrypted != d.AddressEncrypted || parcel.Tracking != d.Tracking || len(parcel.Items) != 1 {
		t.Fatal("shipment lost", err)
	}
	var cards []VoyageIssue
	db.Where("kind = ?", "guard").Find(&cards)
	if len(cards) != 1 || cards[0].ID == issue.ID || cards[0].Gifts[0].Name != "原礼物" {
		t.Fatal("catalog not preserved")
	}
	snapshot := func() string {
		out := map[string]any{}
		for _, table := range []string{"configs", "voyage_issues", "voyage_recipients", "voyage_rosters", "voyage_roster_members", "voyage_batches", "voyage_parcels"} {
			var rows []map[string]any
			db.Table(table).Order("id").Find(&rows)
			out[table] = rows
		}
		raw, _ := json.Marshal(out)
		return string(raw)
	}
	before := snapshot()
	if err := MigrateFeatureParity(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("rerun changed migrated data")
	}
}

func TestFeatureParityRejectsUsedLegacyMergeBeforeMutation(t *testing.T) {
	db := catalogTestDB(t)
	previous := DB
	DB = db
	defer func() { DB = previous }()
	if err := db.Exec("ALTER TABLE voyage_issues ADD COLUMN shipping_target_id integer").Error; err != nil {
		t.Fatal(err)
	}
	issue := VoyageIssue{Title: "merged", Month: "2026-09"}
	db.Create(&issue)
	db.Table("voyage_issues").Where("id = ?", issue.ID).Update("shipping_target_id", 42)
	if err := MigrateFeatureParity(); err == nil {
		t.Fatal("must reject active merge")
	}
	if db.Migrator().HasTable(&VoyageRoster{}) {
		t.Fatal("rejected migration mutated schema")
	}
}

func TestFeatureParityPreservesLegacyGiftNotes(t *testing.T) {
	db := catalogTestDB(t)
	issue := VoyageIssue{Kind: "catalog", Title: "legacy", Month: "2026-09"}
	if err := db.Create(&issue).Error; err != nil {
		t.Fatal(err)
	}
	raw := `[{"name":"gift","kind":"mixed","section":"guard","tiers":["captain"],"audience":"旧领取说明","description":"原介绍"}]`
	if err := db.Model(&issue).UpdateColumn("gifts", raw).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := preserveLegacyGiftNotes(db); err != nil {
			t.Fatal(err)
		}
	}
	var got VoyageIssue
	if err := db.First(&got, issue.ID).Error; err != nil {
		t.Fatal(err)
	}
	want := "原介绍\n原回馈类型：实物与电子礼物\n适用范围：旧领取说明"
	if len(got.Gifts) != 1 || got.Gifts[0].Description != want {
		t.Fatalf("legacy notes lost or duplicated: %#v", got.Gifts)
	}
}
