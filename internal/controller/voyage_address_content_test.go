package controller

import (
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mofufans/internal/database"
)

type addressContentState struct {
	Deliveries  []database.VoyageDelivery
	Preferences []database.VoyageLongTermAddress
	Parcels     []database.VoyageParcel
}

func captureAddressContentState(t *testing.T) addressContentState {
	t.Helper()
	var state addressContentState
	for _, query := range []struct {
		order string
		rows  any
	}{
		{"recipient_id", &state.Deliveries}, {"uid", &state.Preferences}, {"id", &state.Parcels},
	} {
		if err := database.DB.Order(query.order).Find(query.rows).Error; err != nil {
			t.Fatal(err)
		}
	}
	return state
}

func contentDelivery(t *testing.T, recipientID uint) database.VoyageDelivery {
	t.Helper()
	var row database.VoyageDelivery
	if err := database.DB.Where("recipient_id = ?", recipientID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func contentParcel(t *testing.T, id uint) database.VoyageParcel {
	t.Helper()
	var row database.VoyageParcel
	if err := database.DB.First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func TestLongTermSetUsesSavedAddressWithoutChangingDeliveryOrParcel(t *testing.T) {
	r := longTermSetup(t)
	b, roster := longTermBatch(t, r, true, "12345")
	a := testLongTermAddress("测试省测试市服务器已存地址")
	longTermClaim(t, r, b, "12345", gin.H{"action": "save", "version": 0, "address": a}, 200)
	person := fulfillmentPerson(t, b, "12345")
	_, parcels := fulfillmentPlan(t, r, shippingSelection{CollectionID: b.ID, RosterID: roster.ID})
	beforeDelivery, beforeParcel := contentDelivery(t, person.ID), parcels[0]
	next, _ := longTermBatch(t, r, true, "12345")
	// A request may contain a stale or malicious draft. The toggle must ignore it.
	draft := testLongTermAddress("测试省测试市请求草稿不得保存")
	v := longTermClaim(t, r, b, "12345", gin.H{"action": "set_long_term", "version": 1, "long_term_version": 0, "address": draft, "long_term_action": "clear"}, 200)
	if !v.LongTerm.Enabled || v.LongTerm.Version != 1 || !v.LongTerm.MatchesAddress || !sameVoyageAddress(v.Address, &a, true) {
		t.Fatal("toggle did not use the server's saved address")
	}
	if !reflect.DeepEqual(beforeDelivery, contentDelivery(t, person.ID)) || !reflect.DeepEqual(beforeParcel, contentParcel(t, beforeParcel.ID)) {
		t.Fatal("pure preference toggle changed the saved delivery or parcel snapshot")
	}
	p, err := readLongTermAddress(database.DB, "12345")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := openLongTermAddress(p)
	if err != nil || !sameVoyageAddress(&stored, &a, false) || stored.Note != "" {
		t.Fatal("long-term content or note scope is wrong")
	}
	filled, err := readDelivery(database.DB, fulfillmentPerson(t, next, "12345"))
	if err != nil || filled.AddressSource != longTermAddressSource || !sameVoyageAddress(filled.Address, &a, false) || filled.Address.Note != "" {
		t.Fatal("eligible blank collection was not filled correctly")
	}
	beforeRepeat := captureAddressContentState(t)
	longTermClaim(t, r, b, "12345", gin.H{"action": "set_long_term", "version": 1, "long_term_version": 1}, 200)
	if !reflect.DeepEqual(beforeRepeat, captureAddressContentState(t)) {
		t.Fatal("repeated same preference toggle changed persisted state")
	}
}

func TestLongTermReenableNoticeUsesContentNotConsentVersion(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*VoyageAddress)
		changed bool
	}{
		{"same", func(*VoyageAddress) {}, false},
		{"batch-note-only", func(a *VoyageAddress) { a.Note = "本批的新备注" }, false},
		{"name", func(a *VoyageAddress) { a.Name = "另一位收件人" }, true},
		{"phone", func(a *VoyageAddress) { a.Phone = "13900000000" }, true},
		{"address", func(a *VoyageAddress) { a.Address = "测试省测试市另一地址" }, true},
		{"meaningful-spacing", func(a *VoyageAddress) { a.Address = "测试省 测试市原地址" }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := longTermSetup(t)
			b, _ := longTermBatch(t, r, true, "12345")
			a := testLongTermAddress("测试省测试市原地址")
			longTermClaim(t, r, b, "12345", gin.H{"action": "save", "version": 0, "address": a, "long_term_action": "set", "long_term_version": 0}, 200)
			next, _ := longTermBatch(t, r, true, "12345")
			nextPerson := fulfillmentPerson(t, next, "12345")
			before := contentDelivery(t, nextPerson.ID)
			longTermClaim(t, r, b, "12345", gin.H{"action": "cancel_long_term", "long_term_version": 1}, 200)
			cancelled, err := readDelivery(database.DB, nextPerson)
			if err != nil || !cancelled.LongTermChanged {
				t.Fatal("disabled preference must keep the warning")
			}
			updated := a
			tc.mutate(&updated)
			saved := longTermClaim(t, r, b, "12345", gin.H{"action": "save", "version": 1, "address": updated}, 200)
			reenabled := longTermClaim(t, r, b, "12345", gin.H{"action": "set_long_term", "version": saved.Version, "long_term_version": 2}, 200)
			if !reenabled.LongTerm.Enabled || reenabled.LongTerm.Version != 3 {
				t.Fatal("preference was not re-enabled")
			}
			after, err := readDelivery(database.DB, nextPerson)
			if err != nil || after.LongTermChanged != tc.changed {
				t.Fatalf("wrong content-based warning: changed=%v err=%v", after.LongTermChanged, err)
			}
			if !reflect.DeepEqual(before, contentDelivery(t, nextPerson.ID)) {
				t.Fatal("re-enabling changed the independent inherited snapshot")
			}
		})
	}
}

