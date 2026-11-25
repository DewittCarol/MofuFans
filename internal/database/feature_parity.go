package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// This explicit, offline migration never initializes users, themes or seed data.
// The caller supplies this site's config and takes an online SQLite backup first.
func MigrateFeatureParity() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		previous := DB
		DB = tx
		defer func() { DB = previous }()
		if tx.Migrator().HasColumn(&VoyageIssue{}, "shipping_target_id") {
			var count int64
			if err := tx.Table("voyage_issues").Where("shipping_target_id IS NOT NULL AND shipping_target_id <> 0").Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return errors.New("legacy merged shipping is in use; migrate its scopes explicitly before upgrading")
			}
		}
		if err := InitializeVoyageBatches(); err != nil {
			return err
		}
		if err := preserveLegacyGiftNotes(tx); err != nil {
			return err
		}
		if err := migrateLegacyCollectionScopes(tx); err != nil {
			return err
		}
		for _, step := range []func() error{InitializeBilibiliProfiles, InitializeDigitalGifts, InitializeBilibiliMessages, InitializeVoyageFulfillment, InitializeModeration} {
			if err := step(); err != nil {
				return err
			}
		}
		return MigrateVoyageCatalogs(tx)
	})
}

// The new editor no longer has separate kind/audience inputs. Keep their
// meaningful legacy text in the editable description rather than dropping it.
func preserveLegacyGiftNotes(tx *gorm.DB) error {
	var rows []struct {
		ID    uint
		Gifts string
	}
	if err := tx.Table("voyage_issues").Select("id, gifts").Where("kind IN ?", []string{"", "catalog"}).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		var gifts []map[string]any
		if row.Gifts == "" {
			continue
		}
		if err := json.Unmarshal([]byte(row.Gifts), &gifts); err != nil {
			return err
		}
		changed := false
		for _, gift := range gifts {
			kind, _ := gift["kind"].(string)
			notes := []string{}
			if label := map[string]string{"digital": "电子礼物", "mixed": "实物与电子礼物"}[kind]; label != "" {
				notes = append(notes, "原回馈类型："+label)
			}
			section, _ := gift["section"].(string)
			tiers, _ := gift["tiers"].([]any)
			if section == "guard" || section == "" && len(tiers) > 0 {
				if audience, ok := gift["audience"].(string); ok && strings.TrimSpace(audience) != "" {
					notes = append(notes, "适用范围："+audience)
				}
			}
			description, _ := gift["description"].(string)
			for _, note := range notes {
				if !strings.Contains(description, note) {
					description = strings.TrimSpace(description + "\n" + note)
					changed = true
				}
			}
			gift["description"] = description
		}
		if changed {
			raw, err := json.Marshal(gifts)
			if err != nil {
				return err
			}
			if err := tx.Table("voyage_issues").Where("id = ?", row.ID).UpdateColumn("gifts", string(raw)).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// Keep original issue/recipient IDs and encrypted credentials as collection scopes.
// Catalog copies can then be split into monthly cards without invalidating old codes.
func migrateLegacyCollectionScopes(tx *gorm.DB) error {
	var issues []VoyageIssue
	if err := tx.Where("kind <> ? AND id IN (SELECT DISTINCT issue_id FROM voyage_recipients)", "batch").Order("id").Find(&issues).Error; err != nil {
		return err
	}
	for _, issue := range issues {
		var exists int64
		if err := tx.Model(&VoyageBatch{}).Where("id = ?", issue.ID).Count(&exists).Error; err != nil {
			return err
		}
		if exists > 0 {
			return fmt.Errorf("collection scope %d already exists with a catalog kind", issue.ID)
		}
		var recipients []VoyageRecipient
		if err := tx.Where("issue_id = ?", issue.ID).Order("id").Find(&recipients).Error; err != nil {
			return err
		}
		copy := issue
		copy.ID, copy.Kind, copy.Version = 0, "catalog", 1
		if err := tx.Create(&copy).Error; err != nil {
			return err
		}
		category := "monthly"
		if _, err := time.Parse("2006-01", issue.Month); err != nil {
			category = "event"
		}
		for _, row := range recipients {
			if row.Tier == "" {
				category = "event"
			}
		}
		roster := VoyageRoster{Title: issue.Title, Category: category, Month: issue.Month, Version: 1}
		if err := tx.Create(&roster).Error; err != nil {
			return err
		}
		tiers := []string{}
		seen := map[string]bool{}
		for _, row := range recipients {
			member := VoyageRosterMember{RosterID: roster.ID, UID: row.UID, Name: row.Name, Tier: row.Tier, Disabled: row.Disabled, Version: 1}
			if err := tx.Create(&member).Error; err != nil {
				return err
			}
			if !seen[row.Tier] && row.Tier != "" {
				tiers = append(tiers, row.Tier)
				seen[row.Tier] = true
			}
			items, err := json.Marshal([]VoyageBatchItem{VoyageItem(roster, member)})
			if err != nil {
				return err
			}
			if err := tx.Model(&VoyageRecipient{}).Where("id = ?", row.ID).UpdateColumn("batch_items", string(items)).Error; err != nil {
				return err
			}
		}
		batch := VoyageBatch{ID: issue.ID, Title: issue.Title, Note: issue.ShippingNote, Published: issue.Published, Version: 1, Sources: []VoyageBatchSource{{RosterID: roster.ID, Tiers: tiers}}}
		if err := tx.Create(&batch).Error; err != nil {
			return err
		}
		if err := tx.Model(&VoyageIssue{}).Where("id = ?", issue.ID).UpdateColumn("kind", "batch").Error; err != nil {
			return err
		}
	}
	return nil
}
