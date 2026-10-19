package upload

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	ImageLimit    int64 = 15 << 20
	DocumentLimit int64 = 25 << 20
	VoiceLimit    int64 = 10 << 20
	MaxPixels           = 50_000_000
)

type RateLimiter interface {
	Allow(context.Context, string) (bool, time.Duration, error)
}

type rateWindow struct {
	used    int
	expires time.Time
}

// MemoryLimiter limits attempts per authenticated user across all rooms on an API.
type MemoryLimiter struct {
	mu        sync.Mutex
	users     map[string]rateWindow
	limit     int
	window    time.Duration
	now       func() time.Time
	nextSweep time.Time
}

func NewMemoryLimiter(limit int, window time.Duration) (*MemoryLimiter, error) {
	if limit < 1 || window <= 0 {
		return nil, fmt.Errorf("upload rate limit and window must be positive")
	}
	return &MemoryLimiter{users: make(map[string]rateWindow), limit: limit, window: window, now: time.Now}, nil
}

func (l *MemoryLimiter) Allow(ctx context.Context, userID string) (bool, time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, 0, err
	}
	now := l.now()
	if !now.Before(l.nextSweep) {
		for id, entry := range l.users {
			if !now.Before(entry.expires) {
				delete(l.users, id)
			}
		}
		l.nextSweep = now.Add(l.window)
	}
	entry := l.users[userID]
	if !now.Before(entry.expires) {
		entry = rateWindow{expires: now.Add(l.window)}
	}
	if entry.used >= l.limit {
		return false, entry.expires.Sub(now), nil
	}
	entry.used++
	l.users[userID] = entry
	return true, 0, nil
}
