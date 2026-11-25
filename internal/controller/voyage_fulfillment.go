package controller

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mime"
	"mofufans/internal/database"
	"mofufans/internal/secret"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type VoyageFulfillmentController struct{}
type parcelAddress struct {
	AddressSource   string         `json:"address_source,omitempty"`
	LongTermChanged bool           `json:"long_term_changed,omitempty"`
	LongTermOrigin  string         `json:"long_term_origin,omitempty"`
	RecipientID     uint           `json:"recipient_id"`
	Collection      string         `json:"collection"`
	Version         int            `json:"version"`
	Address         *VoyageAddress `json:"address"`
}
type parcelView struct {
	database.VoyageParcel
	Address        *VoyageAddress  `json:"address"`
	Choices        []parcelAddress `json:"choices"`
	AddressChanged bool            `json:"address_changed"`
	BlockReason    string          `json:"block_reason"`
	HasHistory     bool            `json:"has_history,omitempty"`
}
type shippingSelection struct {
	CollectionID uint   `json:"collection_id"`
	RosterID     uint   `json:"roster_id"`
	Tier         string `json:"tier"`
}

func selectionKey(collection, roster uint, tier string) string {
	return fmt.Sprintf("%d/%d/%s", collection, roster, tier)
}
func entitlementKey(uid, key string) string { return uid + "/" + key }
func activeParcels(tx *gorm.DB) ([]database.VoyageParcel, error) {
	rows := []database.VoyageParcel{}
	if !tx.Migrator().HasTable(&database.VoyageParcel{}) {
		return rows, nil
	}
	e := tx.Omit("address_encrypted").Where("cancelled_at IS NULL").Order("id").Find(&rows).Error
	return rows, e
}
func parcelDetails(tx *gorm.DB, p database.VoyageParcel) (parcelView, error) {
	v := parcelView{VoyageParcel: p, Choices: []parcelAddress{}}
	seen := map[uint]bool{}
	for _, i := range p.Items {
		if seen[i.RecipientID] {
			continue
		}
		seen[i.RecipientID] = true
		var r database.VoyageRecipient
		if e := tx.First(&r, i.RecipientID).Error; e != nil {
			return v, e
		}
		var b database.VoyageBatch
		if e := tx.First(&b, i.CollectionID).Error; e != nil {
			return v, e
		}
		if r.UID != p.UID || r.IssueID != i.CollectionID {
			return v, errors.New("包裹归属无效")
		}
		d, e := readDelivery(tx, r)
		if e != nil {
			return v, e
		}
		v.Choices = append(v.Choices, parcelAddress{RecipientID: r.ID, Collection: b.Title, Version: d.Version, Address: d.Address, AddressSource: d.AddressSource, LongTermChanged: d.LongTermChanged, LongTermOrigin: d.LongTermOrigin})
		if !r.Disabled {
			found := false
			for _, item := range r.BatchItems {
				if item.Key == i.Key {
					found = true
				}
			}
			if !found {
				v.BlockReason = "领取范围已变化，请取消这个未寄包裹后重新安排"
			}
		} else {
			v.BlockReason = "用户资格已停用，请先核对名单"
		}
	}
	if p.AddressEncrypted != "" {
		var r database.VoyageRecipient
		if e := tx.First(&r, p.AddressRecipientID).Error; e != nil {
			return v, e
		}
		if r.UID != p.UID {
			return v, errors.New("包裹地址归属无效")
		}
		raw, e := secret.OpenVoyageAddress(recipientScope(r.IssueID, r.UID), p.AddressEncrypted)
		if e != nil {
			return v, e
		}
		if e = json.Unmarshal(raw, &v.Address); e != nil {
			return v, e
		}
		for _, c := range v.Choices {
			if c.RecipientID == p.AddressRecipientID {
				v.AddressChanged = !sameVoyageAddress(v.Address, c.Address, true)
			}
		}
	}
	if p.ShippedAt != nil {
		v.AddressChanged = false
		v.BlockReason = ""
	}
	if v.Address == nil && addressBookAvailable(tx) {
		var count int64
		if err := tx.Model(&database.VoyageAddressBookEntry{}).Where("uid = ?", p.UID).Count(&count).Error; err != nil {
			return v, err
		}
		v.HasHistory = count > 0
	}
	return v, nil
}
func (*VoyageFulfillmentController) List(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	if err := syncLongTermAddresses(database.DB, 0); err != nil {
		Fail(c, 500, "常用地址自动填写失败，请稍后重试")
		return
	}
	type planView struct {
		database.VoyageShipmentPlan
		Total     int64 `json:"total"`
		Shipped   int64 `json:"shipped"`
		Cancelled int64 `json:"cancelled"`
	}
	plans := []database.VoyageShipmentPlan{}
	if e := database.DB.Order("id desc").Find(&plans).Error; e != nil {
		Fail(c, 500, "发货安排读取失败")
		return
	}
	type counts struct {
		PlanID    uint
		Total     int64
		Shipped   int64
		Cancelled int64
	}
	totals := []counts{}
	if e := database.DB.Model(&database.VoyageParcel{}).Select("plan_id, count(*) as total, sum(case when shipped_at is not null then 1 else 0 end) as shipped, sum(case when cancelled_at is not null then 1 else 0 end) as cancelled").Group("plan_id").Scan(&totals).Error; e != nil {
		Fail(c, 500, "包裹统计读取失败")
		return
	}
	result := []planView{}
	for _, p := range plans {
		v := planView{VoyageShipmentPlan: p}
		for _, n := range totals {
			if n.PlanID == p.ID {
				v.Total = n.Total
				v.Shipped = n.Shipped
				v.Cancelled = n.Cancelled
			}
		}
		result = append(result, v)
	}
	views := []parcelView{}
	if c.Query("plan") != "" && c.Query("plan") != "0" {
		var parcels []database.VoyageParcel
		if e := database.DB.Where("plan_id = ?", c.Query("plan")).Order("id").Find(&parcels).Error; e != nil {
			Fail(c, 500, "包裹读取失败")
			return
		}
		for _, p := range parcels {
			v, e := parcelDetails(database.DB, p)
			if e != nil {
				Fail(c, 500, "包裹地址读取失败")
				return
			}
			views = append(views, v)
		}
	}
	Success(c, gin.H{"plans": result, "parcels": views})
}
func (*VoyageFulfillmentController) Pending(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	type group struct {
		shippingSelection
		Label      string `json:"label"`
		Collection string `json:"collection"`
		Category   string `json:"category"`
		Month      string `json:"month"`
		People     int    `json:"people"`
		Addressed  int    `json:"addressed"`
	}
	groups := map[string]*group{}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if e := lockVoyageBatchWrites(tx); e != nil {
			return e
		}
		if e := inheritLongTermAddresses(tx, 0, ""); e != nil {
			return e
		}
		ps, e := activeParcels(tx)
		if e != nil {
			return e
		}
		reserved := map[string]bool{}
		for _, p := range ps {
			for _, i := range p.Items {
				reserved[entitlementKey(p.UID, i.Key)] = true
			}
		}
		var rows []database.VoyageRecipient
		if e = tx.Where("disabled = ?", false).Find(&rows).Error; e != nil {
			return e
		}
		var batches []database.VoyageBatch
		var rosters []database.VoyageRoster
		var addressIDs []uint
		if e = tx.Find(&batches).Error; e != nil {
			return e
		}
		if e = tx.Find(&rosters).Error; e != nil {
			return e
		}
		if e = tx.Model(&database.VoyageDelivery{}).Where("address_encrypted <> ''").Pluck("recipient_id", &addressIDs).Error; e != nil {
			return e
		}
		batchMap := map[uint]database.VoyageBatch{}
		rosterMap := map[uint]database.VoyageRoster{}
		addressMap := map[uint]bool{}
		for _, v := range batches {
			batchMap[v.ID] = v
		}
		for _, v := range rosters {
			rosterMap[v.ID] = v
		}
		for _, id := range addressIDs {
			addressMap[id] = true
		}
		for _, r := range rows {
			b, ok := batchMap[r.IssueID]
			if !ok {
				continue
			}
			for _, i := range r.BatchItems {
				if reserved[entitlementKey(r.UID, i.Key)] {
					continue
				}
				key := selectionKey(b.ID, i.RosterID, i.Tier)
				g := groups[key]
				if g == nil {
					roster, ok := rosterMap[i.RosterID]
					if !ok {
						return errors.New("名单不存在")
					}
					g = &group{shippingSelection: shippingSelection{b.ID, i.RosterID, i.Tier}, Label: i.Label, Collection: b.Title, Category: roster.Category, Month: roster.Month}
					groups[key] = g
				}
				g.People++
				if addressMap[r.ID] {
					g.Addressed++
				}
			}
		}
		return nil
	})
	if err != nil {
		Fail(c, 500, "待寄礼物读取失败")
		return
	}
	result := []group{}
	for _, v := range groups {
		result = append(result, *v)
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.CollectionID != b.CollectionID {
			return a.CollectionID > b.CollectionID
		}
		if a.Month != b.Month {
			return a.Month > b.Month
		}
		return a.Label < b.Label
	})
	Success(c, result)
}

