package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/auth"
	"pulsechat/internal/store/db"
)

type fakeRepository struct {
	Repository
	create  func(context.Context, string, pgtype.UUID) (db.Room, error)
	direct  func(context.Context, pgtype.UUID, pgtype.UUID) (db.Room, error)
	get     func(context.Context, pgtype.UUID) (db.Room, error)
	join    func(context.Context, db.JoinGroupRoomParams) (int64, error)
	leave   func(context.Context, db.RemoveRoomMemberParams) (int64, error)
	rooms   func(context.Context, db.ListRoomsForUserParams) ([]db.Room, error)
	member  func(context.Context, db.GetRoomMemberParams) (db.RoomMember, error)
	members func(context.Context, db.ListRoomMembersParams) ([]db.RoomMember, error)
}

func (f *fakeRepository) CreateGroupRoom(ctx context.Context, name string, id pgtype.UUID) (db.Room, error) {
	return f.create(ctx, name, id)
}
func (f *fakeRepository) GetOrCreateDirectRoom(ctx context.Context, user, peer pgtype.UUID) (db.Room, error) {
	return f.direct(ctx, user, peer)
}
func (f *fakeRepository) GetRoom(ctx context.Context, id pgtype.UUID) (db.Room, error) {
	return f.get(ctx, id)
}
func (f *fakeRepository) JoinGroupRoom(ctx context.Context, p db.JoinGroupRoomParams) (int64, error) {
	return f.join(ctx, p)
}
func (f *fakeRepository) RemoveRoomMember(ctx context.Context, p db.RemoveRoomMemberParams) (int64, error) {
	return f.leave(ctx, p)
}
func (f *fakeRepository) ListRoomsForUser(ctx context.Context, p db.ListRoomsForUserParams) ([]db.Room, error) {
	return f.rooms(ctx, p)
}
func (f *fakeRepository) GetRoomMember(ctx context.Context, p db.GetRoomMemberParams) (db.RoomMember, error) {
	return f.member(ctx, p)
}
func (f *fakeRepository) ListRoomMembers(ctx context.Context, p db.ListRoomMembersParams) ([]db.RoomMember, error) {
	return f.members(ctx, p)
}

func testUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		t.Fatal(err)
	}
	return id
}

