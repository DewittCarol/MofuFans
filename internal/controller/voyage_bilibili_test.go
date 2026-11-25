package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mofufans/internal/bilibili"
	"mofufans/internal/database"
)

type testGuardImporter struct {
	wait    bool
	started chan struct{}
}

func (*testGuardImporter) QR(context.Context) (bilibili.GuardQR, error) {
	return bilibili.GuardQR{URL: "https://passport.bilibili.com/scan?public-qr", Key: "private-poll-key"}, nil
}
func (*testGuardImporter) Poll(context.Context, string) (string, bilibili.GuardAccount, error) {
	return "connected", bilibili.GuardAccount{UID: "42", Name: "主播"}, nil
}
func (f *testGuardImporter) Month(ctx context.Context, month string, _ time.Time, progress func(bilibili.GuardProgress)) (bilibili.GuardResult, error) {
	progress(bilibili.GuardProgress{Day: month + "-01", Total: 30, Records: 2})
	if f.wait {
		close(f.started)
		<-ctx.Done()
		return bilibili.GuardResult{}, ctx.Err()
	}
	return bilibili.GuardResult{Text: "UID,档位,昵称\n12345,captain,测试\n12345,admiral,测试\n", Records: 2, People: 1, From: month + "-01", Through: month + "-30"}, nil
}

