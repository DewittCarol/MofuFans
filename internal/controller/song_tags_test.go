package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"mofufans/internal/database"
	"net/http/httptest"
	"testing"
)

func TestLibrarySongTagAssignments(t *testing.T) {
	libraryTestDB(t)
	if err := database.InitializeSongTags(); err != nil {
		t.Fatal(err)
	}
	if err := database.InitializeSongTags(); err != nil {
		t.Fatal(err)
	}
	var count int64
	database.DB.Model(&database.SongTag{}).Count(&count)
	if count != 20 {
		t.Fatal("seed tags duplicated")
	}
	var language, genre database.SongTag
	database.DB.First(&language, "name = ?", "中文")
	database.DB.First(&genre, "name = ?", "RnB")
	ctl := &SongController{}
	r := gin.New()
	r.POST("/songs", ctl.Save)
	r.PUT("/songs/:id", ctl.Save)
	r.GET("/songs", ctl.List)
	r.POST("/tags", ctl.CreateTag)
	call := func(method, path string, body any) []byte {
		t.Helper()
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		var result struct{ Code int }
		_ = json.Unmarshal(response.Body.Bytes(), &result)
		if result.Code != 200 {
			t.Fatalf("request failed: %s", response.Body.String())
		}
		return response.Body.Bytes()
	}
	var response struct{ Data database.Song }
	raw := call("POST", "/songs", map[string]any{"title": "测试歌曲", "tag_ids": []uint{language.ID, genre.ID, genre.ID}})
	_ = json.Unmarshal(raw, &response)
	if len(response.Data.Tags) != 2 {
		t.Fatal("tag assignment failed")
	}
	path := fmt.Sprintf("/songs/%d", response.Data.ID)
	// Editing through a previous version retains existing tag associations.
	raw = call("PUT", path, map[string]any{"title": "改名"})
	_ = json.Unmarshal(raw, &response)
	if len(response.Data.Tags) != 2 {
		t.Fatal("legacy edit erased tags")
	}
	var listing struct {
		Data struct {
			Total int
			Items []database.Song
		}
	}
	raw = call("GET", fmt.Sprintf("/songs?language_id=%d&genre_id=%d", language.ID, genre.ID), nil)
	_ = json.Unmarshal(raw, &listing)
	if listing.Data.Total != 1 || len(listing.Data.Items) != 1 || len(listing.Data.Items[0].Tags) != 2 {
		t.Fatal("combined tag filters failed")
	}
	raw = call("GET", fmt.Sprintf("/songs?genre_id=%d", language.ID), nil)
	_ = json.Unmarshal(raw, &listing)
	if listing.Data.Total != 0 {
		t.Fatal("filter did not enforce tag category")
	}
	raw = call("PUT", path, map[string]any{"title": "改名", "tag_ids": []uint{}})
	_ = json.Unmarshal(raw, &response)
	if len(response.Data.Tags) != 0 {
		t.Fatal("clearing tags failed")
	}
	var tagResponse struct{ Data database.SongTag }
	raw = call("POST", "/tags", map[string]string{"kind": "genre", "name": " rnb "})
	_ = json.Unmarshal(raw, &tagResponse)
	if tagResponse.Data.ID != genre.ID {
		t.Fatal("case duplicate tag created")
	}
	raw = call("POST", "/tags", map[string]string{"kind": "language", "name": "德文"})
	_ = json.Unmarshal(raw, &tagResponse)
	if tagResponse.Data.ID == 0 || tagResponse.Data.Kind != "language" {
		t.Fatal("custom tag creation failed")
	}
}
