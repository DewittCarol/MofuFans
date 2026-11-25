package bilibili

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// PublicCard reads public profile information without a login cookie.
func (c *Client) PublicCard(ctx context.Context, uid int64) (Profile, error) {
	var response apiResponse[struct {
		Card struct {
			MID  json.RawMessage `json:"mid"`
			Name string          `json:"name"`
			Face string          `json:"face"`
		} `json:"card"`
	}]
	err := c.do(ctx, c.endpoint("/x/web-interface/card?mid="+strconv.FormatInt(uid, 10)), "", &response)
	if err != nil {
		return Profile{}, err
	}
	card := response.Data.Card
	mid, parseErr := strconv.ParseInt(strings.Trim(string(card.MID), `"`), 10, 64)
	if response.Code != 0 || parseErr != nil || mid != uid || strings.TrimSpace(card.Name) == "" {
		return Profile{}, errors.New("公开用户资料暂不可用，请稍后重试")
	}
	return Profile{MID: mid, Name: card.Name, Face: card.Face}, nil
}