func TestLongTermSetRejectsMissingSavedAddress(t *testing.T) {
	r := longTermSetup(t)
	b, _ := longTermBatch(t, r, true, "12345")
	before := captureAddressContentState(t)
	longTermClaim(t, r, b, "12345", gin.H{"action": "set_long_term", "version": 0, "long_term_version": 0, "address": testLongTermAddress("草稿不能代替已保存地址")}, 400)
	if !reflect.DeepEqual(before, captureAddressContentState(t)) {
		t.Fatal("rejected toggle created an address or preference")
	}
}

func TestLongTermSetRejectsInvalidCredentialsEligibilityAndStaleVersions(t *testing.T) {
	cases := []struct {
		name   string
		change func(*testing.T, database.VoyageBatch, gin.H)
		want   int
	}{
		{"wrong-code", func(_ *testing.T, _ database.VoyageBatch, body gin.H) { body["code"] = "wrong" }, 401},
		{"other-uid", func(_ *testing.T, _ database.VoyageBatch, body gin.H) { body["uid"] = "67890" }, 401},
		{"stale-address", func(_ *testing.T, _ database.VoyageBatch, body gin.H) { body["version"] = 0 }, 409},
		{"stale-preference", func(_ *testing.T, _ database.VoyageBatch, body gin.H) { body["long_term_version"] = 0 }, 409},
		{"disabled", func(t *testing.T, b database.VoyageBatch, _ gin.H) {
			if err := database.DB.Model(&database.VoyageRecipient{}).Where("issue_id = ?", b.ID).Update("disabled", true).Error; err != nil {
				t.Fatal(err)
			}
		}, 401},
		{"unpublished", func(t *testing.T, b database.VoyageBatch, _ gin.H) {
			if err := database.DB.Model(&b).Update("published", false).Error; err != nil {
				t.Fatal(err)
			}
		}, 401},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := longTermSetup(t)
			b, _ := longTermBatch(t, r, true, "12345")
			a := testLongTermAddress("测试省测试市保持原地址")
			longTermClaim(t, r, b, "12345", gin.H{"action": "save", "version": 0, "address": a, "long_term_action": "set", "long_term_version": 0}, 200)
			body := gin.H{"action": "set_long_term", "version": 1, "long_term_version": 1}
			tc.change(t, b, body)
			before := captureAddressContentState(t)
			longTermClaim(t, r, b, "12345", body, tc.want)
			if !reflect.DeepEqual(before, captureAddressContentState(t)) {
				t.Fatal("rejected toggle changed persistent address state")
			}
		})
	}
}

