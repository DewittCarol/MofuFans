package controller

import (
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"
	"mofufans/internal/database"
)

type rosterSyncResult struct {
	Collections int `json:"collections"`
	Parcels     int `json:"parcels"`
	Cancelled   int `json:"cancelled"`
	Shipped     int `json:"shipped"`
}

func rosterItemIdentity(uid string, item database.VoyageBatchItem) string {
	return fmt.Sprintf("%s/%d/%s", uid, item.RosterID, item.Tier)
}

// The caller holds the voyage write lock. Membership, collections and unshipped
// parcels change atomically; existing credentials and address records survive.
func syncVoyageRoster(tx *gorm.DB, roster database.VoyageRoster) (rosterSyncResult, error) {
	result := rosterSyncResult{}
	if err := syncDigitalGiftsForRoster(tx, roster.ID); err != nil {
		return result, err
	}
	var batches []database.VoyageBatch
	if err := tx.Order("id").Find(&batches).Error; err != nil {
		return result, err
	}
	affected := map[uint]database.VoyageBatchSource{}
	for _, b := range batches {
		for _, source := range b.Sources {
			if source.RosterID == roster.ID {
				affected[b.ID] = source
			}
		}
	}
	if len(affected) == 0 {
		return result, nil
	}
	var members []database.VoyageRosterMember
	if err := tx.Where("roster_id = ? AND disabled = ?", roster.ID, false).Find(&members).Error; err != nil {
		return result, err
	}
	byKey := map[string]database.VoyageRosterMember{}
	for _, m := range members {
		byKey[strconv.FormatUint(uint64(m.ID), 10)] = m
	}
	valid := func(collection uint, uid string, item database.VoyageBatchItem) (database.VoyageRosterMember, bool) {
		m, ok := byKey[item.Key]
		source, bound := affected[collection]
		if !ok || !bound || m.UID != uid || m.Tier != item.Tier {
			return m, false
		}
		if roster.Category == "event" {
			return m, true
		}
		for _, tier := range source.Tiers {
			if tier != "" && tier == m.Tier {
				return m, true
			}
		}
		return m, false
	}
	// Shipped items are history. Unshipped parcels must stop reserving removed
	// items before collections are recalculated (including a change of tier).
	shipped := map[string]bool{}
	changedParcels := map[uint]bool{}
	if tx.Migrator().HasTable(&database.VoyageParcel{}) {
		var parcels []database.VoyageParcel
		if err := tx.Where("cancelled_at IS NULL").Order("id").Find(&parcels).Error; err != nil {
			return result, err
		}
		for _, p := range parcels {
			items := []database.VoyageParcelItem{}
			changed, protected := false, false
			for _, item := range p.Items {
				if p.ShippedAt != nil {
					shipped[fmt.Sprintf("%d/%s", item.RecipientID, item.Key)] = true
					if item.RosterID == roster.ID {
						if _, ok := valid(item.CollectionID, p.UID, item.VoyageBatchItem); !ok {
							protected = true
						}
					}
					continue
				}
				if item.RosterID != roster.ID {
					items = append(items, item)
					continue
				}
				m, ok := valid(item.CollectionID, p.UID, item.VoyageBatchItem)
				if !ok {
					changed = true
					continue
				}
				next := database.VoyageItem(roster, m)
				if next != item.VoyageBatchItem {
					item.VoyageBatchItem = next
					changed = true
				}
				items = append(items, item)
			}
			if protected {
				result.Shipped++
			}
			if !changed {
				continue
			}
			changedParcels[p.ID] = true
			p.Version++
			p.ExportedAt = nil
			if len(items) == 0 {
				now := time.Now()
				p.CancelledAt = &now
				// Retain the cancelled parcel's old contents for an understandable history.
				result.Cancelled++
			} else {
				p.Items = items
				addressStillIncluded := false
				for _, item := range items {
					if item.RecipientID == p.AddressRecipientID {
						addressStillIncluded = true
					}
				}
				if p.AddressEncrypted != "" && !addressStillIncluded {
					p.AddressEncrypted = ""
					p.AddressRecipientID = 0
					p.AddressVersion = 0
					p.ConfirmedAt = nil
					view, err := parcelDetails(tx, p)
					if err != nil {
						return result, err
					}
					if address, ok := singleParcelAddress(view.Choices); ok {
						if err := applyParcelAddress(tx, &p, view.Choices, address.RecipientID, address.Version, time.Now()); err != nil {
							return result, err
						}
					}
				}
				result.Parcels++
			}
			if err := tx.Save(&p).Error; err != nil {
				return result, err
			}
		}
	}
	// Release obsolete allocations in ALL affected collections first. Otherwise
	// a record moving to a different tier could be blocked by a stale owner.
	var recipients []database.VoyageRecipient
	if err := tx.Order("id").Find(&recipients).Error; err != nil {
		return result, err
	}
	changedBatches := map[uint]bool{}
	for _, r := range recipients {
		if _, ok := affected[r.IssueID]; !ok {
			continue
		}
		items := []database.VoyageBatchItem{}
		for _, item := range r.BatchItems {
			_, keep := valid(r.IssueID, r.UID, item)
			if item.RosterID != roster.ID || keep || shipped[fmt.Sprintf("%d/%s", r.ID, item.Key)] {
				items = append(items, item)
			}
		}
		if len(items) == len(r.BatchItems) {
			continue
		}
		r.BatchItems = items
		r.Version++
		r.PreparedAt = nil
		r.SentAt = nil
		if len(items) == 0 {
			r.Disabled = true
		}
		if err := tx.Save(&r).Error; err != nil {
			return result, err
		}
		changedBatches[r.IssueID] = true
	}
	for _, b := range batches {
		if _, ok := affected[b.ID]; !ok {
			continue
		}
		preview, err := batchPlan(tx, b, b.Sources)
		if err != nil {
			return result, err
		}
		if preview.Added == 0 && preview.Changed == 0 && preview.Removed == 0 && !changedBatches[b.ID] {
			continue
		}
		if err = applyBatchPreview(tx, b.ID, preview); err != nil {
			return result, err
		}
		if err = tx.Model(&b).Update("version", gorm.Expr("version + 1")).Error; err != nil {
			return result, err
		}
		result.Collections++
	}
	// Refresh display names on live parcels as well, without touching shipped
	// snapshots or asking the operator to save the same correction twice.
	if tx.Migrator().HasTable(&database.VoyageParcel{}) {
		var people []database.VoyageRecipient
		if err := tx.Find(&people).Error; err != nil {
			return result, err
		}
		names := map[uint]string{}
		for _, r := range people {
			names[r.ID] = r.Name
		}
		var parcels []database.VoyageParcel
		if err := tx.Omit("address_encrypted").Where("cancelled_at IS NULL AND shipped_at IS NULL").Find(&parcels).Error; err != nil {
			return result, err
		}
		for _, p := range parcels {
			relevant, name := changedParcels[p.ID], ""
			for _, item := range p.Items {
				if item.RosterID == roster.ID {
					relevant = true
				}
				if name == "" {
					name = names[item.RecipientID]
				}
			}
			if !relevant || name == p.Name {
				continue
			}
			if err := tx.Model(&p).Updates(map[string]any{"name": name, "version": gorm.Expr("version + 1"), "exported_at": nil}).Error; err != nil {
				return result, err
			}
			if !changedParcels[p.ID] {
				result.Parcels++
			}
		}
	}
	return result, nil
}
