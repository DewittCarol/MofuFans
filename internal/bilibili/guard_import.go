package bilibili

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/publicsuffix"
)

var ErrGuardLogin = errors.New("B 站登录已失效，请重新扫码")

// GuardImporter owns one short-lived login. Credentials never leave its cookie jar.
type GuardImporter struct {
	device     string
	http       *http.Client
	pause      time.Duration
	gate       *guardRequestGate
	monthMu    sync.Mutex
	checkpoint *guardCheckpoint
}

type guardEntry struct{ uid, tier, name string }
type guardCheckpoint struct {
	month    string
	account  string
	nextDay  int
	requests int
	entries  map[string]guardEntry
}

type guardRestriction struct {
	status int
	until  time.Time
}

func (e *guardRestriction) Error() string {
	return fmt.Sprintf("B 站暂时限制了请求（HTTP %d）。请至少等到 %s 后再重试；若仍被限制，请在 B 站主播后台检查账号状态", e.status, e.until.In(guardZone).Format("01-02 15:04:05"))
}

// Stream requests share one server-wide gate because they use the same outbound IP.
// Reconnecting or opening another tab must not reset an upstream cooldown.
type guardRequestGate struct {
	mu         sync.Mutex
	next       time.Time
	restricted *guardRestriction
}

var sharedGuardGate = &guardRequestGate{}

