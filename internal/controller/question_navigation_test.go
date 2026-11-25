package controller

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"mofufans/internal/adminauth"
	"mofufans/internal/database"
)

type questionNavigationResponse struct {
	Code int `json:"code"`
	Data *struct {
		Questions []database.Question `json:"questions"`
		Total     int                 `json:"total"`
		Page      int                 `json:"page"`
		MinID     uint                `json:"min_id"`
		MaxID     uint                `json:"max_id"`
	} `json:"data"`
}

func TestQuestionNavigationRespectsFiltersAndOrdering(t *testing.T) {
	appearanceDB(t)
	for i := 1; i <= 35; i++ {
		q := database.Question{BaseModel: database.BaseModel{ID: uint(i * 2), CreatedAt: time.Date(2026, 1, 1, 0, i%3, 0, 0, time.UTC)},
			Content: fmt.Sprintf("post %d", i), TagID: i%2 + 1, IsPublish: i%4 != 0, IsSpam: i%5 == 0,
			IsArchive: i%3 == 0, IsRainbow: i%2 == 0, IsHide: i%7 == 0, ImagesNum: i % 4}
		if err := database.DB.Create(&q).Error; err != nil {
			t.Fatal(err)
		}
	}
	// A removed record and non-contiguous IDs must not be treated as row numbers.
	database.DB.Delete(&database.Question{}, 10)
	database.DB.AutoMigrate(&database.Admin{})
	admin := database.Admin{Username: "fixture-owner", Password: "fixture-password"}
	database.DB.Create(&admin)
	r := gin.New()
	r.Use(sessions.Sessions("test", cookie.NewStore([]byte("navigation-test-only"))))
	r.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-Admin") == "yes" {
			sessions.Default(c).Set("authed", true)
			sessions.Default(c).Set("user", admin.ID)
			sessions.Default(c).Set("auth_stamp", adminauth.Stamp(admin))
		}
		c.Next()
	})
	ctl := &QuestionController{}
	r.GET("/question", ctl.Get)
	call := func(query string, admin bool) questionNavigationResponse {
		t.Helper()
		req := httptest.NewRequest("GET", "/question?"+query, nil)
		if admin {
			req.Header.Set("X-Test-Admin", "yes")
		}
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, req)
		var result questionNavigationResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	for _, admin := range []bool{false, true} {
		for _, filter := range []string{"", "&publish=false", "&publish=true&is_spam=false", "&tag_id=1&archive=false", "&search=post+2&rainbow=true", "&hide=true"} {
			for _, orderBy := range []string{"id", "created_at", "images_num", "is_archive", "is_publish", "is_rainbow"} {
				for _, direction := range []string{"asc", "desc"} {
					query := "order_by=" + orderBy + "&order=" + direction + filter
					all := call(query+"&page_size=100", admin)
					if all.Code != 200 || all.Data == nil {
						t.Fatalf("list %s: %+v", query, all)
					}
					if all.Data.Total != len(all.Data.Questions) {
						t.Fatal("count does not match filtered rows")
					}
					for index, q := range all.Data.Questions {
						got := call(query+fmt.Sprintf("&page_size=5&locate_id=%d", q.ID), admin)
						if got.Code != 200 || got.Data == nil {
							t.Fatalf("locate %s #%d: %+v", query, q.ID, got)
						}
						if got.Data.Page != index/5+1 || got.Data.Total != all.Data.Total {
							t.Fatalf("wrong page/count for %s #%d: %+v", query, q.ID, got.Data)
						}
						start := index / 5 * 5
						end := start + 5
						if end > len(all.Data.Questions) {
							end = len(all.Data.Questions)
						}
						if len(got.Data.Questions) != end-start {
							t.Fatal("wrong page size")
						}
						for j, row := range got.Data.Questions {
							if row.ID != all.Data.Questions[start+j].ID {
								t.Fatal("located page differs from ordinary pagination")
							}
						}
						if got.Data.MinID != all.Data.MinID || got.Data.MaxID != all.Data.MaxID {
							t.Fatal("bounds changed while locating")
						}
					}
				}
			}
		}
	}
	public := call("page_size=100&publish=false", false)
	for _, q := range public.Data.Questions {
		if !q.IsPublish {
			t.Fatal("anonymous query leaked unpublished record")
		}
	}
	// Known unpublished ID, deleted ID and unknown ID give the same empty result.
	for _, id := range []uint{8, 10, 999999} {
		got := call(fmt.Sprintf("locate_id=%d&publish=false", id), false)
		if got.Code != 404 || got.Data != nil {
			t.Fatalf("private/missing ID disclosed: %+v", got)
		}
	}
	if got := call("locate_id=4&tag_id=2", true); got.Code != 404 {
		t.Fatal("locate bypassed tag filter")
	}
	empty := call("tag_id=999", false)
	if empty.Code != 200 || empty.Data.Total != 0 || empty.Data.MinID != 0 || empty.Data.MaxID != 0 {
		t.Fatal("empty-list bounds incorrect")
	}
	for _, value := range []string{"0", "-1", "invalid"} {
		if got := call("locate_id="+value, false); got.Code != 400 {
			t.Fatal("invalid target accepted")
		}
	}
}
