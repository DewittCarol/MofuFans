package bilibili

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type guardTransport func(*http.Request) (*http.Response, error)

func (f guardTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func guardTestClient(t *testing.T, handler func(*http.Request) (any, http.Header)) *GuardImporter {
	t.Helper()
	g := NewGuardImporter()
	g.pause = 0
	g.gate = &guardRequestGate{}
	g.http.Transport = guardTransport(func(r *http.Request) (*http.Response, error) {
		body, headers := handler(r)
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: 200, Header: headers, Body: io.NopCloser(strings.NewReader(string(raw))), Request: r}, nil
	})
	return g
}
func guardEnvelope(data any) any { return map[string]any{"code": 0, "data": data} }
func guardNav() any {
	return guardEnvelope(map[string]any{"isLogin": true, "mid": 1234, "uname": "主播"})
}
func guardPage(rows []map[string]any, more int) any {
	return guardEnvelope(map[string]any{"list": rows, "has_more": more})
}

func TestGuardQRSupportsCurrentAndLegacyScanAddresses(t *testing.T) {
	for _, scanURL := range []string{
		"https://account.bilibili.com/h5/account-h5/auth/scan-web?token=test-qr",
		"https://passport.bilibili.com/h5-app/passport/login/scan?qrcode_key=test-qr",
	} {
		g := guardTestClient(t, func(r *http.Request) (any, http.Header) {
			return guardEnvelope(map[string]any{"url": scanURL, "qrcode_key": "test-key"}), nil
		})
		qr, err := g.QR(context.Background())
		if err != nil || qr.URL != scanURL || qr.Key != "test-key" {
			t.Fatalf("official scan address rejected: %v", err)
		}
	}
	for _, scanURL := range []string{
		"http://account.bilibili.com/scan", "https://account.bilibili.com.evil.test/scan",
		"https://user@account.bilibili.com/scan", "https://account.bilibili.com:444/scan",
		"https://api.bilibili.com/scan", "https://passport.biligame.com/scan",
	} {
		g := guardTestClient(t, func(r *http.Request) (any, http.Header) {
			return guardEnvelope(map[string]any{"url": scanURL, "qrcode_key": "test-key"}), nil
		})
		if _, err := g.QR(context.Background()); err == nil {
			t.Fatal("invalid scan address accepted")
		}
	}
}

func TestGuardQRLoginKeepsCookiesAndReturnsOnlyAccount(t *testing.T) {
	for _, queryCookies := range []bool{false, true} {
		t.Run(map[bool]string{true: "cross-domain", false: "set-cookie"}[queryCookies], func(t *testing.T) {
			poll := 0
			g := guardTestClient(t, func(r *http.Request) (any, http.Header) {
				switch r.URL.Path {
				case "/x/passport-login/web/qrcode/generate":
					return guardEnvelope(map[string]any{"url": "https://passport.bilibili.com/scan?qrcode_key=private-key", "qrcode_key": "private-key"}), nil
				case "/x/passport-login/web/qrcode/poll":
					if r.URL.Query().Get("qrcode_key") != "private-key" {
						t.Fatal("missing key")
					}
					poll++
					if poll < 3 {
						return guardEnvelope(map[string]any{"code": []int{86101, 86090}[poll-1]}), nil
					}
					if queryCookies {
						return guardEnvelope(map[string]any{"code": 0, "url": "https://passport.biligame.com/crossDomain?SESSDATA=private-cookie&DedeUserID=1234"}), nil
					}
					return guardEnvelope(map[string]any{"code": 0}), http.Header{"Set-Cookie": []string{"SESSDATA=private-cookie; Domain=.bilibili.com; Path=/; Secure; HttpOnly"}}
				case "/x/web-interface/nav":
					if c, err := r.Cookie("SESSDATA"); err != nil || c.Value != "private-cookie" {
						t.Fatal("cookie not retained")
					}
					return guardNav(), nil
				default:
					t.Fatalf("unexpected endpoint %s", r.URL.Path)
					return nil, nil
				}
			})
			qr, err := g.QR(context.Background())
			if err != nil || qr.Key == "" {
				t.Fatal(err)
			}
			for _, want := range []string{"waiting", "scanned", "connected"} {
				status, account, err := g.Poll(context.Background(), qr.Key)
				if err != nil || status != want {
					t.Fatalf("status %s, error %v", status, err)
				}
				if want == "connected" && account.UID != "1234" {
					t.Fatal("wrong account")
				}
			}
		})
	}
}

