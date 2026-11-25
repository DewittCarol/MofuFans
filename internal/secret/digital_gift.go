package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"github.com/spf13/viper"
)

func giftAEAD() (cipher.AEAD, error) {
	key := viper.GetString("security.bilibili_cookie_key")
	if len(key) < 32 {
		return nil, errors.New("请先配置服务器的数据加密密钥")
	}
	sum := sha256.Sum256([]byte("mofu-digital-gift-v1:" + key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func SealGiftValue(scope, value string) (string, error) {
	a, err := giftAEAD()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(a.Seal(nonce, nonce, []byte(value), []byte(scope))), nil
}

func OpenGiftValue(scope, value string) (string, error) {
	a, err := giftAEAD()
	if err != nil {
		return "", err
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) < a.NonceSize() {
		return "", errors.New("礼物凭据无效")
	}
	plain, err := a.Open(nil, raw[:a.NonceSize()], raw[a.NonceSize():], []byte(scope))
	return string(plain), err
}

func NewGiftPassword() (string, error) {
	// 96 random bits; ASCII, case-sensitive, safe to copy into a private message.
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func CheckGiftPassword(scope, encrypted, password string) bool {
	plain, err := OpenGiftValue(scope, encrypted)
	if err != nil {
		return false
	}
	a, b := sha256.Sum256([]byte(plain)), sha256.Sum256([]byte(password))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}
