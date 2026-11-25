package guard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type testClock struct{ value atomic.Int64 }

func (c *testClock) now() time.Time      { return time.Unix(0, c.value.Load()) }
func (c *testClock) add(d time.Duration) { c.value.Add(int64(d)) }
func rig(t *testing.T, policies map[string]Policy) (*Manager, *testClock) {
	t.Helper()
	clock := &testClock{}
	clock.value.Store(time.Now().UnixNano())
	g, err := New(Options{Directory: t.TempDir(), Secret: strings.Repeat("s", 32), Now: clock.now, Policies: policies})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.Close)
	return g, clock
}
func failAttempt(t *testing.T, g *Manager, ip, scope, target string) {
	t.Helper()
	a, after := g.Begin(ip, scope, target)
	if after != 0 {
		t.Fatalf("unexpected cooldown %d", after)
	}
	a.Failure()
}

func TestGuardCanonicalAddressesCannotSplitBudgets(t *testing.T) {
	g, _ := rig(t, map[string]Policy{"read": {1, 1, 600, 16}})
	for _, pair := range [][2]string{{"::ffff:192.0.2.1", "192.0.2.1"}, {"2001:db8:0:0:0:0:0:1", "2001:db8::1"}} {
		release, after := g.Enter(pair[0], "read")
		if release == nil {
			t.Fatal(after)
		}
		release()
		if release, after = g.Enter(pair[1], "read"); release != nil || after < 1 {
			t.Fatal("IP spelling bypassed budget")
		}
	}
	if _, err := CanonicalIP("fe80::1%eth0"); err == nil {
		t.Fatal("scoped IP accepted")
	}
}
func TestGuardSuccessDoesNotClearIPFailuresOrBlockUnrelatedScopes(t *testing.T) {
	g, _ := rig(t, nil)
	for i := 0; i < 7; i++ {
		failAttempt(t, g, "192.0.2.1", "admin-login", fmt.Sprint(i))
	}
	a, after := g.Begin("192.0.2.1", "admin-login", "valid-owner")
	if after != 0 {
		t.Fatal(after)
	}
	a.Success()
	failAttempt(t, g, "192.0.2.1", "admin-login", "last-target")
	if _, after = g.Begin("192.0.2.1", "admin-login", "new-target"); after < 1 {
		t.Fatal("valid credential cleared source attack history")
	}
	if release, after := g.Enter("192.0.2.1", "read"); release == nil {
		t.Fatal(after)
	} else {
		release()
	}
	if a, after := g.Begin("192.0.2.1", "member-login", "member"); after != 0 {
		t.Fatal("unrelated scope blocked")
	} else {
		a.Cancel()
	}
}
func TestGuardDistributedAttackGetsShortTargetCooldown(t *testing.T) {
	g, clock := rig(t, nil)
	for i := 0; i < 10; i++ {
		failAttempt(t, g, fmt.Sprintf("192.0.2.%d", i+1), "admin-login", "same-owner")
	}
	if _, after := g.Begin("198.51.100.1", "admin-login", "same-owner"); after < 1 || after > 31 {
		t.Fatal("missing or excessive target cooldown", after)
	}
	clock.add(31 * time.Second)
	a, after := g.Begin("198.51.100.1", "admin-login", "same-owner")
	if after != 0 {
		t.Fatal("account remained locked")
	}
	a.Cancel()
}
func TestGuardOnlyOneConcurrentVerificationPerTarget(t *testing.T) {
	g, _ := rig(t, nil)
	var wg sync.WaitGroup
	var count atomic.Int32
	var attemptsMu sync.Mutex
	var attempts []*Attempt
	start := make(chan struct{})
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			a, after := g.Begin(fmt.Sprintf("2001:db8::%x", i+1), "member-login", "same-target")
			if after == 0 {
				count.Add(1)
				attemptsMu.Lock()
				attempts = append(attempts, a)
				attemptsMu.Unlock()
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if count.Load() != 1 {
		t.Fatalf("parallel checks admitted=%d", count.Load())
	}
	attempts[0].Cancel()
	a, after := g.Begin("192.0.2.1", "member-login", "same-target")
	if after != 0 {
		t.Fatal("cancel leaked reservation")
	}
	a.Success()
	a.Failure()
	a.Cancel()
}
func TestGuardConcurrencyAndReservationsAreReleased(t *testing.T) {
	g, _ := rig(t, map[string]Policy{"upload": {600, 100, 6000, 2}})
	a, _ := g.Enter("192.0.2.1", "upload")
	b, _ := g.Enter("192.0.2.2", "upload")
	if a == nil || b == nil {
		t.Fatal("legitimate slots unavailable")
	}
	if release, _ := g.Enter("192.0.2.3", "upload"); release != nil {
		t.Fatal("global concurrency bypass")
	}
	a()
	a()
	c, after := g.Enter("192.0.2.3", "upload")
	if c == nil {
		t.Fatal(after)
	}
	b()
	c()
}
func TestGuardBansSurviveRestartAndOfflineRecoveryIsExclusive(t *testing.T) {
	root := t.TempDir()
	opts := Options{Directory: root, Secret: strings.Repeat("x", 32)}
	g, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.AddBan("192.0.2.1", 60)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Recover(root, "192.0.2.1", "", false); err == nil {
		t.Fatal("offline command overwrote live state")
	}
	g.Close()
	g, err = New(opts)
	if err != nil {
		t.Fatal(err)
	}
	if release, after := g.Enter("::ffff:192.0.2.1", "read"); release != nil || after < 1 {
		t.Fatal("restart removed ban")
	}
	g.Close()
	if n, err := Recover(root, "192.0.2.1", "", false); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	g, err = New(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	if release, after := g.Enter("192.0.2.1", "read"); release == nil {
		t.Fatal(after)
	} else {
		release()
	}
}
func TestGuardAutoBanDurableAndContainsNoRawAccount(t *testing.T) {
	g, _ := rig(t, nil)
	for i := 0; i < 8; i++ {
		failAttempt(t, g, "192.0.2.1", "admin-login", "Private-login-name-123")
	}
	file := filepath.Join(g.opts.Directory, "state.json")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Private-login-name-123") || !strings.Contains(string(raw), "auth_failures") {
		t.Fatal("unsafe or missing persisted record")
	}
	if info, _ := os.Stat(file); runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		t.Fatal("state file is not private")
	}
}
func TestGuardRepeatedAbuseTemporaryBanAndEventsStayBounded(t *testing.T) {
	g, clock := rig(t, map[string]Policy{"post": {1, 1, 6000, 4}})
	release, _ := g.Enter("192.0.2.1", "post")
	release()
	for i := 0; i < 31; i++ {
		g.Enter("192.0.2.1", "post")
	}
	if len(g.View().Bans) == 0 {
		t.Fatal("persistent abuse did not trigger ban")
	}
	clock.add(6 * time.Minute)
	if release, _ = g.Enter("192.0.2.1", "post"); release == nil {
		t.Fatal("temporary ban did not expire")
	} else {
		release()
	}
	for i := 0; i < maxEvents+25; i++ {
		g.Note(fmt.Sprintf("2001:db8::%x", i+1), "stream", "invalid_message")
	}
	if len(g.View().Events) != maxEvents {
		t.Fatal("event cap not enforced")
	}
	clock.add(retention + time.Minute)
	g.mu.Lock()
	g.sweepLocked()
	g.mu.Unlock()
	if len(g.View().Events) != 0 {
		t.Fatal("event retention not enforced")
	}
}
func TestGuardMemoryBudgetCannotEvictActiveRateLimits(t *testing.T) {
	g, _ := rig(t, nil)
	release, _ := g.Enter("192.0.2.1", "read")
	release()
	g.mu.Lock()
	for len(g.buckets) < maxKeys {
		g.buckets[fmt.Sprintf("filled-%d", len(g.buckets))] = &bucket{seen: g.now()}
	}
	g.mu.Unlock()
	if release, _ := g.Enter("198.51.100.2", "read"); release != nil {
		t.Fatal("admitted untracked identity at capacity")
	}
	if len(g.buckets) != maxKeys {
		t.Fatal("memory cap exceeded")
	}
}
func TestGuardCorruptStateFailsClosedWithoutOverwriting(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "state.json")
	_ = os.WriteFile(file, []byte("not valid json"), 0600)
	if g, err := New(Options{Directory: root, Secret: strings.Repeat("x", 32)}); err == nil {
		g.Close()
		t.Fatal("corrupt state silently reset")
	}
	raw, _ := os.ReadFile(file)
	if string(raw) != "not valid json" {
		t.Fatal("corrupt file overwritten")
	}
}
func TestGuardMiddlewareRejectsForgedProxyHeadersAndUnsafeOrigins(t *testing.T) {
	g, _ := rig(t, map[string]Policy{"read": {1, 1, 600, 16}})
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(g.Middleware(nil))
	r.GET("/api/data", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	r.POST("/api/write", func(c *gin.Context) { c.Status(200) })
	call := func(method, path, remote, origin, xff string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.RemoteAddr = remote
		req.Header.Set("Origin", origin)
		req.Header.Set("X-Forwarded-For", xff)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	if call("GET", "/api/data?secret=never-log", "192.0.2.1:1234", "", "198.51.100.1").Code != 200 {
		t.Fatal("initial request failed")
	}
	w := call("GET", "/api/data", "192.0.2.1:1234", "", "198.51.100.2")
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("forged XFF bypass or missing real HTTP status")
	}
	if call("POST", "/api/write", "192.0.2.2:1234", "https://other.example", "").Code != 403 {
		t.Fatal("unsafe browser origin admitted")
	}
	view, _ := json.Marshal(g.View())
	if strings.Contains(string(view), "never-log") {
		t.Fatal("query data stored")
	}
}
func TestGuardTrustedChainChoosesRightmostUntrustedHop(t *testing.T) {
	g, _ := rig(t, nil)
	r := gin.New()
	_ = r.SetTrustedProxies([]string{"127.0.0.1", "192.0.2.1"})
	r.Use(g.Middleware(nil))
	r.GET("/api/ip", func(c *gin.Context) { c.String(200, c.ClientIP()) })
	req := httptest.NewRequest("GET", "http://example.com/api/ip", nil)
	req.RemoteAddr = "127.0.0.1:8000"
	req.Header.Set("X-Forwarded-For", "198.51.100.99, 203.0.113.8, 192.0.2.1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Body.String() != "203.0.113.8" {
		t.Fatal("wrong proxy chain", w.Body.String())
	}
}
func TestGuardHTTPOriginAndSizeBoundaries(t *testing.T) {
	for _, test := range []struct {
		origin, fetch string
		expected      bool
	}{{"http://example.com", "same-origin", true}, {"https://sibling.example.com", "same-site", false}, {"null", "cross-site", false}, {"https://example.com/path", "", false}, {"", "same-site", false}, {"", "", true}} {
		req := httptest.NewRequest("POST", "http://example.com/api/write", nil)
		req.Header.Set("Origin", test.origin)
		req.Header.Set("Sec-Fetch-Site", test.fetch)
		if OriginAllowed(req, nil) != test.expected {
			t.Fatal(test)
		}
	}
	g, _ := rig(t, nil)
	r := gin.New()
	r.Use(g.Middleware(nil))
	r.POST("/api/login", func(c *gin.Context) { t.Fatal("oversized body reached credentials") })
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(strings.Repeat("x", (16<<10)+1)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatal(w.Code)
	}
}
func TestGuardStreamReservationsBoundedAndReusable(t *testing.T) {
	g, _ := rig(t, nil)
	var held []func()
	for i := 0; i < 12; i++ {
		release, ok := g.Stream("192.0.2.1")
		if !ok {
			t.Fatal(i)
		}
		held = append(held, release)
	}
	if _, ok := g.Stream("::ffff:192.0.2.1"); ok {
		t.Fatal("stream cap bypass")
	}
	held[0]()
	held[0]()
	release, ok := g.Stream("192.0.2.1")
	if !ok {
		t.Fatal("closed stream leaked slot")
	}
	release()
	for _, done := range held {
		done()
	}
	if g.View().Connections != 0 {
		t.Fatal("stream count leaked")
	}
}
func TestGuardSlowBodyAndDownloadsDoNotBorrowReadSlots(t *testing.T) {
	body := &progressBody{body: http.NoBody, started: time.Now().Add(-time.Minute), read: 1}
	if _, err := body.Read(make([]byte, 1)); err == nil {
		t.Fatal("slow body retained scarce slot")
	}
	if Classify("GET", "/api/gifts/:id/files/:file") != "download" || Classify("GET", "/api/question") != "read" {
		t.Fatal("downloads share data query slots")
	}
}
func TestGuardClosedManagerCannotOverwriteRecoveredState(t *testing.T) {
	g, _ := rig(t, nil)
	g.Close()
	if _, err := g.AddBan("192.0.2.1", 15); err == nil {
		t.Fatal("closed manager accepted a write")
	}
	if release, _ := g.Enter("192.0.2.1", "read"); release != nil {
		t.Fatal("closed manager admitted a request")
	}
}
func TestGuardEventGroupsCannotAccumulateAnUnboundedHistory(t *testing.T) {
	g, clock := rig(t, nil)
	g.Note("192.0.2.1", "read", "request_rejected")
	clock.add(30 * time.Second)
	g.Note("192.0.2.1", "read", "request_rejected")
	if len(g.View().Events) != 1 || g.View().Events[0].Count != 2 {
		t.Fatal("short repeated events did not coalesce")
	}
	clock.add(time.Minute)
	g.Note("192.0.2.1", "read", "request_rejected")
	if len(g.View().Events) != 2 {
		t.Fatal("continuous event retained an unbounded first timestamp")
	}
}
func TestGuardRotatingIPv6CannotManufactureUnboundedBudgets(t *testing.T) {
	g, _ := rig(t, map[string]Policy{"read": {1, 1, 600, 16}})
	release, _ := g.Enter("2001:db8:1::1", "read")
	release()
	for i := 2; i < 1002; i++ {
		if release, _ := g.Enter(fmt.Sprintf("2001:db8:1::%x", i), "read"); release != nil {
			t.Fatal("IPv6 rotation bypassed prefix budget")
		}
	}
	if len(g.buckets) > 4 {
		t.Fatal("rejected rotations allocated unbounded rate identities")
	}
	if release, _ := g.Enter("2001:db8:2::1", "read"); release == nil {
		t.Fatal("throttled prefix exhausted another prefix's global budget")
	} else {
		release()
	}
	if len(g.View().Bans) != 0 {
		t.Fatal("prefix rate limit became a subnet ban")
	}
}
