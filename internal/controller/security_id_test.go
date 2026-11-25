package controller

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"mofufans/internal/database"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestURLIDCannotBecomeSQLCondition(t *testing.T) {
	for _, method := range []string{"PUT", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			appearanceDB(t)
			tag := database.Tag{BaseModel: database.BaseModel{ID: 2}, TagName: "original"}
			database.DB.Create(&tag)
			q := database.Question{BaseModel: database.BaseModel{ID: 2}, Content: "original", TagID: 2}
			database.DB.Create(&q)
			r := gin.New()
			r.PUT("/tag/:id", (&TagController{}).Put)
			r.DELETE("/question/:id", (&QuestionController{}).Delete)
			path := "/tag/id=2"
			if method == "DELETE" {
				path = "/question/id=2"
			}
			req := httptest.NewRequest(method, path, strings.NewReader(`{"tag_name":"changed","description":"changed"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			var result struct{ Code int }
			json.Unmarshal(w.Body.Bytes(), &result)
			if result.Code != 404 {
				t.Fatalf("SQL-shaped URL selected a row: %s", w.Body.String())
			}
			var stored database.Tag
			database.DB.First(&stored, 2)
			if stored.TagName != "original" {
				t.Fatal("unrelated tag changed")
			}
			var count int64
			database.DB.Model(&database.Question{}).Where("id = ?", 2).Count(&count)
			if count != 1 {
				t.Fatal("unrelated question deleted")
			}
		})
	}
}
