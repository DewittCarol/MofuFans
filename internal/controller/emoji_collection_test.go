package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"gorm.io/gorm"
	"mofufans/internal/database"
)

func TestDeleteEmojiCollectionPreservesHistoryAndSurvivesRestart(t *testing.T) {
	for _, kind := range []string{"legacy-joi", "joi-original", "custom", "empty"} {
		t.Run(kind, func(t *testing.T) {
			router, _ := emojiTestRouter(t)
			var collection database.EmojiCollection
			if kind == "custom" || kind == "empty" {
				_, result := performJSONRequest(router, "POST", "/emoji-collections", `{"name":"待删除合集"}`, nil)
				if result.Code != 200 || json.Unmarshal(result.Data, &collection) != nil {
					t.Fatalf("create: %+v", result)
				}
			} else if err := database.DB.Where("system_key = ?", kind).First(&collection).Error; err != nil {
				t.Fatal(err)
			}
			var pngData bytes.Buffer
			if err := png.Encode(&pngData, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
				t.Fatal(err)
			}
			var uploaded database.EmojiAsset
			if kind == "custom" {
				result := emojiUpload(t, router, "input", "历史图片.png", pngData.Bytes(), collection.ID)
				if result.Code != 200 || json.Unmarshal(result.Data, &uploaded) != nil {
					t.Fatalf("upload: %+v", result)
				}
			}
			var before []database.EmojiAsset
			database.DB.Order("id asc").Find(&before)
			endpoint := fmt.Sprintf("/emoji-collections/%d", collection.ID)
			for range 2 { // A repeated request is safe after a lost response.
				_, result := performJSONRequest(router, "DELETE", endpoint, "", nil)
				if result.Code != 200 {
					t.Fatalf("delete: %+v", result)
				}
			}
			for range 2 {
				if err := database.InitializeEmojiCatalog(); err != nil {
					t.Fatal(err)
				}
			}
			_, response := performJSONRequest(router, "GET", "/emoji-catalog", "", nil)
			var catalog struct {
				Emojis      []database.EmojiAsset      `json:"emojis"`
				Collections []database.EmojiCollection `json:"collections"`
			}
			if response.Code != 200 || json.Unmarshal(response.Data, &catalog) != nil || len(catalog.Emojis) != len(before) || slices.ContainsFunc(catalog.Collections, func(item database.EmojiCollection) bool { return item.ID == collection.ID }) {
				t.Fatalf("deleted collection returned or historical assets disappeared: %+v", response)
			}
			for _, old := range before {
				index := slices.IndexFunc(catalog.Emojis, func(item database.EmojiAsset) bool { return item.ID == old.ID })
				if index < 0 {
					t.Fatalf("missing historical asset %d", old.ID)
				}
				saved := catalog.Emojis[index]
				deleted := old.IsDeleted || (old.Kind == "input" && old.CollectionID == collection.ID)
				if saved.IsDeleted != deleted || saved.Value != old.Value || saved.URL != old.URL || saved.Name != old.Name || saved.SortOrder != old.SortOrder || saved.CollectionID != old.CollectionID {
					t.Fatalf("unexpected history or picker state: %+v => %+v", old, saved)
				}
				if old.Kind == "input" && old.CollectionID == collection.ID {
					_, restored := performJSONRequest(router, "PUT", "/emojis/status", fmt.Sprintf(`{"ids":[%d],"is_deleted":false}`, old.ID), nil)
					if restored.Code != 409 {
						t.Fatalf("stale undo revived removed pack: %+v", restored)
					}
					_, reordered := performJSONRequest(router, "PUT", "/emojis/order", fmt.Sprintf(`{"kind":"input","collection_id":%d,"ids":[%d],"expected_ids":[%d]}`, collection.ID, old.ID, old.ID), nil)
					if reordered.Code != 409 {
						t.Fatalf("stale reorder accepted: %+v", reordered)
					}
				}
			}
			_, renamed := performJSONRequest(router, "PUT", endpoint, `{"name":"不能复活"}`, nil)
			if renamed.Code != 404 {
				t.Fatalf("stale rename revived collection: %+v", renamed)
			}
			if result := emojiUpload(t, router, "input", "不能上传.png", pngData.Bytes(), collection.ID); result.Code != 400 {
				t.Fatalf("upload to removed collection accepted: %+v", result)
			}
			if kind == "legacy-joi" {
				if result := emojiUpload(t, router, "input", "旧客户端.png", pngData.Bytes()); result.Code != 400 {
					t.Fatal("legacy upload recreated deleted common pack")
				}
			}
			if uploaded.ID != 0 {
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, httptest.NewRequest("GET", strings.TrimPrefix(uploaded.URL, "/api"), nil))
				if recorder.Code != 200 || !bytes.Equal(recorder.Body.Bytes(), pngData.Bytes()) {
					t.Fatal("historical image no longer served")
				}
			}
			var saved database.EmojiCollection
			database.DB.First(&saved, collection.ID)
			if !saved.IsDeleted || saved.Name != collection.Name {
				t.Fatal("collection tombstone lost")
			}
		})
	}
}

