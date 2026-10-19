package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/store/db"
)

type replayRepository struct {
	*testRepository
	cursor func(context.Context, db.GetUserMessageParams) (db.Message, error)
	latest func(context.Context, pgtype.UUID) (db.Message, error)
	page   func(context.Context, db.ListReplayMessagesParams) ([]db.Message, error)
}

func (s *replayRepository) GetUserMessage(ctx context.Context, p db.GetUserMessageParams) (db.Message, error) {
	return s.cursor(ctx, p)
}
func (s *replayRepository) GetLatestUserMessage(ctx context.Context, id pgtype.UUID) (db.Message, error) {
	return s.latest(ctx, id)
}
func (s *replayRepository) ListReplayMessages(ctx context.Context, p db.ListReplayMessagesParams) ([]db.Message, error) {
	return s.page(ctx, p)
}

func replayMessage(t *testing.T, n int) db.Message {
	t.Helper()
	var id, user, room pgtype.UUID
	for target, value := range map[*pgtype.UUID]string{
		&id:   fmt.Sprintf("550e8400-e29b-41d4-a716-%012d", n),
		&user: "550e8400-e29b-41d4-a716-446655440000",
		&room: fmt.Sprintf("550e8400-e29b-41d4-a716-%012d", 1000+n%2),
	} {
		if err := target.Scan(value); err != nil {
			t.Fatal(err)
		}
	}
	return db.Message{ID: id, RoomID: room, SenderID: user, Type: "text", Body: fmt.Sprint(n), CreatedAt: pgtype.Timestamptz{Time: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC), Valid: true}}
}

func replaySocket(t *testing.T, h *Handler, token, cursor string) (context.Context, *websocket.Conn) {
	t.Helper()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws?access_token="+token+"&last_message_id="+cursor,
		&websocket.DialOptions{HTTPHeader: map[string][]string{"Origin": {"http://frontend.test"}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	return ctx, conn
}

func TestReplayPages(t *testing.T) {
	for _, count := range []int{0, 1, 50, 125} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			cursor, upper := replayMessage(t, 1), replayMessage(t, count+1)
			messages := make([]db.Message, 0, count)
			for i := 2; i <= count+1; i++ {
				messages = append(messages, replayMessage(t, i))
			}
			next := 0
			repo := &replayRepository{testRepository: &testRepository{},
				cursor: func(_ context.Context, p db.GetUserMessageParams) (db.Message, error) {
					if p.MessageID != cursor.ID || p.UserID != cursor.SenderID {
						t.Error("cursor not scoped to authenticated user")
					}
					return cursor, nil
				},
				latest: func(_ context.Context, id pgtype.UUID) (db.Message, error) {
					if id != cursor.SenderID {
						t.Error("wrong replay user")
					}
					return upper, nil
				},
				page: func(_ context.Context, p db.ListReplayMessagesParams) ([]db.Message, error) {
					wantAfter := cursor
					if next > 0 {
						wantAfter = messages[next-1]
					}
					if p.UserID != cursor.SenderID || p.AfterID != wantAfter.ID || p.AfterCreatedAt != wantAfter.CreatedAt || p.ThroughID != upper.ID || p.ThroughCreatedAt != upper.CreatedAt || p.PageLimit != 50 {
						t.Error("incorrect replay cursor/boundary")
					}
					end := min(next+50, len(messages))
					page := messages[next:end]
					next = end
					return page, nil
				},
			}
			h, token := newTestHandler(t, repo, "http://frontend.test")
			ctx, conn := replaySocket(t, h, token, cursor.ID.String())
			for _, want := range messages {
				_, data, err := conn.Read(ctx)
				if err != nil {
					t.Fatal(err)
				}
				var got outgoingMessage
				if err := json.Unmarshal(data, &got); err != nil {
					t.Fatal(err)
				}
				if got.ID != want.ID || got.RoomID != want.RoomID || got.Body != want.Body || !got.CreatedAt.Equal(want.CreatedAt.Time) {
					t.Fatalf("wrong replay order/event: %s", data)
				}
			}
			if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"ping"}`)); err != nil {
				t.Fatal(err)
			}
			_, data, err := conn.Read(ctx)
			if err != nil || string(data) != `{"type":"pong"}` {
				t.Fatalf("replay not drained: %s, %v", data, err)
			}
		})
	}
}

func TestReplayThenLive(t *testing.T) {
	cursor, missed, live := replayMessage(t, 1), replayMessage(t, 2), replayMessage(t, 3)
	started, release := make(chan struct{}), make(chan struct{})
	repo := &replayRepository{testRepository: &testRepository{},
		cursor: func(context.Context, db.GetUserMessageParams) (db.Message, error) { return cursor, nil },
		latest: func(context.Context, pgtype.UUID) (db.Message, error) { return missed, nil },
		page: func(ctx context.Context, p db.ListReplayMessagesParams) ([]db.Message, error) {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			if p.ThroughID != missed.ID {
				t.Error("live message included in replay boundary")
			}
			return []db.Message{missed}, nil
		},
	}
	h, token := newTestHandler(t, repo, "http://frontend.test")
	ctx, conn := replaySocket(t, h, token, cursor.ID.String())
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	data, err := marshalMessage(live)
	if err != nil {
		t.Fatal(err)
	}
	h.hub.Broadcast(live.RoomID.String(), []string{cursor.SenderID.String()}, data)
	close(release)
	for _, want := range []db.Message{missed, live} {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var got outgoingMessage
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if got.ID != want.ID {
			t.Fatalf("expected %s, got %s", want.ID.String(), data)
		}
	}
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"ping"}`)); err != nil {
		t.Fatal(err)
	}
	_, data, err = conn.Read(ctx)
	if err != nil || string(data) != `{"type":"pong"}` {
		t.Fatalf("unexpected duplicate event: %s, %v", data, err)
	}
}
