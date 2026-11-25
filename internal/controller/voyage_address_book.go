package controller

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mofufans/internal/database"
	"mofufans/internal/secret"
)

type addressBookUseView struct {
	IssueID    uint      `json:"issue_id"`
	Title      string    `json:"title"`
	Note       string    `json:"note,omitempty"`
	Source     string    `json:"source"`
	RecordedAt time.Time `json:"recorded_at"`
}
type addressBookEntryView struct {
	ID         uint                 `json:"id"`
	Address    VoyageAddress        `json:"address"`
	IsDefault  bool                 `json:"is_default"`
	CreatedAt  time.Time            `json:"created_at"`
	LastUsedAt time.Time            `json:"last_used_at"`
	Uses       []addressBookUseView `json:"uses"`
}

func addressBookAvailable(tx *gorm.DB) bool {
	return tx.Migrator().HasTable(&database.VoyageAddressBookEntry{})
}
func bookScope(uid string) string     { return "address-book:" + uid }
func bookNoteScope(uid string) string { return "address-book-note:" + uid }

func openBookEntry(row database.VoyageAddressBookEntry) (VoyageAddress, error) {
	var address VoyageAddress
	raw, err := secret.OpenVoyageAddress(bookScope(row.UID), row.AddressEncrypted)
	if err == nil {
		err = json.Unmarshal(raw, &address)
	}
	if err == nil && !addressValid(&address) {
		err = errors.New("地址记录无效，请联系维护人员")
	}
	address.Note = ""
	return address, err
}

// All callers hold the existing voyage writer lock. Equal addresses share one
// entry; notes and source batches remain separate history records.
func rememberVoyageAddress(tx *gorm.DB, uid string, issueID, recipientID uint, address *VoyageAddress, source string) (database.VoyageAddressBookEntry, error) {
	var entry database.VoyageAddressBookEntry
	if !addressBookAvailable(tx) || address == nil {
		return entry, nil
	}
	a := *address
	if !addressValid(&a) {
		return entry, errors.New("请填写收件人、有效手机号和完整收货地址")
	}
	var entries []database.VoyageAddressBookEntry
	if err := tx.Where("uid = ?", uid).Order("id").Find(&entries).Error; err != nil {
		return entry, err
	}
	for _, old := range entries {
		value, err := openBookEntry(old)
		if err != nil {
			return entry, err
		}
		if sameVoyageAddress(&value, &a, false) {
			entry = old
			break
		}
	}
	now := time.Now()
	if entry.ID == 0 {
		value := a
		value.Note = ""
		raw, _ := json.Marshal(value)
		sealed, err := secret.SealVoyageAddress(bookScope(uid), raw)
		if err != nil {
			return entry, err
		}
		entry = database.VoyageAddressBookEntry{UID: uid, AddressEncrypted: sealed, CreatedAt: now, LastUsedAt: now}
		if err = tx.Create(&entry).Error; err != nil {
			return entry, err
		}
	}
	if issueID == 0 || recipientID == 0 {
		return entry, nil
	}
	var last database.VoyageAddressBookUse
	err := tx.Where("entry_id = ? AND recipient_id = ?", entry.ID, recipientID).Order("id desc").Take(&last).Error
	if err == nil {
		note, e := secret.OpenVoyageAddress(bookNoteScope(uid), last.NoteEncrypted)
		if e != nil {
			return entry, e
		}
		if string(note) == a.Note && last.Source == source {
			return entry, nil
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return entry, err
	}
	sealed, err := secret.SealVoyageAddress(bookNoteScope(uid), []byte(a.Note))
	if err != nil {
		return entry, err
	}
	use := database.VoyageAddressBookUse{EntryID: entry.ID, RecipientID: recipientID, IssueID: issueID, NoteEncrypted: sealed, Source: source, RecordedAt: now}
	if err = tx.Create(&use).Error; err != nil {
		return entry, err
	}
	err = tx.Model(&entry).Update("last_used_at", now).Error
	return entry, err
}

func readAddressBook(tx *gorm.DB, uid string) ([]addressBookEntryView, error) {
	result := []addressBookEntryView{}
	if !addressBookAvailable(tx) {
		return result, nil
	}
	var entries []database.VoyageAddressBookEntry
	if err := tx.Where("uid = ?", uid).Order("last_used_at desc, id desc").Find(&entries).Error; err != nil {
		return result, err
	}
	p, err := readLongTermAddress(tx, uid)
	if err != nil {
		return result, err
	}
	var preferred *VoyageAddress
	if p.Enabled {
		a, e := openLongTermAddress(p)
		if e != nil {
			return result, e
		}
		preferred = &a
	}
	for _, entry := range entries {
		a, err := openBookEntry(entry)
		if err != nil {
			return result, err
		}
		view := addressBookEntryView{ID: entry.ID, Address: a, IsDefault: preferred != nil && sameVoyageAddress(preferred, &a, false), CreatedAt: entry.CreatedAt, LastUsedAt: entry.LastUsedAt, Uses: []addressBookUseView{}}
		var uses []database.VoyageAddressBookUse
		if err = tx.Where("entry_id = ?", entry.ID).Order("id desc").Find(&uses).Error; err != nil {
			return result, err
		}
		for _, use := range uses {
			note, err := secret.OpenVoyageAddress(bookNoteScope(uid), use.NoteEncrypted)
			if err != nil {
				return result, err
			}
			var issue database.VoyageIssue
			if err = tx.Select("id", "title").Where("id = ?", use.IssueID).Take(&issue).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return result, err
			}
			view.Uses = append(view.Uses, addressBookUseView{IssueID: use.IssueID, Title: issue.Title, Note: string(note), Source: use.Source, RecordedAt: use.RecordedAt})
		}
		result = append(result, view)
	}
	return result, nil
}

