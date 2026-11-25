package adminauth

import (
	"crypto/md5"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"mofufans/internal/database"
	"strings"
	"unicode/utf8"
)

var dummyHash = func() []byte {
	b, err := bcrypt.GenerateFromPassword([]byte("00000000000000000000000000000000"), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return b
}()

func DummyCompare() {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte("ffffffffffffffffffffffffffffffff"))
}
func HashNewPassword(password string) (string, error) {
	if utf8.RuneCountInString(password) < 15 || len(password) > 256 || strings.TrimSpace(password) == "" {
		return "", errors.New("密码至少15个字符，合计不超过256字节")
	}
	weak := map[string]bool{"123456789012345": true, "1234567890123456": true, "password123456789": true, "qwertyuiop123456": true}
	allSame := true
	var first rune
	for i, r := range password {
		if i == 0 {
			first = r
		} else if r != first {
			allSame = false
		}
	}
	if weak[strings.ToLower(password)] || allSame {
		return "", errors.New("请使用更难猜测的密码或长口令")
	}
	sum := md5.Sum([]byte(password))
	return HashPassword(hex.EncodeToString(sum[:]))
}

// The existing browser submits the MD5 transport value over TLS. Store only bcrypt.
func HashPassword(password string) (string, error) {
	if len(password) != 32 {
		return "", errors.New("invalid password transport")
	}
	if _, err := hex.DecodeString(password); err != nil {
		return "", err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hashed), err
}
func VerifyPassword(stored, supplied string) (bool, bool) {
	if len(supplied) != 32 {
		return false, false
	}
	if _, err := hex.DecodeString(supplied); err != nil {
		return false, false
	}
	if len(stored) == 32 {
		DummyCompare()
		return subtle.ConstantTimeCompare([]byte(stored), []byte(supplied)) == 1, true
	}
	if len(stored) == 0 {
		DummyCompare()
		return false, false
	}
	return bcrypt.CompareHashAndPassword([]byte(stored), []byte(supplied)) == nil, false
}

// Bind the signed cookie to the current stored credential. A password change revokes it.
func Stamp(admin database.Admin) string {
	sum := sha256.Sum256([]byte(admin.Password))
	return hex.EncodeToString(sum[:])
}
func Current(c *gin.Context) (database.Admin, bool) {
	session := sessions.Default(c)
	var admin database.Admin
	stamp, ok := session.Get("auth_stamp").(string)
	if !ok || session.Get("authed") != true || session.Get("user") == nil {
		return admin, false
	}
	if database.DB.Where("id = ?", session.Get("user")).First(&admin).Error != nil {
		return admin, false
	}
	if subtle.ConstantTimeCompare([]byte(stamp), []byte(Stamp(admin))) != 1 {
		return database.Admin{}, false
	}
	return admin, true
}