func TestDeleteLastEmojiCollectionAndCreateAgain(t *testing.T) {
	router, _ := emojiTestRouter(t)
	var collections []database.EmojiCollection
	database.DB.Find(&collections)
	for _, collection := range collections {
		_, result := performJSONRequest(router, "DELETE", fmt.Sprintf("/emoji-collections/%d", collection.ID), "", nil)
		if result.Code != 200 {
			t.Fatalf("delete last collection: %+v", result)
		}
	}
	if err := database.InitializeEmojiCatalog(); err != nil {
		t.Fatal(err)
	}
	_, response := performJSONRequest(router, "GET", "/emoji-catalog", "", nil)
	var catalog struct {
		Collections []database.EmojiCollection `json:"collections"`
	}
	if json.Unmarshal(response.Data, &catalog) != nil || catalog.Collections == nil || len(catalog.Collections) != 0 {
		t.Fatal("expected empty collection array")
	}
	_, created := performJSONRequest(router, "POST", "/emoji-collections", `{"name":"常用表情"}`, nil)
	var fresh database.EmojiCollection
	if created.Code != 200 || json.Unmarshal(created.Data, &fresh) != nil {
		t.Fatal("cannot create after deleting last pack")
	}
	if slices.ContainsFunc(collections, func(item database.EmojiCollection) bool { return item.ID == fresh.ID }) {
		t.Fatal("reused deleted pack")
	}
	for _, id := range []string{"0", "invalid", "999999"} {
		_, result := performJSONRequest(router, "DELETE", "/emoji-collections/"+id, "", nil)
		want := 400
		if id == "999999" {
			want = 404
		}
		if result.Code != want {
			t.Fatalf("invalid id %s returned %+v", id, result)
		}
	}
}

