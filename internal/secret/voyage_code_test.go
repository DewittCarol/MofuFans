package secret

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"github.com/spf13/viper"
	"strings"
	"testing"
)

func TestVoyageCodeFormats(t *testing.T) {
	viper.Set("security.bilibili_cookie_key", strings.Repeat("k", 32))
	defer viper.Reset()
	for i := 0; i < 20; i++ {
		enc, hash, err := NewVoyageCode("1:1000001")
		if err != nil {
			t.Fatal(err)
		}
		code, err := OpenVoyageCode(enc, "1:1000001")
		if err != nil {
			t.Fatal(err)
		}
		upper, lower, digit := false, false, false
		for _, c := range code {
			upper = upper || c >= 'A' && c <= 'Z'
			lower = lower || c >= 'a' && c <= 'z'
			digit = digit || c >= '0' && c <= '9'
		}
		if len(code) != 6 || !upper || !lower || !digit {
			t.Fatal("invalid generated format")
		}
		if !CheckVoyageCode("1:1000001", code, hash) || CheckVoyageCode("1:1000001", strings.ToUpper(code), hash) || CheckVoyageCode("2:1000001", code, hash) {
			t.Fatal("case or scope check failed")
		}
	}
	old := "ABCD1234EFGH5678"
	scope := "1:1000001"
	a, err := voyageAEAD()
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, a.NonceSize())
	enc := base64.RawStdEncoding.EncodeToString(a.Seal(nonce, nonce, []byte(old), []byte(scope)))
	code, err := OpenVoyageCode(enc, scope)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(scope + ":" + old))
	hash := hex.EncodeToString(digest[:])
	if code != "ABCD-1234-EFGH-5678" || !CheckVoyageCode(scope, strings.ToLower(code), hash) {
		t.Fatal("legacy code compatibility failed")
	}
}