func TestGuardMonthPaginationAndMultipleTiers(t *testing.T) {
	calls := map[string]int{}
	g := guardTestClient(t, func(r *http.Request) (any, http.Header) {
		if r.URL.Path == "/x/web-interface/nav" {
			return guardNav(), nil
		}
		q := r.URL.Query()
		day, gift, cursor := q.Get("begin_time"), q.Get("gift_id"), q.Get("last_id")
		if day != "2026-09-01" && day != "2026-09-02" {
			t.Fatalf("queried outside month range: %s", day)
		}
		calls[day+"/"+gift]++
		if day == "2026-09-01" && gift == "10003" {
			if cursor == "" {
				return guardPage([]map[string]any{{"id": "opaque-cursor:1", "uid": "123456789012345678", "uname": "有,逗号", "gift_id": 10003}}, 1), nil
			}
			if cursor != "opaque-cursor:1" {
				t.Fatal("wrong next cursor")
			}
			return guardPage([]map[string]any{{"id": 2, "uid": "123456789012345678", "uname": "有,逗号", "gift_name": "舰长"}, {"id": 3, "uid": 42, "uname": "另一位"}}, 0), nil
		}
		if day == "2026-09-02" && gift == "10002" {
			return guardPage([]map[string]any{{"id": "4", "uid": "123456789012345678", "uname": "有,逗号", "gift_id": "10002"}}, 0), nil
		}
		return guardPage([]map[string]any{}, 0), nil
	})
	var progress GuardProgress
	result, err := g.Month(context.Background(), "2026-09", time.Date(2026, 9, 1, 17, 0, 0, 0, time.UTC), func(p GuardProgress) { progress = p })
	if err != nil {
		t.Fatal(err)
	}
	if result.Records != 3 || result.People != 2 || result.From != "2026-09-01" || result.Through != "2026-09-02" || len(calls) != 6 || calls["2026-09-01/10003"] != 2 || progress.Completed != 2 {
		t.Fatalf("wrong result %+v", result)
	}
	if !strings.Contains(result.Text, "123456789012345678,admiral") || !strings.Contains(result.Text, `"有,逗号"`) {
		t.Fatal("lost UID precision, tier or CSV quoting")
	}
}

func TestGuardMonthRejectsIncompleteAndUnexpectedData(t *testing.T) {
	for _, mode := range []string{"upstream", "cycle", "missing-more", "wrong-gift", "invalid-uid"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			g := guardTestClient(t, func(r *http.Request) (any, http.Header) {
				if r.URL.Path == "/x/web-interface/nav" {
					return guardNav(), nil
				}
				calls++
				if mode == "upstream" && calls > 1 {
					return map[string]any{"code": -101}, nil
				}
				if mode == "missing-more" {
					return guardEnvelope(map[string]any{"list": []any{}}), nil
				}
				row := map[string]any{"id": "1", "uid": "42"}
				if mode == "wrong-gift" {
					row["gift_id"] = 10001
				}
				if mode == "invalid-uid" {
					row["uid"] = "0"
				}
				return guardPage([]map[string]any{row}, 1), nil
			})
			result, err := g.Month(context.Background(), "2026-09", time.Date(2026, 9, 1, 0, 0, 0, 0, guardZone), nil)
			if err == nil || result.Text != "" || result.Records != 0 {
				t.Fatal("partial/invalid data accepted")
			}
			if mode == "upstream" && !errors.Is(err, ErrGuardLogin) {
				t.Fatal("login expiry lost")
			}
		})
	}
}

func TestGuardDatesAndNetworkBoundaries(t *testing.T) {
	days, err := GuardMonthDays("2024-02", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(days) != 29 || days[28] != "2024-02-29" {
		t.Fatal("leap-month bounds")
	}
	if _, err := GuardMonthDays("2026-10", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("future month allowed")
	}
	for _, raw := range []string{"http://passport.bilibili.com/", "https://api.bilibili.com.evil.test/", "https://api.bilibili.com:444/", "https://x@api.bilibili.com/", "https://127.0.0.1/"} {
		u, _ := url.Parse(raw)
		if guardURLAllowed(u) {
			t.Fatal("unsafe URL allowed")
		}
	}
	g := NewGuardImporter()
	g.http.Transport = guardTransport(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("request with secret-cookie and qrcode_key")
	})
	_, err = g.QR(context.Background())
	if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "qrcode_key") {
		t.Fatal("transport secret leaked")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = g.Month(ctx, "2026-09", time.Date(2026, 9, 1, 0, 0, 0, 0, guardZone), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored")
	}
}

