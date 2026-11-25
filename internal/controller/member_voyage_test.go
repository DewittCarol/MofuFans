package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"mofufans/internal/database"
)

func memberVoyageSetup(t *testing.T) (*gin.Engine, func(int64, bool) []*http.Cookie) {
	t.Helper()
	r := addressBookSetup(t)
	if err := database.DB.AutoMigrate(&database.User{}); err != nil {
		t.Fatal(err)
	}
	r.Use(sessions.Sessions("session", cookie.NewStore([]byte("member-voyage-test-session-secret-32"))))
	account := &AccountController{LoginProfileReader: unavailableLoginProfileReader{}}
	members := &MemberVoyageController{}
	r.POST("/member-login", account.Login)
	r.GET("/my/book", members.AddressBook)
	r.POST("/my/book", members.SaveAddressBook)
	r.GET("/my/collections", members.Collections)
	r.POST("/my/collections/:id/claim", (&VoyageDeliveryController{}).MemberClaim)
	login := func(uid int64, strong bool) []*http.Cookie {
		hash, err := bcrypt.GenerateFromPassword([]byte("member-voyage-password"), bcrypt.MinCost)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now()
		user := database.User{BilibiliUID: uid, Username: fmt.Sprint(uid), PasswordHash: string(hash), VerifiedAt: now, AuthVersion: fmt.Sprintf("random-auth-version-%d", uid)}
		if strong {
			user.SignatureVerifiedAt = &now
		}
		if err := database.DB.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
		recorder, result := performJSONRequest(r, "POST", "/member-login", fmt.Sprintf(`{"bilibili_uid":"%d","password":"member-voyage-password"}`, uid), nil)
		if result.Code != 200 {
			t.Fatalf("member login: %+v", result)
		}
		return recorder.Result().Cookies()
	}
	return r, login
}

func memberVoyageCall(t *testing.T, r *gin.Engine, cookies []*http.Cookie, method, path string, body any, want int) testAPIResponse {
	t.Helper()
	raw, _ := json.Marshal(body)
	recorder, res := performJSONRequest(r, method, path, string(raw), cookies)
	if res.Code != want {
		t.Fatalf("%s %s wanted %d got %+v", method, path, want, res)
	}
	if !strings.Contains(recorder.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("private member response cacheable")
	}
	return res
}

func TestMemberAddressBookNeedsStrongIdentityAndKeepsSingleDefault(t *testing.T) {
	r, login := memberVoyageSetup(t)
	strong := login(12345, true)
	weak := login(67890, false)
	for _, credentials := range [][]*http.Cookie{nil, weak} {
		memberVoyageCall(t, r, credentials, "GET", "/my/book", nil, 403)
		memberVoyageCall(t, r, credentials, "POST", "/my/book", gin.H{"action": "save"}, 403)
	}
	a := testLongTermAddress("测试省测试市个人地址甲")
	a.Note = ""
	b := a
	b.Address = "测试省测试市个人地址乙"
	res := memberVoyageCall(t, r, strong, "POST", "/my/book", gin.H{"action": "save", "address": a}, 200)
	var book memberAddressBookView
	json.Unmarshal(res.Data, &book)
	if len(book.Entries) != 1 || book.LongTerm.Enabled {
		t.Fatal("saving address created implicit consent")
	}
	firstID := book.Entries[0].ID
	memberVoyageCall(t, r, strong, "POST", "/my/book", gin.H{"action": "save", "address": a}, 200)
	res = memberVoyageCall(t, r, strong, "POST", "/my/book", gin.H{"action": "save", "address": b}, 200)
	json.Unmarshal(res.Data, &book)
	if len(book.Entries) != 2 {
		t.Fatal("address dedupe failed")
	}
	res = memberVoyageCall(t, r, strong, "POST", "/my/book", gin.H{"action": "set_default", "entry_id": firstID, "long_term_version": 0}, 200)
	json.Unmarshal(res.Data, &book)
	defaults := 0
	for _, entry := range book.Entries {
		if entry.IsDefault {
			defaults++
		}
	}
	if !book.LongTerm.Enabled || defaults != 1 {
		t.Fatal("default address invariant failed")
	}
	memberVoyageCall(t, r, strong, "POST", "/my/book", gin.H{"action": "clear_default", "long_term_version": 0}, 409)
	memberVoyageCall(t, r, strong, "POST", "/my/book", gin.H{"action": "clear_default", "long_term_version": book.LongTerm.Version}, 200)
	memberVoyageCall(t, r, strong, "POST", "/my/book", gin.H{"action": "save", "uid": "67890", "address": a}, 403)
	if err := database.DB.Model(&database.User{}).Where("bilibili_uid = ?", 12345).Update("auth_version", "revoked").Error; err != nil {
		t.Fatal(err)
	}
	memberVoyageCall(t, r, strong, "GET", "/my/book", nil, 403)
}

