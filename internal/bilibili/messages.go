package bilibili

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type MessageError struct {
	Note      string
	Uncertain bool
	Login     bool
	Recipient bool
	RetryAt   time.Time
}

func (e *MessageError) Error() string { return e.Note }

func messageHTTPRestriction(status int, retryAfter string) *MessageError {
	after := time.Now().Add(10 * time.Minute)
	if t, err := http.ParseTime(retryAfter); err == nil && t.After(after) {
		after = t
	}
	if sec, err := strconv.ParseInt(retryAfter, 10, 32); err == nil && sec > 600 {
		after = time.Now().Add(time.Duration(sec) * time.Second)
	}
	return &MessageError{Note: fmt.Sprintf("B 站限制了请求（HTTP %d），等待冷却后继续，请检查账号状态", status), RetryAt: after}
}

type savedGuardLogin struct {
	Cookies map[string]string `json:"cookies"`
	Device  string            `json:"device"`
}

var loginCookieNames = map[string]bool{"SESSDATA": true, "bili_jct": true, "DedeUserID": true, "DedeUserID__ckMd5": true, "buvid3": true, "buvid4": true, "b_nut": true}
var messageUID = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)

func (g *GuardImporter) ExportLogin() (string, error) {
	u, _ := url.Parse("https://api.bilibili.com/")
	s := savedGuardLogin{Cookies: map[string]string{}, Device: g.device}
	for _, c := range g.http.Jar.Cookies(u) {
		if loginCookieNames[c.Name] {
			s.Cookies[c.Name] = c.Value
		}
	}
	if s.Cookies["SESSDATA"] == "" || s.Cookies["bili_jct"] == "" || !messageUID.MatchString(s.Cookies["DedeUserID"]) {
		return "", ErrGuardLogin
	}
	raw, err := json.Marshal(s)
	return string(raw), err
}
func RestoreGuardImporter(value string) (*GuardImporter, error) {
	var s savedGuardLogin
	if json.Unmarshal([]byte(value), &s) != nil || s.Cookies["SESSDATA"] == "" || s.Cookies["bili_jct"] == "" || !messageUID.MatchString(s.Cookies["DedeUserID"]) {
		return nil, ErrGuardLogin
	}
	g := NewGuardImporter()
	if s.Device != "" {
		g.device = s.Device
	}
	u, _ := url.Parse("https://api.bilibili.com/")
	for name, value := range s.Cookies {
		if loginCookieNames[name] {
			g.http.Jar.SetCookies(u, []*http.Cookie{{Name: name, Value: value, Domain: ".bilibili.com", Path: "/", Secure: true, HttpOnly: true}})
		}
	}
	return g, nil
}
func newMessageDevice() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	s := hex.EncodeToString(b[:])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}

// WBI is the normal web client's request signature, not a retry or challenge bypass.
func messageSignature(values url.Values, image, sub string, now time.Time) (string, error) {
	key := strings.TrimSuffix(path.Base(image), path.Ext(image)) + strings.TrimSuffix(path.Base(sub), path.Ext(sub))
	order := []int{46, 47, 18, 2, 53, 8, 23, 32, 15, 50, 10, 31, 58, 3, 45, 35, 27, 43, 5, 49, 33, 9, 42, 19, 29, 28, 14, 39, 12, 38, 41, 13}
	if len(key) != 64 {
		return "", errors.New("B 站验证信息不完整，请稍后重试")
	}
	mixed := make([]byte, 32)
	for i, n := range order {
		mixed[i] = key[n]
	}
	values.Set("wts", strconv.FormatInt(now.Unix(), 10))
	for k, v := range values {
		values.Set(k, strings.Map(func(r rune) rune {
			if strings.ContainsRune("!'()*", r) {
				return -1
			}
			return r
		}, v[0]))
	}
	query := strings.ReplaceAll(values.Encode(), "+", "%20")
	hash := md5.Sum([]byte(query + string(mixed)))
	return query + "&w_rid=" + hex.EncodeToString(hash[:]), nil
}

