package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"

	"github.com/spf13/viper"
)

func voyageAEAD() (cipher.AEAD, error) {
	key := viper.GetString("security.bilibili_cookie_key")
	if len(key) < 32 {
		return nil, errors.New("请先配置服务器的数据加密密钥")
	}
	digest := sha256.Sum256([]byte("mofu-voyage-code-v1:" + key))
	block, err := aes.NewCipher(digest[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Six case-sensitive alphanumeric characters, with at least one character from each class.
// Whole-candidate rejection keeps the accepted codes uniformly distributed.
func randomVoyageCode() (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	for {
		code := make([]byte, 6)
		upper, lower, digit := false, false, false
		for i := range code {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
			if err != nil {
				return "", err
			}
			code[i] = alphabet[n.Int64()]
			upper = upper || (code[i] >= 'A' && code[i] <= 'Z')
			lower = lower || (code[i] >= 'a' && code[i] <= 'z')
			digit = digit || (code[i] >= '0' && code[i] <= '9')
		}
		if upper && lower && digit {
			return string(code), nil
		}
	}
}

// The issue and UID remain authenticated as scope for both old and new codes.
func NewVoyageCode(scope string) (encrypted, hash string, err error) {
	aead, err := voyageAEAD()
	if err != nil {
		return "", "", err
	}
	code, err := randomVoyageCode()
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256([]byte(scope + ":" + code))
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", "", err
	}
	sealed := aead.Seal(nonce, nonce, []byte(code), []byte(scope))
	return base64.RawStdEncoding.EncodeToString(sealed), hex.EncodeToString(digest[:]), nil
}

func OpenVoyageCode(encrypted, scope string) (string, error) {
	aead, err := voyageAEAD()
	if err != nil {
		return "", err
	}
	raw, err := base64.RawStdEncoding.DecodeString(encrypted)
	if err != nil || len(raw) < aead.NonceSize() {
		return "", errors.New("认证码数据无效")
	}
	plain, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], []byte(scope))
	if err != nil || (len(plain) != 16 && len(plain) != 6) {
		return "", errors.New("认证码无法读取，请检查加密密钥")
	}
	code := string(plain)
	if len(code) == 6 {
		return code, nil
	}
	return strings.Join([]string{code[:4], code[4:8], code[8:12], code[12:]}, "-"), nil
}