func TestAddressSameSaveKeepsFingerprintAndSourceConfirmationOnlyChangesSource(t *testing.T) {
	r := longTermSetup(t)
	b, _ := longTermBatch(t, r, true, "12345")
	a := testLongTermAddress("测试省测试市地址内容不变")
	longTermClaim(t, r, b, "12345", gin.H{"action": "save", "version": 0, "address": a, "long_term_action": "set", "long_term_version": 0}, 200)
	person := fulfillmentPerson(t, b, "12345")
	before := contentDelivery(t, person.ID)
	queue := database.BilibiliMessageQueue{Scope: "voyage", ScopeID: b.ID}
	message, err := readMessageSource(database.DB, queue, "12345")
	if err != nil {
		t.Fatal(err)
	}
	longTermClaim(t, r, b, "12345", gin.H{"action": "save", "version": 1, "address": a}, 200)
	afterMessage, err := readMessageSource(database.DB, queue, "12345")
	if err != nil || message.Fingerprint != afterMessage.Fingerprint || !reflect.DeepEqual(before, contentDelivery(t, person.ID)) {
		t.Fatal("unchanged manual save invalidated its message preview or address")
	}
	// An inherited address is explicitly confirmed by saving, but its address
	// version, ciphertext and submitted timestamp still describe the same content.
	next, roster := longTermBatch(t, r, true, "12345")
	nextPerson := fulfillmentPerson(t, next, "12345")
	_, parcels := fulfillmentPlan(t, r, shippingSelection{CollectionID: next.ID, RosterID: roster.ID})
	beforeInherited := contentDelivery(t, nextPerson.ID)
	inherited, err := readDelivery(database.DB, nextPerson)
	if err != nil {
		t.Fatal(err)
	}
	queue.ScopeID = next.ID
	message, err = readMessageSource(database.DB, queue, "12345")
	if err != nil {
		t.Fatal(err)
	}
	longTermClaim(t, r, next, "12345", gin.H{"action": "save", "version": 1, "address": inherited.Address}, 200)
	afterInherited := contentDelivery(t, nextPerson.ID)
	if afterInherited.AddressSource != "" || afterInherited.LongTermVersion != 0 || afterInherited.LongTermSavedAt != nil {
		t.Fatal("manual confirmation retained inherited source")
	}
	expected := beforeInherited
	expected.AddressSource, expected.LongTermVersion, expected.LongTermSavedAt = "", 0, nil
	if !reflect.DeepEqual(expected, afterInherited) {
		t.Fatal("same-address confirmation changed more than source metadata")
	}
	afterMessage, err = readMessageSource(database.DB, queue, "12345")
	if err != nil || message.Fingerprint == afterMessage.Fingerprint {
		t.Fatal("source confirmation failed to invalidate the old inherited notice")
	}
	v, err := parcelDetails(database.DB, parcels[0])
	if err != nil || v.AddressChanged {
		t.Fatal("source-only confirmation reported an address content change")
	}
}

