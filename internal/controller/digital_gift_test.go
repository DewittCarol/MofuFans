package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"gorm.io/gorm"
	"mime/multipart"
	"mofufans/internal/database"
	"mofufans/internal/secret"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func giftTestRouter(t *testing.T) (*gin.Engine, *DigitalGiftController) {
	t.Helper()
	appearanceDB(t)
	old := viper.Get("security.bilibili_cookie_key")
	viper.Set("security.bilibili_cookie_key", strings.Repeat("g", 32))
	t.Cleanup(func() { viper.Set("security.bilibili_cookie_key", old) })
	if err := database.InitializeVoyageBatches(); err != nil {
		t.Fatal(err)
	}
	if err := database.InitializeDigitalGifts(); err != nil {
		t.Fatal(err)
	}
	ctl := &DigitalGiftController{Directory: t.TempDir()}
	r := gin.New()
	r.GET("/api/gifts", ctl.List)
	r.GET("/api/gifts/:id", ctl.Detail)
	r.POST("/api/gifts/:id/unlock", ctl.Unlock)
	r.POST("/api/gifts/:id/lock", ctl.Lock)
	r.GET("/api/gifts/:id/files/:file", ctl.File)
	a := r.Group("/api/admin/gifts", func(c *gin.Context) { c.Set("authed", true) })
	a.GET("", ctl.List)
	a.POST("", ctl.Save)
	a.GET("/:id", ctl.Detail)
	a.PUT("/:id", ctl.Save)
	a.DELETE("/:id", ctl.Delete)
	a.GET("/:id/password", ctl.Password)
	a.POST("/:id/files", ctl.Upload)
	a.DELETE("/:id/files/:file", ctl.DeleteFile)
	a.GET("/:id/recipients", ctl.Recipients)
	a.GET("/:id/rosters", ctl.Rosters)
	a.PUT("/:id/rosters", ctl.SaveRosters)
	a.POST("/:id/uploads", ctl.StartUpload)
	a.GET("/:id/uploads/:upload", ctl.UploadStatus)
	a.PUT("/:id/uploads/:upload", ctl.UploadChunk)
	a.POST("/:id/uploads/:upload/complete", ctl.CompleteUpload)
	a.DELETE("/:id/uploads/:upload", ctl.CancelUpload)
	a.PUT("/:id/recipients/:recipient", ctl.UpdateRecipient)
	return r, ctl
}

func giftJSON(t *testing.T, r http.Handler, method, path string, body any, cookies []*http.Cookie, want int) (*httptest.ResponseRecorder, json.RawMessage) {
	t.Helper()
	raw, _ := json.Marshal(body)
	w, res := performJSONRequest(r, method, path, string(raw), cookies)
	if res.Code != want {
		t.Fatalf("%s %s: want %d got %d (%s)", method, path, want, res.Code, res.Message)
	}
	return w, res.Data
}
func giftCreate(t *testing.T, r http.Handler, mode string, published bool) database.DigitalGift {
	t.Helper()
	_, data := giftJSON(t, r, "POST", "/api/admin/gifts", map[string]any{"title": "测试礼物", "intro": "公开介绍", "content": "解锁后才可看 SECRET_CONTENT", "mode": mode, "published": published}, nil, 200)
	var gift database.DigitalGift
	if err := json.Unmarshal(data, &gift); err != nil {
		t.Fatal(err)
	}
	return gift
}
func giftPassword(t *testing.T, r http.Handler, id uint, uid string) string {
	t.Helper()
	_, data := giftJSON(t, r, "GET", fmt.Sprintf("/api/admin/gifts/%d/password?uid=%s", id, uid), nil, nil, 200)
	var result struct {
		Password string `json:"password"`
	}
	_ = json.Unmarshal(data, &result)
	if result.Password == "" {
		t.Fatal("missing password")
	}
	return result.Password
}
func giftUpload(t *testing.T, r http.Handler, id uint, name, contents string) database.DigitalGiftFile {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, _ := form.CreateFormFile("file", name)
	_, _ = part.Write([]byte(contents))
	_ = form.Close()
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/admin/gifts/%d/files", id), &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var res testAPIResponse
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.Code != 200 {
		t.Fatalf("upload failed: %s", w.Body.String())
	}
	var file database.DigitalGiftFile
	_ = json.Unmarshal(res.Data, &file)
	return file
}

