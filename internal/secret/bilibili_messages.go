package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"github.com/spf13/viper"
)

func biliMessageAEAD() (cipher.AEAD, error) {
	key := viper.GetString("security.bilibili_cookie_key")
	if len(key) < 32 {
		return nil, ErrEncryptionKeyMissing
	}
	sum := sha256.Sum256([]byte("mofu-bilibili-messages-v1:" + key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func SealBilibiliValue(scope, value string) (string, error) {
	a, err := biliMessageAEAD()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(a.Seal(nonce, nonce, []byte(value), []byte(scope))), nil
}
func OpenBilibiliValue(scope, value string) (string, error) {
	a, err := biliMessageAEAD()
	if err != nil {
		return "", err
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) < a.NonceSize() {
		return "", errors.New("登录或私信数据无法读取")
	}
	plain, err := a.Open(nil, raw[:a.NonceSize()], raw[a.NonceSize():], []byte(scope))
	return string(plain), err
}
