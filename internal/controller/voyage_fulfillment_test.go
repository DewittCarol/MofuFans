package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"mofufans/internal/database"
	"mofufans/internal/secret"
	"testing"
	"time"
)

func fulfillmentSetup(t *testing.T) *gin.Engine {
	r := batchTestSetup(t)
	if e := database.InitializeVoyageFulfillment(); e != nil {
		t.Fatal(e)
	}
	ctl := &VoyageFulfillmentController{}
	r.GET("/fulfillment", ctl.List)
	r.GET("/pending", ctl.Pending)
	r.POST("/fulfillment", ctl.Create)
	r.POST("/fulfillment/:id/parcels", ctl.Change)
	return r
}
func fulfillmentRoster(t *testing.T, category string, members ...database.VoyageRosterMember) database.VoyageRoster {
	list := database.VoyageRoster{Title: "9月转盘", Category: category, Month: "2026-09", Version: 1}
	if e := database.DB.Create(&list).Error; e != nil {
		t.Fatal(e)
	}
	for _, m := range members {
		m.RosterID = list.ID
		m.Version = 1
		if e := database.DB.Create(&m).Error; e != nil {
			t.Fatal(e)
		}
	}
	return list
}
func fulfillmentPerson(t *testing.T, b database.VoyageBatch, uid string) database.VoyageRecipient {
	var r database.VoyageRecipient
	if e := database.DB.Where("issue_id = ? AND uid = ?", b.ID, uid).First(&r).Error; e != nil {
		t.Fatal(e)
	}
	return r
}
func fulfillmentAddress(t *testing.T, r database.VoyageRecipient, address string) {
	a := VoyageAddress{Name: "测试", Phone: "13800000000", Address: address}
	raw, _ := json.Marshal(a)
	sealed, e := secret.SealVoyageAddress(recipientScope(r.IssueID, r.UID), raw)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	var d database.VoyageDelivery
	database.DB.First(&d, "recipient_id = ?", r.ID)
	d.RecipientID = r.ID
	d.AddressEncrypted = sealed
	d.SubmittedAt = &now
	d.Version++
	if e = database.DB.Save(&d).Error; e != nil {
		t.Fatal(e)
	}
}
func fulfillmentPlan(t *testing.T, r *gin.Engine, selections ...shippingSelection) (database.VoyageShipmentPlan, []database.VoyageParcel) {
	body := gin.H{"title": "九月第一批", "selections": selections}
	res := batchCall(t, r, "POST", "/fulfillment", body, 200)
	var result struct {
		Preview fulfillmentPreview          `json:"preview"`
		Plan    database.VoyageShipmentPlan `json:"plan"`
	}
	json.Unmarshal(res.Data, &result)
	body["confirm"] = true
	body["token"] = result.Preview.Token
	res = batchCall(t, r, "POST", "/fulfillment", body, 200)
	json.Unmarshal(res.Data, &result)
	var parcels []database.VoyageParcel
	database.DB.Where("plan_id = ?", result.Plan.ID).Order("id").Find(&parcels)
	return result.Plan, parcels
}
func fulfillmentChange(t *testing.T, r *gin.Engine, p *database.VoyageParcel, action string, extra gin.H, want int) {
	item := gin.H{"id": p.ID, "version": p.Version}
	for k, v := range extra {
		item[k] = v
	}
	batchCall(t, r, "POST", fmt.Sprintf("/fulfillment/%d/parcels", p.PlanID), gin.H{"action": action, "items": []gin.H{item}}, want)
	database.DB.First(p, p.ID)
}
func TestFulfillmentPartialShippingAddressRevisionAndScope(t *testing.T) {
	r := fulfillmentSetup(t)
	roster := fulfillmentRoster(t, "monthly", database.VoyageRosterMember{UID: "10001", Tier: "captain"}, database.VoyageRosterMember{UID: "10001", Tier: "governor"}, database.VoyageRosterMember{UID: "10002", Tier: "captain"})
	b := newTestBatch(t, r)
	sources := []database.VoyageBatchSource{{RosterID: roster.ID, Tiers: []string{"captain", "governor"}}}
	syncTestBatch(t, r, &b, sources)
	database.DB.Model(&b).Update("published", true)
	person := fulfillmentPerson(t, b, "10001")
	fulfillmentAddress(t, person, "浙江省杭州市旧地址123")
	_, ps := fulfillmentPlan(t, r, shippingSelection{b.ID, roster.ID, "captain"})
	if len(ps) != 2 {
		t.Fatalf("want two captain parcels: %d", len(ps))
	}
	p := ps[0]
	if p.AddressEncrypted == "" || p.AddressRecipientID != person.ID || p.AddressVersion != 1 {
		t.Fatal("one available address should be ready without a separate confirmation")
	}
	fulfillmentChange(t, r, &p, "address", gin.H{"address_recipient_id": person.ID, "address_version": 1}, 200)
	fulfillmentChange(t, r, &p, "draft", gin.H{"courier": "顺丰", "tracking": "SF123456"}, 200)
	// No sent_at prerequisite. A changed address must block ship/export until reviewed.
	if person.SentAt != nil {
		t.Fatal("fixture unexpectedly notified")
	}
	fulfillmentAddress(t, person, "浙江省杭州市新地址456")
	fulfillmentChange(t, r, &p, "ship", gin.H{"courier": "顺丰", "tracking": "SF123456"}, 409)
	fulfillmentChange(t, r, &p, "export", nil, 409)
	fulfillmentChange(t, r, &p, "address", gin.H{"address_recipient_id": person.ID, "address_version": 1}, 409)
	fulfillmentChange(t, r, &p, "address", gin.H{"address_recipient_id": person.ID, "address_version": 2}, 200)
	fulfillmentChange(t, r, &p, "ship", gin.H{"courier": "顺丰", "tracking": "SF123456"}, 200)
	// Export the confirmed snapshot, with a parcel-specific workbook header.
	body, _ := json.Marshal(gin.H{"action": "export", "items": []gin.H{{"id": p.ID, "version": p.Version}}})
	response, _ := performJSONRequest(r, "POST", fmt.Sprintf("/fulfillment/%d/parcels", p.PlanID), string(body), nil)
	book, e := excelize.OpenReader(bytes.NewReader(response.Body.Bytes()))
	if e != nil {
		t.Fatal(e)
	}
	header, e := book.GetCellValue("收货地址", "C1")
	if e != nil || header != "包裹编号" {
		t.Fatalf("bad parcel export header: %q %v", header, e)
	}
	book.Close()
	database.DB.First(&p, p.ID)
	snapshot := p.AddressEncrypted
	fulfillmentChange(t, r, &p, "address", gin.H{"address_recipient_id": person.ID, "address_version": 2}, 409)
	// Closure does not freeze remaining fulfillment or changes to an existing address.
	database.DB.Model(&b).Update("closed", true)
	code, _ := secret.OpenVoyageCode(person.CodeEncrypted, recipientScope(b.ID, person.UID))
	res := batchCall(t, r, "POST", fmt.Sprintf("/issues/%d/claim", b.ID), gin.H{"uid": person.UID, "code": code, "action": "save", "version": 2, "address": VoyageAddress{Name: "测试", Phone: "13800000000", Address: "浙江省杭州市第二次新地址789"}}, 200)
	var view deliveryView
	json.Unmarshal(res.Data, &view)
	if len(view.Items) != 2 || len(view.Parcels) != 1 || !view.CanEditAddress {
		t.Fatalf("bad public progress: %+v", view)
	}
	shipped, pending := 0, 0
	for _, i := range view.Items {
		if i.Status == "已寄出" {
			shipped++
		} else {
			pending++
		}
	}
	if shipped != 1 || pending != 1 {
		t.Fatal("must only mark captain as shipped")
	}
	database.DB.First(&p, p.ID)
	if p.AddressEncrypted != snapshot {
		t.Fatal("shipped address overwritten")
	}
	_, governors := fulfillmentPlan(t, r, shippingSelection{b.ID, roster.ID, "governor"})
	if len(governors) != 1 || len(governors[0].Items) != 1 {
		t.Fatal("governor should be independent")
	}
	// Another member can be added after first parcel ships.
	m := database.VoyageRosterMember{RosterID: roster.ID, UID: "10003", Tier: "captain", Version: 1}
	database.DB.Create(&m)
	database.DB.Model(&roster).Update("version", 2)
	database.DB.First(&b, b.ID)
	preview := syncTestBatch(t, r, &b, sources)
	if preview.Total != 3 {
		t.Fatalf("new person lost: %d", preview.Total)
	}
	// Dropping selected sources preserves allocated/shipped item snapshots.
	preview = syncTestBatch(t, r, &b, []database.VoyageBatchSource{})
	if preview.Total != 2 || len(preview.Warnings) == 0 {
		t.Fatalf("allocated items not retained: %+v", preview)
	}
	fulfillmentChange(t, r, &p, "cancel", nil, 409)
	g := governors[0]
	fulfillmentChange(t, r, &g, "cancel", nil, 200)
	// A recipient with missing address remains present but cannot ship.
	missing := ps[1]
	fulfillmentChange(t, r, &missing, "ship", gin.H{"courier": "顺丰", "tracking": "SF987654"}, 409)
	// Other codes cannot reveal this user's parcels.
	batchCall(t, r, "POST", fmt.Sprintf("/issues/%d/claim", b.ID), gin.H{"uid": "10002", "code": code, "action": "view"}, 401)
}
func TestFulfillmentCrossCollectionAddressChoicePrivacyAndCancellation(t *testing.T) {
	r := fulfillmentSetup(t)
	monthly := fulfillmentRoster(t, "monthly", database.VoyageRosterMember{UID: "10001", Tier: "captain"})
	event := fulfillmentRoster(t, "event", database.VoyageRosterMember{UID: "10001"})
	first, second := newTestBatch(t, r), newTestBatch(t, r)
	syncTestBatch(t, r, &first, []database.VoyageBatchSource{{RosterID: monthly.ID, Tiers: []string{"captain"}}})
	syncTestBatch(t, r, &second, []database.VoyageBatchSource{{RosterID: event.ID}})
	a, b := fulfillmentPerson(t, first, "10001"), fulfillmentPerson(t, second, "10001")
	fulfillmentAddress(t, a, "上海市第一处地址123")
	fulfillmentAddress(t, b, "北京市另一处地址456")
	selections := []shippingSelection{{first.ID, monthly.ID, "captain"}, {second.ID, event.ID, ""}}
	preview, e := shippingPreview(database.DB, selections)
	if e != nil || preview.Conflicts != 1 || preview.Total != 1 {
		t.Fatalf("conflict missing: %+v %v", preview, e)
	}
	plan, ps := fulfillmentPlan(t, r, selections...)
	summary := batchCall(t, r, "GET", "/fulfillment", nil, 200)
	var summaries struct {
		Parcels []parcelView `json:"parcels"`
	}
	json.Unmarshal(summary.Data, &summaries)
	if len(summaries.Parcels) != 0 {
		t.Fatal("opening plan list should not download all private addresses")
	}
	detail := batchCall(t, r, "GET", fmt.Sprintf("/fulfillment?plan=%d", plan.ID), nil, 200)
	json.Unmarshal(detail.Data, &summaries)
	if len(summaries.Parcels) != 1 || len(summaries.Parcels[0].Choices) != 2 {
		t.Fatal("selected plan missing address choices")
	}
	pending := batchCall(t, r, "GET", "/pending", nil, 200)
	var groups []gin.H
	json.Unmarshal(pending.Data, &groups)
	if len(groups) != 0 {
		t.Fatal("allocated items still offered in pending")
	}
	p := ps[0]
	if len(p.Items) != 2 || p.AddressEncrypted != "" {
		t.Fatal("combined parcel must await explicit address")
	}
	fulfillmentChange(t, r, &p, "ship", gin.H{"address_recipient_id": a.ID, "address_version": 1, "courier": "顺丰", "tracking": "SF123456"}, 409)
	// Another actor cannot reuse an old preview and double-book these items.
	batchCall(t, r, "POST", "/fulfillment", gin.H{"title": "重复", "selections": selections, "confirm": true, "token": preview.Token}, 409)
	fulfillmentChange(t, r, &p, "address", gin.H{"address_recipient_id": 999, "address_version": 1}, 409)
	fulfillmentChange(t, r, &p, "address", gin.H{"address_recipient_id": b.ID, "address_version": 1}, 200)
	fulfillmentChange(t, r, &p, "ship", gin.H{"courier": "顺丰", "tracking": "SF123456"}, 200)
	items, parcels, _, e := claimFulfillment(database.DB, a)
	if e != nil || len(items) != 1 || len(parcels) != 1 || len(parcels[0].Labels) != 1 || parcels[0].Labels[0] == "9月转盘" {
		t.Fatalf("cross-collection leak: %+v %v", parcels, e)
	}
	// Parcel IDs from another plan must be rejected.
	batchCall(t, r, "POST", fmt.Sprintf("/fulfillment/%d/parcels", plan.ID+1), gin.H{"action": "draft", "items": []gin.H{{"id": p.ID, "version": p.Version, "courier": "顺丰", "tracking": "SF444"}}}, 409)
	// Cancel an unshipped allocation, then it is available to a new plan.
	third := newTestBatch(t, r)
	another := fulfillmentRoster(t, "event", database.VoyageRosterMember{UID: "10002"})
	syncTestBatch(t, r, &third, []database.VoyageBatchSource{{RosterID: another.ID}})
	_, unshipped := fulfillmentPlan(t, r, shippingSelection{third.ID, another.ID, ""})
	q := unshipped[0]
	fulfillmentChange(t, r, &q, "cancel", nil, 200)
	_, recreated := fulfillmentPlan(t, r, shippingSelection{third.ID, another.ID, ""})
	if recreated[0].ID == q.ID {
		t.Fatal("cancelled history must not be overwritten")
	}
}
func TestFulfillmentAtomicBulkAndMigration(t *testing.T) {
	r := fulfillmentSetup(t)
	list := fulfillmentRoster(t, "monthly", database.VoyageRosterMember{UID: "10001", Tier: "captain"}, database.VoyageRosterMember{UID: "10002", Tier: "captain"})
	b := newTestBatch(t, r)
	syncTestBatch(t, r, &b, []database.VoyageBatchSource{{RosterID: list.ID, Tiers: []string{"captain"}}})
	person := fulfillmentPerson(t, b, "10001")
	fulfillmentAddress(t, person, "上海市测试地址123")
	plan, ps := fulfillmentPlan(t, r, shippingSelection{b.ID, list.ID, "captain"})
	batchCall(t, r, "POST", fmt.Sprintf("/fulfillment/%d/parcels", plan.ID), gin.H{"action": "address", "items": []gin.H{{"id": ps[0].ID, "version": ps[0].Version, "address_recipient_id": person.ID, "address_version": 1}, {"id": ps[1].ID, "version": ps[1].Version, "address_recipient_id": person.ID, "address_version": 1}}}, 409)
	var unchanged database.VoyageParcel
	database.DB.First(&unchanged, ps[0].ID)
	if unchanged.AddressEncrypted != ps[0].AddressEncrypted || unchanged.Version != ps[0].Version {
		t.Fatal("partial bulk transaction committed")
	}
	// Migration preserves a legacy snapshot once, without modifying source address.
	now := time.Now()
	database.DB.Model(&database.VoyageDelivery{}).Where("recipient_id = ?", person.ID).Updates(map[string]any{"shipped_at": &now, "tracking": "OLD123", "courier": "顺丰"})
	if e := database.InitializeVoyageFulfillment(); e != nil {
		t.Fatal(e)
	}
	if e := database.InitializeVoyageFulfillment(); e != nil {
		t.Fatal(e)
	}
	var count int64
	database.DB.Model(&database.VoyageParcel{}).Where("legacy_recipient_id = ?", person.ID).Count(&count)
	if count != 1 {
		t.Fatalf("migration duplicated: %d", count)
	}
}

