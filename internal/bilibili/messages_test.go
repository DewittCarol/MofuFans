package bilibili

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func messageTestLogin(t *testing.T) *GuardImporter {
	t.Helper()
	g, err := RestoreGuardImporter(`{"cookies":{"SESSDATA":"secret","bili_jct":"csrf","DedeUserID":"1234","buvid3":"server-device"},"device":"stable-client-id"}`)
	if err != nil {
		t.Fatal(err)
	}
	return g
}
func TestMessageProtocolAndNoAutomaticReplay(t *testing.T) {
	for _, outcome := range []string{"ok", "risk", "unknown", "login", "recipient", "ordinary"} {
		t.Run(outcome, func(t *testing.T) {
			g := messageTestLogin(t)
			posts := 0
			g.http.Transport = guardTransport(func(r *http.Request) (*http.Response, error) {
				body := `{"code":0,"data":{"isLogin":true,"mid":1234,"wbi_img":{"img_url":"https://i0.hdslb.com/bfs/wbi/7cd084941338484aae1ad9425b84077c.png","sub_url":"https://i0.hdslb.com/bfs/wbi/4932caff0ff746eab6f01bf08b70ac45.png"}}}`
				status := 200
				if r.Method == "POST" {
					posts++
					if r.URL.Host != "api.vc.bilibili.com" || r.URL.Query().Get("w_rid") == "" {
						t.Fatal("missing normal signed endpoint")
					}
					if err := r.ParseForm(); err != nil {
						t.Fatal(err)
					}
					if r.PostForm.Get("csrf") != "csrf" || r.PostForm.Get("msg[receiver_id]") != "10001" || r.PostForm.Get("msg[content]") != `{"content":"测试消息"}` {
						t.Fatal("invalid message form")
					}
					if cookie, err := r.Cookie("SESSDATA"); err != nil || cookie.Value != "secret" {
						t.Fatal("login not sent")
					}
					body = `{"code":0,"data":{"msg_key":1234567890123456789}}`
					switch outcome {
					case "risk":
						status = 412
						body = "blocked"
					case "unknown":
						return nil, errors.New("network error containing secret URL")
					case "login":
						body = `{"code":-101}`
					case "recipient":
						body = `{"code":21047}`
					case "ordinary":
						body = `{"code":25003}`
					}
				}
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})
			key, err := g.SendMessage(context.Background(), "1234", "10001", "测试消息")
			if posts != 1 {
				t.Fatal("message automatically replayed")
			}
			if outcome == "ok" {
				if err != nil || key != "1234567890123456789" {
					t.Fatalf("receipt lost precision: %s %v", key, err)
				}
			} else {
				var detail *MessageError
				if !errors.As(err, &detail) || strings.Contains(err.Error(), "secret") {
					t.Fatal("unsafe or missing error")
				}
				if outcome == "unknown" && !detail.Uncertain {
					t.Fatal("transport failure treated as unsent")
				}
				if outcome == "risk" && detail.RetryAt.IsZero() {
					t.Fatal("missing cooldown")
				}
				if outcome == "login" && !detail.Login {
					t.Fatal("invalid login not detected")
				}
				if outcome == "recipient" && (!detail.Recipient || !detail.RetryAt.IsZero() || detail.Uncertain || detail.Login) {
					t.Fatal("recipient rejection incorrectly blocked the whole account")
				}
				if outcome == "ordinary" && (!detail.RetryAt.IsZero() || detail.Uncertain || detail.Login) {
					t.Fatal("single ordinary rejection requested cooldown")
				}
			}
		})
	}
}
func TestPersistedLoginRetainsOnlyAllowedCookiesAndDevice(t *testing.T) {
	g := messageTestLogin(t)
	raw, err := g.ExportLogin()
	if err != nil {
		t.Fatal(err)
	}
	next, err := RestoreGuardImporter(raw)
	if err != nil {
		t.Fatal(err)
	}
	if next.device != g.device {
		t.Fatal("device changed on restore")
	}
	if _, err = next.SendMessage(context.Background(), "1234", "1234", "test"); err == nil {
		t.Fatal("self message accepted")
	}
}

func TestMessageNavigationRateLimitKeepsExplicitRetryAfter(t *testing.T) {
	g := messageTestLogin(t)
	posts := 0
	g.http.Transport = guardTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			posts++
		}
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"1200"}}, Body: io.NopCloser(strings.NewReader("limited")), Request: r}, nil
	})
	_, err := g.SendMessage(context.Background(), "1234", "10001", "测试消息")
	var detail *MessageError
	if !errors.As(err, &detail) || time.Until(detail.RetryAt) < 19*time.Minute || posts != 0 {
		t.Fatal("navigation limit bypassed or its wait shortened")
	}
}
