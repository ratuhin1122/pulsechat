package auth

import (
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"
)

func TestCookieModes(t *testing.T) {
	for _, tt := range []struct {
		name   string
		local  bool
		cookie string
		secure bool
	}{
		{"production", false, "__Secure-pulsechat_refresh", true},
		{"local HTTP", true, "pulsechat_refresh_dev", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := NewWithOptions(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{LocalHTTP: tt.local})
			for _, maxAge := range []int{3600, -1} {
				cookie := h.refreshCookie("test", time.Now().Add(time.Hour), maxAge)
				if cookie.Name != tt.cookie || cookie.Secure != tt.secure || !cookie.HttpOnly || cookie.Path != "/auth" || cookie.SameSite != http.SameSiteStrictMode || cookie.MaxAge != maxAge {
					t.Fatalf("cookie=%+v", cookie)
				}
			}
		})
	}
}