type fulfillmentPreview struct {
	Token     string       `json:"token"`
	Rows      []parcelView `json:"rows"`
	Total     int          `json:"total"`
	Missing   int          `json:"missing"`
	Conflicts int          `json:"conflicts"`
}

func shippingPreview(tx *gorm.DB, selections []shippingSelection) (fulfillmentPreview, error) {
	result := fulfillmentPreview{Rows: []parcelView{}}
	if len(selections) == 0 || len(selections) > 180 {
		return result, errors.New("请选择本次要寄送的月份、档位或活动")
	}
	selected := map[string]bool{}
	for _, s := range selections {
		k := selectionKey(s.CollectionID, s.RosterID, s.Tier)
		if selected[k] {
			return result, errors.New("寄送范围重复")
		}
		selected[k] = true
	}
	ps, e := activeParcels(tx)
	if e != nil {
		return result, e
	}
	reserved := map[string]bool{}
	for _, p := range ps {
		for _, i := range p.Items {
			reserved[entitlementKey(p.UID, i.Key)] = true
		}
	}
	var rows []database.VoyageRecipient
	if e = tx.Where("disabled = ?", false).Order("id").Find(&rows).Error; e != nil {
		return result, e
	}
	people := map[string]*database.VoyageParcel{}
	for _, r := range rows {
		for _, i := range r.BatchItems {
			if !selected[selectionKey(r.IssueID, i.RosterID, i.Tier)] || reserved[entitlementKey(r.UID, i.Key)] {
				continue
			}
			p := people[r.UID]
			if p == nil {
				p = &database.VoyageParcel{UID: r.UID, Name: r.Name, Version: 1, Items: []database.VoyageParcelItem{}}
				people[r.UID] = p
			}
			p.Items = append(p.Items, database.VoyageParcelItem{VoyageBatchItem: i, RecipientID: r.ID, CollectionID: r.IssueID})
			reserved[entitlementKey(r.UID, i.Key)] = true
		}
	}
	keys := []string{}
	for uid := range people {
		keys = append(keys, uid)
	}
	sort.Strings(keys)
	for _, uid := range keys {
		v, e := parcelDetails(tx, *people[uid])
		if e != nil {
			return result, e
		}
		addresses := map[string]bool{}
		for _, a := range v.Choices {
			if a.Address != nil {
				raw, _ := json.Marshal(a.Address)
				addresses[string(raw)] = true
			}
		}
		if len(addresses) == 0 {
			result.Missing++
		}
		if len(addresses) > 1 {
			result.Conflicts++
		}
		result.Rows = append(result.Rows, v)
	}
	result.Total = len(result.Rows)
	if result.Total == 0 {
		return result, errors.New("所选礼物已安排或没有待寄用户，请刷新待寄清单")
	}
	if result.Total > 10000 {
		return result, errors.New("一次最多安排10000个包裹")
	}
	raw, _ := json.Marshal(result.Rows)
	result.Token = fmt.Sprintf("%x", sha256.Sum256(raw))
	return result, nil
}