func ownedBookAddress(tx *gorm.DB, uid string, id uint) (VoyageAddress, error) {
	var entry database.VoyageAddressBookEntry
	if !addressBookAvailable(tx) || id == 0 {
		return VoyageAddress{}, errBookEntry
	}
	if err := tx.Where("id = ? AND uid = ?", id, uid).Take(&entry).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return VoyageAddress{}, errBookEntry
		}
		return VoyageAddress{}, err
	}
	return openBookEntry(entry)
}

// Consent remains one row per UID. Source and audit are updated in the same
// transaction as the permission; a user revocation stops administrator custody.
func changeAddressPreference(tx *gorm.DB, uid, action string, version int, address *VoyageAddress, issueID, recipientID uint, source string, adminID uint, confirmedAt *time.Time, channel string) error {
	before, err := readLongTermAddress(tx, uid)
	if err != nil {
		return err
	}
	if err = saveLongTermAddress(tx, uid, action, version, address); err != nil {
		return err
	}
	if action == "" || action == "keep" || !addressBookAvailable(tx) {
		return nil
	}
	after, err := readLongTermAddress(tx, uid)
	if err != nil {
		return err
	}
	if after.Version == before.Version {
		return nil
	}
	entryID := before.EntryID
	if action == "set" {
		entry, e := rememberVoyageAddress(tx, uid, issueID, recipientID, address, source)
		if e != nil {
			return e
		}
		entryID = entry.ID
	}
	if err = tx.Model(&database.VoyageLongTermAddress{}).Where("uid = ? AND version = ?", uid, after.Version).Updates(map[string]any{
		"entry_id": entryID, "source": source, "admin_id": adminID, "confirmed_at": confirmedAt, "confirmed_via": channel,
	}).Error; err != nil {
		return err
	}
	return tx.Create(&database.VoyageAddressConsent{UID: uid, EntryID: entryID, RecipientID: recipientID, Action: source + "_" + action, PreferenceVersion: after.Version, AdminID: adminID, Channel: channel, ConfirmedAt: confirmedAt, OccurredAt: time.Now()}).Error
}

var errBookEntry = errors.New("地址记录不可用，请刷新后重试")
var errBookConfirmation = errors.New("请确认已与用户核实，并填写本次确认的时间和渠道")
var errBookOccupied = errors.New("本批次已有地址或已发货，请核对现有记录")

type VoyageAddressBookController struct{}

