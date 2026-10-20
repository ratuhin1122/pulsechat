package chat

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"pulsechat/internal/store/db"
)

type readRepository struct {
	*fakeRepository
	mark func(context.Context, db.SetRoomReadMarkerParams) (pgtype.UUID, error)
}

func (f *readRepository) MarkRoomRead(ctx context.Context, p db.SetRoomReadMarkerParams) (pgtype.UUID, error) {
	return f.mark(ctx, p)
}

type readPublisherFunc func(context.Context, pgtype.UUID, pgtype.UUID, pgtype.UUID) error

func (f readPublisherFunc) PublishRead(ctx context.Context, r, u, m pgtype.UUID) error {
	return f(ctx, r, u, m)
}

func TestMarkRead(t *testing.T) {
	room := testUUID(t, "550e8400-e29b-41d4-a716-446655440001")
	message := testUUID(t, "550e8400-e29b-41d4-a716-446655440002")
	for _, tt := range []struct {
		name, body                                                string
		status                                                    int
		noAuth, nonMember, missing, storeFail, publishFail, stale bool
	}{
		{name: "read", body: `{"message_id":"` + message.String() + `"}`, status: 204},
		{name: "stale request broadcasts effective marker", body: `{"message_id":"` + message.String() + `"}`, status: 204, stale: true},
		{name: "unauthenticated", body: `{}`, status: 401, noAuth: true},
		{name: "non member", body: `{}`, status: 403, nonMember: true},
		{name: "invalid JSON", body: `{`, status: 400},
		{name: "invalid message", body: `{"message_id":"bad"}`, status: 400},
		{name: "missing or wrong-room message", body: `{"message_id":"` + message.String() + `"}`, status: 404, missing: true},
		{name: "storage failure", body: `{"message_id":"` + message.String() + `"}`, status: 500, storeFail: true},
		{name: "notification failure", body: `{"message_id":"` + message.String() + `"}`, status: 503, publishFail: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stored, published := false, false
			effective := message
			if tt.stale {
				effective = testUUID(t, "550e8400-e29b-41d4-a716-446655440003")
			}
			repo := &readRepository{fakeRepository: &fakeRepository{member: func(context.Context, db.GetRoomMemberParams) (db.RoomMember, error) {
				if tt.nonMember {
					return db.RoomMember{}, pgx.ErrNoRows
				}
				return db.RoomMember{}, nil
			}}, mark: func(_ context.Context, p db.SetRoomReadMarkerParams) (pgtype.UUID, error) {
				stored = true
				if p.RoomID != room || p.MessageID != message || !p.UserID.Valid {
					t.Fatalf("params=%+v", p)
				}
				if tt.missing {
					return pgtype.UUID{}, pgx.ErrNoRows
				}
				if tt.storeFail {
					return pgtype.UUID{}, errors.New("database")
				}
				return effective, nil
			}}
			publisher := readPublisherFunc(func(_ context.Context, r, u, m pgtype.UUID) error {
				published = true
				if !stored || r != room || m != effective || !u.Valid {
					t.Fatal("invalid publish")
				}
				if tt.publishFail {
					return errors.New("redis")
				}
				return nil
			})
			mux, token := testRoutes(t, repo, publisher)
			r := httptest.NewRequest("POST", "/rooms/"+room.String()+"/read", strings.NewReader(tt.body))
			if !tt.noAuth {
				r.Header.Set("Authorization", "Bearer "+token)
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if published != (tt.status == 204 || tt.publishFail) {
				t.Fatal("unexpected read broadcast")
			}
		})
	}
}
