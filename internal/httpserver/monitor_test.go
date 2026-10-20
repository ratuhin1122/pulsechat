package httpserver

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"pulsechat/internal/metrics"
)

func TestMonitor(t *testing.T) {
	var logs bytes.Buffer
	m, err := NewMonitor(slog.New(slog.NewJSONHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rooms/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	handler := m.Wrap(mux, mux)
	for _, id := range []string{"private-room-one", "private-room-two"} {
		r := httptest.NewRequest("GET", "/rooms/"+id+"?access_token=secret-token", nil)
		r.Header.Set("Authorization", "Bearer secret-header")
		handler.ServeHTTP(httptest.NewRecorder(), r)
	}
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	body := w.Body.String()
	if !strings.Contains(body, `pulsechat_http_requests_total{method="GET",route="GET /rooms/{id}",status="204"} 2`) {
		t.Fatalf("missing bounded counter: %s", body)
	}
	for _, text := range []string{body, logs.String()} {
		for _, secret := range []string{"private-room-one", "private-room-two", "secret-token", "secret-header"} {
			if strings.Contains(text, secret) {
				t.Fatalf("leaked %s", secret)
			}
		}
	}
	if !strings.Contains(body, "go_goroutines") || !strings.Contains(body, "pulsechat_http_request_duration_seconds_count") {
		t.Fatal("missing runtime/duration metrics")
	}
	// Separate instances own separate registries; never use global registration.
	if _, err := NewMonitor(slog.Default()); err != nil {
		t.Fatal(err)
	}
}

func TestMonitorWebSocketUpgrade(t *testing.T) {
	m, err := NewMonitor(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	finished := make(chan struct{})
	mux.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
		defer close(finished)
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		defer conn.CloseNow()
		if err := conn.Write(r.Context(), websocket.MessageText, []byte("hello")); err != nil {
			t.Errorf("write: %v", err)
		}
	})
	server := httptest.NewServer(metrics.New().Middleware(mux, m.Wrap(mux, mux)))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	_, message, err := conn.Read(ctx)
	if err != nil || string(message) != "hello" {
		t.Fatalf("read=%s err=%v", message, err)
	}
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestMonitorRangeAndFlush(t *testing.T) {
	for _, tt := range []struct {
		name, path, rangeHeader string
		status                  int
		body                    string
	}{
		{"audio range", "/audio", "bytes=1-3", 206, "bcd"},
		{"streaming", "/stream", "", 200, "hello"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, err := NewMonitor(slog.New(slog.NewJSONHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			mux.HandleFunc("GET /audio", func(w http.ResponseWriter, r *http.Request) {
				http.ServeContent(w, r, "voice.wav", time.Time{}, strings.NewReader("abcdef"))
			})
			mux.HandleFunc("GET /stream", func(w http.ResponseWriter, _ *http.Request) {
				if _, err := io.WriteString(w, "hello"); err != nil {
					t.Error(err)
				}
				if err := http.NewResponseController(w).Flush(); err != nil {
					t.Error(err)
				}
			})
			r := httptest.NewRequest("GET", tt.path, nil)
			r.Header.Set("Range", tt.rangeHeader)
			w := httptest.NewRecorder()
			m.Wrap(mux, mux).ServeHTTP(w, r)
			if w.Code != tt.status || w.Body.String() != tt.body {
				t.Fatalf("response=%d %s", w.Code, w.Body.String())
			}
		})
	}
}