func testRoutes(t *testing.T, repo Repository) (*http.ServeMux, string) {
	t.Helper()
	tokens, err := auth.NewTokens(strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := tokens.Issue("550e8400-e29b-41d4-a716-446655440000")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	New(repo, slog.New(slog.NewTextHandler(io.Discard, nil))).RegisterRoutes(mux, tokens)
	return mux, raw
}

func TestRoomHandlers(t *testing.T) {
	user := testUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	peer := testUUID(t, "550e8400-e29b-41d4-a716-446655440001")
	room := db.Room{ID: testUUID(t, "550e8400-e29b-41d4-a716-446655440002"), Type: "group", Name: "team"}
	base := "/rooms/" + room.ID.String()
	backendErr := errors.New("database unavailable")
	for _, tt := range []struct {
		name, method, path, body string
		status                   int
		err                      error
		dm, noMember, empty      bool
	}{
		{name: "create", method: "POST", path: "/rooms", body: `{"name":" team "}`, status: 201},
		{name: "create failure", method: "POST", path: "/rooms", body: `{"name":"team"}`, err: backendErr, status: 500},
		{name: "join", method: "POST", path: base + "/join", status: 204},
		{name: "join already member", method: "POST", path: base + "/join", empty: true, status: 204},
		{name: "join missing room", method: "POST", path: base + "/join", err: pgx.ErrNoRows, status: 404},
		{name: "join dm forbidden", method: "POST", path: base + "/join", dm: true, status: 403},
		{name: "leave", method: "POST", path: base + "/leave", status: 204},
		{name: "leave already absent", method: "POST", path: base + "/leave", empty: true, status: 204},
		{name: "leave dm forbidden", method: "POST", path: base + "/leave", dm: true, status: 403},
		{name: "leave failure", method: "POST", path: base + "/leave", err: backendErr, status: 500},
		{name: "direct room", method: "POST", path: "/rooms/dm", body: `{"user_id":"` + peer.String() + `"}`, status: 200},
		{name: "direct peer missing", method: "POST", path: "/rooms/dm", body: `{"user_id":"` + peer.String() + `"}`, err: pgx.ErrNoRows, status: 404},
		{name: "direct failure", method: "POST", path: "/rooms/dm", body: `{"user_id":"` + peer.String() + `"}`, err: backendErr, status: 500},
		{name: "my rooms", method: "GET", path: "/rooms?limit=10&offset=2", status: 200},
		{name: "empty rooms", method: "GET", path: "/rooms", empty: true, status: 200},
		{name: "rooms failure", method: "GET", path: "/rooms", err: backendErr, status: 500},
		{name: "members", method: "GET", path: base + "/members?limit=10&offset=2", status: 200},
		{name: "empty members page", method: "GET", path: base + "/members", empty: true, status: 200},
		{name: "members forbidden", method: "GET", path: base + "/members", noMember: true, status: 403},
		{name: "membership failure", method: "GET", path: base + "/members", err: backendErr, status: 500},
	} {
		t.Run(tt.name, func(t *testing.T) {
			checkUser := func(id pgtype.UUID) {
				t.Helper()
				if id != user {
					t.Fatalf("wrong acting user: %s", id.String())
				}
			}
			checkRoom := func(id pgtype.UUID) {
				t.Helper()
				if id != room.ID {
					t.Fatalf("wrong room: %s", id.String())
				}
			}
			checkPage := func(limit, offset int32) {
				t.Helper()
				wantLimit, wantOffset := int32(50), int32(0)
				if strings.Contains(tt.path, "limit=10") {
					wantLimit, wantOffset = 10, 2
				}
				if limit != wantLimit || offset != wantOffset {
					t.Fatalf("pagination = %d, %d", limit, offset)
				}
			}
			repo := &fakeRepository{
				create: func(ctx context.Context, name string, id pgtype.UUID) (db.Room, error) {
					checkUser(id)
					if name != "team" {
						t.Errorf("name = %q", name)
					}
					if ctx == nil {
						t.Fatal("missing context")
					}
					return room, tt.err
				},
				direct: func(_ context.Context, id, other pgtype.UUID) (db.Room, error) {
					checkUser(id)
					if other != peer {
						t.Fatal("wrong DM peer")
					}
					dm := room
					dm.Type = "dm"
					return dm, tt.err
				},
				get: func(_ context.Context, id pgtype.UUID) (db.Room, error) {
					checkRoom(id)
					result := room
					if tt.dm {
						result.Type = "dm"
					}
					return result, tt.err
				},
				join: func(_ context.Context, p db.JoinGroupRoomParams) (int64, error) {
					checkRoom(p.RoomID)
					checkUser(p.UserID)
					if tt.dm {
						t.Fatal("attempted DM join")
					}
					if tt.empty {
						return 0, nil
					}
					return 1, tt.err
				},
				leave: func(_ context.Context, p db.RemoveRoomMemberParams) (int64, error) {
					checkRoom(p.RoomID)
					checkUser(p.UserID)
					if tt.dm {
						t.Fatal("attempted DM leave")
					}
					if tt.empty {
						return 0, nil
					}
					return 1, tt.err
				},
				rooms: func(_ context.Context, p db.ListRoomsForUserParams) ([]db.Room, error) {
					checkUser(p.UserID)
					checkPage(p.Limit, p.Offset)
					if tt.empty {
						return nil, nil
					}
					return []db.Room{room}, tt.err
				},
				member: func(_ context.Context, p db.GetRoomMemberParams) (db.RoomMember, error) {
					checkRoom(p.RoomID)
					checkUser(p.UserID)
					if tt.noMember {
						return db.RoomMember{}, pgx.ErrNoRows
					}
					return db.RoomMember{RoomID: room.ID, UserID: user}, tt.err
				},
				members: func(_ context.Context, p db.ListRoomMembersParams) ([]db.RoomMember, error) {
					if tt.noMember {
						t.Fatal("listed members without membership")
					}
					checkRoom(p.RoomID)
					checkPage(p.Limit, p.Offset)
					if tt.empty {
						return nil, nil
					}
					return []db.RoomMember{{RoomID: room.ID, UserID: user}}, tt.err
				},
			}
			mux, token := testRoutes(t, repo)
			r := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.status, w.Body.String())
			}
			if tt.status == 200 || tt.status == 201 {
				if !json.Valid(w.Body.Bytes()) {
					t.Fatal("response is not JSON")
				}
				if tt.empty && strings.TrimSpace(w.Body.String()) != "[]" {
					t.Errorf("empty list = %s", w.Body.String())
				}
				if !tt.empty && !strings.Contains(w.Body.String(), room.ID.String()) && !strings.Contains(w.Body.String(), user.String()) {
					t.Fatal("response missing expected ID")
				}
			}
		})
	}
}