func TestDigitalGiftSharedPrivacyAndRevocation(t *testing.T) {
	r, _ := giftTestRouter(t)
	gift := giftCreate(t, r, "shared", false)
	base := fmt.Sprintf("/api/gifts/%d", gift.ID)
	admin := fmt.Sprintf("/api/admin/gifts/%d", gift.ID)
	_, data := giftJSON(t, r, "GET", "/api/gifts", nil, nil, 200)
	if string(data) != "[]" {
		t.Fatal("draft is visible")
	}
	password := giftPassword(t, r, gift.ID, "")
	var stored database.DigitalGift
	database.DB.First(&stored, gift.ID)
	if stored.PasswordEncrypted == password || strings.Contains(stored.PasswordEncrypted, password) {
		t.Fatal("plaintext password stored")
	}
	gift.Published = true
	_, data = giftJSON(t, r, "PUT", admin, gift, nil, 200)
	_ = json.Unmarshal(data, &gift)
	_, data = giftJSON(t, r, "GET", "/api/gifts", nil, nil, 200)
	for _, value := range []string{password, "SECRET_CONTENT", "password", "files", "version"} {
		if strings.Contains(string(data), value) {
			t.Fatalf("locked list leaked %s", value)
		}
	}
	file := giftUpload(t, r, gift.ID, "礼物.html", "<html>never execute</html>")
	fileURL := fmt.Sprintf("%s/files/%d?preview=1", base, file.ID)
	req := httptest.NewRequest("GET", fileURL, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("anonymous download: %d", w.Code)
	}
	giftJSON(t, r, "GET", base, nil, nil, 401)
	giftJSON(t, r, "POST", base+"/unlock", map[string]string{"password": "wrong"}, nil, 403)
	w, _ = giftJSON(t, r, "POST", base+"/unlock", map[string]string{"password": password}, nil, 200)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Path != base {
		t.Fatalf("unsafe cookies: %+v", cookies)
	}
	giftJSON(t, r, "GET", base, nil, cookies, 200)
	req = httptest.NewRequest("GET", fileURL, nil)
	req.AddCookie(cookies[0])
	req.Header.Set("Range", "bytes=0-5")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 206 || w.Body.String() != "<html>" || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") || w.Header().Get("Cache-Control") != "private, no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("unsafe or broken file transfer: %d %v", w.Code, w.Header())
	}
	other := giftCreate(t, r, "shared", true)
	stolen := *cookies[0]
	stolen.Name = giftCookie(other.ID)
	giftJSON(t, r, "GET", fmt.Sprintf("/api/gifts/%d", other.ID), nil, []*http.Cookie{&stolen}, 401)
	_, data = giftJSON(t, r, "PUT", admin, map[string]any{"title": gift.Title, "mode": gift.Mode, "published": true, "version": gift.Version, "password": "Replacement123"}, nil, 200)
	_ = json.Unmarshal(data, &gift)
	giftJSON(t, r, "GET", base, nil, cookies, 401)
	giftJSON(t, r, "POST", base+"/unlock", map[string]string{"password": password}, nil, 403)
	w, _ = giftJSON(t, r, "POST", base+"/unlock", map[string]string{"password": "Replacement123"}, nil, 200)
	cookies = w.Result().Cookies()
	stale := gift
	gift.Published = false
	_, data = giftJSON(t, r, "PUT", admin, gift, nil, 200)
	_ = json.Unmarshal(data, &gift)
	giftJSON(t, r, "PUT", admin, stale, nil, 409)
	giftJSON(t, r, "GET", base, nil, cookies, 404)
	gift.Published = true
	giftJSON(t, r, "PUT", admin, gift, nil, 200)
	giftJSON(t, r, "GET", base, nil, cookies, 401)
}

