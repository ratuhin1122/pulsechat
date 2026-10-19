package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestTokenValidation(t *testing.T) {
	secret := strings.Repeat("a", 32)
	tokens, err := NewTokens(secret)
	if err != nil {
		t.Fatal(err)
	}
	id := "550e8400-e29b-41d4-a716-446655440000"
	issued, err := tokens.Issue(id)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _, err := jwt.NewParser().ParseUnverified(issued, &jwt.RegisteredClaims{})
	if err != nil {
		t.Fatal(err)
	}
	claims := parsed.Claims.(*jwt.RegisteredClaims)
	if claims.ExpiresAt.Sub(claims.IssuedAt.Time) != AccessTokenTTL {
		t.Fatal("access token lifetime is not 15 minutes")
	}
	for _, tt := range []struct {
		name    string
		change  func(*jwt.RegisteredClaims)
		method  jwt.SigningMethod
		key     string
		wantErr bool
	}{
		{name: "valid"},
		{name: "expired", change: func(c *jwt.RegisteredClaims) { c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute)) }, wantErr: true},
		{name: "missing expiry", change: func(c *jwt.RegisteredClaims) { c.ExpiresAt = nil }, wantErr: true},
		{name: "wrong issuer", change: func(c *jwt.RegisteredClaims) { c.Issuer = "other" }, wantErr: true},
		{name: "wrong audience", change: func(c *jwt.RegisteredClaims) { c.Audience = jwt.ClaimStrings{"other"} }, wantErr: true},
		{name: "invalid subject", change: func(c *jwt.RegisteredClaims) { c.Subject = "invalid" }, wantErr: true},
		{name: "missing subject", change: func(c *jwt.RegisteredClaims) { c.Subject = "" }, wantErr: true},
		{name: "future issued at", change: func(c *jwt.RegisteredClaims) { c.IssuedAt = jwt.NewNumericDate(time.Now().Add(time.Hour)) }, wantErr: true},
		{name: "wrong algorithm", method: jwt.SigningMethodHS384, wantErr: true},
		{name: "wrong signature", key: strings.Repeat("b", 32), wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := *claims
			if tt.change != nil {
				tt.change(&c)
			}
			method := tt.method
			if method == nil {
				method = jwt.SigningMethodHS256
			}
			key := tt.key
			if key == "" {
				key = secret
			}
			raw, err := jwt.NewWithClaims(method, c).SignedString([]byte(key))
			if err != nil {
				t.Fatal(err)
			}
			got, err := tokens.Validate(raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v", err)
			}
			if !tt.wantErr && got != id {
				t.Errorf("subject = %q", got)
			}
		})
	}
	if _, err := tokens.Validate("malformed"); err == nil {
		t.Error("accepted malformed token")
	}
	if _, err := tokens.Issue("invalid"); err == nil {
		t.Error("accepted invalid subject")
	}
	if _, err := NewTokens("short"); err == nil {
		t.Error("accepted weak secret")
	}
}

func TestMiddleware(t *testing.T) {
	tokens, err := NewTokens(strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	id := "550e8400-e29b-41d4-a716-446655440000"
	raw, err := tokens.Issue(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, header string
		status       int
	}{
		{"valid", "Bearer " + raw, http.StatusNoContent},
		{"missing", "", http.StatusUnauthorized},
		{"wrong scheme", "Basic " + raw, http.StatusUnauthorized},
		{"invalid", "Bearer invalid", http.StatusUnauthorized},
	} {
		t.Run(tt.name, func(t *testing.T) {
			handler := tokens.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got, ok := UserID(r.Context())
				if !ok || got != id {
					t.Errorf("context user ID = %q, %v", got, ok)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("Authorization", tt.header)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Errorf("status = %d", w.Code)
			}
		})
	}
}
