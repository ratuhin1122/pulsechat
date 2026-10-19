package chat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/store/db"
)

type historyRepository struct {
	*fakeRepository
	cursor  func(context.Context, db.GetRoomMessageParams) (db.Message, error)
	history func(context.Context, db.ListRoomHistoryParams) ([]db.Message, error)
}

func (s *historyRepository) GetRoomMessage(ctx context.Context, p db.GetRoomMessageParams) (db.Message, error) {
	return s.cursor(ctx, p)
}
func (s *historyRepository) ListRoomHistory(ctx context.Context, p db.ListRoomHistoryParams) ([]db.Message, error) {
	return s.history(ctx, p)
}

func TestMessageHistory(t *testing.T) {
	room := testUUID(t, "550e8400-e29b-41d4-a716-446655440001")
	user := testUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	cursorID := testUUID(t, "550e8400-e29b-41d4-a716-446655440010")
	stamp := pgtype.Timestamptz{Time: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC), Valid: true}
	for _, tt := range []struct {
		name, query                   string
		status                        int
		limit                         int32
		memberErr, cursorErr, listErr error
		empty, noAuth                 bool
	}{
		{name: "default page", status: 200, limit: 50},
		{name: "before cursor", query: "?before=" + cursorID.String() + "&limit=2", status: 200, limit: 2},
		{name: "empty history", status: 200, limit: 50, empty: true},
		{name: "invalid limit", query: "?limit=51", status: 400},
		{name: "zero limit", query: "?limit=0", status: 400},
		{name: "negative limit", query: "?limit=-1", status: 400},
		{name: "nonnumeric limit", query: "?limit=bad", status: 400},
		{name: "empty limit", query: "?limit=", status: 400},
		{name: "duplicate limit", query: "?limit=2&limit=3", status: 400},
		{name: "invalid cursor", query: "?before=bad", status: 400},
		{name: "empty cursor", query: "?before=", status: 400},
		{name: "duplicate cursor", query: "?before=" + cursorID.String() + "&before=" + cursorID.String(), status: 400},
		{name: "cursor missing or another room", query: "?before=" + cursorID.String(), cursorErr: pgx.ErrNoRows, status: 404},
		{name: "cursor failure", query: "?before=" + cursorID.String(), cursorErr: errors.New("database unavailable"), status: 500},
		{name: "list failure", limit: 50, listErr: errors.New("database unavailable"), status: 500},
		{name: "nonmember", memberErr: pgx.ErrNoRows, status: 403},
		{name: "membership failure", memberErr: errors.New("database unavailable"), status: 500},
		{name: "unauthenticated", noAuth: true, status: 401},
	} {
		t.Run(tt.name, func(t *testing.T) {
			listed := false
			repo := &historyRepository{
				fakeRepository: &fakeRepository{member: func(_ context.Context, p db.GetRoomMemberParams) (db.RoomMember, error) {
					if p.RoomID != room || p.UserID != user {
						t.Error("wrong membership arguments")
					}
					return db.RoomMember{}, tt.memberErr
				}},
				cursor: func(_ context.Context, p db.GetRoomMessageParams) (db.Message, error) {
					if p.RoomID != room || p.ID != cursorID {
						t.Error("cursor not scoped to room")
					}
					return db.Message{ID: cursorID, RoomID: room, CreatedAt: stamp}, tt.cursorErr
				},
				history: func(_ context.Context, p db.ListRoomHistoryParams) ([]db.Message, error) {
					listed = true
					if tt.status != 200 && tt.listErr == nil {
						t.Fatal("queried messages before authorization/validation")
					}
					if p.RoomID != room || p.PageLimit != tt.limit {
						t.Errorf("wrong page parameters: %+v", p)
					}
					if tt.name == "before cursor" {
						if p.BeforeID != cursorID || p.BeforeCreatedAt != stamp {
							t.Error("cursor timestamp and ID not forwarded")
						}
					} else if p.BeforeID.Valid || p.BeforeCreatedAt.Valid {
						t.Error("initial page has a cursor")
					}
					if tt.empty {
						return nil, nil
					}
					return []db.Message{{ID: user, RoomID: room, SenderID: user, Type: "text", Body: "hello", CreatedAt: stamp}}, tt.listErr
				},
			}
			mux, token := testRoutes(t, repo)
			r := httptest.NewRequest("GET", "/rooms/"+room.String()+"/messages"+tt.query, nil)
			if !tt.noAuth {
				r.Header.Set("Authorization", "Bearer "+token)
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Fatalf("status = %d: %s", w.Code, w.Body.String())
			}
			if tt.status != 200 {
				return
			}
			if !listed {
				t.Fatal("history query not called")
			}
			var messages []messageResponse
			if err := json.Unmarshal(w.Body.Bytes(), &messages); err != nil {
				t.Fatal(err)
			}
			if tt.empty {
				if w.Body.String() != "[]\n" {
					t.Error("empty history must be []")
				}
			} else if len(messages) != 1 || messages[0].Body != "hello" || messages[0].RoomID != room || !messages[0].CreatedAt.Equal(stamp.Time) {
				t.Fatalf("unexpected response: %s", w.Body.String())
			}
		})
	}
}