func TestDigitalGiftIndividualImportResetAndIsolation(t *testing.T) {
	r, _ := giftTestRouter(t)
	gift := giftCreate(t, r, "individual", true)
	base := fmt.Sprintf("/api/gifts/%d", gift.ID)
	admin := fmt.Sprintf("/api/admin/gifts/%d", gift.ID)
	roster := database.VoyageRoster{Title: "测试名单", Category: "monthly", Month: "2026-09"}
	if err := database.DB.Create(&roster).Error; err != nil {
		t.Fatal(err)
	}
	for _, m := range []database.VoyageRosterMember{{RosterID: roster.ID, UID: "12345", Name: "one", Tier: "captain"}, {RosterID: roster.ID, UID: "67890", Name: "two", Tier: "captain"}, {RosterID: roster.ID, UID: "12345", Name: "one", Tier: "admiral"}} {
		if err := database.DB.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
	}
	giftJSON(t, r, "PUT", admin+"/rosters", map[string]any{"roster_ids": []uint{roster.ID}, "version": gift.Version}, nil, 200)
	gift.Version++
	var count int64
	database.DB.Model(&database.DigitalGiftRecipient{}).Count(&count)
	if count != 2 {
		t.Fatalf("duplicate UID generated %d passwords", count)
	}
	one, two := giftPassword(t, r, gift.ID, "12345"), giftPassword(t, r, gift.ID, "67890")
	if one == two {
		t.Fatal("passwords not individual")
	}
	giftJSON(t, r, "POST", base+"/unlock", map[string]string{"uid": "67890", "password": one}, nil, 403)
	w, _ := giftJSON(t, r, "POST", base+"/unlock", map[string]string{"uid": "12345", "password": one}, nil, 200)
	cookie1 := w.Result().Cookies()
	w, _ = giftJSON(t, r, "POST", base+"/unlock", map[string]string{"uid": "67890", "password": two}, nil, 200)
	cookie2 := w.Result().Cookies()
	var row database.DigitalGiftRecipient
	database.DB.First(&row, "uid = ?", "12345")
	path := fmt.Sprintf("%s/recipients/%d", admin, row.ID)
	_, data := giftJSON(t, r, "PUT", path, map[string]any{"version": row.Version, "action": "sent"}, nil, 200)
	_ = json.Unmarshal(data, &row)
	giftJSON(t, r, "PUT", admin+"/rosters", map[string]any{"roster_ids": []uint{roster.ID}, "version": gift.Version}, nil, 200)
	if giftPassword(t, r, gift.ID, "12345") != one {
		t.Fatal("reimport changed password")
	}
	var current database.DigitalGiftRecipient
	database.DB.First(&current, row.ID)
	if current.SentAt == nil {
		t.Fatal("reimport removed sent status")
	}
	_, data = giftJSON(t, r, "PUT", path, map[string]any{"version": row.Version, "action": "reset"}, nil, 200)
	_ = json.Unmarshal(data, &row)
	if row.SentAt != nil {
		t.Fatal("reset must require new notification")
	}
	giftJSON(t, r, "GET", base, nil, cookie1, 401)
	giftJSON(t, r, "GET", base, nil, cookie2, 200)
	giftJSON(t, r, "POST", base+"/unlock", map[string]string{"uid": "12345", "password": one}, nil, 403)
	one = giftPassword(t, r, gift.ID, "12345")
	w, _ = giftJSON(t, r, "POST", base+"/unlock", map[string]string{"uid": "12345", "password": one}, nil, 200)
	cookie1 = w.Result().Cookies()
	_, data = giftJSON(t, r, "PUT", path, map[string]any{"version": row.Version, "action": "disable"}, nil, 200)
	_ = json.Unmarshal(data, &row)
	giftJSON(t, r, "GET", base, nil, cookie1, 401)
	giftJSON(t, r, "GET", base, nil, cookie2, 200)
	giftJSON(t, r, "POST", base+"/unlock", map[string]string{"uid": "12345", "password": one}, nil, 403)
	_, data = giftJSON(t, r, "PUT", path, map[string]any{"version": row.Version, "action": "enable"}, nil, 200)
	_ = json.Unmarshal(data, &row)
	giftJSON(t, r, "GET", base, nil, cookie1, 401)
	_, data = giftJSON(t, r, "GET", admin+"/recipients", nil, nil, 200)
	if strings.Contains(string(data), one) || strings.Contains(string(data), "password") {
		t.Fatal("roster leaks passwords")
	}
}

