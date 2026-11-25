package controller

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mofufans/internal/database"
)

func TestLibraryUpdatedIsGlobalAndPublishedOnly(t *testing.T) {
	libraryTestDB(t)
	r := gin.New()
	r.GET("/songs", (&SongController{}).List)
	r.GET("/gallery", (&GalleryController{}).List)
	check := func(path string, want *time.Time) {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		var response struct {
			Code int
			Data struct {
				LastUpdated *time.Time `json:"last_updated_at"`
			}
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Code != 200 {
			t.Fatalf("%s: %s (%v)", path, w.Body.String(), err)
		}
		got := response.Data.LastUpdated
		if (want == nil) != (got == nil) || (want != nil && !want.Equal(*got)) {
			t.Fatalf("%s: got %v, want %v", path, got, want)
		}
	}
	check("/songs", nil)
	check("/gallery", nil)
	old := time.Date(2026, 9, 1, 4, 30, 0, 0, time.UTC)
	recent := old.Add(24 * time.Hour)
	private := recent.Add(24 * time.Hour)
	for _, song := range []database.Song{
		{BaseModel: database.BaseModel{UpdatedAt: old}, Title: "旧歌", Performed: true},
		{BaseModel: database.BaseModel{UpdatedAt: recent}, Title: "新歌"},
	} {
		if err := database.DB.Create(&song).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, work := range []database.GalleryWork{
		{BaseModel: database.BaseModel{UpdatedAt: old}, Source: "streamer", Status: "published"},
		{BaseModel: database.BaseModel{UpdatedAt: recent}, Source: "fan", Status: "published"},
		{BaseModel: database.BaseModel{UpdatedAt: private}, Source: "fan", Status: "pending"},
		{BaseModel: database.BaseModel{UpdatedAt: private}, Source: "streamer", Status: "rejected"},
	} {
		if err := database.DB.Create(&work).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"/songs", "/songs?performed=true", "/songs?search=不存在&page=4", "/gallery", "/gallery?source=streamer", "/gallery?source=fan&page=3&status=pending"} {
		check(path, &recent)
	}
}
