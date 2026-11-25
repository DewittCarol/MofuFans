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

func TestProtocolsSendDistinctParameters(t *testing.T) {
	for _, protocol := range []string{"openai_chat", "openai_responses", "anthropic"} {
		t.Run(protocol, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["model"] != "custom-model" {
					t.Errorf("model not preserved: %v", body)
				}
				if _, ok := body["temperature"]; ok {
					t.Error("temperature was forced")
				}
				if protocol == "anthropic" {
					if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "test-secret" || r.Header.Get("anthropic-version") != "2023-06-01" || r.Header.Get("Authorization") != "" {
						t.Error("wrong Anthropic request")
					}
					if body["output_config"].(map[string]any)["effort"] != "high" || body["thinking"].(map[string]any)["budget_tokens"] != float64(1024) {
						t.Error("Anthropic effort/budget omitted")
					}
					fmt.Fprint(w, `{"stop_reason":"end_turn","content":[{"type":"thinking","thinking":"unrelated"},{"type":"text","text":"{\"is_spam\":true,\"reason\":\"test reason\"}"}]}`)
				} else {
					if r.Header.Get("Authorization") != "Bearer test-secret" || r.Header.Get("x-api-key") != "" {
						t.Error("wrong OpenAI authentication")
					}
					if _, ok := body["thinking"]; ok {
						t.Error("Anthropic thinking sent to OpenAI")
					}
					if protocol == "openai_chat" {
						if r.URL.Path != "/v1/chat/completions" || body["reasoning_effort"] != "high" || body["max_completion_tokens"] != float64(4096) {
							t.Error("wrong chat request")
						}
						fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"is_spam\":true,\"reason\":\"test reason\"}","reasoning_content":"ignored"}}]}`)
					} else {
						if r.URL.Path != "/v1/responses" || body["store"] != false || body["reasoning"].(map[string]any)["effort"] != "high" || body["max_output_tokens"] != float64(4096) {
							t.Error("wrong Responses request")
						}
						fmt.Fprint(w, `{"status":"completed","output":[{"type":"reasoning","content":[{"type":"output_text","text":"ignore"}]},{"type":"message","content":[{"type":"output_text","text":"{\"is_spam\":true,\"reason\":\"test reason\"}"}]}]}`)
					}
				}
			}))
			defer upstream.Close()
			options := Default()
			options.Protocol = "openai_chat"
			options.Protocol = protocol
			options.BaseURL = upstream.URL + "/v1"
			options.Model = "custom-model"
			options.ReasoningEffort = "high"
			if protocol == "anthropic" {
				options.ThinkingMode = "enabled"
				options.ThinkingBudget = 1024
			}
			value, err := NewClient().CheckSpam(context.Background(), options, "test-secret", "ads", "user content", 2)
			if err != nil || !value {
				t.Fatalf("unexpected result: %v %v", value, err)
			}
		})
	}
}
func TestModerationRejectsInvalidOrIncompleteOutput(t *testing.T) {
	for _, answer := range []string{`{}`, `{"is_spam":"false"}`, `{"is_spam":null}`, `{"is_spam":false,"extra":1}`, `{"is_spam":true} {"is_spam":false}`, `not JSON`} {
		t.Run(answer, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}}}})
			}))
			defer server.Close()
			options := Default()
			options.Protocol = "openai_chat"
			options.BaseURL = server.URL
			options.Model = "custom"
			if _, err := NewClient().CheckSpam(context.Background(), options, "test-key", "rules", "text", 0); err == nil {
				t.Error("invalid answer accepted")
			}
		})
	}
}
func TestCompatibilityDefaultsAndFalseVerdict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["reasoning_effort"]; ok {
			t.Error("default effort should omit parameter")
		}
		if _, ok := body["max_completion_tokens"]; ok {
			t.Error("wrong legacy token field")
		}
		if body["max_tokens"] != float64(4096) {
			t.Error("token limit missing")
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"is_spam\":false,\"reason\":\"no issue\"}"}}]}`)
	}))
	defer server.Close()
	options := Default()
	options.Protocol = "openai_chat"
	options.BaseURL = server.URL
	options.Model = "custom"
	options.ChatTokenField = "max_tokens"
	verdict, err := NewClient().CheckSpam(context.Background(), options, "test-key", "rules", "text", 0)
	if err != nil || verdict {
		t.Fatalf("valid negative verdict failed: %v %v", verdict, err)
	}
}
func TestModelListingPagination(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/models" || r.URL.Query().Get("limit") != "100" {
			t.Error("wrong model endpoint")
		}
		if calls == 1 {
			fmt.Fprint(w, `{"data":[{"id":"b","display_name":"Bee"}],"has_more":true,"last_id":"b"}`)
		} else {
			if r.URL.Query().Get("after_id") != "b" {
				t.Error("missing pagination cursor")
			}
			fmt.Fprint(w, `{"data":[{"id":"b"},{"id":"a"}],"has_more":false}`)
		}
	}))
	defer server.Close()
	options := Default()
	options.Protocol = "openai_chat"
	options.Protocol = "anthropic"
	options.BaseURL = server.URL
	models, err := NewClient().Models(context.Background(), options, "test-key")
	if err != nil || calls != 2 || models.Truncated || len(models.Models) != 2 || models.Models[0].ID != "a" || models.Models[1].Name != "Bee" {
		t.Fatalf("bad model listing: %v %v", models, err)
	}
}
func TestRedirectsAndErrorsDoNotLeakSecrets(t *testing.T) {
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer source.Close()
	options := Default()
	options.Protocol = "openai_chat"
	options.BaseURL = source.URL
	if _, err := NewClient().Models(context.Background(), options, "sensitive-key"); err == nil || reached {
		t.Fatal("redirect forwarded credentials")
	}
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprint(w, "sensitive-key private question details")
	}))
	defer failed.Close()
	options.BaseURL = failed.URL
	_, err := NewClient().Models(context.Background(), options, "sensitive-key")
	if err == nil || strings.Contains(err.Error(), "sensitive-key") || strings.Contains(err.Error(), "private question") {
		t.Fatalf("upstream detail leaked: %v", err)
	}
}
func TestValidation(t *testing.T) {
	options := Default()
	options.Protocol = "openai_chat"
	options.Protocol = "anthropic"
	options.Model = "custom"
	options.ThinkingMode = "enabled"
	for _, budget := range []int{0, 1023, 4096, 8192} {
		options.ThinkingBudget = budget
		if Validate(options, true) == nil {
			t.Errorf("invalid budget accepted: %d", budget)
		}
	}
	options.ThinkingBudget = 1024
	if err := Validate(options, true); err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"https://key@example.com/v1", "http://example.com/v1", "https://example.com/v1?key=secret", "file:///tmp/api"} {
		options.BaseURL = address
		if Validate(options, false) == nil {
			t.Errorf("unsafe URL accepted: %s", address)
		}
	}
	if Normalize(Options{BaseURL: " https://api.example.com/ "}).BaseURL != "https://api.example.com/v1" {
		t.Fatal("root URL normalization failed")
	}
}