func TestGuardCooldownResumesOnlyCompletedDays(t *testing.T) {
	stage := 0
	calls := map[string]int{}
	g := guardTestClient(t, func(r *http.Request) (any, http.Header) {
		if r.URL.Path == "/x/web-interface/nav" {
			return guardNav(), nil
		}
		day, gift := r.URL.Query().Get("begin_time"), r.URL.Query().Get("gift_id")
		if day == "2026-09-01" && gift == "10003" {
			return guardPage([]map[string]any{{"id": 1, "uid": 100, "uname": "第一天"}}, 0), nil
		}
		if day == "2026-09-02" && (gift == "10003" || gift == "10002") {
			uid := 201
			if stage == 0 {
				uid = 200
			}
			return guardPage([]map[string]any{{"id": 2, "uid": uid, "uname": "第二天"}}, 0), nil
		}
		return guardPage([]map[string]any{}, 0), nil
	})
	base := g.http.Transport
	g.http.Transport = guardTransport(func(r *http.Request) (*http.Response, error) {
		key := r.URL.Query().Get("begin_time") + "/" + r.URL.Query().Get("gift_id")
		calls[key]++
		if stage == 0 && key == "2026-09-02/10002" {
			return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"600"}}, Body: io.NopCloser(strings.NewReader("limited")), Request: r}, nil
		}
		return base.RoundTrip(r)
	})
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, guardZone)
	result, err := g.Month(context.Background(), "2026-09", now, nil)
	var restricted *guardRestriction
	if !errors.As(err, &restricted) || result.Text != "" || g.checkpoint == nil || g.checkpoint.nextDay != 1 || len(g.checkpoint.entries) != 1 {
		t.Fatalf("partial day incorrectly checkpointed: %v", err)
	}
	if time.Until(restricted.until) < 599*time.Second {
		t.Fatal("ignored Retry-After")
	}
	before := calls["/"]
	if _, err = g.Month(context.Background(), "2026-09", now, nil); !errors.As(err, &restricted) || calls["/"] != before {
		t.Fatal("retried account request while cooling down")
	}
	// A second login on the same server still respects the shared gate.
	other := guardTestClient(t, func(r *http.Request) (any, http.Header) {
		t.Fatal("cooldown bypassed by another login")
		return nil, nil
	})
	other.gate = g.gate
	if _, err = other.Month(context.Background(), "2026-09", now, nil); !errors.As(err, &restricted) {
		t.Fatal("cooldown not shared")
	}
	g.gate.mu.Lock()
	g.gate.restricted.until = time.Now().Add(-time.Second)
	g.gate.mu.Unlock()
	stage = 1
	result, err = g.Month(context.Background(), "2026-09", now, nil)
	if err != nil || result.Records != 3 || result.People != 2 || strings.Contains(result.Text, "200,") || calls["2026-09-01/10003"] != 1 || calls["2026-09-02/10003"] != 2 {
		t.Fatalf("incorrect resume: %+v, %v", result, err)
	}
	if g.checkpoint != nil {
		t.Fatal("completed result retained as resume cache")
	}
	if _, err = g.Month(context.Background(), "2026-09", now, nil); err != nil || calls["2026-09-01/10003"] != 2 {
		t.Fatal("fresh completed-month fetch did not refresh data")
	}
}

func TestGuardCheckpointStaysWithAccountAndMonth(t *testing.T) {
	for _, scope := range []string{"account", "month"} {
		t.Run(scope, func(t *testing.T) {
			g := guardTestClient(t, func(r *http.Request) (any, http.Header) {
				if r.URL.Path == "/x/web-interface/nav" {
					return guardNav(), nil
				}
				return guardPage([]map[string]any{}, 0), nil
			})
			g.checkpoint = &guardCheckpoint{month: "2026-09", account: "1234", nextDay: 1, entries: map[string]guardEntry{"old/captain": {uid: "999", tier: "captain", name: "旧数据"}}}
			if scope == "account" {
				g.checkpoint.account = "different-account"
			} else {
				g.checkpoint.month = "2026-08"
			}
			result, err := g.Month(context.Background(), "2026-09", time.Date(2026, 9, 1, 12, 0, 0, 0, guardZone), nil)
			if err != nil || result.Records != 0 || strings.Contains(result.Text, "999") {
				t.Fatal("checkpoint crossed account/month boundary")
			}
		})
	}
}

func TestGuardGatePacingAndRetryDates(t *testing.T) {
	g := &guardRequestGate{}
	if err := g.wait(context.Background(), 3*time.Second); err != nil {
		t.Fatal(err)
	}
	if time.Until(g.next) < 2900*time.Millisecond {
		t.Fatal("request spacing not applied")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if !errors.Is(g.wait(ctx, 3*time.Second), context.DeadlineExceeded) {
		t.Fatal("pacing wait ignored cancellation")
	}
	retryAt := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)
	var blocked *guardRestriction
	if !errors.As(g.block(412, retryAt.Format(http.TimeFormat)), &blocked) || blocked.until.Before(retryAt) {
		t.Fatal("HTTP-date Retry-After not honored")
	}
	if !errors.As(g.block(429, "1"), &blocked) || blocked.until.Before(retryAt) {
		t.Fatal("shorter response reset existing cooldown")
	}
	if NewGuardImporter().gate != NewGuardImporter().gate {
		t.Fatal("new logins don't share production gate")
	}
}
