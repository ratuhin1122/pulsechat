package ws

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/store/db"
)

func TestReplayDeduplicatesDelayedPubSub(t *testing.T) {
	cursor, missed := replayMessage(t, 1), replayMessage(t, 2)
	started, release := make(chan struct{}), make(chan struct{})
	repo := &replayRepository{testRepository: &testRepository{},
		cursor: func(context.Context, db.GetUserMessageParams) (db.Message, error) { return cursor, nil },
		latest: func(context.Context, pgtype.UUID) (db.Message, error) { close(started); <-release; return missed, nil },
		page: func(context.Context, db.ListReplayMessagesParams) ([]db.Message, error) {
			return []db.Message{missed}, nil
		},
	}
	h, token := newTestHandler(t, repo, "http://frontend.test")
	server := httptest.NewServer(h)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws?access_token="+token+"&last_message_id="+cursor.ID.String(), &websocket.DialOptions{HTTPHeader: map[string][]string{"Origin": {"http://frontend.test"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	data, err := marshalMessage(missed)
	if err != nil {
		t.Fatal(err)
	}
	// An event from another instance arrives while the replay boundary is read.
	h.hub.deliver(Event{RoomID: missed.RoomID.String(), UserIDs: []string{cursor.SenderID.String()}, Payload: data})
	close(release)
	_, first, err := conn.Read(ctx)
	if err != nil || string(first) != string(data) {
		t.Fatalf("missing replay: %s, %v", first, err)
	}
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"ping"}`)); err != nil {
		t.Fatal(err)
	}
	_, next, err := conn.Read(ctx)
	if err != nil || string(next) != `{"type":"pong"}` {
		t.Fatalf("duplicate replay/live event: %s, %v", next, err)
	}
}
