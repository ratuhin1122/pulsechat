package ws

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	"pulsechat/internal/store/db"
)

func TestReplayCursorRejections(t *testing.T) {
	id := "550e8400-e29b-41d4-a716-446655440001"
	for _, tt := range []struct {
		name, query string
		err         error
		status      int
	}{
		{"invalid UUID", "bad", nil, 400},
		{"empty cursor", "", nil, 400},
		{"duplicate cursor", id + "&last_message_id=" + id, nil, 400},
		{"missing or inaccessible cursor", id, pgx.ErrNoRows, 404},
		{"cursor query failure", id, errors.New("database unavailable"), 500},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &replayRepository{testRepository: &testRepository{}, cursor: func(context.Context, db.GetUserMessageParams) (db.Message, error) { return db.Message{}, tt.err }}
			h, token := newTestHandler(t, repo, "http://frontend.test")
			r := httptest.NewRequest("GET", "/ws?last_message_id="+tt.query, nil)
			r.Header.Set("Origin", "http://frontend.test")
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Fatalf("status = %d: %s", w.Code, w.Body.String())
			}
			if len(h.hub.clients) != 0 {
				t.Error("registered rejected connection")
			}
		})
	}
}
