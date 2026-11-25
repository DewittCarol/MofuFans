package controller

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mofufans/internal/database"
)

func addressBookSetup(t *testing.T) *gin.Engine {
	t.Helper()
	r := longTermSetup(t)
	if err := database.InitializeVoyageAddressBook(); err != nil {
		t.Fatal(err)
	}
	ctl := &VoyageAddressBookController{}
	r.GET("/book", ctl.List)
	r.GET("/book/:uid", ctl.Detail)
	r.POST("/book/:uid/action", func(c *gin.Context) { admin := database.Admin{}; admin.ID = 1; c.Set("user", admin); c.Next() }, ctl.Action)
	r.POST("/anonymous/:uid/action", ctl.Action)
	return r
}
func readTestBook(t *testing.T, r *gin.Engine, uid string) []addressBookEntryView {
	t.Helper()
	response := batchCall(t, r, "GET", "/book/"+uid, nil, 200)
	var data struct {
		Entries []addressBookEntryView `json:"entries"`
	}
	if err := json.Unmarshal(response.Data, &data); err != nil {
		t.Fatal(err)
	}
	return data.Entries
}
func bookAdminRequest(action string, entryID uint, version int) gin.H {
	return gin.H{"action": action, "entry_id": entryID, "version": version, "confirmed": true, "confirmed_at": time.Now(), "channel": "B站私信"}
}
func TestAddressBookKeepsMultipleAddressesButOnlyOneDefault(t *testing.T) {
	r := addressBookSetup(t)
	b, _ := longTermBatch(t, r, true, "12345", "67890")
	a := testLongTermAddress("离线地址甲省甲市一号")
	a.Note = "仅甲批次备注"
	v := longTermClaim(t, r, b, "12345", gin.H{"action": "save", "version": 0, "address": a}, 200)
	if v.LongTerm.Enabled || len(v.AddressBook) != 1 {
		t.Fatal("history silently enabled default")
	}
	idA := v.AddressBook[0].ID
	a.Address = "离线地址乙省乙市二号"
	a.Note = "仅乙批次备注"
	v = longTermClaim(t, r, b, "12345", gin.H{"action": "save", "version": 1, "address": a}, 200)
	if len(v.AddressBook) != 2 {
		t.Fatal("new address overwrote history")
	}
	idB := v.AddressBook[0].ID
	v = longTermClaim(t, r, b, "12345", gin.H{"action": "set_book_default", "book_entry_id": idA, "long_term_version": 0}, 200)
	if !v.LongTerm.Enabled || v.LongTerm.MatchesAddress || v.Address.Address != a.Address || v.Version != 2 {
		t.Fatal("default selection overwrote current batch")
	}
	v = longTermClaim(t, r, b, "12345", gin.H{"action": "set_book_default", "book_entry_id": idB, "long_term_version": 1}, 200)
	defaults := 0
	for _, entry := range v.AddressBook {
		if entry.IsDefault {
			defaults++
			if entry.ID != idB {
				t.Fatal("wrong default")
			}
		}
		if entry.Address.Note != "" {
			t.Fatal("batch note entered default address")
		}
	}
	if defaults != 1 || !v.LongTerm.MatchesAddress {
		t.Fatal("multiple or missing default")
	}
	response := batchCall(t, r, "GET", "/book", nil, 200)
	var summary struct {
		Total int `json:"total"`
		Rows  []struct {
			Count int `json:"count"`
		} `json:"rows"`
	}
	json.Unmarshal(response.Data, &summary)
	if summary.Total != 1 || len(summary.Rows) != 1 || summary.Rows[0].Count != 2 {
		t.Fatal("multiple addresses were counted as multiple people")
	}
	longTermClaim(t, r, b, "67890", gin.H{"action": "set_book_default", "book_entry_id": idA, "long_term_version": 0}, 409)
	other := readTestBook(t, r, "67890")
	if len(other) != 0 {
		t.Fatal("cross-UID selection created history")
	}
	v = longTermClaim(t, r, b, "12345", gin.H{"action": "cancel_long_term", "long_term_version": 2}, 200)
	for _, entry := range v.AddressBook {
		if entry.IsDefault {
			t.Fatal("revoked default is still active")
		}
	}
	if len(v.AddressBook) != 2 {
		t.Fatal("revocation erased address book")
	}
	var stored []database.VoyageAddressBookEntry
	database.DB.Find(&stored)
	for _, entry := range stored {
		if strings.Contains(entry.AddressEncrypted, a.Phone) || strings.Contains(entry.AddressEncrypted, a.Address) {
			t.Fatal("plaintext stored")
		}
		copy := entry
		copy.UID = "67890"
		if _, err := openBookEntry(copy); err == nil {
			t.Fatal("ciphertext accepted for other UID")
		}
	}
}
func TestAddressBookCustodyRevocationAndFreshConsent(t *testing.T) {
	r := addressBookSetup(t)
	b, _ := longTermBatch(t, r, true, "12345")
	a := testLongTermAddress("离线托管地址省市一号")
	v := longTermClaim(t, r, b, "12345", gin.H{"action": "save", "version": 0, "address": a}, 200)
	entry := v.AddressBook[0]
	next, _ := longTermBatch(t, r, true, "12345")
	row := fulfillmentPerson(t, next, "12345")
	readTestBook(t, r, "12345")
	d, _ := readDelivery(database.DB, row)
	if d.Address != nil {
		t.Fatal("book query materialized an address without permission")
	}
	body := bookAdminRequest("set_default", entry.ID, 0)
	batchCall(t, r, "POST", "/anonymous/12345/action", body, 401)
	batchCall(t, r, "POST", "/book/12345/action", body, 200)
	p, _ := readLongTermAddress(database.DB, "12345")
	if !p.Enabled || p.Source != "admin" || p.AdminID != 1 || p.EntryID != entry.ID {
		t.Fatal("custody source not stored")
	}
	d, _ = readDelivery(database.DB, row)
	if d.Address == nil || d.Address.Note != "" || d.LongTermOrigin != "admin" {
		t.Fatal("custody did not fill qualified empty batch without notes")
	}
	longTermClaim(t, r, next, "12345", gin.H{"action": "cancel_long_term", "long_term_version": 1}, 200)
	d, _ = readDelivery(database.DB, row)
	if d.Address == nil || !d.LongTermChanged {
		t.Fatal("stop erased filled address or hid warning")
	}
	third, _ := longTermBatch(t, r, true, "12345")
	thirdRow := fulfillmentPerson(t, third, "12345")
	d, _ = readDelivery(database.DB, thirdRow)
	if d.Address != nil {
		t.Fatal("custody survived user revocation")
	}
	batchCall(t, r, "POST", "/book/12345/action", body, 409)
	body["version"] = 2
	batchCall(t, r, "POST", "/book/12345/action", body, 409)
	body["confirmed_at"] = time.Now()
	batchCall(t, r, "POST", "/book/12345/action", body, 200)
	d, _ = readDelivery(database.DB, thirdRow)
	if d.Address == nil {
		t.Fatal("new confirmation could not restart custody")
	}
	d, _ = readDelivery(database.DB, row)
	if d.LongTermChanged {
		t.Fatal("same address restart falsely says address changed")
	}
	var events []database.VoyageAddressConsent
	database.DB.Order("id").Find(&events)
	if len(events) != 3 || events[0].Action != "admin_set" || events[1].Action != "user_clear" || events[2].Action != "admin_set" {
		t.Fatal("consent audit missing")
	}
}
func TestAddressBookOneTimeConfirmationDoesNotEnableFutureFills(t *testing.T) {
	r := addressBookSetup(t)
	b, _ := longTermBatch(t, r, true, "12345", "67890")
	a := testLongTermAddress("离线单次确认省市地址一号")
	v := longTermClaim(t, r, b, "12345", gin.H{"action": "save", "version": 0, "address": a}, 200)
	next, _ := longTermBatch(t, r, true, "12345")
	row := fulfillmentPerson(t, next, "12345")
	body := bookAdminRequest("confirm_once", v.AddressBook[0].ID, 0)
	body["recipient_id"] = row.ID
	body["delivery_version"] = 0
	body["confirmed"] = false
	batchCall(t, r, "POST", "/book/12345/action", body, 400)
	body["confirmed"] = true
	batchCall(t, r, "POST", "/book/12345/action", body, 200)
	d, _ := readDelivery(database.DB, row)
	if d.Address == nil || d.AddressSource != "admin_confirmed" || d.Address.Note != "" {
		t.Fatal("one-time address invalid")
	}
	p, _ := readLongTermAddress(database.DB, "12345")
	if p.Enabled || p.Version != 0 {
		t.Fatal("one-time use changed permission")
	}
	third, _ := longTermBatch(t, r, true, "12345")
	d, _ = readDelivery(database.DB, fulfillmentPerson(t, third, "12345"))
	if d.Address != nil {
		t.Fatal("one-time use filled another batch")
	}
	batchCall(t, r, "POST", "/book/12345/action", body, 409)
	body["delivery_version"] = 1
	batchCall(t, r, "POST", "/book/12345/action", body, 409)
	batchCall(t, r, "POST", "/book/67890/action", body, 409)
	response := batchCall(t, r, "GET", "/book", nil, 200)
	var list struct {
		Total int `json:"total"`
		Rows  []struct {
			UID   string `json:"uid"`
			Count int    `json:"count"`
		} `json:"rows"`
	}
	json.Unmarshal(response.Data, &list)
	if list.Total != 1 || len(list.Rows) != 1 || list.Rows[0].UID != "12345" || list.Rows[0].Count != 1 {
		t.Fatalf("bad UID summary: %s", response.Data)
	}
	if strings.Contains(string(response.Data), a.Phone) || strings.Contains(string(response.Data), a.Address) {
		t.Fatal("address leaked into summary list")
	}
}
func TestAddressBookClaimRevocationAndBatchNotesStayPrivate(t *testing.T) {
	r := addressBookSetup(t)
	b, _ := longTermBatch(t, r, true, "12345")
	a := testLongTermAddress("离线隐私测试省市地址一号")
	v := longTermClaim(t, r, b, "12345", gin.H{"action": "save", "version": 0, "address": a}, 200)
	if len(v.AddressBook[0].Uses) != 1 || v.AddressBook[0].Uses[0].Note != a.Note {
		t.Fatal("batch note history missing")
	}
	row := fulfillmentPerson(t, b, "12345")
	response := batchCall(t, r, "POST", fmt.Sprintf("/issues/%d/claim", b.ID), gin.H{"uid": "12345", "code": "wrong", "action": "view"}, 401)
	if strings.Contains(string(response.Data), a.Address) {
		t.Fatal("invalid credential exposed history")
	}
	database.DB.Model(&row).Update("disabled", true)
	longTermClaim(t, r, b, "12345", gin.H{"action": "view"}, 401)
}
func TestAddressBookMigrationIsAdditiveAndRepeatable(t *testing.T) {
	appearanceDB(t)
	if err := database.DB.Exec(`CREATE TABLE voyage_long_term_addresses (uid text PRIMARY KEY,address_encrypted text NOT NULL,enabled numeric NOT NULL DEFAULT false,version integer NOT NULL DEFAULT 1,updated_at datetime)`).Error; err != nil {
		t.Fatal(err)
	}
	database.DB.Exec(`INSERT INTO voyage_long_term_addresses VALUES ('12345','opaque-preserved',0,4,'2026-10-25 00:00:00')`)
	for i := 0; i < 2; i++ {
		if err := database.InitializeVoyageAddressBook(); err != nil {
			t.Fatal(err)
		}
	}
	var p database.VoyageLongTermAddress
	database.DB.First(&p, "uid = ?", "12345")
	if p.Enabled || p.Version != 4 || p.AddressEncrypted != "opaque-preserved" || p.EntryID != 0 {
		t.Fatal("migration altered existing permission")
	}
	var count int64
	database.DB.Model(&database.VoyageAddressBookEntry{}).Count(&count)
	if count != 0 {
		t.Fatal("migration invented address history")
	}
}
