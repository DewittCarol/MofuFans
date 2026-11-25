package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"image"
	"image/png"
	"mime/multipart"
	"mofufans/internal/database"
	"mofufans/internal/secret"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLibraryAppendPreservesAlbumAndRejectsStaleUpload(t *testing.T) {
	libraryTestDB(t)
	ctl := &GalleryController{Directory: t.TempDir()}
	r := gin.New()
	r.POST("/albums", func(c *gin.Context) { c.Set("authed", true) }, ctl.Create)
	r.POST("/albums/:id/images", func(c *gin.Context) { c.Set("authed", true) }, ctl.Create)
	upload := func(path string, version string) int {
		var body bytes.Buffer
		w := multipart.NewWriter(&body)
		w.WriteField("title", "相册")
		w.WriteField("source", "fan")
		w.WriteField("version", version)
		f, _ := w.CreateFormFile("images", "sample.png")
		png.Encode(f, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
		w.Close()
		req := httptest.NewRequest("POST", path, &body)
		req.Header.Set("Content-Type", w.FormDataContentType())
		out := httptest.NewRecorder()
		r.ServeHTTP(out, req)
		var data struct{ Code int }
		json.Unmarshal(out.Body.Bytes(), &data)
		return data.Code
	}
	if upload("/albums", "") != 200 {
		t.Fatal("create")
	}
	var before database.GalleryWork
	database.DB.Preload("Images").First(&before)
	if before.Source != "fan" || before.Status != "published" {
		t.Fatal("admin curation source")
	}
	path := fmt.Sprintf("/albums/%d/images", before.ID)
	if upload(path, "1") != 200 {
		t.Fatal("append")
	}
	if upload(path, "1") != 409 {
		t.Fatal("stale append accepted")
	}
	var after database.GalleryWork
	database.DB.Preload("Images").First(&after, before.ID)
	if len(after.Images) != 2 || after.Images[0].Filename != before.Images[0].Filename || after.Version != 2 {
		t.Fatal("album content lost")
	}
}

func TestVoyageAddressHistoryRequiresCurrentCredentialsAndSameUID(t *testing.T) {
	appearanceDB(t)
	database.DB.AutoMigrate(&database.VoyageIssue{}, &database.VoyageRecipient{}, &database.VoyageDelivery{})
	viper.Set("security.bilibili_cookie_key", "offline-history-key-at-least-32-bytes")
	t.Cleanup(func() { viper.Set("security.bilibili_cookie_key", "") })
	currentIssue := database.VoyageIssue{Title: "本期", Published: true}
	database.DB.Create(&currentIssue)
	enc, hash, _ := secret.NewVoyageCode(recipientScope(currentIssue.ID, "1000001"))
	code, _ := secret.OpenVoyageCode(enc, recipientScope(currentIssue.ID, "1000001"))
	current := database.VoyageRecipient{IssueID: currentIssue.ID, UID: "1000001", CodeEncrypted: enc, CodeHash: hash}
	database.DB.Create(&current)
	for i, uid := range []string{"1000001", "1000002", "1000001"} {
		issue := database.VoyageIssue{Title: fmt.Sprint("历史", i)}
		database.DB.Create(&issue)
		person := database.VoyageRecipient{IssueID: issue.ID, UID: uid}
		database.DB.Create(&person)
		addr := VoyageAddress{Name: uid, Phone: "13800000000", Address: "某省某市某区一号", Note: "往期备注不要复制"}
		raw, _ := json.Marshal(addr)
		sealed, _ := secret.SealVoyageAddress(recipientScope(issue.ID, uid), raw)
		now := time.Now()
		database.DB.Create(&database.VoyageDelivery{RecipientID: person.ID, AddressEncrypted: sealed, SubmittedAt: &now})
	}
	r := gin.New()
	r.POST("/issues/:id/claim", (&VoyageDeliveryController{}).Claim)
	path := fmt.Sprintf("/issues/%d/claim", currentIssue.ID)
	for _, key := range []string{"invalid", code} {
		payload, _ := json.Marshal(map[string]string{"uid": "1000001", "code": key, "action": "view"})
		_, res := performJSONRequest(r, "POST", path, string(payload), nil)
		if key == "invalid" {
			if res.Code != 401 || strings.Contains(string(res.Data), "history") {
				t.Fatal("unauthenticated history")
			}
			continue
		}
		var v deliveryView
		if res.Code != 200 || json.Unmarshal(res.Data, &v) != nil || len(v.History) != 1 || v.History[0].Address.Name != "1000001" || v.History[0].Address.Note != "" {
			t.Fatal("history scope/dedup failed", res.Code, string(res.Data))
		}
	}
}
