package appearance

import (
	_ "embed"
	"encoding/json"
	"strings"
)

// Unicode Emoji 16.0, fully-qualified sequences from:
// https://www.unicode.org/Public/emoji/16.0/emoji-test.txt
// Distributed under the accompanying UNICODE-LICENSE.txt.
//
//go:embed unicode-emoji.json
var unicodeEmojiJSON []byte

var unicodeEmojis = func() map[string]string {
	var values []string
	if err := json.Unmarshal(unicodeEmojiJSON, &values); err != nil {
		panic(err)
	}
	result := make(map[string]string, len(values))
	for _, value := range values {
		result[strings.ReplaceAll(value, "\ufe0f", "")] = value
	}
	return result
}()

// NormalizeEmoji accepts a single emoji, including joined, skin-tone and flag
// sequences, and normalizes optional presentation selectors for deduplication.
func NormalizeEmoji(value string) (string, bool) {
	canonical, ok := unicodeEmojis[strings.ReplaceAll(strings.TrimSpace(value), "\ufe0f", "")]
	return canonical, ok
}