// A single address (or identical addresses in several collections) needs no
// extra selection step. Conflicts still require an explicit address choice.
func singleParcelAddress(choices []parcelAddress) (parcelAddress, bool) {
	var first parcelAddress
	for _, choice := range choices {
		if choice.Address == nil {
			continue
		}
		if first.Address == nil {
			first = choice
			continue
		}
		if *choice.Address != *first.Address {
			return parcelAddress{}, false
		}
	}
	return first, first.Address != nil
}
func applyParcelAddress(tx *gorm.DB, p *database.VoyageParcel, choices []parcelAddress, id uint, version int, now time.Time) error {
	for _, choice := range choices {
		if choice.RecipientID != id {
			continue
		}
		if choice.Address == nil {
			return errors.New("这个地址尚未填写")
		}
		if choice.Version != version {
			return errors.New("地址刚刚发生变化，请刷新后核对")
		}
		var d database.VoyageDelivery
		if e := tx.First(&d, "recipient_id = ?", choice.RecipientID).Error; e != nil {
			return e
		}
		p.AddressRecipientID = id
		p.AddressVersion = version
		p.AddressEncrypted = d.AddressEncrypted
		p.ConfirmedAt = &now
		p.ExportedAt = nil
		return nil
	}
	return errors.New("请选择这个用户在本次收集范围内的地址")
}

