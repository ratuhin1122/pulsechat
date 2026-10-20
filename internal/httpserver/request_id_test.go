package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pulsechat/internal/logging"
)

func TestRequestID(t *testing.T) {
	for _, tt := range []struct {
		name, id string
		preserve bool
	}{
		{"generated", "", false}, {"accepted", "trace-ABC_123.4", true}, {"invalid characters", "bad id\r\nvalue", false}, {"too long", strings.Repeat("a", 129), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := logging.New(&logs, "api")
			monitor, err := NewMonitor(logger)
			if err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			mux.HandleFunc("GET /rooms/{roomID}", func(w http.ResponseWriter, r *http.Request) {
				logging.Set(r.Context(), logging.Fields{UserID: "user-1", RoomID: r.PathValue("roomID"), AttachmentID: "attachment-1"})
				w.WriteHeader(http.StatusInternalServerError)
			})
			request := httptest.NewRequest("GET", "/rooms/room-1?access_token=never-log-this", nil)
			request.Header.Set("X-Request-ID", tt.id)
			response := httptest.NewRecorder()
			RequestID(monitor.Wrap(mux, mux)).ServeHTTP(response, request)
			id := response.Header().Get("X-Request-ID")
			if !logging.ValidID(id) || tt.preserve && id != tt.id || !tt.preserve && id == tt.id {
				t.Fatalf("unexpected request ID %q", id)
			}
			var record map[string]any
			if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			for key, want := range map[string]string{"request_id": id, "user_id": "user-1", "room_id": "room-1", "attachment_id": "attachment-1", "level": "ERROR"} {
				if record[key] != want {
					t.Errorf("%s=%v,want %s", key, record[key], want)
				}
			}
			if strings.Contains(logs.String(), "never-log-this") {
				t.Fatal("logged secret query")
			}
		})
	}
}
