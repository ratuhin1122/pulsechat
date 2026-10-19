package upload

import (
	"bytes"
	"context"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMemoryLimiter(t *testing.T) {
	limiter, err := NewMemoryLimiter(2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	limiter.now = func() time.Time { return now }
	for _, tt := range []struct {
		user string
		want bool
	}{{"alice", true}, {"alice", true}, {"alice", false}, {"bob", true}} {
		got, retry, err := limiter.Allow(context.Background(), tt.user)
		if err != nil || got != tt.want || !got && retry != time.Minute {
			t.Fatalf("Allow(%s) = %v, %v, %v", tt.user, got, retry, err)
		}
	}
	now = now.Add(time.Minute)
	if allowed, _, err := limiter.Allow(context.Background(), "alice"); err != nil || !allowed {
		t.Fatal("window did not reset")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := limiter.Allow(ctx, "alice"); err == nil {
		t.Fatal("ignored canceled context")
	}
}

func TestConcurrentLimiter(t *testing.T) {
	limiter, err := NewMemoryLimiter(3, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _, err := limiter.Allow(context.Background(), "alice")
			if err != nil {
				t.Error(err)
			}
			if ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if allowed.Load() != 3 {
		t.Fatalf("allowed %d concurrent uploads", allowed.Load())
	}
}

func TestUploadRateLimitAcrossRooms(t *testing.T) {
	repo := &testRepository{}
	mux, token, _ := testHandler(t, repo, 1)
	for i := 0; i < 2; i++ {
		r := fileRequest(t, "document.pdf", bytes.NewReader(append([]byte("%PDF-1.7\n"), make([]byte, 100)...)))
		if i == 1 {
			r.URL.Path = "/rooms/550e8400-e29b-41d4-a716-446655440002/upload"
		}
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if i == 0 && w.Code != 202 {
			t.Fatalf("initial upload = %d", w.Code)
		}
		if i == 1 && (w.Code != 429 || w.Header().Get("Retry-After") == "") {
			t.Fatalf("limited upload = %d", w.Code)
		}
	}
	if len(repo.inserted) != 1 {
		t.Fatal("rate-limited upload inserted an attachment")
	}
}