func (*VoyageFulfillmentController) Create(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	var req struct {
		Title      string              `json:"title"`
		Selections []shippingSelection `json:"selections"`
		Confirm    bool                `json:"confirm"`
		Token      string              `json:"token"`
	}
	if c.ShouldBindJSON(&req) != nil {
		Fail(c, 400, "寄送范围无效")
		return
	}
	var preview fulfillmentPreview
	var plan database.VoyageShipmentPlan
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if e := lockVoyageBatchWrites(tx); e != nil {
			return e
		}
		if e := inheritLongTermAddresses(tx, 0, ""); e != nil {
			return e
		}
		var e error
		preview, e = shippingPreview(tx, req.Selections)
		if e != nil {
			return e
		}
		if !req.Confirm {
			return nil
		}
		if req.Token == "" || req.Token != preview.Token {
			return errors.New("名单、地址或待寄状态已变化，请重新预览")
		}
		title := strings.TrimSpace(req.Title)
		if title == "" || utf8.RuneCountInString(title) > 80 {
			return errors.New("请填写发货安排名称，最多80字")
		}
		plan = database.VoyageShipmentPlan{Title: title, Version: 1}
		if e = tx.Create(&plan).Error; e != nil {
			return e
		}
		for _, v := range preview.Rows {
			p := v.VoyageParcel
			p.PlanID = plan.ID
			if address, ok := singleParcelAddress(v.Choices); ok {
				if e = applyParcelAddress(tx, &p, v.Choices, address.RecipientID, address.Version, time.Now()); e != nil {
					return e
				}
			}
			if e = tx.Create(&p).Error; e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		Fail(c, 409, err.Error())
		return
	}
	Success(c, gin.H{"preview": preview, "plan": plan})
}

