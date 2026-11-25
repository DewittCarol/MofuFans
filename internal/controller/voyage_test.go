package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"mofufans/internal/database"
)

func TestVoyageDraftPublishUnpublishAndConflict(t *testing.T) {
	appearanceDB(t)
	if err := database.DB.AutoMigrate(&database.VoyageIssue{}, &database.VoyageRecipient{}, &database.VoyageDispatchConfig{}); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	controller := &VoyageController{Directory: directory}
	router := gin.New()
	router.GET("/voyage", controller.List)
	router.GET("/voyage/issues", controller.AdminList)
	router.POST("/voyage/issues", controller.Save)
	router.PUT("/voyage/issues/:id", controller.Save)
	router.DELETE("/voyage/issues/:id", controller.Delete)
	// Reuse the multipart helper to check the shared uploader's separate asset namespace.
	router.POST("/emojis", controller.Upload)
	router.GET("/voyage-assets/:filename", controller.File)
	request := database.VoyageIssue{Kind: "guard", Month: "2026-09", Title: "测试期次", Note: "登记条件以本期说明为准", Gifts: []database.VoyageGift{{Name: "实体样品", Tiers: []string{"captain", "governor"}, Description: "尺寸说明", Images: []string{}}}}
	save := func() testAPIResponse {
		body, _ := json.Marshal(request)
		method, path := "POST", "/voyage/issues"
		if request.ID != 0 {
			method, path = "PUT", fmt.Sprintf("/voyage/issues/%d", request.ID)
		}
		_, result := performJSONRequest(router, method, path, string(body), nil)
		return result
	}
	created := save()
	if created.Code != 200 || json.Unmarshal(created.Data, &request) != nil || request.ID == 0 || request.Version != 1 {
		t.Fatalf("create: %+v", created)
	}
	_, private := performJSONRequest(router, "GET", "/voyage", "", nil)
	if string(private.Data) != "[]" {
		t.Fatalf("draft leaked: %s", private.Data)
	}
	request.Published = true
	if result := save(); result.Code != 200 || json.Unmarshal(result.Data, &request) != nil {
		t.Fatal("a text-only reward should be publishable")
	}
	var imageData bytes.Buffer
	_ = png.Encode(&imageData, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
	uploaded := emojiUpload(t, router, "", "sample.png", imageData.Bytes())
	var asset struct {
		URL string `json:"url"`
	}
	if uploaded.Code != 200 || json.Unmarshal(uploaded.Data, &asset) != nil || !strings.HasPrefix(asset.URL, "/api/voyage-assets/") {
		t.Fatalf("image upload: %+v", uploaded)
	}
	if _, err := os.Stat(filepath.Join(directory, filepath.Base(asset.URL))); err != nil {
		t.Fatal(err)
	}
	request.Gifts[0].Images = []string{asset.URL}
	request.Gifts = append(request.Gifts, database.VoyageGift{Name: "立牌", Tiers: []string{"admiral"}})
	published := save()
	if published.Code != 200 {
		t.Fatalf("publish: %+v", published)
	}
	if stale := save(); stale.Code != 409 {
		t.Fatalf("stale update: %+v", stale)
	}
	if err := json.Unmarshal(published.Data, &request); err != nil {
		t.Fatal(err)
	}
	if request.Gifts[1].Images == nil {
		t.Fatal("nil image list would break public rendering")
	}
	_, public := performJSONRequest(router, "GET", "/voyage", "", nil)
	var issues []database.VoyageIssue
	if json.Unmarshal(public.Data, &issues) != nil || len(issues) != 1 || len(issues[0].Gifts) != 2 || issues[0].Gifts[0].Images[0] != asset.URL {
		t.Fatalf("gift document did not persist: %s", public.Data)
	}
	for _, invalid := range []struct {
		tiers []string
		image string
	}{
		{[]string{"unknown"}, asset.URL}, {[]string{"captain", "captain"}, asset.URL},
		{[]string{"captain"}, "https://evil.invalid/pixel.png"}, {[]string{"captain"}, "/api/voyage-assets/../../config.json"},
	} {
		request.Gifts[0].Tiers = invalid.tiers
		request.Gifts[0].Images = []string{invalid.image}
		if result := save(); result.Code != 400 {
			t.Fatalf("invalid gift accepted: %+v", result)
		}
	}
	// All rejected writes left the published issue unchanged.
	_, unchanged := performJSONRequest(router, "GET", "/voyage", "", nil)
	if !bytes.Equal(public.Data, unchanged.Data) {
		t.Fatal("invalid request changed public issue")
	}
	request = issues[0]
	request.Published = false
	if result := save(); result.Code != 200 {
		t.Fatalf("unpublish: %+v", result)
	}
	_, hidden := performJSONRequest(router, "GET", "/voyage", "", nil)
	if string(hidden.Data) != "[]" {
		t.Fatal("unpublished issue remained public")
	}
	_, admin := performJSONRequest(router, "GET", "/voyage/issues", "", nil)
	if !bytes.Contains(admin.Data, []byte("测试期次")) {
		t.Fatal("admin lost its draft")
	}
	_, removed := performJSONRequest(router, "DELETE", fmt.Sprintf("/voyage/issues/%d", request.ID), "", nil)
	if removed.Code != 200 {
		t.Fatal("delete failed")
	}
}

func TestVoyageLegacyGiftMetadataDoesNotNeedMigration(t *testing.T) {
	appearanceDB(t)
	if err := database.DB.AutoMigrate(&database.VoyageIssue{}); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	filename := strings.Repeat("a", 64) + ".png"
	if err := os.WriteFile(filepath.Join(directory, filename), []byte("existing image"), 0600); err != nil {
		t.Fatal(err)
	}
	legacyGift := map[string]any{
		"section": "other", "event": "", "audience": "中奖用户",
		"shipping_note": "月底寄出", "name": "纪念礼物", "kind": "mixed",
		"tiers": []any{}, "description": "礼物尺寸说明",
		"images": []any{"/api/voyage-assets/" + filename}, "preview": "pet:3d",
	}
	legacyJSON, err := json.Marshal([]map[string]any{legacyGift})
	if err != nil {
		t.Fatal(err)
	}
	issue := database.VoyageIssue{Kind: "lottery", Month: "2026-09", Title: "已有期次", Published: true, Version: 1}
	if err := database.DB.Create(&issue).Error; err != nil {
		t.Fatal(err)
	}
	// Model the existing JSON column without rewriting or migrating old records.
	if err := database.DB.Model(&issue).UpdateColumn("gifts", string(legacyJSON)).Error; err != nil {
		t.Fatal(err)
	}
	controller := &VoyageController{Directory: directory}
	router := gin.New()
	router.GET("/voyage", controller.List)
	router.GET("/voyage/issues", controller.AdminList)
	router.PUT("/voyage/issues/:id", controller.Save)
	expectedGift := map[string]any{}
	for key, value := range legacyGift {
		if key != "kind" && key != "preview" {
			expectedGift[key] = value
		}
	}
	assertGifts := func(raw json.RawMessage) {
		t.Helper()
		var gifts []map[string]any
		if err := json.Unmarshal(raw, &gifts); err != nil {
			t.Fatal(err)
		}
		if len(gifts) != 1 || !reflect.DeepEqual(gifts[0], expectedGift) {
			t.Fatalf("gift content changed or retired metadata remained: %s", raw)
		}
	}
	for _, path := range []string{"/voyage", "/voyage/issues"} {
		_, response := performJSONRequest(router, "GET", path, "", nil)
		var issues []struct {
			Gifts json.RawMessage `json:"gifts"`
		}
		if response.Code != 200 || json.Unmarshal(response.Data, &issues) != nil || len(issues) != 1 {
			t.Fatalf("read legacy issue at %s: %+v", path, response)
		}
		assertGifts(issues[0].Gifts)
	}
	var stored string
	if err := database.DB.Raw("SELECT gifts FROM voyage_issues WHERE id = ?", issue.ID).Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored != string(legacyJSON) {
		t.Fatal("reading an existing issue rewrote its stored JSON")
	}
	// An already-open editor can still save its old payload. Only retired fields
	// are discarded; all remaining gift details survive the versioned update.
	body, err := json.Marshal(map[string]any{
		"kind": issue.Kind, "month": issue.Month, "title": issue.Title, "published": true, "version": issue.Version,
		"gifts": []map[string]any{legacyGift},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, updated := performJSONRequest(router, "PUT", fmt.Sprintf("/voyage/issues/%d", issue.ID), string(body), nil)
	var saved struct {
		Version int             `json:"version"`
		Gifts   json.RawMessage `json:"gifts"`
	}
	if updated.Code != 200 || json.Unmarshal(updated.Data, &saved) != nil || saved.Version != 2 {
		t.Fatalf("save legacy payload: %+v", updated)
	}
	assertGifts(saved.Gifts)
	if err := database.DB.Raw("SELECT gifts FROM voyage_issues WHERE id = ?", issue.ID).Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	assertGifts(json.RawMessage(stored))
}

func TestVoyageMonthlyCategoryContract(t *testing.T) {
	appearanceDB(t)
	if err := database.DB.AutoMigrate(&database.VoyageIssue{}); err != nil {
		t.Fatal(err)
	}
	controller := &VoyageController{Directory: t.TempDir()}
	router := gin.New()
	router.POST("/issues", controller.Save)
	router.PUT("/issues/:id", controller.Save)
	base := database.VoyageIssue{Kind: "lottery", Month: "2026-09", Title: "转盘活动", Published: true, Gifts: []database.VoyageGift{{Name: "中奖礼物", Audience: "中奖者", Tiers: []string{}}}}
	save := func(value database.VoyageIssue, path string) testAPIResponse {
		raw, _ := json.Marshal(value)
		method := "POST"
		if path != "/issues" {
			method = "PUT"
		}
		_, result := performJSONRequest(router, method, path, string(raw), nil)
		return result
	}
	for _, bad := range []database.VoyageIssue{
		{Kind: "catalog", Month: base.Month, Title: base.Title},
		{Kind: "guard", Title: base.Title},
		{Kind: "guard", Month: "2026-13", Title: base.Title},
		{Kind: "guard", Month: base.Month, Title: base.Title, Gifts: base.Gifts},
		{Kind: "lottery", Month: base.Month, Title: base.Title, Gifts: []database.VoyageGift{{Name: "奖品", Tiers: []string{"captain"}}}},
	} {
		if result := save(bad, "/issues"); result.Code != 400 {
			t.Fatalf("accepted invalid category/month: %+v", result)
		}
	}
	result := save(base, "/issues")
	var created database.VoyageIssue
	if result.Code != 200 || json.Unmarshal(result.Data, &created) != nil {
		t.Fatalf("standalone lottery: %+v", result)
	}
	if created.Kind != "lottery" || created.Gifts[0].Section != "other" || len(created.Gifts[0].Tiers) != 0 || created.Gifts[0].Audience != "中奖者" {
		t.Fatal("lottery data changed")
	}
	changed := created
	changed.Kind = "guard"
	changed.Gifts = []database.VoyageGift{{Name: "舰礼", Tiers: []string{"captain"}}}
	if result := save(changed, fmt.Sprintf("/issues/%d", created.ID)); result.Code != 409 {
		t.Fatalf("changed category in place: %+v", result)
	}
	// Multiple independent activities in one month and a separate voyage card are supported.
	if result := save(base, "/issues"); result.Code != 200 {
		t.Fatal("second monthly activity rejected")
	}
	changed.ID = 0
	if result := save(changed, "/issues"); result.Code != 200 {
		t.Fatal("monthly voyage rejected")
	}
}

func TestVoyageCardMaterialsStayScopedRetainedAndPrivate(t *testing.T) {
	appearanceDB(t)
	if err := database.DB.AutoMigrate(&database.VoyageIssue{}); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	ctl := &VoyageController{Directory: directory}
	r := gin.New()
	r.POST("/issues", ctl.Save)
	r.PUT("/issues/:id", ctl.Save)
	r.GET("/admin-issues", ctl.AdminList)
	r.GET("/public-issues", ctl.List)
	r.GET("/assets/:filename", ctl.File)
	urls := []string{}
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("%064x.png", i)
		if err := os.WriteFile(filepath.Join(directory, name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		urls = append(urls, "/api/voyage-assets/"+name)
	}
	makeCard := func(kind, month string, images, materials []string) voyageAdminIssue {
		t.Helper()
		input := voyageAdminIssue{VoyageIssue: database.VoyageIssue{Kind: kind, Month: month, Title: month, Images: images, Gifts: []database.VoyageGift{{Name: "礼物", Tiers: []string{}}}}, AssetImages: materials}
		if kind == "guard" {
			input.Gifts[0].Tiers = []string{"captain"}
		}
		body, _ := json.Marshal(input)
		_, response := performJSONRequest(r, "POST", "/issues", string(body), nil)
		if response.Code != 200 || json.Unmarshal(response.Data, &input) != nil {
			t.Fatalf("create failed: %+v", response)
		}
		return input
	}
	first := makeCard("guard", "2026-09", []string{urls[0]}, []string{urls[1]})
	second := makeCard("lottery", "2026-10", []string{urls[2]}, nil)
	if len(first.AssetImages) != 2 || len(second.AssetImages) != 1 {
		t.Fatal("card materials not isolated")
	}
	// An older client can remove the last placement without knowing about material retention.
	oldClient := first.VoyageIssue
	oldClient.Images = []string{}
	oldClient.Published = true
	body, _ := json.Marshal(oldClient)
	_, saved := performJSONRequest(r, "PUT", fmt.Sprintf("/issues/%d", first.ID), string(body), nil)
	var updated voyageAdminIssue
	if saved.Code != 200 || json.Unmarshal(saved.Data, &updated) != nil || len(updated.AssetImages) != 2 {
		t.Fatalf("removed materials lost: %+v", saved)
	}
	_, stale := performJSONRequest(r, "PUT", fmt.Sprintf("/issues/%d", first.ID), string(body), nil)
	if stale.Code != 409 {
		t.Fatal("material update bypassed version check")
	}
	_, admin := performJSONRequest(r, "GET", "/admin-issues", "", nil)
	var listed []voyageAdminIssue
	if json.Unmarshal(admin.Data, &listed) != nil || len(listed) != 2 {
		t.Fatal("admin materials missing")
	}
	for _, item := range listed {
		if item.ID == first.ID && len(item.AssetImages) != 2 {
			t.Fatal("materials missing after reopen")
		}
		if item.ID == second.ID && (len(item.AssetImages) != 1 || item.AssetImages[0] != urls[2]) {
			t.Fatal("another card's images leaked into this card")
		}
	}
	_, public := performJSONRequest(r, "GET", "/public-issues", "", nil)
	if bytes.Contains(public.Data, []byte("asset_images")) || bytes.Contains(public.Data, []byte(urls[1])) {
		t.Fatal("private material library exposed to visitors")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/assets/"+filepath.Base(urls[0]), nil))
	if w.Code != 404 {
		t.Fatal("unused material became publicly viewable")
	}
	updated.Images = []string{urls[0]}
	body, _ = json.Marshal(updated)
	_, reused := performJSONRequest(r, "PUT", fmt.Sprintf("/issues/%d", updated.ID), string(body), nil)
	if reused.Code != 200 {
		t.Fatalf("retained material could not be reused: %+v", reused)
	}
	if json.Unmarshal(reused.Data, &updated) != nil {
		t.Fatal("invalid save result")
	}
	updated.AssetImages = append(updated.AssetImages, "/api/voyage-assets/../../config.json")
	body, _ = json.Marshal(updated)
	_, invalid := performJSONRequest(r, "PUT", fmt.Sprintf("/issues/%d", updated.ID), string(body), nil)
	if invalid.Code != 400 {
		t.Fatal("invalid private material URL accepted")
	}
}
