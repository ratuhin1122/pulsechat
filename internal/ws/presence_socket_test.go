package ws

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgtype"
)

// Two shared rooms must still produce just one event for each transition.
type sharedRoomsRepository struct{ *presenceRepository }

func (s *sharedRoomsRepository) ListUserRoomIDs(ctx context.Context, user pgtype.UUID) ([]pgtype.UUID, error) {
	rooms, err := s.testRepository.ListUserRoomIDs(ctx, user)
	return append(rooms, user), err
}

func TestMultiTabPresenceAndAppPing(t *testing.T) {
	repo := &sharedRoomsRepository{presenceRepository: &presenceRepository{testRepository: &testRepository{}}}
	h, token := newTestHandler(t, repo, "http://frontend.test")
	server := httptest.NewServer(h)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	userID := "550e8400-e29b-41d4-a716-446655440000"
	peerToken, err := h.tokens.Issue("550e8400-e29b-41d4-a716-446655440002")
	if err != nil {
		t.Fatal(err)
	}
	dial := func(raw string) *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws?access_token="+raw,
			&websocket.DialOptions{HTTPHeader: map[string][]string{"Origin": {"http://frontend.test"}}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.CloseNow() })
		return conn
	}
	pong := func(conn *websocket.Conn) {
		t.Helper()
		if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"ping"}`)); err != nil {
			t.Fatal(err)
		}
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != `{"type":"pong"}` {
			t.Fatalf("expected pong without extra presence events: %s", data)
		}
	}
	event := func(conn *websocket.Conn, online bool) {
		t.Helper()
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var value presenceEvent
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatal(err)
		}
		if value.Type != "presence" || value.UserID != userID || value.Online != online {
			t.Fatalf("unexpected event: %s", data)
		}
	}
	waitCount := func(want int) {
		t.Helper()
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for {
			counts, err := h.presence.Counts(ctx, []string{userID})
			if err != nil {
				t.Fatal(err)
			}
			if counts[userID] == want {
				return
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-ticker.C:
			}
		}
	}
	observer := dial(peerToken)
	pong(observer)
	first := dial(token)
	pong(first)
	event(observer, true)
	pong(observer) // One online event despite sharing two rooms.
	second := dial(token)
	pong(second)
	waitCount(2)
	pong(observer) // No 1->2 event.
	first.CloseNow()
	waitCount(1)
	pong(observer) // No 2->1 event.
	second.CloseNow()
	event(observer, false)
	waitCount(0)
	pong(observer)
}