func TestDigitalGiftExpiryLimitsCSRFAndFileScope(t *testing.T) {
	r, ctl := giftTestRouter(t)
	gift := giftCreate(t, r, "shared", true)
	base := fmt.Sprintf("/api/gifts/%d", gift.ID)
	password := giftPassword(t, r, gift.ID, "")
	w, _ := giftJSON(t, r, "POST", base+"/unlock", map[string]string{"password": password}, nil, 200)
	cookies := w.Result().Cookies()
	file := giftUpload(t, r, gift.ID, "test.bin", strings.Repeat("X", 20000))
	other := giftCreate(t, r, "shared", true)
	giftUpload(t, r, other.ID, "other.bin", "PRIVATE_OTHER")
	url := fmt.Sprintf("%s/files/%d", base, file.ID+1)
	req := httptest.NewRequest("GET", url, nil)
	req.AddCookie(cookies[0])
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if res.Code != 404 {
		t.Fatal("cross-gift file served")
	}
	var stored database.DigitalGift
	database.DB.First(&stored, gift.ID)
	raw, _ := json.Marshal(giftGrant{Expires: time.Now().Add(-time.Minute).Unix(), Version: stored.AccessVersion})
	token, _ := secret.SealGiftValue(fmt.Sprintf("gift:%d:grant", gift.ID), string(raw))
	expired := *cookies[0]
	expired.Value = token
	giftJSON(t, r, "GET", base, nil, []*http.Cookie{&expired}, 401)
	req = httptest.NewRequest("POST", base+"/unlock", strings.NewReader(`{"password":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.invalid")
	res = httptest.NewRecorder()
	r.ServeHTTP(res, req)
	var envelope testAPIResponse
	_ = json.Unmarshal(res.Body.Bytes(), &envelope)
	if envelope.Code != 403 {
		t.Fatal("cross-site unlock accepted")
	}
	for i := 0; i < 12; i++ {
		if !ctl.limit.allow("limit-test") {
			t.Fatal("limit too strict")
		}
	}
	if ctl.limit.allow("limit-test") {
		t.Fatal("limit did not engage")
	}
	w, _ = giftJSON(t, r, "POST", base+"/lock", map[string]string{}, cookies, 200)
	if len(w.Result().Cookies()) != 1 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("lock did not clear access cookie")
	}
	giftJSON(t, r, "DELETE", fmt.Sprintf("/api/admin/gifts/%d", gift.ID), map[string]int{"version": gift.Version}, nil, 200)
	giftJSON(t, r, "GET", base, nil, cookies, 404)
}

func TestDigitalGiftOpenPublicationAndFiles(t *testing.T) {
	r, _ := giftTestRouter(t)
	gift := giftCreate(t, r, "open", false)
	base := fmt.Sprintf("/api/gifts/%d", gift.ID)
	admin := fmt.Sprintf("/api/admin/gifts/%d", gift.ID)
	file := giftUpload(t, r, gift.ID, "公开.bin", "public gift")
	giftJSON(t, r, "GET", base, nil, nil, 404)
	gift.Published = true
	_, data := giftJSON(t, r, "PUT", admin, gift, nil, 200)
	json.Unmarshal(data, &gift)
	_, data = giftJSON(t, r, "GET", base, nil, nil, 200)
	if !strings.Contains(string(data), "SECRET_CONTENT") {
		t.Fatal("open content absent")
	}
	req := httptest.NewRequest("GET", fmt.Sprintf("%s/files/%d", base, file.ID), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 || w.Body.String() != "public gift" {
		t.Fatal("open download failed")
	}
	giftJSON(t, r, "GET", admin+"/password", nil, nil, 400)
	gift.Published = false
	giftJSON(t, r, "PUT", admin, gift, nil, 200)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatal("unpublished file visible")
	}
}
func TestGiftRosterAutoSyncAndLocalEligibility(t *testing.T) {
	r, _ := giftTestRouter(t)
	gift := giftCreate(t, r, "individual", true)
	first := database.VoyageRoster{Title: "九月", Category: "monthly", Month: "2026-09"}
	second := database.VoyageRoster{Title: "抽奖", Category: "event"}
	database.DB.Create(&first)
	database.DB.Create(&second)
	a := database.VoyageRosterMember{RosterID: first.ID, UID: "123", Name: "旧昵称", Tier: "captain"}
	b := database.VoyageRosterMember{RosterID: second.ID, UID: "123", Name: "第二名单"}
	database.DB.Create(&a)
	database.DB.Create(&b)
	admin := fmt.Sprintf("/api/admin/gifts/%d", gift.ID)
	base := fmt.Sprintf("/api/gifts/%d", gift.ID)
	giftJSON(t, r, "PUT", admin+"/rosters", map[string]any{"roster_ids": []uint{first.ID, second.ID}, "version": gift.Version}, nil, 200)
	password := giftPassword(t, r, gift.ID, "123")
	w, _ := giftJSON(t, r, "POST", base+"/unlock", map[string]string{"uid": "123", "password": password}, nil, 200)
	cookie := w.Result().Cookies()
	sync := func(roster database.VoyageRoster, change func(*gorm.DB) error) {
		t.Helper()
		if err := database.DB.Transaction(func(tx *gorm.DB) error {
			if err := lockVoyageBatchWrites(tx); err != nil {
				return err
			}
			if err := change(tx); err != nil {
				return err
			}
			_, err := syncVoyageRoster(tx, roster)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	sync(first, func(tx *gorm.DB) error { return tx.Model(&a).Update("name", "新昵称").Error })
	var row database.DigitalGiftRecipient
	database.DB.First(&row, "gift_id = ?", gift.ID)
	if row.Name != "新昵称" {
		t.Fatal("name did not sync")
	}
	if giftPassword(t, r, gift.ID, "123") != password {
		t.Fatal("sync rotated password")
	}
	path := fmt.Sprintf("%s/recipients/%d", admin, row.ID)
	giftJSON(t, r, "PUT", path, map[string]any{"version": row.Version, "action": "disable"}, nil, 200)
	sync(first, func(tx *gorm.DB) error { return tx.Model(&a).Update("name", "再次改名").Error })
	database.DB.First(&row, row.ID)
	if !row.Disabled {
		t.Fatal("source edit restored local cancelled eligibility")
	}
	var source database.VoyageRosterMember
	database.DB.First(&source, a.ID)
	if source.Disabled {
		t.Fatal("gift cancellation changed source membership")
	}
	giftJSON(t, r, "PUT", path, map[string]any{"version": row.Version, "action": "enable"}, nil, 200)
	sync(first, func(tx *gorm.DB) error { return tx.Delete(&a).Error })
	database.DB.First(&row, row.ID)
	if !row.RosterActive {
		t.Fatal("second roster membership ignored")
	}
	if giftPassword(t, r, gift.ID, "123") != password {
		t.Fatal("remaining source lost password")
	}
	sync(second, func(tx *gorm.DB) error { return tx.Model(&b).Update("disabled", true).Error })
	giftJSON(t, r, "GET", base, nil, cookie, 401)
	giftJSON(t, r, "POST", base+"/unlock", map[string]string{"uid": "123", "password": password}, nil, 403)
	sync(second, func(tx *gorm.DB) error { return tx.Model(&b).Update("disabled", false).Error })
	if giftPassword(t, r, gift.ID, "123") != password {
		t.Fatal("restoring source replaced password")
	}
	giftJSON(t, r, "GET", base, nil, cookie, 401)
	// A stale binding editor cannot overwrite a newer selection.
	giftJSON(t, r, "PUT", admin+"/rosters", map[string]any{"roster_ids": []uint{}, "version": gift.Version}, nil, 409)
}
func TestGiftLegacyRosterMigrationIsIdempotent(t *testing.T) {
	r, _ := giftTestRouter(t)
	gift := giftCreate(t, r, "individual", true)
	now := time.Now()
	row := database.DigitalGiftRecipient{GiftID: gift.ID, UID: "123", Name: "原名单", Disabled: true, SentAt: &now, PasswordEncrypted: "untouched-encrypted", Version: 5, AccessVersion: 3}
	database.DB.Create(&row)
	database.DB.Model(&gift).Update("roster_managed", false)
	for i := 0; i < 2; i++ {
		if err := database.InitializeDigitalGifts(); err != nil {
			t.Fatal(err)
		}
	}
	var bindings []database.DigitalGiftRoster
	database.DB.Where("gift_id = ?", gift.ID).Find(&bindings)
	if len(bindings) != 1 {
		t.Fatalf("migration bindings: %d", len(bindings))
	}
	var members []database.VoyageRosterMember
	database.DB.Where("roster_id = ?", bindings[0].RosterID).Find(&members)
	if len(members) != 1 || members[0].UID != "123" || members[0].Disabled {
		t.Fatal("invalid shared roster migration")
	}
	database.DB.First(&row, row.ID)
	if row.PasswordEncrypted != "untouched-encrypted" || !row.Disabled || row.Version != 5 || row.AccessVersion != 3 || row.SentAt == nil {
		t.Fatal("migration changed credentials or gift eligibility")
	}
}
func TestGiftChunkedUploadRecoveryAndScope(t *testing.T) {
	r, ctl := giftTestRouter(t)
	gift := giftCreate(t, r, "open", true)
	base := fmt.Sprintf("/api/admin/gifts/%d/uploads", gift.ID)
	size := int64(26 << 20)
	_, data := giftJSON(t, r, "POST", base, map[string]any{"name": "大文件.bin", "size": size}, nil, 200)
	var upload database.DigitalGiftUpload
	json.Unmarshal(data, &upload)
	path := base + "/" + upload.Token
	giftJSON(t, r, "POST", path+"/complete", map[string]any{}, nil, 409)
	put := func(offset int64, body []byte, want int) {
		t.Helper()
		req := httptest.NewRequest("PUT", path, bytes.NewReader(body))
		req.Header.Set("Upload-Offset", fmt.Sprint(offset))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var res testAPIResponse
		json.Unmarshal(w.Body.Bytes(), &res)
		if res.Code != want {
			t.Fatalf("chunk: %s", w.Body.String())
		}
	}
	for offset := int64(0); offset < size; {
		length := int64(giftChunkSize)
		if length > size-offset {
			length = size - offset
		}
		put(offset, bytes.Repeat([]byte("x"), int(length)), 200)
		if offset == 0 {
			put(offset, []byte("x"), 409)
		}
		offset += length
	}
	_, data = giftJSON(t, r, "GET", path, nil, nil, 200)
	json.Unmarshal(data, &upload)
	if upload.Received != size {
		t.Fatal("resume status incorrect")
	}
	put(size, []byte("overflow"), 400)
	_, data = giftJSON(t, r, "POST", path+"/complete", map[string]any{}, nil, 200)
	var file database.DigitalGiftFile
	json.Unmarshal(data, &file)
	_, again := giftJSON(t, r, "POST", path+"/complete", map[string]any{}, nil, 200)
	var retry database.DigitalGiftFile
	json.Unmarshal(again, &retry)
	if retry.ID != file.ID || file.Size != size {
		t.Fatal("completion retry duplicated or truncated file")
	}
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/gifts/%d/files/%d", gift.ID, file.ID), nil)
	req.Header.Set("Range", "bytes=27262970-27262975")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 206 || w.Body.String() != "xxxxxx" {
		t.Fatalf("large file range failed: %d", w.Code)
	}
	other := giftCreate(t, r, "open", true)
	giftJSON(t, r, "GET", fmt.Sprintf("/api/admin/gifts/%d/uploads/%s", other.ID, upload.Token), nil, nil, 404)
	giftJSON(t, r, "DELETE", path, nil, nil, 200) // Cancelling a completed session must keep the file.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 206 {
		t.Fatal("completed file deleted with session")
	}
	_, data = giftJSON(t, r, "POST", base, map[string]any{"name": "取消.bin", "size": int64(1) << 40}, nil, 200)
	json.Unmarshal(data, &upload)
	path = base + "/" + upload.Token
	var stored database.DigitalGiftUpload
	database.DB.First(&stored, "token = ?", upload.Token)
	disk, _ := ctl.uploadPath(stored)
	put(0, []byte("partial"), 200)
	giftJSON(t, r, "DELETE", path, nil, nil, 200)
	if _, err := os.Stat(disk); !os.IsNotExist(err) {
		t.Fatal("cancel did not remove partial file")
	}
	_, data = giftJSON(t, r, "POST", base, map[string]any{"name": "empty.txt", "size": 0}, nil, 200)
	json.Unmarshal(data, &upload)
	giftJSON(t, r, "POST", base+"/"+upload.Token+"/complete", map[string]any{}, nil, 200)
}
