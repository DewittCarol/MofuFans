package controller

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"mofufans/internal/database"
	"mofufans/internal/secret"
)

const longTermAddressSource = "long_term"

type longTermAddressView struct {
	Enabled        bool       `json:"enabled"`
	Version        int        `json:"version"`
	MatchesAddress bool       `json:"matches_address"`
	UpdatedAt      *time.Time `json:"updated_at"`
	EntryID        uint       `json:"entry_id"`
	Source         string     `json:"source"`
}

func longTermAddressScope(uid string) string { return "long-term-address:" + uid }

func readLongTermAddress(tx *gorm.DB, uid string) (database.VoyageLongTermAddress, error) {
	var p database.VoyageLongTermAddress
	// Old isolated fixtures and staged upgrades can still read ordinary addresses.
	if !tx.Migrator().HasTable(&p) {
		return p, nil
	}
	err := tx.Where("uid = ?", uid).Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
	}
	return p, err
}

func openLongTermAddress(p database.VoyageLongTermAddress) (VoyageAddress, error) {
	var a VoyageAddress
	raw, err := secret.OpenVoyageAddress(longTermAddressScope(p.UID), p.AddressEncrypted)
	if err != nil {
		return a, err
	}
	err = json.Unmarshal(raw, &a)
	if err == nil && !addressValid(&a) {
		err = errors.New("长期地址记录无效，请联系维护人员")
	}
	a.Note = "" // Gift-specific notes must never silently follow future gifts.
	return a, err
}

func longTermState(tx *gorm.DB, uid string, current *VoyageAddress) (*longTermAddressView, error) {
	p, err := readLongTermAddress(tx, uid)
	if err != nil {
		return nil, err
	}
	v := &longTermAddressView{Enabled: p.Enabled, Version: p.Version, EntryID: p.EntryID, Source: p.Source}
	if p.Version != 0 {
		v.UpdatedAt = &p.UpdatedAt
	}
	if p.Enabled {
		a, err := openLongTermAddress(p)
		if err != nil {
			return nil, err
		}
		v.MatchesAddress = sameVoyageAddress(&a, current, false)
	}
	return v, nil
}

// Called after current-collection credential verification, under the voyage
// writer lock. An omitted action from an older client preserves the preference.
func saveLongTermAddress(tx *gorm.DB, uid, action string, version int, address *VoyageAddress) error {
	if action == "" || action == "keep" {
		return nil
	}
	if action != "set" && action != "clear" {
		return errors.New("长期地址操作无效")
	}
	if !tx.Migrator().HasTable(&database.VoyageLongTermAddress{}) {
		return errors.New("长期地址功能尚未初始化，请稍后再试")
	}
	p, err := readLongTermAddress(tx, uid)
	if err != nil {
		return err
	}
	if p.Version != version {
		return errDeliveryConflict
	}
	if action == "clear" && !p.Enabled {
		return nil
	}
	sealed := ""
	if action == "set" {
		if !addressValid(address) {
			return errors.New("请填写收件人、有效手机号和完整收货地址")
		}
		a := *address
		a.Note = ""
		// A confirmation of the same address doesn't obsolete other snapshots.
		if p.Enabled {
			old, err := openLongTermAddress(p)
			if err != nil {
				return err
			}
			if sameVoyageAddress(&old, &a, false) {
				return nil
			}
		}
		raw, _ := json.Marshal(a)
		sealed, err = secret.SealVoyageAddress(longTermAddressScope(uid), raw)
		if err != nil {
			return err
		}
	}
	now := time.Now()
	if p.Version == 0 {
		return tx.Create(&database.VoyageLongTermAddress{UID: uid, AddressEncrypted: sealed, Enabled: action == "set", Version: 1, UpdatedAt: now}).Error
	}
	result := tx.Model(&database.VoyageLongTermAddress{}).Where("uid = ? AND version = ?", uid, version).Updates(map[string]any{
		"address_encrypted": sealed, "enabled": action == "set", "version": version + 1, "updated_at": now,
	})
	if result.Error == nil && result.RowsAffected != 1 {
		return errDeliveryConflict
	}
	return result.Error
}

// Materialize a per-collection snapshot, so exports, counts and parcel creation
// share exactly the same address. Never overwrite a registered or shipped row.
// Caller holds lockVoyageBatchWrites; all consent changes use that same lock.
func inheritLongTermAddresses(tx *gorm.DB, issueID uint, uid string) error {
	if !tx.Migrator().HasTable(&database.VoyageLongTermAddress{}) || !tx.Migrator().HasTable(&database.VoyageBatch{}) {
		return nil
	}
	today := time.Now().In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
	q := tx.Model(&database.VoyageRecipient{}).Select("voyage_recipients.*").
		Joins("JOIN voyage_batches b ON b.id = voyage_recipients.issue_id").
		Joins("JOIN voyage_long_term_addresses p ON p.uid = voyage_recipients.uid AND p.enabled = ?", true).
		Joins("LEFT JOIN voyage_deliveries d ON d.recipient_id = voyage_recipients.id").
		Where("voyage_recipients.disabled = ? AND b.published = ? AND b.closed = ?", false, true, false).
		Where("(b.opens = '' OR b.opens <= ?) AND (b.closes = '' OR b.closes >= ?)", today, today).
		Where("(d.recipient_id IS NULL OR d.address_encrypted = '') AND d.shipped_at IS NULL")
	if issueID != 0 {
		q = q.Where("voyage_recipients.issue_id = ?", issueID)
	}
	if uid != "" {
		q = q.Where("voyage_recipients.uid = ?", uid)
	}
	var rows []database.VoyageRecipient
	if err := q.Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if len(row.BatchItems) == 0 {
			continue
		}
		p, err := readLongTermAddress(tx, row.UID)
		if err != nil {
			return err
		}
		if !p.Enabled {
			continue
		}
		a, err := openLongTermAddress(p)
		if err != nil {
			return err
		}
		raw, _ := json.Marshal(a)
		sealed, err := secret.SealVoyageAddress(recipientScope(row.IssueID, row.UID), raw)
		if err != nil {
			return err
		}
		var d database.VoyageDelivery
		err = tx.Where("recipient_id = ?", row.ID).Take(&d).Error
		now := time.Now()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = tx.Create(&database.VoyageDelivery{RecipientID: row.ID, AddressEncrypted: sealed, SubmittedAt: &now, Version: 1,
				AddressSource: longTermAddressSource, LongTermVersion: p.Version, LongTermSavedAt: &p.UpdatedAt, LongTermOrigin: p.Source}).Error
		} else if err == nil {
			err = tx.Model(&database.VoyageDelivery{}).Where("recipient_id = ? AND version = ? AND address_encrypted = '' AND shipped_at IS NULL", row.ID, d.Version).
				Updates(map[string]any{"address_encrypted": sealed, "submitted_at": now, "version": d.Version + 1,
					"address_source": longTermAddressSource, "long_term_version": p.Version, "long_term_saved_at": p.UpdatedAt, "long_term_origin": p.Source}).Error
		}
		if err != nil {
			return err
		}
		if _, err = rememberVoyageAddress(tx, row.UID, row.IssueID, row.ID, &a, "long_term"); err != nil {
			return err
		}
	}
	return nil
}

func syncLongTermAddresses(db *gorm.DB, issueID uint) error {
	if !db.Migrator().HasTable(&database.VoyageLongTermAddress{}) {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := lockVoyageBatchWrites(tx); err != nil {
			return err
		}
		return inheritLongTermAddresses(tx, issueID, "")
	})
}
