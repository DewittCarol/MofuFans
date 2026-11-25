package bilibili

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// PublicSignature performs one fresh, cookie-free lookup. It never shares the
// seven-day display-profile cache and never falls back to a stale value.
func (c *Client) PublicSignature(ctx context.Context, uid int64) (Profile, error) {
	if uid <= 0 {
		return Profile{}, errors.New("B 站 UID 无效")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/x/web-interface/card?mid="+strconv.FormatInt(uid, 10)), nil)
	if err != nil {
		return Profile{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://space.bilibili.com/"+strconv.FormatInt(uid, 10))
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	configured := c.HTTPClient
	if configured == nil {
		configured = &http.Client{Timeout: 8 * time.Second}
	}
	client := *configured
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return Profile{}, errors.New("暂时无法读取 B 站签名，请稍后重试")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Profile{}, errors.New("B 站暂未返回签名，请稍后重试")
	}
	var payload struct {
		Code *int `json:"code"`
		Data struct {
			Card struct {
				MID  json.RawMessage `json:"mid"`
				Name string          `json:"name"`
				Face string          `json:"face"`
				Sign *string         `json:"sign"`
			} `json:"card"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 256<<10)).Decode(&payload) != nil || payload.Code == nil || *payload.Code != 0 {
		return Profile{}, errors.New("B 站签名读取失败，请稍后重试")
	}
	card := payload.Data.Card
	mid, e := strconv.ParseInt(strings.Trim(string(card.MID), `"`), 10, 64)
	if e != nil || mid != uid || card.Sign == nil || len(*card.Sign) > 4096 || strings.TrimSpace(card.Name) == "" || utf8.RuneCountInString(card.Name) > 255 || len(card.Face) > 1024 {
		return Profile{}, errors.New("B 站资料暂不完整，请稍后重试")
	}
	return Profile{MID: mid, Name: card.Name, Face: card.Face, Signature: *card.Sign}, nil
}
