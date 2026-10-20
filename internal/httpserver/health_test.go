package httpserver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealthRoutes(t *testing.T) {
	for _, tt := range []struct {
		name, failed string
		shutdown     bool
		want         int
	}{
		{name: "ready", want: 200}, {name: "postgres unavailable", failed: "postgres", want: 503},
		{name: "redis unavailable", failed: "redis", want: 503}, {name: "clamav unavailable", failed: "clamav", want: 503},
		{name: "shutting down", shutdown: true, want: 503},
	} {
		t.Run(tt.name, func(t *testing.T) {
			lifecycle, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.shutdown {
				cancel()
			}
			calls := 0
			checks := []DependencyCheck{}
			for _, name := range []string{"postgres", "redis", "clamav"} {
				checks = append(checks, DependencyCheck{Name: name, Ping: func(ctx context.Context) error {
					calls++
					deadline, ok := ctx.Deadline()
					if !ok || time.Until(deadline) > 2*time.Second {
						t.Error("missing bounded probe context")
					}
					if name == tt.failed {
						return errors.New("private-dsn-and-credentials")
					}
					return nil
				}})
			}
			mux := http.NewServeMux()
			HealthRoutes(mux, lifecycle, slog.New(slog.NewTextHandler(io.Discard, nil)), checks...)
			live := httptest.NewRecorder()
			mux.ServeHTTP(live, httptest.NewRequest("GET", "/healthz", nil))
			if live.Code != 200 || live.Body.String() != "ok\n" || calls != 0 {
				t.Fatal("liveness invoked dependencies or failed")
			}
			ready := httptest.NewRecorder()
			mux.ServeHTTP(ready, httptest.NewRequest("GET", "/readyz", nil))
			if ready.Code != tt.want {
				t.Fatalf("ready status=%d,want %d", ready.Code, tt.want)
			}
			if strings.Contains(ready.Body.String(), "private") || ready.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("readiness leaked error or is cacheable")
			}
			if tt.want == 200 && calls != 3 {
				t.Fatalf("checked %d dependencies", calls)
			}
			if tt.shutdown && calls != 0 {
				t.Fatal("checked dependencies after shutdown")
			}
		})
	}
}

func TestReadinessHonorsCancellation(t *testing.T) {
	for _, source := range []string{"request", "shutdown"} {
		t.Run(source, func(t *testing.T) {
			lifecycle, stop := context.WithCancel(context.Background())
			defer stop()
			requestCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			if source == "shutdown" {
				requestCtx = context.Background()
			}
			mux := http.NewServeMux()
			HealthRoutes(mux, lifecycle, slog.New(slog.NewTextHandler(io.Discard, nil)), DependencyCheck{Name: "blocked", Ping: func(ctx context.Context) error {
				if source == "shutdown" {
					stop()
				}
				<-ctx.Done()
				return ctx.Err()
			}})
			started := time.Now()
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest("GET", "/readyz", nil).WithContext(requestCtx))
			if response.Code != 503 || time.Since(started) > 2*time.Second {
				t.Fatal("readiness did not stop on cancellation")
			}
		})
	}
}