func TestFulfillmentLateAddressDirectShippingAndVersionCheck(t *testing.T) {
	r := fulfillmentSetup(t)
	list := fulfillmentRoster(t, "monthly", database.VoyageRosterMember{UID: "10001", Tier: "captain"})
	b := newTestBatch(t, r)
	syncTestBatch(t, r, &b, []database.VoyageBatchSource{{RosterID: list.ID, Tiers: []string{"captain"}}})
	person := fulfillmentPerson(t, b, "10001")
	_, ps := fulfillmentPlan(t, r, shippingSelection{b.ID, list.ID, "captain"})
	p := ps[0]
	if p.AddressEncrypted != "" {
		t.Fatal("missing address must remain pending")
	}
	fulfillmentAddress(t, person, "上海市首次登记地址123")
	fulfillmentAddress(t, person, "上海市已更新地址456")
	fulfillmentChange(t, r, &p, "ship", gin.H{"address_recipient_id": person.ID, "address_version": 1, "courier": "顺丰", "tracking": "SF123456"}, 409)
	if p.AddressEncrypted != "" || p.ShippedAt != nil {
		t.Fatal("stale displayed address must not be used")
	}
	fulfillmentChange(t, r, &p, "ship", gin.H{"address_recipient_id": person.ID, "address_version": 2, "courier": "顺丰", "tracking": "SF123456"}, 200)
	if p.AddressEncrypted == "" || p.ShippedAt == nil || p.AddressVersion != 2 || p.Tracking != "SF123456" {
		t.Fatal("address and tracking should be saved together when shipping")
	}
	fulfillmentChange(t, r, &p, "notified", nil, 409)
}
func TestFulfillmentIdenticalAddressesAndLateExport(t *testing.T) {
	r := fulfillmentSetup(t)
	monthly := fulfillmentRoster(t, "monthly", database.VoyageRosterMember{UID: "10001", Tier: "captain"}, database.VoyageRosterMember{UID: "10002", Tier: "captain"})
	event := fulfillmentRoster(t, "event", database.VoyageRosterMember{UID: "10001"})
	first, second := newTestBatch(t, r), newTestBatch(t, r)
	syncTestBatch(t, r, &first, []database.VoyageBatchSource{{RosterID: monthly.ID, Tiers: []string{"captain"}}})
	syncTestBatch(t, r, &second, []database.VoyageBatchSource{{RosterID: event.ID}})
	a, b := fulfillmentPerson(t, first, "10001"), fulfillmentPerson(t, second, "10001")
	fulfillmentAddress(t, a, "上海市同一收货地址123")
	fulfillmentAddress(t, b, "上海市同一收货地址123")
	_, ps := fulfillmentPlan(t, r, shippingSelection{first.ID, monthly.ID, "captain"}, shippingSelection{second.ID, event.ID, ""})
	if len(ps) != 2 || ps[0].AddressEncrypted == "" || len(ps[0].Items) != 2 {
		t.Fatal("identical addresses should not need a choice")
	}
	p := ps[1]
	person := fulfillmentPerson(t, first, "10002")
	fulfillmentAddress(t, person, "上海市晚填收货地址456")
	body, _ := json.Marshal(gin.H{"action": "export", "items": []gin.H{{"id": p.ID, "version": p.Version, "address_recipient_id": person.ID, "address_version": 1}}})
	response, _ := performJSONRequest(r, "POST", fmt.Sprintf("/fulfillment/%d/parcels", p.PlanID), string(body), nil)
	book, e := excelize.OpenReader(bytes.NewReader(response.Body.Bytes()))
	if e != nil {
		t.Fatal(e)
	}
	exported, e := book.GetCellValue("收货地址", "F2")
	book.Close()
	if e != nil || exported != "上海市晚填收货地址456" {
		t.Fatalf("late address missing from workbook: %q %v", exported, e)
	}
	database.DB.First(&p, p.ID)
	if p.AddressEncrypted == "" || p.ExportedAt == nil || p.ShippedAt != nil {
		t.Fatal("export should snapshot the address without marking shipped")
	}
}
