package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"github.com/spf13/viper"
	"strings"
)

func CheckVoyageCode(scope, code, expected string) bool {
	code = strings.TrimSpace(code)
	if len(code) == 6 {
		for _, ch := range code {
			if !(ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9') {
				return false
			}
		}
	} else {
		// Codes sent before the six-character format retain their original compatibility.
		code = strings.ToUpper(strings.NewReplacer("-", "", " ", "", "\t", "", "\r", "", "\n", "").Replace(code))
		if len(code) != 16 {
			return false
		}
	}
	digest := sha256.Sum256([]byte(scope + ":" + code))
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(digest[:])), []byte(expected)) == 1
}
func addressAEAD() (cipher.AEAD, error) {
	key := viper.GetString("security.bilibili_cookie_key")
	if len(key) < 32 {
		return nil, errors.New("地址加密配置不可用")
	}
	sum := sha256.Sum256([]byte("mofu-voyage-address-v1:" + key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func SealVoyageAddress(scope string, data []byte) (string, error) {
	a, err := addressAEAD()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(a.Seal(nonce, nonce, data, []byte(scope))), nil
}
func OpenVoyageAddress(scope, value string) ([]byte, error) {
	a, err := addressAEAD()
	if err != nil {
		return nil, err
	}
	raw, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil || len(raw) < a.NonceSize() {
		return nil, errors.New("地址数据无效")
	}
	return a.Open(nil, raw[:a.NonceSize()], raw[a.NonceSize():], []byte(scope))
}
