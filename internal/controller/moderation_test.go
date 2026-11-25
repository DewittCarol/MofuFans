package controller

import (
	"fmt"
	"mofufans/internal/database"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestModerationOnlyBroadcastsNewArchive(t *testing.T) {
	appearanceDB(t)
	q := database.Question{TagID: 1, Content: "moderation test"}
	if err := database.DB.Create(&q).Error; err != nil {
		t.Fatal(err)
	}
	controller := &QuestionController{eventChan: make(chan SSEvent, 8)}
	router := gin.New()
	router.PUT("/question/:id", controller.Put)
	for _, step := range []struct {
		name    string
		body    string
		archive bool
		event   bool
	}{
		{"publish", `{"tag_id":1,"is_publish":true}`, false, false},
		{"rainbow", `{"tag_id":1,"is_publish":true,"is_rainbow":true}`, false, false},
		{"archive", `{"tag_id":1,"is_publish":true,"is_archive":true}`, true, true},
		{"edit archived", `{"tag_id":1,"is_publish":false,"is_archive":true}`, true, false},
		{"unarchive", `{"tag_id":1,"is_archive":false}`, false, false},
		{"intercept", `{"tag_id":1,"is_spam":true}`, false, false},
	} {
		t.Run(step.name, func(t *testing.T) {
			_, response := performJSONRequest(router, "PUT", fmt.Sprintf("/question/%d", q.ID), step.body, nil)
			if response.Code != 200 {
				t.Fatalf("update failed: %+v", response)
			}
			var saved database.Question
			if err := database.DB.First(&saved, q.ID).Error; err != nil {
				t.Fatal(err)
			}
			if saved.IsArchive != step.archive {
				t.Fatalf("incorrect archive state: %v", saved.IsArchive)
			}
			select {
			case event := <-controller.eventChan:
				if !step.event || event.Type != SSEventArchive || event.Data != q.ID {
					t.Fatalf("unexpected archive event: %+v", event)
				}
			default:
				if step.event {
					t.Fatal("missing archive event")
				}
			}
		})
	}
}