func setupBiliController(t *testing.T, f *testGuardImporter) (*gin.Engine, *VoyageBilibiliController, database.VoyageRoster) {
	r := batchTestSetup(t)
	ctl := NewVoyageBilibiliController()
	ctl.newClient = func() guardImportClient { return f }
	r.Use(func(c *gin.Context) {
		var admin database.Admin
		admin.ID = 1
		if c.GetHeader("X-Test-Admin") == "other" {
			admin.ID = 2
		}
		c.Set("user", admin)
		c.Next()
	})
	r.GET(voyageBiliPath+"/session", ctl.Status)
	r.POST(voyageBiliPath+"/session", ctl.Connect)
	r.POST(voyageBiliPath+"/session/poll", ctl.Poll)
	r.DELETE(voyageBiliPath+"/session", ctl.Disconnect)
	r.POST(voyageBiliPath+"/rosters/:roster/fetch", ctl.Fetch)
	r.DELETE(voyageBiliPath+"/job", ctl.Cancel)
	roster := database.VoyageRoster{Title: "测试月份", Month: "2026-01", Category: "monthly", Version: 1}
	if err := database.DB.Create(&roster).Error; err != nil {
		t.Fatal(err)
	}
	return r, ctl, roster
}
func biliCall(t *testing.T, r *gin.Engine, method, path, token, session string, headers map[string]string, want int) (*httptest.ResponseRecorder, voyageBiliView) {
	t.Helper()
	req := httptest.NewRequest(method, voyageBiliPath+path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: voyageBiliCookie, Value: token})
	}
	if session != "" {
		req.AddCookie(&http.Cookie{Name: "session", Value: session})
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var res struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    voyageBiliView `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Code != want {
		t.Fatalf("%s %s: code %d, want %d, %s", method, path, res.Code, want, res.Message)
	}
	if strings.Contains(w.Body.String(), "private-poll-key") {
		t.Fatal("QR polling key leaked")
	}
	return w, res.Data
}
func connectBiliTest(t *testing.T, r *gin.Engine) string {
	w, view := biliCall(t, r, "POST", "/session", "", "mofu-test", nil, 200)
	if view.Status != "waiting" || view.QRURL == "" {
		t.Fatal("missing QR")
	}
	var token string
	for _, c := range w.Result().Cookies() {
		if c.Name == voyageBiliCookie {
			token = c.Value
			if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != voyageBiliPath {
				t.Fatal("insecure connection cookie")
			}
		}
	}
	if token == "" {
		t.Fatal("no token")
	}
	_, view = biliCall(t, r, "POST", "/session/poll", token, "mofu-test", nil, 200)
	if view.Status != "connected" || view.QRURL != "" || view.Account.UID != "42" {
		t.Fatal("not connected")
	}
	return token
}

func TestBiliImportIsSessionBoundAndPreviewOnly(t *testing.T) {
	r, _, roster := setupBiliController(t, &testGuardImporter{})
	biliCall(t, r, "POST", "/session", "", "", nil, 401)
	biliCall(t, r, "POST", "/session", "", "mofu-test", map[string]string{"Origin": "https://other.example"}, 403)
	biliCall(t, r, "POST", "/session", "", "mofu-test", map[string]string{"Content-Type": "text/plain"}, 400)
	token := connectBiliTest(t, r)
	for _, identity := range []struct {
		session string
		headers map[string]string
	}{{"new-login", nil}, {"mofu-test", map[string]string{"X-Test-Admin": "other"}}} {
		_, view := biliCall(t, r, "GET", "/session", token, identity.session, identity.headers, 200)
		if view.Status != "disconnected" || view.Account != nil {
			t.Fatal("cross-session account exposure")
		}
		biliCall(t, r, "POST", fmt.Sprintf("/rosters/%d/fetch", roster.ID), token, identity.session, identity.headers, 401)
	}
	biliCall(t, r, "POST", fmt.Sprintf("/rosters/%d/fetch", roster.ID), token, "mofu-test", nil, 200)
	var view voyageBiliView
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		_, view = biliCall(t, r, "GET", "/session", token, "mofu-test", nil, 200)
		if view.Job.Status == "done" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if view.Job == nil || view.Job.Status != "done" || view.Job.Result.Records != 2 {
		t.Fatal("job did not complete")
	}
	var count int64
	database.DB.Model(&database.VoyageRosterMember{}).Count(&count)
	if count != 0 {
		t.Fatal("fetch wrote roster")
	}
	body := gin.H{"text": view.Job.Result.Text, "version": roster.Version}
	res := batchCall(t, r, "POST", fmt.Sprintf("/rosters/%d/members/preview", roster.ID), body, 200)
	var preview struct {
		Added int `json:"added"`
	}
	json.Unmarshal(res.Data, &preview)
	if preview.Added != 2 {
		t.Fatal("different tiers collapsed")
	}
	database.DB.Model(&database.VoyageRosterMember{}).Count(&count)
	if count != 0 {
		t.Fatal("preview wrote roster")
	}
	batchCall(t, r, "POST", fmt.Sprintf("/rosters/%d/members/import", roster.ID), body, 200)
	database.DB.Model(&database.VoyageRosterMember{}).Count(&count)
	if count != 2 {
		t.Fatal("explicit import failed")
	}
	biliCall(t, r, "DELETE", "/session", token, "mofu-test", nil, 200)
	_, view = biliCall(t, r, "GET", "/session", token, "mofu-test", nil, 200)
	if view.Status != "disconnected" || view.Job != nil {
		t.Fatal("disconnect retained personal data")
	}
}

func TestBiliImportCancelExpiryAndRosterScope(t *testing.T) {
	f := &testGuardImporter{wait: true, started: make(chan struct{})}
	r, ctl, roster := setupBiliController(t, f)
	token := connectBiliTest(t, r)
	event := database.VoyageRoster{Title: "活动", Category: "event", Version: 1}
	database.DB.Create(&event)
	biliCall(t, r, "POST", fmt.Sprintf("/rosters/%d/fetch", event.ID), token, "mofu-test", nil, 400)
	biliCall(t, r, "POST", fmt.Sprintf("/rosters/%d/fetch", roster.ID), token, "mofu-test", nil, 200)
	select {
	case <-f.started:
	case <-time.After(time.Second):
		t.Fatal("job not started")
	}
	biliCall(t, r, "POST", fmt.Sprintf("/rosters/%d/fetch", roster.ID), token, "mofu-test", nil, 409)
	biliCall(t, r, "POST", "/session", token, "mofu-test", nil, 409)
	_, view := biliCall(t, r, "DELETE", "/job", token, "mofu-test", nil, 200)
	if view.Job != nil || view.Status != "connected" {
		t.Fatal("cancel state incorrect")
	}
	ctl.mu.Lock()
	s := ctl.sessions[token]
	ctl.mu.Unlock()
	s.mu.Lock()
	s.expires = time.Now().Add(-time.Second)
	s.mu.Unlock()
	_, view = biliCall(t, r, "GET", "/session", token, "mofu-test", nil, 200)
	if view.Status != "disconnected" {
		t.Fatal("expired session accessible")
	}
}
