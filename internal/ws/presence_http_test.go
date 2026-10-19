package ws

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type presenceRepository struct {
	*testRepository
	membersErr error
}

func (s *presenceRepository) ListRoomMemberIDs(ctx context.Context, room pgtype.UUID) ([]pgtype.UUID, error) {
	ids, err := s.testRepository.ListRoomMemberIDs(ctx, room)
	if err != nil {
		return nil, err
	}
	var peer pgtype.UUID
	if err := peer.Scan("550e8400-e29b-41d4-a716-446655440002"); err != nil {
		return nil, err
	}
	return append(ids, peer), s.membersErr
}

func TestPresenceEndpoint(t *testing.T) {
	for _, tt := range []struct {
		name, query               string
		membershipErr, membersErr error
		unauthenticated           bool
		status                    int
	}{
		{name: "snapshot", query: "?room_id=550e8400-e29b-41d4-a716-446655440001", status: 200},
		{name: "missing room", status: 400},
		{name: "invalid room", query: "?room_id=bad", status: 400},
		{name: "unauthenticated", unauthenticated: true, status: 401},
		{name: "nonmember", query: "?room_id=550e8400-e29b-41d4-a716-446655440001", membershipErr: pgx.ErrNoRows, status: 403},
		{name: "membership failure", query: "?room_id=550e8400-e29b-41d4-a716-446655440001", membershipErr: errors.New("database unavailable"), status: 500},
		{name: "members failure", query: "?room_id=550e8400-e29b-41d4-a716-446655440001", membersErr: errors.New("database unavailable"), status: 500},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &presenceRepository{testRepository: &testRepository{membershipErr: tt.membershipErr}, membersErr: tt.membersErr}
			h, token := newTestHandler(t, repo, "")
			if _, err := h.presence.Connect(context.Background(), "550e8400-e29b-41d4-a716-446655440000"); err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			h.RegisterRoutes(mux)
			r := httptest.NewRequest("GET", "/presence"+tt.query, nil)
			if !tt.unauthenticated {
				r.Header.Set("Authorization", "Bearer "+token)
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Fatalf("status = %d: %s", w.Code, w.Body.String())
			}
			if tt.status == 200 {
				var result []presenceStatus
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if len(result) != 2 || !result[0].Online || result[1].Online {
					t.Fatalf("snapshot = %s", w.Body.String())
				}
				if w.Header().Get("Cache-Control") != "no-store" {
					t.Error("presence response is cacheable")
				}
			}
		})
	}
}