func TestAddressNoOpStillRejectsStaleVersionsAndRollsBackPreferenceConflict(t *testing.T) {
	r := longTermSetup(t)
	b, _ := longTermBatch(t, r, true, "12345")
	a := testLongTermAddress("测试省测试市地址恢复后仍检查版本")
	longTermClaim(t, r, b, "12345", gin.H{"action": "save", "version": 0, "address": a, "long_term_action": "set", "long_term_version": 0}, 200)
	changed := a
	changed.Address = "测试省测试市中间地址"
	longTermClaim(t, r, b, "12345", gin.H{"action": "save", "version": 1, "address": changed}, 200)
	longTermClaim(t, r, b, "12345", gin.H{"action": "save", "version": 2, "address": a}, 200)
	before := captureAddressContentState(t)
	longTermClaim(t, r, b, "12345", gin.H{"action": "save", "version": 1, "address": a}, 409)
	longTermClaim(t, r, b, "12345", gin.H{"action": "set_long_term", "version": 1, "long_term_version": 1}, 409)
	longTermClaim(t, r, b, "12345", gin.H{"action": "save", "version": 3, "address": a, "long_term_action": "set", "long_term_version": 0}, 409)
	if !reflect.DeepEqual(before, captureAddressContentState(t)) {
		t.Fatal("stale no-op request changed state")
	}
	next, _ := longTermBatch(t, r, true, "12345")
	v, err := readDelivery(database.DB, fulfillmentPerson(t, next, "12345"))
	if err != nil {
		t.Fatal(err)
	}
	before = captureAddressContentState(t)
	longTermClaim(t, r, next, "12345", gin.H{"action": "save", "version": 1, "address": v.Address, "long_term_action": "set", "long_term_version": 0}, 409)
	if !reflect.DeepEqual(before, captureAddressContentState(t)) {
		t.Fatal("preference conflict did not roll back source-only confirmation")
	}
}

func TestParcelAddressContentDetectsRealChangesIncludingNotes(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*VoyageAddress)
		changed bool
	}{
		{"same", func(*VoyageAddress) {}, false},
		{"name", func(a *VoyageAddress) { a.Name = "新收件人" }, true},
		{"phone", func(a *VoyageAddress) { a.Phone = "13900000000" }, true},
		{"address", func(a *VoyageAddress) { a.Address = "测试省测试市新地址" }, true},
		{"note", func(a *VoyageAddress) { a.Note = "新的打包备注" }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := longTermSetup(t)
			b, roster := longTermBatch(t, r, true, "12345")
			a := testLongTermAddress("测试省测试市包裹原地址")
			longTermClaim(t, r, b, "12345", gin.H{"action": "save", "version": 0, "address": a}, 200)
			_, parcels := fulfillmentPlan(t, r, shippingSelection{CollectionID: b.ID, RosterID: roster.ID})
			p := parcels[0]
			frozen := p.AddressEncrypted
			updated := a
			tc.mutate(&updated)
			v := longTermClaim(t, r, b, "12345", gin.H{"action": "save", "version": 1, "address": updated}, 200)
			wantVersion := 1
			if tc.changed {
				wantVersion = 2
			}
			if v.Version != wantVersion {
				t.Fatal("save version does not reflect content change")
			}
			details, err := parcelDetails(database.DB, p)
			if err != nil || details.AddressChanged != tc.changed {
				t.Fatal("parcel content-change decision is wrong")
			}
			if tc.changed {
				fulfillmentChange(t, r, &p, "ship", gin.H{"courier": "离线快递", "tracking": "TEST123456"}, 409)
				err = database.DB.Transaction(func(tx *gorm.DB) error {
					_, err := changeParcel(tx, p.PlanID, "export", parcelChange{ID: p.ID, Version: p.Version})
					return err
				})
				if err == nil {
					t.Fatal("export accepted a real unconfirmed address change")
				}
				fulfillmentChange(t, r, &p, "address", gin.H{"address_recipient_id": p.AddressRecipientID, "address_version": 1}, 409)
				fulfillmentChange(t, r, &p, "address", gin.H{"address_recipient_id": p.AddressRecipientID, "address_version": 2}, 200)
			} else {
				// Existing parcels from an older release may retain an older address
				// version even though their complete snapshot matches current content.
				person := fulfillmentPerson(t, b, "12345")
				if err = database.DB.Model(&database.VoyageDelivery{}).Where("recipient_id = ?", person.ID).Update("version", 2).Error; err != nil {
					t.Fatal(err)
				}
				details, err = parcelDetails(database.DB, p)
				if err != nil || details.AddressChanged {
					t.Fatal("legacy same-content version difference caused a false warning")
				}
				err = database.DB.Transaction(func(tx *gorm.DB) error {
					_, err := changeParcel(tx, p.PlanID, "export", parcelChange{ID: p.ID, Version: p.Version})
					return err
				})
				if err != nil {
					t.Fatal(err)
				}
				p = contentParcel(t, p.ID)
				if p.AddressVersion != 1 || p.AddressEncrypted != frozen {
					t.Fatal("export rewrote the same-content snapshot")
				}
			}
			fulfillmentChange(t, r, &p, "ship", gin.H{"courier": "离线快递", "tracking": "TEST123456"}, 200)
			if p.ShippedAt == nil {
				t.Fatal("confirmed content could not ship")
			}
		})
	}
}

