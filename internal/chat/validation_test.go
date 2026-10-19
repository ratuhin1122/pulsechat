package chat

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"pulsechat/internal/store/db"
)

func TestRoomRoutesRequireAuth(t *testing.T) {
	mux, _ := testRoutes(t, &fakeRepository{})
	for _, tt := range []struct{ method, path string }{
		{"POST", "/rooms"}, {"POST", "/rooms/dm"}, {"GET", "/rooms"},
		{"POST", "/rooms/id/join"}, {"POST", "/rooms/id/leave"}, {"GET", "/rooms/id/members"},
	} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			for _, header := range []string{"", "Bearer invalid"} {
				r := httptest.NewRequest(tt.method, tt.path, nil)
				r.Header.Set("Authorization", header)
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				if w.Code != 401 {
					t.Errorf("status = %d", w.Code)
				}
			}
		})
	}
}

func TestInvalidRoomRequests(t *testing.T) {
	mux, token := testRoutes(t, &fakeRepository{})
	for _, tt := range []struct{ name, method, path, body string }{
		{"blank name", "POST", "/rooms", `{"name":" "}`},
		{"long name", "POST", "/rooms", `{"name":"` + strings.Repeat("x", 101) + `"}`},
		{"invalid JSON", "POST", "/rooms", `{`},
		{"extra JSON", "POST", "/rooms", `{"name":"team"} {}`},
		{"unknown field", "POST", "/rooms", `{"name":"team","type":"dm"}`},
		{"oversized body", "POST", "/rooms", `{"name":"` + strings.Repeat("x", 5000) + `"}`},
		{"invalid peer", "POST", "/rooms/dm", `{"user_id":"bad"}`},
		{"missing peer", "POST", "/rooms/dm", `{}`},
		{"self DM", "POST", "/rooms/dm", `{"user_id":"550e8400-e29b-41d4-a716-446655440000"}`},
		{"bad join ID", "POST", "/rooms/bad/join", ""},
		{"bad leave ID", "POST", "/rooms/bad/leave", ""},
		{"bad members ID", "GET", "/rooms/bad/members", ""},
		{"zero limit", "GET", "/rooms?limit=0", ""},
		{"large limit", "GET", "/rooms?limit=101", ""},
		{"bad limit", "GET", "/rooms?limit=x", ""},
		{"negative offset", "GET", "/rooms?offset=-1", ""},
		{"overflow offset", "GET", "/rooms?offset=2147483648", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != 400 {
				t.Errorf("status = %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestRequireMembership(t *testing.T) {
	id := testUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	backendErr := errors.New("database unavailable")
	for _, tt := range []struct {
		name          string
		backend, want error
	}{
		{"member", nil, nil}, {"absent", pgx.ErrNoRows, ErrNotMember}, {"failure", backendErr, backendErr},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			repo := &fakeRepository{member: func(got context.Context, p db.GetRoomMemberParams) (db.RoomMember, error) {
				if got != ctx || p.RoomID != id || p.UserID != id {
					t.Fatal("membership arguments not forwarded")
				}
				return db.RoomMember{}, tt.backend
			}}
			if err := RequireMembership(ctx, repo, id, id); !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
		})
	}
}