func (g *GuardImporter) SendMessage(ctx context.Context, sender, uid, body string) (string, error) {
	if !messageUID.MatchString(uid) || uid == sender || strings.TrimSpace(body) == "" || utf8.RuneCountInString(body) > 500 {
		return "", errors.New("私信收件人或内容无效，不能给登录账号自己发送")
	}
	var nav apiResponse[struct {
		IsLogin bool        `json:"isLogin"`
		MID     json.Number `json:"mid"`
		WBI     struct {
			Image string `json:"img_url"`
			Sub   string `json:"sub_url"`
		} `json:"wbi_img"`
	}]
	if err := g.request(ctx, "https://api.bilibili.com/x/web-interface/nav", &nav); err != nil {
		return "", err
	}
	if nav.Code == -101 || nav.Code == -111 || (nav.Code == 0 && (!nav.Data.IsLogin || nav.Data.MID.String() != sender)) {
		return "", &MessageError{Note: "B 站登录已失效，请重新扫码", Login: true}
	}
	if err := guardAPIError(nav.Code); err != nil {
		return "", err
	}
	u, _ := url.Parse("https://api.bilibili.com/")
	csrf := ""
	for _, c := range g.http.Jar.Cookies(u) {
		if c.Name == "bili_jct" {
			csrf = c.Value
		}
	}
	if csrf == "" || g.device == "" {
		return "", &MessageError{Note: "登录信息不完整，请重新扫码", Login: true}
	}
	// Ask Bilibili for its own browser device cookies when QR login did not set them.
	hasDevice := false
	for _, c := range g.http.Jar.Cookies(u) {
		if c.Name == "buvid3" && c.Value != "" {
			hasDevice = true
		}
	}
	if !hasDevice {
		var fingerprint apiResponse[struct {
			B3 string `json:"b_3"`
			B4 string `json:"b_4"`
		}]
		if err := g.request(ctx, "https://api.bilibili.com/x/frontend/finger/spi", &fingerprint); err != nil {
			return "", err
		}
		if fingerprint.Code != 0 || fingerprint.Data.B3 == "" {
			return "", errors.New("B 站未能建立设备会话，请稍后重试")
		}
		cookies := []*http.Cookie{{Name: "buvid3", Value: fingerprint.Data.B3, Domain: ".bilibili.com", Path: "/", Secure: true}}
		if fingerprint.Data.B4 != "" {
			cookies = append(cookies, &http.Cookie{Name: "buvid4", Value: fingerprint.Data.B4, Domain: ".bilibili.com", Path: "/", Secure: true})
		}
		g.http.Jar.SetCookies(u, cookies)
	}
	query, err := messageSignature(url.Values{"w_sender_uid": {sender}, "w_receiver_id": {uid}, "w_dev_id": {g.device}}, nav.Data.WBI.Image, nav.Data.WBI.Sub, time.Now())
	if err != nil {
		return "", err
	}
	content, _ := json.Marshal(map[string]string{"content": body})
	form := url.Values{"msg[sender_uid]": {sender}, "msg[receiver_id]": {uid}, "msg[receiver_type]": {"1"}, "msg[msg_type]": {"1"}, "msg[msg_status]": {"0"}, "msg[content]": {string(content)}, "msg[timestamp]": {strconv.FormatInt(time.Now().Unix(), 10)}, "msg[new_face_version]": {"0"}, "msg[dev_id]": {g.device}, "msg[canal_token]": {""}, "from_firework": {"0"}, "build": {"0"}, "mobi_app": {"web"}, "csrf": {csrf}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.vc.bilibili.com/web_im/v1/web_im/send_msg?"+query, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Origin", "https://message.bilibili.com")
	req.Header.Set("Referer", "https://message.bilibili.com/")
	// Do not redirect a message POST or replay it after an ambiguous result.
	client := *g.http
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return "", &MessageError{Note: "未收到发送结果。请到 B 站确认这条私信是否已发出", Uncertain: true}
	}
	defer resp.Body.Close()
	if resp.StatusCode == 412 || resp.StatusCode == 429 {
		return "", messageHTTPRestriction(resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	if resp.StatusCode != 200 {
		return "", &MessageError{Note: fmt.Sprintf("B 站返回 HTTP %d，发送结果需要人工核对", resp.StatusCode), Uncertain: true}
	}
	var res apiResponse[struct {
		Key json.RawMessage `json:"msg_key"`
	}]
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&res) != nil {
		return "", &MessageError{Note: "B 站响应无法识别，请核对这条私信是否已发出", Uncertain: true}
	}
	if res.Code != 0 {
		if res.Code == 21047 {
			return "", &MessageError{Note: "B 站未接受这条私信（错误码 21047）。请检查账号或收件人的私信设置", Recipient: true}
		}
		return "", &MessageError{Note: fmt.Sprintf("B 站未接受这条私信（错误码 %d）。请检查账号或收件人的私信设置", res.Code), Login: res.Code == -101 || res.Code == -111}
	}
	key := strings.Trim(string(res.Data.Key), `"`)
	if !messageUID.MatchString(key) {
		return "", &MessageError{Note: "B 站未返回私信编号，请核对是否已经发出", Uncertain: true}
	}
	return key, nil
}
