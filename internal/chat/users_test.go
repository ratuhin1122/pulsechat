package chat

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"pulsechat/internal/store/db"
)

type directoryRepository struct {
	*fakeRepository
	search func(context.Context, db.SearchUsersParams) ([]db.SearchUsersRow, error)
	peers  func(context.Context, db.ListDirectRoomPeersParams) ([]db.ListDirectRoomPeersRow, error)
}

func (f *directoryRepository) SearchUsers(ctx context.Context, p db.SearchUsersParams) ([]db.SearchUsersRow, error) {
	return f.search(ctx, p)
}
func (f *directoryRepository) ListDirectRoomPeers(ctx context.Context, p db.ListDirectRoomPeersParams) ([]db.ListDirectRoomPeersRow, error) {
	return f.peers(ctx, p)
}

func TestSearchUsers(t *testing.T) {
	for _, tt := range []struct {
		name, query            string
		noAuth, failure, empty bool
		status                 int
	}{
		{name: "search", query: "ali", status: 200},
		{name: "empty results", query: "nobody", empty: true, status: 200},
		{name: "authentication required", query: "ali", noAuth: true, status: 401},
		{name: "short search", query: "a", status: 400},
		{name: "blank search", query: "", status: 400},
		{name: "long search", query: strings.Repeat("a", 65), status: 400},
		{name: "database error", query: "ali", failure: true, status: 500},
	} {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			repo := &directoryRepository{fakeRepository: &fakeRepository{}, search: func(ctx context.Context, p db.SearchUsersParams) ([]db.SearchUsersRow, error) {
				called = true
				if p.Search != tt.query || p.ViewerID.String() != "550e8400-e29b-41d4-a716-446655440000" || ctx == nil {
					t.Fatalf("params=%+v", p)
				}
				if tt.failure {
					return nil, errors.New("database unavailable")
				}
				if tt.empty {
					return nil, nil
				}
				return []db.SearchUsersRow{{ID: testUUID(t, "550e8400-e29b-41d4-a716-446655440001"), Username: "alice"}}, nil
			}}
			mux, token := testRoutes(t, repo)
			r := httptest.NewRequest("GET", "/users?q="+tt.query, nil)
			if !tt.noAuth {
				r.Header.Set("Authorization", "Bearer "+token)
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if called != (tt.status == 200 || tt.failure) {
				t.Fatal("unexpected query execution")
			}
			if tt.status == 200 {
				if strings.Contains(w.Body.String(), "email") || strings.Contains(w.Body.String(), "password") {
					t.Fatal("private profile exposed")
				}
				if tt.empty && strings.TrimSpace(w.Body.String()) != "[]" {
					t.Fatal("empty results must be []")
				}
				if !tt.empty && !strings.Contains(w.Body.String(), `"username":"alice"`) {
					t.Fatal(w.Body.String())
				}
			}
		})
	}
}

func TestDirectRoomDisplayName(t *testing.T) {
	user := testUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	room := db.Room{ID: testUUID(t, "550e8400-e29b-41d4-a716-446655440002"), Type: "dm"}
	repo := &directoryRepository{fakeRepository: &fakeRepository{
		rooms: func(context.Context, db.ListRoomsForUserParams) ([]db.Room, error) { return []db.Room{room}, nil },
	}, peers: func(_ context.Context, p db.ListDirectRoomPeersParams) ([]db.ListDirectRoomPeersRow, error) {
		if p.ViewerID != user || len(p.RoomIds) != 1 || p.RoomIds[0] != room.ID {
			t.Fatalf("params=%+v", p)
		}
		return []db.ListDirectRoomPeersRow{{RoomID: room.ID, Username: "alice"}}, nil
	}}
	mux, token := testRoutes(t, repo)
	r := httptest.NewRequest("GET", "/rooms", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"alice"`) {
		t.Fatalf("response=%d %s", w.Code, w.Body.String())
	}
}
