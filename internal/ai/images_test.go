package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOptionalImagesAcrossProtocols(t *testing.T) {
	for _, protocol := range []string{"openai_chat", "openai_responses", "anthropic"} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", protocol, enabled), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					key := "messages"
					index := 0
					if protocol == "openai_chat" {
						index = 1
					}
					if protocol == "openai_responses" {
						key = "input"
					}
					content := body[key].([]any)[index].(map[string]any)["content"]
					if !enabled {
						if _, ok := content.(string); !ok {
							t.Error("disabled vision sent an image")
						}
					} else {
						parts, ok := content.([]any)
						if !ok || len(parts) != 3 {
							t.Fatalf("wrong parts: %#v", content)
						}
						part := parts[2].(map[string]any)
						switch protocol {
						case "openai_chat":
							if part["type"] != "image_url" || part["image_url"].(map[string]any)["url"] != "data:image/png;base64,AQID" {
								t.Error("invalid chat image")
							}
						case "openai_responses":
							if part["type"] != "input_image" || part["image_url"] != "data:image/png;base64,AQID" {
								t.Error("invalid responses image")
							}
						case "anthropic":
							source := part["source"].(map[string]any)
							if part["type"] != "image" || source["type"] != "base64" || source["media_type"] != "image/png" || source["data"] != "AQID" {
								t.Error("invalid anthropic image")
							}
						}
					}
					answer := `{"is_spam":false,"reason":"未发现明显问题"}`
					switch protocol {
					case "openai_chat":
						json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": answer}}}})
					case "openai_responses":
						json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]string{"type": "output_text", "text": answer}}}}})
					case "anthropic":
						json.NewEncoder(w).Encode(map[string]any{"stop_reason": "end_turn", "content": []any{map[string]string{"type": "text", "text": answer}}})
					}
				}))
				defer server.Close()
				o := Default()
				o.Protocol = protocol
				o.ReviewImages = enabled
				o.Model = "custom"
				o.BaseURL = server.URL
				r, e := NewClient().Review(context.Background(), o, "test-key", "rules", "text", "topic", 1, []Image{{MIME: "image/png", Data: []byte{1, 2, 3}}})
				if e != nil || r.IsSpam || !strings.Contains(r.Reason, "未发现") {
					t.Fatalf("bad result: %+v %v", r, e)
				}
			})
		}
	}
}
