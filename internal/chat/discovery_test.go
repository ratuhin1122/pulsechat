package chat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"pulsechat/internal/store/db"
)

type publicRepository struct {
	*fakeRepository
	list func(context.Context, db.ListPublicRoomsParams) ([]db.Room, error)
}

func (f *publicRepository) ListPublicRooms(ctx context.Context, p db.ListPublicRoomsParams) ([]db.Room, error) {
	return f.list(ctx, p)
}

func TestPublicRooms(t *testing.T) {
	for _, tt := range []struct {
		name, query    string
		status         int
		failure, empty bool
	}{
		{"anonymous search", "q=team&limit=10&offset=20", 200, false, false},
		{"browse", "", 200, false, false},
		{"empty", "q=missing", 200, false, true},
		{"bad limit", "limit=101", 400, false, false},
		{"bad offset", "offset=-1", 400, false, false},
		{"long search", "q=" + strings.Repeat("x", 101), 400, false, false},
		{"database failure", "", 500, true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			repo := &publicRepository{fakeRepository: &fakeRepository{}, list: func(ctx context.Context, p db.ListPublicRoomsParams) ([]db.Room, error) {
				called = true
				want, _ := url.ParseQuery(tt.query)
				if p.Search != want.Get("q") || ctx == nil {
					t.Fatalf("params=%+v", p)
				}
				if tt.name == "anonymous search" && (p.PageLimit != 10 || p.PageOffset != 20) {
					t.Fatalf("pagination=%+v", p)
				}
				if tt.failure {
					return nil, errors.New("database")
				}
				if tt.empty {
					return nil, nil
				}
				return []db.Room{{Type: "group", Name: "team", IsPublic: true}}, nil
			}}
			mux, _ := testRoutes(t, repo)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("GET", "/rooms/public?"+tt.query, nil))
			if w.Code != tt.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if called != (tt.status != 400) {
				t.Fatal("unexpected repository query")
			}
			if tt.empty && strings.TrimSpace(w.Body.String()) != "[]" {
				t.Fatal(w.Body.String())
			}
		})
	}
}

func TestUserQDisplayName(t *testing.T) {
	self := testUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	repo := &directoryRepository{fakeRepository: &fakeRepository{}, search: func(_ context.Context, p db.SearchUsersParams) ([]db.SearchUsersRow, error) {
		if p.ViewerID != self || p.Search != "Ali" {
			t.Fatalf("params=%+v", p)
		}
		return []db.SearchUsersRow{{ID: testUUID(t, "550e8400-e29b-41d4-a716-446655440001"), Username: "alice", DisplayName: "Alice Cooper"}}, nil
	}}
	mux, token := testRoutes(t, repo)
	r := httptest.NewRequest("GET", "/users?q=Ali&search=ignored", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	var users []map[string]any
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &users) != nil || len(users) != 1 || users[0]["display_name"] != "Alice Cooper" || len(users[0]) != 3 {
		t.Fatal(w.Body.String())
	}
}

type summaryRepository struct {
	*fakeRepository
	summary db.ListRoomSummariesRow
}

func (f *summaryRepository) ListRoomSummaries(_ context.Context, p db.ListRoomSummariesParams) ([]db.ListRoomSummariesRow, error) {
	return []db.ListRoomSummariesRow{f.summary}, nil
}

func TestRoomSummaries(t *testing.T) {
	room := testUUID(t, "550e8400-e29b-41d4-a716-446655440001")
	for _, last := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "preview"}[last], func(t *testing.T) {
			summary := db.ListRoomSummariesRow{RoomID: room, UnreadCount: 3}
			if last {
				summary.LastMessageID = room
				summary.LastType = "text"
				summary.LastBody = "hello"
				summary.LastSenderID = room
			} else {
				summary.UnreadCount = 0
			}
			repo := &summaryRepository{fakeRepository: &fakeRepository{rooms: func(context.Context, db.ListRoomsForUserParams) ([]db.Room, error) {
				return []db.Room{{ID: room, Type: "group"}}, nil
			}}, summary: summary}
			mux, token := testRoutes(t, repo)
			r := httptest.NewRequest("GET", "/rooms", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			var body []struct {
				Unread int             `json:"unread_count"`
				Last   *messagePreview `json:"last_message"`
			}
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body) != 1 || body[0].Unread != int(summary.UnreadCount) || (body[0].Last != nil) != last {
				t.Fatal(w.Body.String())
			}
			if last && body[0].Last.Body != "hello" {
				t.Fatal("missing preview")
			}
		})
	}
}

type publicCreateRepository struct {
	*fakeRepository
	public bool
}

func (f *publicCreateRepository) CreateGroupRoom(_ context.Context, name string, _ pgtype.UUID, public bool) (db.Room, error) {
	f.public = public
	return db.Room{Type: "group", Name: name, IsPublic: public}, nil
}
func TestCreatePublicRoom(t *testing.T) {
	repo := &publicCreateRepository{fakeRepository: &fakeRepository{}}
	mux, token := testRoutes(t, repo)
	r := httptest.NewRequest("POST", "/rooms", strings.NewReader(`{"name":"team","is_public":true}`))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 201 || !repo.public || !strings.Contains(w.Body.String(), `"is_public":true`) {
		t.Fatal(w.Body.String())
	}
}
