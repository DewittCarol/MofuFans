package adminauth

import (
	"mofufans/internal/database"
	"testing"
)

func TestPasswordAndCredentialRevocation(t *testing.T) {
	password := "0123456789abcdef0123456789abcdef"
	if ok, legacy := VerifyPassword(password, password); !ok || !legacy {
		t.Fatal("legacy upgrade not supported")
	}
	hashed, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if ok, legacy := VerifyPassword(hashed, password); !ok || legacy {
		t.Fatal("bcrypt login failed")
	}
	if ok, _ := VerifyPassword(hashed, "abcdef0123456789abcdef0123456789"); ok {
		t.Fatal("wrong password accepted")
	}
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("invalid transport accepted")
	}
	a := database.Admin{Password: hashed}
	before := Stamp(a)
	a.Password, _ = HashPassword(password)
	if before == Stamp(a) {
		t.Fatal("credential rotation must revoke old sessions")
	}
}