func (g *guardRequestGate) restriction() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.restricted != nil && time.Now().Before(g.restricted.until) {
		copy := *g.restricted
		return &copy
	}
	return nil
}
func (g *guardRequestGate) wait(ctx context.Context, interval time.Duration) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		g.mu.Lock()
		now := time.Now()
		if g.restricted != nil && now.Before(g.restricted.until) {
			copy := *g.restricted
			g.mu.Unlock()
			return &copy
		}
		delay := g.next.Sub(now)
		if delay <= 0 {
			g.next = now.Add(interval)
			g.mu.Unlock()
			return nil
		}
		g.mu.Unlock()
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
func (g *guardRequestGate) block(status int, retryAfter string) error {
	now := time.Now()
	delay := time.Minute
	if status == http.StatusPreconditionFailed {
		delay = 5 * time.Minute
	}
	until := now.Add(delay)
	// Honor Retry-After when it asks us to wait longer than the conservative default.
	if seconds, err := strconv.ParseInt(strings.TrimSpace(retryAfter), 10, 64); err == nil && seconds > 0 {
		if seconds > int64((1<<63-1)/time.Second) {
			until = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
		} else if later := now.Add(time.Duration(seconds) * time.Second); later.After(until) {
			until = later
		}
	} else if later, err := http.ParseTime(retryAfter); err == nil && later.After(until) {
		until = later
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.restricted == nil || until.After(g.restricted.until) {
		g.restricted = &guardRestriction{status: status, until: until}
	}
	copy := *g.restricted
	return &copy
}

type GuardAccount struct {
	UID  string `json:"uid"`
	Name string `json:"name"`
}
type GuardQR struct{ URL, Key string }
type GuardProgress struct {
	Day       string `json:"day"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
	Records   int    `json:"records"`
}
type GuardResult struct {
	Text    string `json:"text"`
	Records int    `json:"records"`
	People  int    `json:"people"`
	From    string `json:"from"`
	Through string `json:"through"`
}

func guardSecureURL(u *url.URL) bool {
	return u.Scheme == "https" && u.User == nil && (u.Port() == "" || u.Port() == "443")
}

// The QR payload is displayed for the app to scan, not requested by our server.
// Bilibili now issues account.bilibili.com URLs; older QR payloads used passport.
func guardQRURLAllowed(u *url.URL) bool {
	return guardSecureURL(u) && (u.Hostname() == "account.bilibili.com" || u.Hostname() == "passport.bilibili.com")
}

func guardURLAllowed(u *url.URL) bool {
	if !guardSecureURL(u) {
		return false
	}
	switch u.Hostname() {
	case "passport.bilibili.com", "api.bilibili.com", "api.live.bilibili.com", "www.bilibili.com", "passport.biligame.com":
		return true
	}
	return false
}

func NewGuardImporter() *GuardImporter {
	jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	return &GuardImporter{device: newMessageDevice(), http: &http.Client{Jar: jar, Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !guardURLAllowed(req.URL) {
			return errors.New("登录跳转无效")
		}
		return nil
	}}, pause: 3 * time.Second, gate: sharedGuardGate}
}

func (g *GuardImporter) request(ctx context.Context, endpoint string, out any) error {
	u, err := url.Parse(endpoint)
	if err != nil || !guardURLAllowed(u) {
		return errors.New("B 站接口地址无效")
	}
	stream := u.Hostname() == "api.live.bilibili.com"
	if stream {
		if err := g.gate.wait(ctx, g.pause); err != nil {
			return err
		}
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://link.bilibili.com/p/center/index")
	resp, err := g.http.Do(req)
	// Transport errors can contain the URL (and login secrets). Never return them.
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("暂时无法连接 B 站，请稍后重试")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 412 || resp.StatusCode == 429 {
		if stream {
			return g.gate.block(resp.StatusCode, resp.Header.Get("Retry-After"))
		}
		return messageHTTPRestriction(resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("B 站接口暂不可用（HTTP %d）", resp.StatusCode)
	}
	if out == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return err
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out) != nil {
		return errors.New("B 站返回了无法识别的数据，请稍后重试")
	}
	return nil
}

func guardAPIError(code int) error {
	if code == 0 {
		return nil
	}
	if code == -101 || code == -111 || code == 401 {
		return ErrGuardLogin
	}
	return fmt.Errorf("B 站拒绝了请求（错误码 %d），请确认扫码的是主播本人账号，或稍后重试", code)
}

func (g *GuardImporter) QR(ctx context.Context) (GuardQR, error) {
	var res apiResponse[struct {
		URL string `json:"url"`
		Key string `json:"qrcode_key"`
	}]
	err := g.request(ctx, "https://passport.bilibili.com/x/passport-login/web/qrcode/generate", &res)
	if err == nil {
		err = guardAPIError(res.Code)
	}
	if err != nil {
		return GuardQR{}, err
	}
	u, err := url.Parse(res.Data.URL)
	if err != nil || !guardQRURLAllowed(u) || res.Data.Key == "" {
		return GuardQR{}, errors.New("未能取得登录二维码，请重试")
	}
	return GuardQR{URL: res.Data.URL, Key: res.Data.Key}, nil
}

func (g *GuardImporter) hasSession() bool {
	u, _ := url.Parse("https://api.bilibili.com/")
	for _, c := range g.http.Jar.Cookies(u) {
		if c.Name == "SESSDATA" && c.Value != "" {
			return true
		}
	}
	return false
}

func (g *GuardImporter) Poll(ctx context.Context, key string) (string, GuardAccount, error) {
	var res apiResponse[struct {
		Code int    `json:"code"`
		URL  string `json:"url"`
	}]
	err := g.request(ctx, "https://passport.bilibili.com/x/passport-login/web/qrcode/poll?qrcode_key="+url.QueryEscape(key), &res)
	if err == nil {
		err = guardAPIError(res.Code)
	}
	if err != nil {
		return "", GuardAccount{}, err
	}
	switch res.Data.Code {
	case 86101:
		return "waiting", GuardAccount{}, nil
	case 86090:
		return "scanned", GuardAccount{}, nil
	case 86038:
		return "expired", GuardAccount{}, nil
	case 0:
		if !g.hasSession() {
			u, e := url.Parse(res.Data.URL)
			if e != nil || !guardURLAllowed(u) {
				return "", GuardAccount{}, errors.New("登录凭据无效，请重新扫码")
			}
			// Passport may return credentials in a cross-domain URL instead of Set-Cookie.
			cookies := []*http.Cookie{}
			for _, name := range []string{"SESSDATA", "DedeUserID", "DedeUserID__ckMd5", "bili_jct"} {
				if value := u.Query().Get(name); value != "" {
					cookies = append(cookies, &http.Cookie{Name: name, Value: value, Domain: ".bilibili.com", Path: "/", Secure: true, HttpOnly: true})
				}
			}
			origin, _ := url.Parse("https://passport.bilibili.com/")
			g.http.Jar.SetCookies(origin, cookies)
			if !g.hasSession() {
				if err := g.request(ctx, u.String(), nil); err != nil {
					return "", GuardAccount{}, err
				}
			}
		}
		account, err := g.Account(ctx)
		return "connected", account, err
	default:
		return "", GuardAccount{}, errors.New("B 站登录未完成，请重新扫码")
	}
}

func (g *GuardImporter) Account(ctx context.Context) (GuardAccount, error) {
	var res apiResponse[struct {
		IsLogin bool        `json:"isLogin"`
		MID     json.Number `json:"mid"`
		Name    string      `json:"uname"`
	}]
	err := g.request(ctx, "https://api.bilibili.com/x/web-interface/nav", &res)
	if err == nil {
		err = guardAPIError(res.Code)
	}
	if err != nil {
		return GuardAccount{}, err
	}
	if !res.Data.IsLogin || res.Data.MID == "" || res.Data.MID == "0" {
		return GuardAccount{}, ErrGuardLogin
	}
	return GuardAccount{UID: string(res.Data.MID), Name: res.Data.Name}, nil
}

var guardZone = time.FixedZone("Asia/Shanghai", 8*60*60)

func GuardMonthDays(month string, now time.Time) ([]string, error) {
	start, err := time.ParseInLocation("2006-01", month, guardZone)
	if err != nil {
		return nil, errors.New("名单月份无效")
	}
	now = now.In(guardZone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, guardZone)
	if start.After(today) {
		return nil, errors.New("这个月份还没开始，暂时无法获取名单")
	}
	end := start.AddDate(0, 1, 0)
	if end.After(today) {
		end = today.AddDate(0, 0, 1)
	}
	days := []string{}
	for day := start; day.Before(end); day = day.AddDate(0, 0, 1) {
		days = append(days, day.Format("2006-01-02"))
	}
	return days, nil
}

// Pagination cursors are opaque: some streams return strings, others numbers.
type guardCursor string

func (c *guardCursor) UnmarshalJSON(raw []byte) error {
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		*c = guardCursor(value)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return err
	}
	*c = guardCursor(number)
	return nil
}

type guardStreamRow struct {
	ID       guardCursor `json:"id"`
	UID      json.Number `json:"uid"`
	Name     string      `json:"uname"`
	GiftID   json.Number `json:"gift_id"`
	GiftName string      `json:"gift_name"`
}

// Month reads complete daily purchase streams, not the current active-guard snapshot.
// Completed days remain private to this login for resuming an interrupted month.
// Partial results are never returned for importing.
func (g *GuardImporter) Month(ctx context.Context, month string, now time.Time, progress func(GuardProgress)) (GuardResult, error) {
	g.monthMu.Lock()
	defer g.monthMu.Unlock()
	if ctx.Err() != nil {
		return GuardResult{}, ctx.Err()
	}
	days, err := GuardMonthDays(month, now)
	if err != nil {
		return GuardResult{}, err
	}
	checkpoint := g.checkpoint
	if checkpoint != nil && checkpoint.month != month {
		checkpoint = nil
	}
	if checkpoint != nil && progress != nil && checkpoint.nextDay < len(days) {
		progress(GuardProgress{Day: days[checkpoint.nextDay], Completed: checkpoint.nextDay, Total: len(days), Records: len(checkpoint.entries)})
	}
	if err := g.gate.restriction(); err != nil {
		return GuardResult{}, err
	}
	account, err := g.Account(ctx)
	if err != nil {
		if errors.Is(err, ErrGuardLogin) {
			g.checkpoint = nil
		}
		return GuardResult{}, err
	}
	if checkpoint == nil || checkpoint.account != account.UID {
		checkpoint = &guardCheckpoint{month: month, account: account.UID, entries: map[string]guardEntry{}}
		g.checkpoint = checkpoint
	}
	entries := checkpoint.entries
	requests := checkpoint.requests
	for dayIndex, day := range days {
		if dayIndex < checkpoint.nextDay {
			continue
		}
		dayEntries := map[string]guardEntry{}
		newEntries := 0
		for _, gift := range []struct{ id, tier, name string }{{"10003", "captain", "舰长"}, {"10002", "admiral", "提督"}, {"10001", "governor", "总督"}} {
			cursor := ""
			seen := map[string]bool{}
			for {
				requests++
				if requests > 6000 {
					return GuardResult{}, errors.New("本月流水过多，请从主播后台导出文件后导入")
				}
				q := url.Values{"limit": {"100"}, "gift_id": {gift.id}, "begin_time": {day}}
				if cursor != "" {
					q.Set("last_id", cursor)
				}
				var res apiResponse[struct {
					List json.RawMessage `json:"list"`
					More *int            `json:"has_more"`
				}]
				err := g.request(ctx, "https://api.live.bilibili.com/xlive/revenue/v1/giftStream/getReceivedGiftStreamNextList?"+q.Encode(), &res)
				if err == nil {
					err = guardAPIError(res.Code)
				}
				if err != nil {
					if errors.Is(err, ErrGuardLogin) {
						g.checkpoint = nil
						return GuardResult{}, fmt.Errorf("获取 %s 时失败：%w", day, err)
					}
					return GuardResult{}, fmt.Errorf("获取 %s 时暂停，已保留前 %d 天进度，下次从这天继续：%w", day, dayIndex, err)
				}
				var rows []guardStreamRow
				if len(res.Data.List) == 0 || res.Data.More == nil || (*res.Data.More != 0 && *res.Data.More != 1) || json.Unmarshal(res.Data.List, &rows) != nil {
					return GuardResult{}, errors.New("B 站流水格式发生变化，未导入任何记录")
				}
				for _, row := range rows {
					uid := string(row.UID)
					if n, err := strconv.ParseUint(uid, 10, 64); err != nil || n == 0 {
						return GuardResult{}, errors.New("B 站流水中有无效 UID，未导入任何记录")
					}
					if (row.GiftID != "" && string(row.GiftID) != gift.id) || (row.GiftName != "" && row.GiftName != gift.name) {
						return GuardResult{}, errors.New("B 站流水的档位与查询不符，未导入任何记录")
					}
					key := uid + "/" + gift.tier
					if _, exists := entries[key]; !exists {
						if _, existsToday := dayEntries[key]; !existsToday {
							newEntries++
						}
					}
					dayEntries[key] = guardEntry{uid, gift.tier, row.Name}
					if len(entries)+newEntries > 2000 {
						return GuardResult{}, errors.New("本月超过 2000 条名单记录，请导出文件后分批导入")
					}
				}
				if progress != nil {
					progress(GuardProgress{Day: day, Completed: dayIndex, Total: len(days), Records: len(entries) + newEntries})
				}
				if *res.Data.More == 0 {
					break
				}
				if len(rows) == 0 {
					return GuardResult{}, errors.New("B 站流水分页不完整，请稍后重试")
				}
				cursor = string(rows[len(rows)-1].ID)
				if cursor == "" || seen[cursor] {
					return GuardResult{}, errors.New("B 站流水分页重复，请稍后重试")
				}
				seen[cursor] = true
			}
		}
		for key, row := range dayEntries {
			entries[key] = row
		}
		checkpoint.nextDay = dayIndex + 1
		checkpoint.requests = requests
		if progress != nil {
			progress(GuardProgress{Day: day, Completed: dayIndex + 1, Total: len(days), Records: len(entries)})
		}
	}
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var buf strings.Builder
	w := csv.NewWriter(&buf)
	w.Write([]string{"UID", "档位", "昵称"})
	people := map[string]bool{}
	for _, key := range keys {
		row := entries[key]
		people[row.uid] = true
		w.Write([]string{row.uid, row.tier, row.name})
	}
	w.Flush()
	g.checkpoint = nil
	return GuardResult{Text: buf.String(), Records: len(entries), People: len(people), From: days[0], Through: days[len(days)-1]}, w.Error()
}