type parcelChange struct {
	ID                 uint   `json:"id"`
	Version            int    `json:"version"`
	AddressRecipientID uint   `json:"address_recipient_id"`
	AddressVersion     int    `json:"address_version"`
	Courier            string `json:"courier"`
	Tracking           string `json:"tracking"`
	Note               string `json:"note"`
}

func changeParcel(tx *gorm.DB, planID uint, action string, item parcelChange) (parcelView, error) {
	var p database.VoyageParcel
	if e := tx.Where("id = ? AND plan_id = ?", item.ID, planID).First(&p).Error; e != nil {
		return parcelView{}, errors.New("包裹不属于本次发货安排")
	}
	if p.Version != item.Version {
		return parcelView{}, errDeliveryConflict
	}
	v, e := parcelDetails(tx, p)
	if e != nil {
		return v, e
	}
	if p.CancelledAt != nil {
		return v, errors.New("包裹已取消，请刷新")
	}
	now := time.Now()
	// An address submitted after plan creation can be used in the same action.
	// The browser must send the version it displayed; never silently accept a
	// newly changed address, a conflict, or replace an existing parcel snapshot.
	if (action == "ship" || action == "export") && p.AddressEncrypted == "" {
		address, ok := singleParcelAddress(v.Choices)
		if !ok {
			return v, errors.New("请先补齐地址；有不同地址时需展开选择")
		}
		if item.AddressRecipientID != address.RecipientID || item.AddressVersion != address.Version {
			return v, errors.New("地址刚刚发生变化，请刷新后核对")
		}
		if e = applyParcelAddress(tx, &p, v.Choices, address.RecipientID, address.Version, now); e != nil {
			return v, e
		}
		v.Address = address.Address
	}
	switch action {
	case "cancel":
		if p.ShippedAt != nil {
			return v, errors.New("已寄出的包裹不能取消")
		}
		p.CancelledAt = &now
	case "address":
		if p.ShippedAt != nil {
			return v, errors.New("已寄出的地址记录不能改写")
		}
		if v.BlockReason != "" {
			return v, errors.New(v.BlockReason)
		}
		if e = applyParcelAddress(tx, &p, v.Choices, item.AddressRecipientID, item.AddressVersion, now); e != nil {
			return v, e
		}
	case "draft", "ship":
		if v.BlockReason != "" {
			return v, errors.New(v.BlockReason)
		}
		p.Courier = strings.TrimSpace(item.Courier)
		p.Tracking = strings.TrimSpace(item.Tracking)
		p.Note = strings.TrimSpace(item.Note)
		if p.Courier == "" || utf8.RuneCountInString(p.Courier) > 40 || !trackingPattern.MatchString(p.Tracking) || utf8.RuneCountInString(p.Note) > 300 {
			return v, errors.New("请填写快递公司和有效运单号，备注最多300字")
		}
		if action == "ship" {
			if v.Address == nil {
				return v, errors.New("请先确认本包裹的收货地址")
			}
			if v.AddressChanged {
				return v, errors.New("用户更新了地址，请重新核对并确认，已导出的面单也需更新")
			}
			if p.ShippedAt == nil {
				p.ShippedAt = &now
			}
		}
	case "export":
		if v.BlockReason != "" {
			return v, errors.New(v.BlockReason)
		}
		if v.Address == nil || v.AddressChanged {
			return v, errors.New("请先核对并确认地址；地址变化后须重新确认")
		}
		p.ExportedAt = &now
	default:
		return v, errors.New("包裹操作无效")
	}
	p.Version++
	if e = tx.Save(&p).Error; e != nil {
		return v, e
	}
	return parcelDetails(tx, p)
}
func (*VoyageFulfillmentController) Change(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	id, ok := batchID(c)
	if !ok {
		return
	}
	var req struct {
		Action string         `json:"action"`
		Items  []parcelChange `json:"items"`
	}
	if c.ShouldBindJSON(&req) != nil || len(req.Items) == 0 || len(req.Items) > 500 {
		Fail(c, 400, "每次请选择1至500个包裹")
		return
	}
	results := []parcelView{}
	var workbook []byte
	e := database.DB.Transaction(func(tx *gorm.DB) error {
		if e := lockVoyageBatchWrites(tx); e != nil {
			return e
		}
		seen := map[uint]bool{}
		for _, i := range req.Items {
			if seen[i.ID] {
				return errors.New("包裹重复")
			}
			seen[i.ID] = true
			v, e := changeParcel(tx, id, req.Action, i)
			if e != nil {
				return fmt.Errorf("包裹 %d：%s；本次操作均未保存", i.ID, e)
			}
			results = append(results, v)
		}
		if req.Action == "export" {
			data := [][]string{}
			for _, v := range results {
				labels := []string{}
				for _, i := range v.Items {
					labels = append(labels, i.Label)
				}
				a := v.Address
				stamp := func(t *time.Time) string {
					if t == nil {
						return ""
					}
					return t.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02 15:04:05")
				}
				state := "待寄出"
				if v.ShippedAt != nil {
					state = "已寄出"
				}
				data = append(data, []string{v.UID, v.Name, fmt.Sprintf("包裹%d", v.ID), a.Name, a.Phone, a.Address, a.Note, state, v.Courier, v.Tracking, v.Note, stamp(v.ConfirmedAt), stamp(v.ShippedAt), strings.Join(labels, "、")})
			}
			var e error
			workbook, e = addressWorkbookColumns(data, []string{"UID", "昵称", "包裹编号", "姓名", "手机号码", "地址", "收件备注", "状态", "快递公司", "运单号", "物流备注", "地址确认时间", "寄出时间", "本包裹礼物"})
			return e
		}
		return nil
	})
	if e != nil {
		Fail(c, 409, e.Error())
		return
	}
	if req.Action == "export" {
		c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": fmt.Sprintf("发货安排%d-包裹地址.xlsx", id)}))
		c.Header("X-Content-Type-Options", "nosniff")
		c.Data(200, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", workbook)
		return
	}
	Success(c, results)
}

// Project only items covered by the verified collection. Other collection labels
// and addresses never appear through an unrelated collection credential.
type claimItemState struct {
	Label  string `json:"label"`
	Status string `json:"status"`
}
type claimParcel struct {
	ID        uint       `json:"id"`
	Labels    []string   `json:"labels"`
	Status    string     `json:"status"`
	Courier   string     `json:"courier"`
	Tracking  string     `json:"tracking"`
	Note      string     `json:"note"`
	ShippedAt *time.Time `json:"shipped_at"`
}

func claimFulfillment(tx *gorm.DB, r database.VoyageRecipient) ([]claimItemState, []claimParcel, bool, error) {
	ps, e := activeParcels(tx.Where("uid = ?", r.UID))
	if e != nil {
		return nil, nil, false, e
	}
	states := map[string]string{}
	parcels := []claimParcel{}
	for _, p := range ps {
		if p.UID != r.UID {
			continue
		}
		labels := []string{}
		status := "准备寄送"
		if p.ShippedAt != nil {
			status = "已寄出"
		}
		for _, i := range p.Items {
			if i.RecipientID == r.ID {
				states[i.Key] = status
				labels = append(labels, i.Label)
			}
		}
		if len(labels) == 0 {
			continue
		}
		v := claimParcel{ID: p.ID, Labels: labels, Status: status, ShippedAt: p.ShippedAt}
		if p.ShippedAt != nil {
			v.Courier = p.Courier
			v.Tracking = p.Tracking
			v.Note = p.Note
		}
		parcels = append(parcels, v)
	}
	items := []claimItemState{}
	pending := false
	for _, i := range r.BatchItems {
		state := states[i.Key]
		if state == "" {
			state = "等待安排"
		}
		if state != "已寄出" {
			pending = true
		}
		items = append(items, claimItemState{i.Label, state})
	}
	return items, parcels, pending, nil
}