func TestMemberCollectionsNeverExposeOtherUIDDraftsOrDisabledClaims(t *testing.T) {
	r, login := memberVoyageSetup(t)
	own := login(12345, true)
	open, _ := longTermBatch(t, r, true, "12345", "67890")
	draft, _ := longTermBatch(t, r, false, "12345")
	other, _ := longTermBatch(t, r, true, "67890")
	res := memberVoyageCall(t, r, own, "GET", "/my/collections", nil, 200)
	var rows []struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(res.Data, &rows)
	if len(rows) != 1 || rows[0].ID != open.ID {
		t.Fatalf("wrong own collections: %s", res.Data)
	}
	path := fmt.Sprintf("/my/collections/%d/claim", open.ID)
	memberVoyageCall(t, r, own, "POST", path, gin.H{"action": "view"}, 200)
	memberVoyageCall(t, r, own, "POST", path, gin.H{"action": "view", "uid": "67890"}, 403)
	for _, id := range []uint{draft.ID, other.ID, 99999} {
		memberVoyageCall(t, r, own, "POST", fmt.Sprintf("/my/collections/%d/claim", id), gin.H{"action": "view"}, 401)
	}
	a := testLongTermAddress("测试省测试市会员批次地址")
	res = memberVoyageCall(t, r, own, "POST", path, gin.H{"action": "save", "version": 0, "address": a}, 200)
	var view deliveryView
	json.Unmarshal(res.Data, &view)
	if view.Address == nil || view.Address.Address != a.Address {
		t.Fatal("member address not saved")
	}
	memberVoyageCall(t, r, own, "POST", path, gin.H{"action": "save", "version": 0, "address": a}, 409)
	p := fulfillmentPerson(t, open, "12345")
	if err := database.DB.Model(&p).Update("disabled", true).Error; err != nil {
		t.Fatal(err)
	}
	memberVoyageCall(t, r, own, "POST", path, gin.H{"action": "view"}, 401)
	res = memberVoyageCall(t, r, own, "GET", "/my/collections", nil, 200)
	json.Unmarshal(res.Data, &rows)
	if len(rows) != 0 {
		t.Fatal("disabled recipient remained visible")
	}
}

func TestMemberDefaultInheritsOnlyEligibleBlankAddressesAndStopPreservesSnapshot(t *testing.T) {
	r, login := memberVoyageSetup(t)
	own := login(12345, true)
	other := login(67890, true)
	a := testLongTermAddress("测试省测试市提前登记地址")
	a.Note = ""
	res := memberVoyageCall(t, r, own, "POST", "/my/book", gin.H{"action": "save", "address": a}, 200)
	var book memberAddressBookView
	json.Unmarshal(res.Data, &book)
	id := book.Entries[0].ID
	memberVoyageCall(t, r, other, "POST", "/my/book", gin.H{"action": "set_default", "entry_id": id, "long_term_version": 0}, 409)
	res = memberVoyageCall(t, r, other, "GET", "/my/book", nil, 200)
	if strings.Contains(string(res.Data), a.Address) {
		t.Fatal("other UID read address")
	}
	memberVoyageCall(t, r, own, "POST", "/my/book", gin.H{"action": "set_default", "entry_id": id, "long_term_version": 0}, 200)
	batch, _ := longTermBatch(t, r, true, "12345", "67890")
	row := fulfillmentPerson(t, batch, "12345")
	snap, err := readDelivery(database.DB, row)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Address == nil || snap.Address.Address != a.Address || snap.Address.Note != "" {
		t.Fatal("own eligible batch did not inherit clean address")
	}
	otherSnap, _ := readDelivery(database.DB, fulfillmentPerson(t, batch, "67890"))
	if otherSnap.Address != nil {
		t.Fatal("other UID inherited")
	}
	memberVoyageCall(t, r, own, "POST", "/my/book", gin.H{"action": "clear_default", "long_term_version": 1}, 200)
	res = memberVoyageCall(t, r, own, "POST", fmt.Sprintf("/my/collections/%d/claim", batch.ID), gin.H{"action": "view"}, 200)
	var view deliveryView
	json.Unmarshal(res.Data, &view)
	if view.Address == nil || !view.LongTermChanged {
		t.Fatal("stop erased existing snapshot or failed to flag consent change")
	}
	newBatch, _ := longTermBatch(t, r, true, "12345")
	newSnap, _ := readDelivery(database.DB, fulfillmentPerson(t, newBatch, "12345"))
	if newSnap.Address != nil {
		t.Fatal("cancelled consent still inherited")
	}
}

