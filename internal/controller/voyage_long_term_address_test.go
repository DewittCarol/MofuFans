package controller

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mofufans/internal/database"
	"mofufans/internal/secret"
)

func longTermSetup(t *testing.T) *gin.Engine {
	r := fulfillmentSetup(t)
	if err := database.DB.AutoMigrate(&database.VoyageLongTermAddress{}); err != nil {
		t.Fatal(err)
	}
	r.GET("/issues/:id/shipments", (&VoyageDeliveryController{}).List)
	return r
}
func longTermBatch(t *testing.T, r *gin.Engine, published bool, uids ...string) (database.VoyageBatch, database.VoyageRoster) {
	members := []database.VoyageRosterMember{}
	for _, uid := range uids {
		members = append(members, database.VoyageRosterMember{UID: uid})
	}
	roster := fulfillmentRoster(t, "event", members...)
	b := newTestBatch(t, r)
	syncTestBatch(t, r, &b, []database.VoyageBatchSource{{RosterID: roster.ID}})
	if published {
		batchCall(t, r, "PUT", fmt.Sprintf("/batches/%d", b.ID), gin.H{"title": b.Title, "published": true, "version": b.Version}, 200)
		database.DB.First(&b, b.ID)
	}
	return b, roster
}
func longTermClaim(t *testing.T, r *gin.Engine, b database.VoyageBatch, uid string, extra gin.H, want int) deliveryView {
	p := fulfillmentPerson(t, b, uid)
	code, err := secret.OpenVoyageCode(p.CodeEncrypted, recipientScope(b.ID, uid))
	if err != nil {
		t.Fatal(err)
	}
	body := gin.H{"uid": uid, "code": code, "action": "view"}
	for k, v := range extra {
		body[k] = v
	}
	res := batchCall(t, r, "POST", fmt.Sprintf("/issues/%d/claim", b.ID), body, want)
	var view deliveryView
	if want == 200 {
		if err = json.Unmarshal(res.Data, &view); err != nil {
			t.Fatal(err)
		}
	}
	return view
}
func testLongTermAddress(text string) VoyageAddress {
	return VoyageAddress{Name: "离线收件人", Phone: "13800000000", Address: text, Note: "仅本批礼物备注"}
}
func TestLongTermConsentInheritanceAndRealFulfillment(t *testing.T) {
	r := longTermSetup(t)
	b1, _ := longTermBatch(t, r, true, "12345")
	a := testLongTermAddress("测试省测试市长期地址一号")
	v := longTermClaim(t, r, b1, "12345", gin.H{"action": "save", "version": 0, "address": a, "long_term_action": "set", "long_term_version": 0}, 200)
	if !v.LongTerm.Enabled || !v.LongTerm.MatchesAddress || v.LongTerm.Version != 1 {
		t.Fatalf("consent missing: %+v", v.LongTerm)
	}
	p, err := readLongTermAddress(database.DB, "12345")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p.AddressEncrypted, a.Phone) || strings.Contains(p.AddressEncrypted, a.Address) {
		t.Fatal("plaintext persisted")
	}
	if _, err = secret.OpenVoyageAddress(longTermAddressScope("67890"), p.AddressEncrypted); err == nil {
		t.Fatal("cross UID ciphertext accepted")
	}
	if _, err = secret.OpenVoyageAddress(recipientScope(b1.ID, "12345"), p.AddressEncrypted); err == nil {
		t.Fatal("profile ciphertext accepted as collection ciphertext")
	}
	b2, roster := longTermBatch(t, r, true, "12345", "67890")
	row := fulfillmentPerson(t, b2, "12345")
	d, err := readDelivery(database.DB, row)
	if err != nil {
		t.Fatal(err)
	}
	if d.Address == nil || d.Address.Address != a.Address || d.Address.Note != "" || d.AddressSource != longTermAddressSource || d.LongTermChanged || d.LongTermSavedAt == nil {
		t.Fatalf("bad inherited metadata: %+v", d)
	}
	other, _ := readDelivery(database.DB, fulfillmentPerson(t, b2, "67890"))
	if other.Address != nil {
		t.Fatal("inherited to another UID")
	}
	v = longTermClaim(t, r, b2, "12345", nil, 200)
	if !v.LongTerm.MatchesAddress || v.AddressSource != longTermAddressSource {
		t.Fatal("claim did not expose correct consent state")
	}
	before := d.Version
	if err = syncLongTermAddresses(database.DB, b2.ID); err != nil {
		t.Fatal(err)
	}
	d, _ = readDelivery(database.DB, row)
	if d.Version != before {
		t.Fatal("refresh was not idempotent")
	}
	res := batchCall(t, r, "GET", "/pending", nil, 200)
	var groups []struct {
		CollectionID      uint `json:"collection_id"`
		People, Addressed int
	}
	json.Unmarshal(res.Data, &groups)
	found := false
	for _, g := range groups {
		if g.CollectionID == b2.ID {
			found = true
			if g.People != 2 || g.Addressed != 1 {
				t.Fatal("wrong inherited counts")
			}
		}
	}
	if !found {
		t.Fatal("missing fulfillment group")
	}
	_, parcels := fulfillmentPlan(t, r, shippingSelection{CollectionID: b2.ID, RosterID: roster.ID})
	for _, parcel := range parcels {
		if parcel.UID == "12345" && parcel.AddressEncrypted == "" {
			t.Fatal("inherited address unusable for shipping")
		}
	}
	longTermClaim(t, r, b2, "12345", gin.H{"code": "wrong"}, 401)
}
func TestLongTermUpdateCancelAndManualAddressesRemainIndependent(t *testing.T) {
	r := longTermSetup(t)
	b1, _ := longTermBatch(t, r, true, "12345")
	a := testLongTermAddress("测试省测试市原长期地址")
	longTermClaim(t, r, b1, "12345", gin.H{"action": "save", "version": 0, "address": a, "long_term_action": "set", "long_term_version": 0}, 200)
	b2, _ := longTermBatch(t, r, true, "12345")
	temporary := testLongTermAddress("测试省测试市本次临时地址")
	v := longTermClaim(t, r, b2, "12345", gin.H{"action": "save", "version": 1, "address": temporary, "long_term_action": "keep"}, 200)
	if v.AddressSource != "" || !v.LongTerm.Enabled || v.LongTerm.MatchesAddress {
		t.Fatal("manual confirmation/provisional address lost")
	}
	b3, _ := longTermBatch(t, r, true, "12345")
	updated := testLongTermAddress("测试省测试市新的长期地址")
	longTermClaim(t, r, b1, "12345", gin.H{"action": "save", "version": 1, "address": updated, "long_term_action": "set", "long_term_version": 1}, 200)
	old, _ := readDelivery(database.DB, fulfillmentPerson(t, b3, "12345"))
	if old.Address.Address != a.Address || !old.LongTermChanged {
		t.Fatal("old snapshot overwritten or stale warning missing")
	}
	v = longTermClaim(t, r, b2, "12345", nil, 200)
	if v.Address.Address != temporary.Address {
		t.Fatal("manual collection overwritten")
	}
	b4, _ := longTermBatch(t, r, true, "12345")
	v = longTermClaim(t, r, b4, "12345", nil, 200)
	if v.Address.Address != updated.Address {
		t.Fatal("new collection used old preference")
	}
	database.DB.Model(&b1).Update("closed", true)
	longTermClaim(t, r, b1, "12345", gin.H{"action": "cancel_long_term", "long_term_version": 1}, 409)
	v = longTermClaim(t, r, b1, "12345", gin.H{"action": "cancel_long_term", "long_term_version": 2}, 200)
	if v.LongTerm.Enabled || v.Address.Address != updated.Address {
		t.Fatal("cancel destroyed registered address")
	}
	p, _ := readLongTermAddress(database.DB, "12345")
	if p.AddressEncrypted != "" || p.Version != 3 {
		t.Fatal("cancel retained active profile")
	}
	b5, _ := longTermBatch(t, r, true, "12345")
	v = longTermClaim(t, r, b5, "12345", nil, 200)
	if v.Address != nil {
		t.Fatal("cancelled preference still inherited")
	}
}
func TestLongTermEligibilityDatesAndDelayedOpening(t *testing.T) {
	r := longTermSetup(t)
	for _, uid := range []string{"12345", "67890"} {
		a := testLongTermAddress("测试省测试市资格验证地址")
		if err := database.DB.Transaction(func(tx *gorm.DB) error { return saveLongTermAddress(tx, uid, "set", 0, &a) }); err != nil {
			t.Fatal(err)
		}
	}
	draft, _ := longTermBatch(t, r, false, "12345")
	closed, _ := longTermBatch(t, r, false, "12345")
	future, _ := longTermBatch(t, r, false, "12345")
	disabled, _ := longTermBatch(t, r, false, "12345")
	manual, _ := longTermBatch(t, r, false, "12345")
	for _, b := range []database.VoyageBatch{closed, future, disabled, manual} {
		database.DB.Model(&b).Update("published", true)
		database.DB.Model(&database.VoyageIssue{}).Where("id = ?", b.ID).Update("published", true)
	}
	database.DB.Model(&closed).Update("closed", true)
	database.DB.Model(&future).Update("opens", "2099-01-01")
	database.DB.Model(&database.VoyageRecipient{}).Where("issue_id = ?", disabled.ID).Update("disabled", true)
	fulfillmentAddress(t, fulfillmentPerson(t, manual, "12345"), "测试省测试市已填优先地址")
	if err := syncLongTermAddresses(database.DB, 0); err != nil {
		t.Fatal(err)
	}
	for _, b := range []database.VoyageBatch{draft, closed, future, disabled} {
		d, _ := readDelivery(database.DB, fulfillmentPerson(t, b, "12345"))
		if d.Address != nil {
			t.Fatal("ineligible collection inherited")
		}
	}
	d, _ := readDelivery(database.DB, fulfillmentPerson(t, manual, "12345"))
	if d.Address.Address != "测试省测试市已填优先地址" {
		t.Fatal("existing address replaced")
	}
	database.DB.Model(&future).Update("opens", "")
	batchCall(t, r, "GET", fmt.Sprintf("/issues/%d/shipments", future.ID), nil, 200)
	d, _ = readDelivery(database.DB, fulfillmentPerson(t, future, "12345"))
	if d.AddressSource != longTermAddressSource {
		t.Fatal("scheduled collection failed to inherit on opening")
	}
	var count int64
	database.DB.Model(&database.VoyageRecipient{}).Where("uid = ?", "67890").Count(&count)
	if count != 0 {
		t.Fatal("consent created eligibility")
	}
}
func TestLongTermConcurrentPreferenceConflictRollsBackWholeSave(t *testing.T) {
	r := longTermSetup(t)
	b, _ := longTermBatch(t, r, true, "12345")
	a := testLongTermAddress("测试省测试市原地址一号")
	longTermClaim(t, r, b, "12345", gin.H{"action": "save", "address": a, "version": 0, "long_term_action": "set", "long_term_version": 0}, 200)
	changed := testLongTermAddress("测试省测试市冲突地址二号")
	longTermClaim(t, r, b, "12345", gin.H{"action": "save", "address": changed, "version": 1, "long_term_action": "set", "long_term_version": 0}, 409)
	v := longTermClaim(t, r, b, "12345", nil, 200)
	if v.Version != 1 || v.Address.Address != a.Address || v.LongTerm.Version != 1 {
		t.Fatal("conflicting save was not atomic")
	}
	// Old clients omit the new preference fields. Their save cannot revoke consent.
	v = longTermClaim(t, r, b, "12345", gin.H{"action": "save", "address": changed, "version": 1}, 200)
	if !v.LongTerm.Enabled || v.LongTerm.MatchesAddress {
		t.Fatal("old client changed long-term setting")
	}
	longTermClaim(t, r, b, "12345", gin.H{"action": "save", "address": changed, "version": 1}, 409)
}
func TestLongTermShippedSnapshotAndMessagePreviewInvalidation(t *testing.T) {
	r := longTermSetup(t)
	b1, _ := longTermBatch(t, r, true, "12345")
	a := testLongTermAddress("测试省测试市寄送前长期地址")
	longTermClaim(t, r, b1, "12345", gin.H{"action": "save", "address": a, "version": 0, "long_term_action": "set", "long_term_version": 0}, 200)
	b2, roster := longTermBatch(t, r, true, "12345")
	q := database.BilibiliMessageQueue{Scope: "voyage", ScopeID: b2.ID}
	first, err := readMessageSource(database.DB, q, "12345")
	if err != nil {
		t.Fatal(err)
	}
	_, parcels := fulfillmentPlan(t, r, shippingSelection{CollectionID: b2.ID, RosterID: roster.ID})
	parcel := parcels[0]
	fulfillmentChange(t, r, &parcel, "ship", gin.H{"courier": "离线测试快递", "tracking": "TEST123456"}, 200)
	frozen := parcel.AddressEncrypted
	changed := testLongTermAddress("测试省测试市寄送后新地址")
	longTermClaim(t, r, b1, "12345", gin.H{"action": "save", "address": changed, "version": 1, "long_term_action": "set", "long_term_version": 1}, 200)
	next, err := readMessageSource(database.DB, q, "12345")
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint == next.Fingerprint {
		t.Fatal("outdated confirmation notice remained sendable")
	}
	database.DB.First(&parcel, parcel.ID)
	if parcel.AddressEncrypted != frozen || parcel.ShippedAt == nil {
		t.Fatal("shipped snapshot changed")
	}
}
