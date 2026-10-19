package auth

import (
	"strings"
	"testing"
)

func TestPasswords(t *testing.T) {
	hash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	other, err := HashPassword("correct-password")
	if err != nil || hash == other {
		t.Fatalf("salt did not produce distinct hashes: %v", err)
	}
	for _, tt := range []struct {
		name, password, hash string
		want                 bool
	}{
		{"valid", "correct-password", hash, true},
		{"wrong password", "wrong-password", hash, false},
		{"empty hash", "correct-password", "", false},
		{"wrong algorithm", "correct-password", strings.Replace(hash, "argon2id", "argon2i", 1), false},
		{"unsafe cost", "correct-password", strings.Replace(hash, "65536", "999999999", 1), false},
		{"invalid base64", "correct-password", passwordPrefix + "!$!", false},
		{"short salt", "correct-password", passwordPrefix + "YQ$YQ", false},
		{"extra fields", "correct-password", hash + "$extra", false},
		{"oversized password", strings.Repeat("x", 1025), hash, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := VerifyPassword(tt.password, tt.hash); got != tt.want {
				t.Errorf("VerifyPassword() = %v, want %v", got, tt.want)
			}
		})
	}
	for _, password := range []string{"", "short", strings.Repeat("x", 1025)} {
		if _, err := HashPassword(password); err == nil {
			t.Errorf("HashPassword accepted length %d", len(password))
		}
	}
}
