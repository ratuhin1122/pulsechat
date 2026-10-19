package ws

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandshakeRejections(t *testing.T) {
	h, token := newTestHandler(t, &testRepository{}, "https://frontend.test")
	for _, tt := range []struct {
		name, origin, authorization, query string
		status                             int
	}{
		{name: "missing origin", authorization: "Bearer " + token, status: 403},
		{name: "foreign origin", origin: "https://evil.test", authorization: "Bearer " + token, status: 403},
		{name: "same host bypass forbidden", origin: "http://api.test", authorization: "Bearer " + token, status: 403},
		{name: "wrong scheme", origin: "http://frontend.test", authorization: "Bearer " + token, status: 403},
		{name: "missing token", origin: "https://frontend.test", status: 401},
		{name: "invalid token", origin: "https://frontend.test", authorization: "Bearer invalid", status: 401},
		{name: "wrong auth scheme", origin: "https://frontend.test", authorization: "Basic " + token, status: 401},
		{name: "duplicate query tokens", origin: "https://frontend.test", query: "?access_token=" + token + "&access_token=" + token, status: 401},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://api.test/ws"+tt.query, nil)
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			if tt.authorization != "" {
				r.Header.Set("Authorization", tt.authorization)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Fatalf("status = %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestOriginPolicy(t *testing.T) {
	for _, origin := range []string{"*", "https://*.test", "https://test/path", "https://test/", "https://test?query=1", "https://user@test", "null"} {
		if err := validateOrigin(origin); err == nil {
			t.Errorf("accepted origin %q", origin)
		}
	}
	h := &Handler{}
	for _, tt := range []struct {
		origin string
		want   bool
	}{
		{"http://api.test", true}, {"https://api.test", false}, {"http://api.test:80", false}, {"", false},
	} {
		r := httptest.NewRequest("GET", "http://api.test/ws", nil)
		if tt.origin != "" {
			r.Header.Set("Origin", tt.origin)
		}
		if _, got := h.checkOrigin(r); got != tt.want {
			t.Errorf("origin %q accepted = %v", tt.origin, got)
		}
	}
	r := httptest.NewRequest("GET", "http://api.test/ws", nil)
	r.Header.Add("Origin", "http://api.test")
	r.Header.Add("Origin", "http://api.test")
	if _, ok := h.checkOrigin(r); ok {
		t.Fatal("accepted multiple Origin headers")
	}
}

func TestParseMessage(t *testing.T) {
	for _, data := range []string{
		`{}`, `null`, `{"type":"join"}`, `{"type":"message","room_id":"bad","body":"hello"}`,
		`{"type":"message","room_id":"550e8400-e29b-41d4-a716-446655440001","body":" "}`,
		`{"type":"message","room_id":"550e8400-e29b-41d4-a716-446655440001","body":"hello","sender_id":"spoof"}`,
		`{"type":"message","room_id":"550e8400-e29b-41d4-a716-446655440001","body":"hello"} {}`,
		`{"type":"message","room_id":"550e8400-e29b-41d4-a716-446655440001","body":"` + strings.Repeat("x", MaxBodySize+1) + `"}`,
	} {
		if _, _, err := parseMessage([]byte(data)); err == nil {
			t.Errorf("accepted invalid message: %.100s", data)
		}
	}
}
