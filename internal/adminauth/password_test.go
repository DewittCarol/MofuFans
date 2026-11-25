package adminauth

import "testing"

func TestNewAdminPasswordPolicyAndLegacyLoginCompatibility(t *testing.T) {
	for _, password := range []string{"short", "123456789012345", "aaaaaaaaaaaaaaa"} {
		if _, err := HashNewPassword(password); err == nil {
			t.Fatalf("weak password accepted: %q", password)
		}
	}
	if _, err := HashNewPassword("A longer personal passphrase"); err != nil {
		t.Fatal(err)
	}
	if valid, legacy := VerifyPassword("0123456789abcdef0123456789abcdef", "0123456789abcdef0123456789abcdef"); !valid || !legacy {
		t.Fatal("legacy valid credential broken")
	}
	if valid, _ := VerifyPassword("", "0123456789abcdef0123456789abcdef"); valid {
		t.Fatal("missing account accepted")
	}
}