func validBookUID(uid string) bool {
	id, err := strconv.ParseInt(uid, 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == uid
}

// Admin reads are deliberately independent of the auto-fill list endpoints.
func (*VoyageAddressBookController) List(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	if !addressBookAvailable(database.DB) {
		Fail(c, 503, "地址簿尚未准备好，请稍后重试")
		return
	}
	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		page = 1
	}
	if page > 100000 {
		page = 100000
	}
	q := database.DB.Model(&database.VoyageAddressBookEntry{})
	search := strings.TrimSpace(c.Query("q"))
	if len(search) > 160 {
		Fail(c, 400, "搜索内容过长")
		return
	}
	if search != "" {
		like := "%" + search + "%"
		if database.DB.Migrator().HasTable(&database.BilibiliProfile{}) {
			q = q.Where("uid LIKE ? OR uid IN (SELECT uid FROM bilibili_profiles WHERE name LIKE ?)", like, like)
		} else {
			q = q.Where("uid LIKE ?", like)
		}
	}
	var total int64
	if err := q.Distinct("uid").Count(&total).Error; err != nil {
		Fail(c, 500, "地址簿暂时无法读取")
		return
	}
	type row struct {
		UID       string `json:"uid"`
		Name      string `json:"name"`
		Count     int    `json:"count"`
		UpdatedAt string `json:"updated_at"`
		Enabled   bool   `json:"enabled"`
		Source    string `json:"source"`
	}
	rows := []row{}
	if err := q.Select("uid, count(*) as count, max(last_used_at) as updated_at").Group("uid").Order("updated_at desc, uid").Offset((page - 1) * 25).Limit(25).Scan(&rows).Error; err != nil {
		Fail(c, 500, "地址簿暂时无法读取")
		return
	}
	for i := range rows {
		p, err := readLongTermAddress(database.DB, rows[i].UID)
		if err != nil {
			Fail(c, 500, "地址设置暂时无法读取")
			return
		}
		rows[i].Enabled, rows[i].Source = p.Enabled, p.Source
		if database.DB.Migrator().HasTable(&database.BilibiliProfile{}) {
			var profile database.BilibiliProfile
			database.DB.Where("uid = ?", rows[i].UID).Limit(1).Find(&profile)
			rows[i].Name = profile.Name
		}
		var latest database.VoyageAddressBookEntry
		if err := database.DB.Where("uid = ?", rows[i].UID).Order("last_used_at desc, id desc").Take(&latest).Error; err != nil {
			Fail(c, 500, "地址簿暂时无法读取")
			return
		}
		rows[i].UpdatedAt = latest.LastUsedAt.Format(time.RFC3339Nano)
	}
	Success(c, gin.H{"rows": rows, "total": total, "page": page})
}

type bookCollection struct {
	IssueID     uint   `json:"issue_id"`
	RecipientID uint   `json:"recipient_id"`
	Title       string `json:"title"`
	Version     int    `json:"version"`
}

func (*VoyageAddressBookController) Detail(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	uid := c.Param("uid")
	if !validBookUID(uid) {
		Fail(c, 400, "请输入有效的 UID")
		return
	}
	entries, err := readAddressBook(database.DB, uid)
	if err != nil {
		Fail(c, 500, "地址簿暂时无法读取")
		return
	}
	p, err := longTermState(database.DB, uid, nil)
	if err != nil {
		Fail(c, 500, "地址设置暂时无法读取")
		return
	}
	collections := []bookCollection{}
	var rows []database.VoyageRecipient
	if err = database.DB.Where("uid = ? AND disabled = ?", uid, false).Order("id desc").Find(&rows).Error; err != nil {
		Fail(c, 500, "收集批次暂时无法读取")
		return
	}
	for _, row := range rows {
		if len(row.BatchItems) == 0 {
			continue
		}
		var issue database.VoyageIssue
		if e := database.DB.Where("id = ? AND published = ?", row.IssueID, true).Take(&issue).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				continue
			}
			Fail(c, 500, "收集批次暂时无法读取")
			return
		}
		d, e := readDelivery(database.DB, row)
		if e != nil {
			Fail(c, 500, "收货信息暂时无法读取")
			return
		}
		_, _, pending, e := claimFulfillment(database.DB, row)
		if e != nil {
			Fail(c, 500, "寄送状态暂时无法读取")
			return
		}
		if !pending {
			continue
		}
		if d.Address == nil && d.ShippedAt == nil {
			collections = append(collections, bookCollection{row.IssueID, row.ID, issue.Title, d.Version})
		}
	}
	events := []database.VoyageAddressConsent{}
	if addressBookAvailable(database.DB) {
		if err = database.DB.Where("uid = ?", uid).Order("id desc").Limit(50).Find(&events).Error; err != nil {
			Fail(c, 500, "确认记录暂时无法读取")
			return
		}
	}
	Success(c, gin.H{"uid": uid, "entries": entries, "long_term": p, "collections": collections, "events": events})
}

