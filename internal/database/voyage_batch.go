package database

import (
	"fmt"
	"gorm.io/gorm/clause"
	"time"
)

// Rosters are independent of both the public gift catalog and shipping batches.
type VoyageRoster struct {
	BaseModel
	Title    string `gorm:"size:160" json:"title"`
	Category string `gorm:"size:16;index" json:"category"`
	Month    string `gorm:"size:7;index" json:"month"`
	Version  int    `gorm:"not null;default:1" json:"version"`
}
type VoyageRosterMember struct {
	BaseModel
	RosterID uint   `gorm:"not null;uniqueIndex:roster_uid_tier" json:"roster_id"`
	UID      string `gorm:"size:20;not null;uniqueIndex:roster_uid_tier" json:"uid"`
	Name     string `gorm:"size:160" json:"name"`
	Tier     string `gorm:"size:16;not null;default:'';uniqueIndex:roster_uid_tier" json:"tier"`
	Disabled bool   `gorm:"not null;default:false" json:"disabled"`
	Version  int    `gorm:"not null;default:1" json:"version"`
}
type VoyageBatchSource struct {
	RosterID uint     `json:"roster_id"`
	Tiers    []string `json:"tiers"`
}
type VoyageBatchItem struct {
	Key      string `json:"key"`
	RosterID uint   `json:"roster_id"`
	Tier     string `json:"tier"`
	Label    string `json:"label"`
}

// A single transaction-write lock serializes scope edits and shipment changes
// across processes on SQLite and MySQL. It must precede any business reads.
type VoyageBatchSyncLock struct {
	ID      uint   `gorm:"primaryKey;autoIncrement:false"`
	Version uint64 `gorm:"not null;default:1"`
}

// ID refers to the credential/delivery scope in VoyageIssue. New scopes have
// Kind=batch and never appear in the gift catalog.
type VoyageBatch struct {
	ID        uint                `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Title     string              `gorm:"size:160" json:"title"`
	Note      string              `gorm:"type:text" json:"note"`
	Published bool                `gorm:"not null;default:false" json:"published"`
	Closed    bool                `gorm:"not null;default:false" json:"closed"`
	Opens     string              `gorm:"size:10" json:"opens"`
	Closes    string              `gorm:"size:10" json:"closes"`
	Sources   []VoyageBatchSource `gorm:"serializer:json;type:text" json:"sources"`
	Version   int                 `gorm:"not null;default:1" json:"version"`
}

func VoyageItem(roster VoyageRoster, member VoyageRosterMember) VoyageBatchItem {
	label := roster.Title
	if roster.Category == "monthly" {
		tier := map[string]string{"captain": "舰长", "admiral": "提督", "governor": "总督"}[member.Tier]
		if tier == "" {
			tier = "待核对档位"
		}
		month := roster.Month
		if date, e := time.Parse("2006-01", month); e == nil {
			month = date.Format("2006年1月")
		}
		label = month + tier + "礼物"
	}
	return VoyageBatchItem{Key: fmt.Sprintf("%d", member.ID), RosterID: roster.ID, Tier: member.Tier, Label: label}
}

func InitializeVoyageBatches() error {
	// Older lists allowed only one tier per UID. Preserve all existing member IDs
	// and frozen batch keys while allowing an independent row for each new tier.
	if DB.Migrator().HasTable(&VoyageRosterMember{}) {
		if err := DB.Model(&VoyageRosterMember{}).Where("tier IS NULL").UpdateColumn("tier", "").Error; err != nil {
			return err
		}
	}
	if err := DB.AutoMigrate(&VoyageIssue{}, &VoyageRecipient{}, &VoyageRoster{}, &VoyageRosterMember{}, &VoyageBatch{}, &VoyageBatchSyncLock{}); err != nil {
		return err
	}
	if DB.Migrator().HasIndex(&VoyageRosterMember{}, "roster_uid") {
		if err := DB.Migrator().DropIndex(&VoyageRosterMember{}, "roster_uid"); err != nil {
			return err
		}
	}
	// The unused issue-merge feature was retired. Remove its schema, not data.
	for _, obsolete := range []struct {
		model                any
		table, column, index string
	}{
		{&VoyageIssue{}, "voyage_issues", "shipping_target_id", "idx_voyage_issues_shipping_target_id"},
		{&VoyageRoster{}, "voyage_rosters", "legacy_issue_id", "idx_voyage_rosters_legacy_issue_id"},
		{&VoyageBatch{}, "voyage_batches", "legacy", ""},
	} {
		if obsolete.index != "" && DB.Migrator().HasIndex(obsolete.model, obsolete.index) {
			if e := DB.Migrator().DropIndex(obsolete.model, obsolete.index); e != nil {
				return e
			}
		}
		if DB.Migrator().HasColumn(obsolete.model, obsolete.column) {
			var e error
			if DB.Dialector.Name() == "sqlite" {
				// Native DROP COLUMN preserves all other indexes. The bundled older GORM
				// SQLite table rewriter cannot remove a final column reliably.
				e = DB.Exec("ALTER TABLE `" + obsolete.table + "` DROP COLUMN `" + obsolete.column + "`").Error
			} else {
				e = DB.Migrator().DropColumn(obsolete.model, obsolete.column)
			}
			if e != nil {
				return e
			}
		}
	}
	return DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&VoyageBatchSyncLock{ID: 1, Version: 1}).Error
}