func TestMemberShippingHidesDraftTrackingAndOtherRecipients(t *testing.T) {
	r, login := memberVoyageSetup(t)
	own := login(12345, true)
	other := login(67890, true)
	batch, roster := longTermBatch(t, r, true, "12345", "67890")
	person := fulfillmentPerson(t, batch, "12345")
	fulfillmentAddress(t, person, "测试省测试市待发货地址123")
	_, parcels := fulfillmentPlan(t, r, shippingSelection{CollectionID: batch.ID, RosterID: roster.ID})
	var p database.VoyageParcel
	for _, parcel := range parcels {
		if parcel.UID == "12345" {
			p = parcel
		}
	}
	if p.ID == 0 {
		t.Fatal("missing parcel")
	}
	fulfillmentChange(t, r, &p, "draft", gin.H{"courier": "测试快递", "tracking": "TEST-PRIVATE-12345"}, 200)
	path := fmt.Sprintf("/my/collections/%d/claim", batch.ID)
	res := memberVoyageCall(t, r, own, "POST", path, gin.H{"action": "view"}, 200)
	if strings.Contains(string(res.Data), "TEST-PRIVATE-12345") {
		t.Fatal("draft tracking leaked")
	}
	fulfillmentChange(t, r, &p, "ship", gin.H{"courier": "测试快递", "tracking": "TEST-PRIVATE-12345"}, 200)
	res = memberVoyageCall(t, r, own, "POST", path, gin.H{"action": "view"}, 200)
	if !strings.Contains(string(res.Data), "TEST-PRIVATE-12345") {
		t.Fatal("confirmed tracking unavailable to owner")
	}
	res = memberVoyageCall(t, r, other, "POST", path, gin.H{"action": "view"}, 200)
	if strings.Contains(string(res.Data), "TEST-PRIVATE-12345") || strings.Contains(string(res.Data), "待发货地址") {
		t.Fatal("other member received shipping details")
	}
	res = memberVoyageCall(t, r, own, "POST", path, gin.H{"action": "view"}, 200)
	var view deliveryView
	json.Unmarshal(res.Data, &view)
	// An open collection still accepts a later address, but the sent parcel
	// remains an immutable snapshot. Closing the collection prevents edits.
	before := contentParcel(t, p.ID)
	memberVoyageCall(t, r, own, "POST", path, gin.H{"action": "save", "version": view.Version, "address": testLongTermAddress("测试省测试市寄出后新地址")}, 200)
	after := contentParcel(t, p.ID)
	if before.AddressEncrypted != after.AddressEncrypted {
		t.Fatal("member edit changed a sent parcel address")
	}
	if err := database.DB.Model(&batch).Update("closed", true).Error; err != nil {
		t.Fatal(err)
	}
	res = memberVoyageCall(t, r, own, "POST", path, gin.H{"action": "view"}, 200)
	json.Unmarshal(res.Data, &view)
	memberVoyageCall(t, r, own, "POST", path, gin.H{"action": "save", "version": view.Version, "address": testLongTermAddress("测试省测试市不能覆盖已发货")}, 409)
}

func TestMemberClaimDoesNotRevealUnpublishedCollectionHistory(t *testing.T) {
	r, login := memberVoyageSetup(t)
	own := login(12345, true)
	visible, _ := longTermBatch(t, r, true, "12345")
	hidden, _ := longTermBatch(t, r, true, "12345")
	secretTitle := "RETRACTED-COLLECTION-TITLE"
	if err := database.DB.Model(&database.VoyageIssue{}).Where("id = ?", hidden.ID).Update("title", secretTitle).Error; err != nil {
		t.Fatal(err)
	}
	a := testLongTermAddress("测试省测试市历史地址")
	a.Note = "RETRACTED-COLLECTION-NOTE"
	memberVoyageCall(t, r, own, "POST", fmt.Sprintf("/my/collections/%d/claim", hidden.ID), gin.H{"action": "save", "address": a, "version": 0}, 200)
	if err := database.DB.Model(&hidden).Update("published", false).Error; err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{fmt.Sprintf("/my/collections/%d/claim", visible.ID), "/my/book"} {
		method := "POST"
		var body any = gin.H{"action": "view"}
		if path == "/my/book" {
			method = "GET"
			body = nil
		}
		res := memberVoyageCall(t, r, own, method, path, body, 200)
		if strings.Contains(string(res.Data), secretTitle) || strings.Contains(string(res.Data), a.Note) {
			t.Fatal("unpublished collection history disclosed")
		}
	}
}

func TestMemberReadsRejectStalePageSubject(t *testing.T) {
	r, login := memberVoyageSetup(t)
	own := login(12345, true)
	for _, path := range []string{"/my/book", "/my/collections"} {
		memberVoyageCall(t, r, own, "GET", path+"?expected_uid=67890", nil, 403)
		memberVoyageCall(t, r, own, "GET", path+"?expected_uid=12345", nil, 200)
	}
}
