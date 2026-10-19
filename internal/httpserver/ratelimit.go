package httpserver

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

type window struct {
	count int
	until time.Time
}

// RateLimits bounds attempts per client IP on one API instance. Upload handlers
// additionally enforce the existing authenticated per-user limit.
type RateLimits struct {
	mu        sync.Mutex
	entries   map[string]window
	trusted   []netip.Prefix
	now       func() time.Time
	nextSweep time.Time
}

func NewRateLimits(cidrs string) (*RateLimits, error) {
	l := &RateLimits{entries: make(map[string]window), now: time.Now}
	if strings.TrimSpace(cidrs) == "" {
		return l, nil
	}
	for _, value := range strings.Split(cidrs, ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("parse trusted proxy CIDR: %w", err)
		}
		l.trusted = append(l.trusted, prefix)
	}
	return l, nil
}

func (l *RateLimits) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return "unknown"
	}
	peer = peer.Unmap()
	for _, prefix := range l.trusted {
		if prefix.Contains(peer) {
			// Caddy overwrites this with a single address. Ignore lists or bad input.
			if forwarded, err := netip.ParseAddr(r.Header.Get("X-Forwarded-For")); err == nil {
				return forwarded.Unmap().String()
			}
			break
		}
	}
	return peer.String()
}

func (l *RateLimits) allow(key string, limit int) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if !now.Before(l.nextSweep) {
		for key, entry := range l.entries {
			if !now.Before(entry.until) {
				delete(l.entries, key)
			}
		}
		l.nextSweep = now.Add(time.Minute)
	}
	entry := l.entries[key]
	if !now.Before(entry.until) {
		// Bound memory even if an attacker rotates addresses rapidly.
		if entry.until.IsZero() && len(l.entries) >= 10_000 {
			return false, 60
		}
		entry = window{until: now.Add(time.Minute)}
	}
	if entry.count >= limit {
		return false, max(1, int(math.Ceil(entry.until.Sub(now).Seconds())))
	}
	entry.count++
	l.entries[key] = entry
	return true, 0
}

func (l *RateLimits) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var bucket string
			var limit int
			switch r.URL.Path {
			case "/auth/register", "/auth/login", "/auth/refresh", "/auth/logout":
				bucket, limit = "auth:", 20
			default:
				parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
				if len(parts) == 3 && parts[0] == "rooms" && parts[2] == "upload" {
					bucket, limit = "upload:", 60
				}
			}
			if limit > 0 {
				if allowed, retry := l.allow(bucket+l.clientIP(r), limit); !allowed {
					w.Header().Set("Retry-After", strconv.Itoa(retry))
					w.Header().Set("Cache-Control", "no-store")
					http.Error(w, "too many requests", http.StatusTooManyRequests)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
