package httpserver

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimits(t *testing.T) {
	for _, tt := range []struct {
		name, method, path string
		limit              int
	}{
		{"login", "POST", "/auth/login", 20},
		{"register", "POST", "/auth/register", 20},
		{"refresh", "POST", "/auth/refresh", 20},
		{"logout", "POST", "/auth/logout", 20},
		{"upload", "POST", "/rooms/room-id/upload", 60},
		{"history is unrestricted", "GET", "/rooms/id/messages", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			limits, err := NewRateLimits("")
			if err != nil {
				t.Fatal(err)
			}
			now := time.Unix(1000, 0)
			limits.now = func() time.Time { return now }
			handler := limits.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
			request := func(ip string) *httptest.ResponseRecorder {
				r := httptest.NewRequest(tt.method, tt.path, nil)
				r.RemoteAddr = ip
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				return w
			}
			for i := range max(tt.limit, 70) {
				w := request("192.0.2.1:1234")
				want := 204
				if tt.limit > 0 && i >= tt.limit {
					want = 429
				}
				if w.Code != want {
					t.Fatalf("request %d status=%d want=%d", i, w.Code, want)
				}
			}
			if tt.limit > 0 {
				w := request("192.0.2.1:5678")
				if w.Code != 429 || w.Header().Get("Retry-After") != "60" {
					t.Fatalf("limit status=%d headers=%v", w.Code, w.Header())
				}
				if w := request("192.0.2.2:1234"); w.Code != 204 {
					t.Fatalf("other IP status=%d", w.Code)
				}
				now = now.Add(time.Minute)
				if w := request("192.0.2.1:1234"); w.Code != 204 {
					t.Fatalf("reset status=%d", w.Code)
				}
			}
		})
	}
}

func TestClientIP(t *testing.T) {
	for _, tt := range []struct{ name, cidrs, peer, forwarded, want string }{
		{"direct", "", "192.0.2.1:123", "203.0.113.1", "192.0.2.1"},
		{"untrusted proxy", "10.0.0.0/8", "192.0.2.1:123", "203.0.113.1", "192.0.2.1"},
		{"trusted proxy", "10.0.0.0/8", "10.0.0.2:123", "203.0.113.1", "203.0.113.1"},
		{"forged list", "10.0.0.0/8", "10.0.0.2:123", "203.0.113.1, 192.0.2.1", "10.0.0.2"},
		{"invalid header", "10.0.0.0/8", "10.0.0.2:123", "invalid", "10.0.0.2"},
		{"ipv6", "::1/128", "[::1]:123", "2001:db8::1", "2001:db8::1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			limits, err := NewRateLimits(tt.cidrs)
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("POST", "/auth/login", nil)
			r.RemoteAddr = tt.peer
			r.Header.Set("X-Forwarded-For", tt.forwarded)
			if got := limits.clientIP(r); got != tt.want {
				t.Fatalf("IP=%s want=%s", got, tt.want)
			}
		})
	}
	if _, err := NewRateLimits("bad-cidr"); err == nil {
		t.Fatal("invalid CIDR accepted")
	}
}

func TestConcurrentAndBoundedLimits(t *testing.T) {
	l, err := NewRateLimits("")
	if err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Int32
	var group sync.WaitGroup
	for range 100 {
		group.Go(func() {
			if ok, _ := l.allow("auth:ip", 20); ok {
				accepted.Add(1)
			}
		})
	}
	group.Wait()
	if accepted.Load() != 20 {
		t.Fatalf("accepted=%d", accepted.Load())
	}
	for i := range 10_000 {
		l.allow(fmt.Sprint(i), 20)
	}
	if len(l.entries) != 10_000 {
		t.Fatalf("entries=%d", len(l.entries))
	}
	if allowed, _ := l.allow("new-address", 20); allowed {
		t.Fatal("unbounded entry accepted")
	}
}