func TestDeleteEmojiCollectionRollsBackOnAssetFailure(t *testing.T) {
	router, _ := emojiTestRouter(t)
	var collection database.EmojiCollection
	database.DB.First(&collection)
	callback := "test:fail_collection_assets"
	if err := database.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "emoji_assets" {
			tx.AddError(errors.New("injected asset write failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer database.DB.Callback().Update().Remove(callback)
	_, result := performJSONRequest(router, "DELETE", fmt.Sprintf("/emoji-collections/%d", collection.ID), "", nil)
	if result.Code != 500 {
		t.Fatalf("expected failed transaction: %+v", result)
	}
	database.DB.First(&collection, collection.ID)
	var deleted int64
	database.DB.Model(&database.EmojiAsset{}).Where("collection_id = ? AND is_deleted = ?", collection.ID, true).Count(&deleted)
	if collection.IsDeleted || deleted != 0 {
		t.Fatal("failed deletion partially committed")
	}
}

func TestEmojiCollectionsUploadAndStableHistory(t *testing.T) {
	router, _ := emojiTestRouter(t)
	var original database.EmojiCollection
	if err := database.DB.Where("system_key = ?", "legacy-joi").First(&original).Error; err != nil || original.Name != "常用表情" {
		t.Fatalf("default collection: %+v %v", original, err)
	}
	var missing int64
	database.DB.Model(&database.EmojiAsset{}).Where("kind = ? AND collection_id = 0", "input").Count(&missing)
	if missing != 0 {
		t.Fatal("legacy input emojis were not grouped")
	}
	_, created := performJSONRequest(router, "POST", "/emoji-collections", `{"name":"示例日常"}`, nil)
	var collection database.EmojiCollection
	if created.Code != 200 || json.Unmarshal(created.Data, &collection) != nil {
		t.Fatalf("create failed: %+v", created)
	}
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	result := emojiUpload(t, router, "input", "你好.png", data.Bytes(), collection.ID)
	var uploaded database.EmojiAsset
	if result.Code != 200 || json.Unmarshal(result.Data, &uploaded) != nil || uploaded.CollectionID != collection.ID {
		t.Fatalf("upload did not land in selected collection: %+v", result)
	}
	bad := emojiUpload(t, router, "input", "wrong.png", data.Bytes(), 999999)
	if bad.Code != 400 {
		t.Fatal("nonexistent collection accepted")
	}
	_, renamed := performJSONRequest(router, "PUT", fmt.Sprintf("/emoji-collections/%d", collection.ID), `{"name":"示例表情"}`, nil)
	if renamed.Code != 200 {
		t.Fatalf("rename failed: %+v", renamed)
	}
	// Restart must preserve names, deleted entries, memberships and permanent text tokens.
	database.DB.Model(&uploaded).Updates(map[string]any{"is_deleted": true, "sort_order": 9})
	database.DB.Model(&original).Update("name", "管理员改过的名字")
	if err := database.InitializeEmojiCatalog(); err != nil {
		t.Fatal(err)
	}
	var saved database.EmojiAsset
	database.DB.First(&saved, uploaded.ID)
	if saved.Value != uploaded.Value || saved.URL != uploaded.URL || !saved.IsDeleted || saved.CollectionID != collection.ID || saved.SortOrder != 9 {
		t.Fatalf("history changed: %+v", saved)
	}
	database.DB.First(&original, original.ID)
	if original.Name != "管理员改过的名字" {
		t.Fatal("default collection name overwritten by restart")
	}
	_, catalog := performJSONRequest(router, "GET", "/emoji-catalog", "", nil)
	var content struct {
		Emojis      []database.EmojiAsset      `json:"emojis"`
		Collections []database.EmojiCollection `json:"collections"`
	}
	if catalog.Code != 200 || json.Unmarshal(catalog.Data, &content) != nil || len(content.Collections) != 3 || !slices.ContainsFunc(content.Emojis, func(asset database.EmojiAsset) bool { return asset.ID == uploaded.ID && asset.IsDeleted }) {
		t.Fatal("catalog lost historical metadata")
	}
}

func TestEmojiOrderScopedAtomicAndPersistent(t *testing.T) {
	router, _ := emojiTestRouter(t)
	var scopes []struct {
		Kind         string
		CollectionID uint
	}
	database.DB.Model(&database.EmojiAsset{}).Distinct("kind", "collection_id").Find(&scopes)
	for _, scope := range scopes {
		kind := scope.Kind
		var initial []database.EmojiAsset
		database.DB.Where("kind = ? AND collection_id = ? AND is_deleted = ?", kind, scope.CollectionID, false).Order("sort_order asc, id asc").Find(&initial)
		ids := make([]uint, len(initial))
		for i, item := range initial {
			ids[i] = item.ID
		}
		wanted := slices.Clone(ids)
		slices.Reverse(wanted)
		save := func(order, expected []uint) testAPIResponse {
			body, _ := json.Marshal(map[string]any{"kind": kind, "collection_id": initial[0].CollectionID, "ids": order, "expected_ids": expected})
			_, result := performJSONRequest(router, "PUT", "/emojis/order", string(body), nil)
			return result
		}
		if res := save(wanted, ids); res.Code != 200 {
			t.Fatalf("sort failed: %+v", res)
		}
		if err := database.InitializeEmojiCatalog(); err != nil {
			t.Fatal(err)
		}
		if res := save(ids, ids); res.Code != 409 {
			t.Fatal("stale browser silently overwrote order")
		}
		invalid := slices.Clone(wanted)
		invalid[0] = invalid[1]
		if res := save(invalid, wanted); res.Code != 400 {
			t.Fatal("duplicate IDs accepted")
		}
		var other database.EmojiAsset
		database.DB.Where("kind != ?", kind).First(&other)
		invalid[0] = other.ID
		if res := save(invalid, wanted); res.Code != 400 {
			t.Fatal("cross-kind order accepted")
		}
		var saved []database.EmojiAsset
		database.DB.Where("kind = ? AND collection_id = ? AND is_deleted = ?", kind, scope.CollectionID, false).Order("sort_order asc, id asc").Find(&saved)
		for i, item := range saved {
			if item.ID != wanted[i] {
				t.Fatal("order did not persist or rejected request partially changed it")
			}
		}
		order, err := database.NextEmojiSort(database.DB, kind, initial[0].CollectionID)
		if err != nil || order <= saved[len(saved)-1].SortOrder {
			t.Fatal("new emojis would not append")
		}
	}
}
