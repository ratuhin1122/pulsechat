package ws

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/auth"
	"pulsechat/internal/store/db"
)

type testRepository struct {
	membershipErr error
	insertErr     error
	attachments   func(context.Context, pgtype.UUID) ([]db.Attachment, error)
}

func (s *testRepository) ListAttachmentsForMessage(ctx context.Context, id pgtype.UUID) ([]db.Attachment, error) {
	if s.attachments != nil {
		return s.attachments(ctx, id)
	}
	return nil, nil
}

func (s *testRepository) GetRoomMember(_ context.Context, p db.GetRoomMemberParams) (db.RoomMember, error) {
	return db.RoomMember{RoomID: p.RoomID, UserID: p.UserID}, s.membershipErr
}

func (s *testRepository) CreateMemberMessage(_ context.Context, p db.CreateMemberMessageParams) (db.Message, error) {
	return db.Message{ID: p.RoomID, RoomID: p.RoomID, SenderID: p.SenderID, Type: "text", Body: p.Body,
		CreatedAt: pgtype.Timestamptz{Time: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC), Valid: true}}, s.insertErr
}

func (s *testRepository) ListRoomMemberIDs(_ context.Context, _ pgtype.UUID) ([]pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan("550e8400-e29b-41d4-a716-446655440000"); err != nil {
		return nil, err
	}
	return []pgtype.UUID{id}, nil
}

func (s *testRepository) ListUserRoomIDs(_ context.Context, _ pgtype.UUID) ([]pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan("550e8400-e29b-41d4-a716-446655440001"); err != nil {
		return nil, err
	}
	return []pgtype.UUID{id}, nil
}

func (s *testRepository) GetUserMessage(context.Context, db.GetUserMessageParams) (db.Message, error) {
	return db.Message{}, pgx.ErrNoRows
}

func (s *testRepository) GetLatestUserMessage(context.Context, pgtype.UUID) (db.Message, error) {
	return db.Message{}, pgx.ErrNoRows
}

func (s *testRepository) ListReplayMessages(context.Context, db.ListReplayMessagesParams) ([]db.Message, error) {
	return nil, nil
}

func newTestHandler(t *testing.T, repo Repository, origin string) (*Handler, string) {
	t.Helper()
	tokens, err := auth.NewTokens(strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := tokens.Issue("550e8400-e29b-41d4-a716-446655440000")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	hub := NewHub()
	t.Cleanup(func() { cancel(); hub.Close() })
	h, err := New(ctx, repo, tokens, hub, NewMemoryPresence(), slog.New(slog.NewTextHandler(io.Discard, nil)), origin)
	if err != nil {
		t.Fatal(err)
	}
	return h, raw
}

func TestWebSocketMessages(t *testing.T) {
	valid := `{"type":"message","room_id":"550e8400-e29b-41d4-a716-446655440001","body":"hello"}`
	for _, tt := range []struct {
		name, data               string
		kind                     websocket.MessageType
		membershipErr, insertErr error
		closeCode                websocket.StatusCode
	}{
		{name: "persist and broadcast", data: valid},
		{name: "nonmember", data: valid, membershipErr: pgx.ErrNoRows, closeCode: websocket.StatusPolicyViolation},
		{name: "membership lost before insert", data: valid, insertErr: pgx.ErrNoRows, closeCode: websocket.StatusPolicyViolation},
		{name: "database failure", data: valid, insertErr: errors.New("database unavailable"), closeCode: websocket.StatusInternalError},
		{name: "invalid JSON", data: "{", closeCode: websocket.StatusPolicyViolation},
		{name: "binary frame", data: valid, kind: websocket.MessageBinary, closeCode: websocket.StatusPolicyViolation},
		{name: "too large", data: strings.Repeat("x", MaxMessageSize+1), closeCode: websocket.StatusMessageTooBig},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h, token := newTestHandler(t, &testRepository{membershipErr: tt.membershipErr, insertErr: tt.insertErr}, "http://frontend.test")
			server := httptest.NewServer(h)
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws?access_token="+token,
				&websocket.DialOptions{HTTPHeader: map[string][]string{"Origin": {"http://frontend.test"}}})
			if err != nil {
				t.Fatal(err)
			}
			defer conn.CloseNow()
			kind := tt.kind
			if kind == 0 {
				kind = websocket.MessageText
			}
			if err := conn.Write(ctx, kind, []byte(tt.data)); err != nil {
				t.Fatal(err)
			}
			_, data, err := conn.Read(ctx)
			if tt.closeCode != 0 {
				if websocket.CloseStatus(err) != tt.closeCode {
					t.Fatalf("close = %v, want %d", err, tt.closeCode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var event outgoingMessage
			if err := json.Unmarshal(data, &event); err != nil {
				t.Fatal(err)
			}
			if event.Type != "message" || event.Body != "hello" || !event.ID.Valid || event.SenderID.String() != "550e8400-e29b-41d4-a716-446655440000" || event.CreatedAt.Format(time.RFC3339) != "2026-10-19T00:00:00Z" {
				t.Fatalf("incorrect persisted event: %s", data)
			}
		})
	}
}