func TestLongTermToggleAfterClosurePreservesShippedSnapshotAndCancelCompatibility(t *testing.T) {
	r := longTermSetup(t)
	b, roster := longTermBatch(t, r, true, "12345")
	a := testLongTermAddress("测试省测试市已寄地址")
	longTermClaim(t, r, b, "12345", gin.H{"action": "save", "version": 0, "address": a}, 200)
	_, parcels := fulfillmentPlan(t, r, shippingSelection{CollectionID: b.ID, RosterID: roster.ID})
	p := parcels[0]
	fulfillmentChange(t, r, &p, "ship", gin.H{"courier": "离线快递", "tracking": "TEST123456"}, 200)
	// Later collection edits must never rewrite or flag the shipped snapshot.
	changed := a
	changed.Address = "测试省测试市寄出后新地址"
	longTermClaim(t, r, b, "12345", gin.H{"action": "save", "version": 1, "address": changed}, 200)
	if err := database.DB.Model(&b).Update("closed", true).Error; err != nil {
		t.Fatal(err)
	}
	person := fulfillmentPerson(t, b, "12345")
	beforeDelivery, beforeParcel := contentDelivery(t, person.ID), contentParcel(t, p.ID)
	v := longTermClaim(t, r, b, "12345", gin.H{"action": "set_long_term", "version": 2, "long_term_version": 0}, 200)
	if v.CanEditAddress || !v.LongTerm.Enabled {
		t.Fatal("pure preference toggle should work independently of address editing")
	}
	if !reflect.DeepEqual(beforeDelivery, contentDelivery(t, person.ID)) || !reflect.DeepEqual(beforeParcel, contentParcel(t, p.ID)) {
		t.Fatal("closed toggle changed delivery or shipped parcel")
	}
	details, err := parcelDetails(database.DB, beforeParcel)
	if err != nil || details.AddressChanged {
		t.Fatal("shipped parcel reported a later address change")
	}
	// Legacy cancel has no delivery version requirement and remains available
	// through an eligible collection whose address has never been registered.
	empty, _ := longTermBatch(t, r, false, "12345")
	if err = database.DB.Model(&empty).Updates(map[string]any{"published": true, "closed": true}).Error; err != nil {
		t.Fatal(err)
	}
	longTermClaim(t, r, empty, "12345", gin.H{"action": "cancel_long_term", "long_term_version": 1}, 200)
	if d, err := readDelivery(database.DB, fulfillmentPerson(t, empty, "12345")); err != nil || d.Address != nil {
		t.Fatal("cancel filled or required an empty collection address")
	}
	if !reflect.DeepEqual(beforeParcel, contentParcel(t, p.ID)) {
		t.Fatal("cancellation changed shipped snapshot")
	}
}