func (*VoyageAddressBookController) Action(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	uid := c.Param("uid")
	adminID := biliAdmin(c)
	if adminID == 0 {
		Fail(c, 401, "请重新登录网站后台")
		return
	}
	if !validBookUID(uid) {
		Fail(c, 400, "请输入有效的 UID")
		return
	}
	var req struct {
		Action          string    `json:"action"`
		EntryID         uint      `json:"entry_id"`
		Version         int       `json:"version"`
		RecipientID     uint      `json:"recipient_id"`
		DeliveryVersion int       `json:"delivery_version"`
		Confirmed       bool      `json:"confirmed"`
		ConfirmedAt     time.Time `json:"confirmed_at"`
		Channel         string    `json:"channel"`
	}
	if c.ShouldBindJSON(&req) != nil {
		Fail(c, 400, "请检查填写内容")
		return
	}
	if req.Action != "set_default" && req.Action != "clear_default" && req.Action != "confirm_once" {
		Fail(c, 400, "操作无效")
		return
	}
	if req.Action != "clear_default" && (!req.Confirmed || req.ConfirmedAt.IsZero() || req.ConfirmedAt.After(time.Now().Add(time.Minute)) || (req.Channel != "B站私信" && req.Channel != "其他渠道")) {
		Fail(c, 400, errBookConfirmation.Error())
		return
	}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if e := lockVoyageBatchWrites(tx); e != nil {
			return e
		}
		if !addressBookAvailable(tx) {
			return errBookEntry
		}
		p, e := readLongTermAddress(tx, uid)
		if e != nil {
			return e
		}
		if p.Version != req.Version {
			return errDeliveryConflict
		}
		if req.Action == "clear_default" {
			return changeAddressPreference(tx, uid, "clear", req.Version, nil, 0, 0, "admin", adminID, nil, "")
		}
		a, e := ownedBookAddress(tx, uid, req.EntryID)
		if e != nil {
			return e
		}
		if req.Action == "set_default" {
			if p.Version > 0 && !req.ConfirmedAt.After(p.UpdatedAt) {
				return errBookConfirmation
			}
			if e = changeAddressPreference(tx, uid, "set", req.Version, &a, 0, 0, "admin", adminID, &req.ConfirmedAt, req.Channel); e != nil {
				return e
			}
			return inheritLongTermAddresses(tx, 0, uid)
		}
		var row database.VoyageRecipient
		if e = tx.Where("id = ? AND uid = ? AND disabled = ?", req.RecipientID, uid, false).Take(&row).Error; e != nil {
			return errBookEntry
		}
		if len(row.BatchItems) == 0 {
			return errBookEntry
		}
		var issue database.VoyageIssue
		if e = tx.Where("id = ? AND published = ?", row.IssueID, true).Take(&issue).Error; e != nil {
			return errBookEntry
		}
		d, e := readDelivery(tx, row)
		if e != nil {
			return e
		}
		_, _, pending, e := claimFulfillment(tx, row)
		if e != nil {
			return e
		}
		if !pending {
			return errBookOccupied
		}
		if d.Version != req.DeliveryVersion {
			return errDeliveryConflict
		}
		if d.Address != nil || d.ShippedAt != nil {
			return errBookOccupied
		}
		raw, _ := json.Marshal(a)
		sealed, e := secret.SealVoyageAddress(recipientScope(row.IssueID, uid), raw)
		if e != nil {
			return e
		}
		now := time.Now()
		if d.Version == 0 {
			e = tx.Create(&database.VoyageDelivery{RecipientID: row.ID, AddressEncrypted: sealed, AddressSource: "admin_confirmed", Version: 1, SubmittedAt: &now}).Error
		} else {
			update := tx.Model(&database.VoyageDelivery{}).Where("recipient_id = ? AND version = ? AND address_encrypted = '' AND shipped_at IS NULL", row.ID, d.Version).Updates(map[string]any{"address_encrypted": sealed, "address_source": "admin_confirmed", "submitted_at": now, "version": d.Version + 1})
			e = update.Error
			if e == nil && update.RowsAffected != 1 {
				e = errDeliveryConflict
			}
		}
		if e != nil {
			return e
		}
		if _, e = rememberVoyageAddress(tx, uid, row.IssueID, row.ID, &a, "admin_confirmed"); e != nil {
			return e
		}
		return tx.Create(&database.VoyageAddressConsent{UID: uid, EntryID: req.EntryID, RecipientID: row.ID, Action: "admin_confirm_once", PreferenceVersion: p.Version, AdminID: adminID, Channel: req.Channel, ConfirmedAt: &req.ConfirmedAt, OccurredAt: now}).Error
	})
	if err != nil {
		if errors.Is(err, errDeliveryConflict) || errors.Is(err, errBookEntry) || errors.Is(err, errBookOccupied) || errors.Is(err, errBookConfirmation) {
			Fail(c, 409, err.Error())
		} else {
			Fail(c, 500, "地址设置未能保存，请刷新后重试")
		}
		return
	}
	Success(c, gin.H{"saved": true})
}
