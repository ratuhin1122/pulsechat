package auth

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgtype"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"pulsechat/internal/storage"
	"pulsechat/internal/store/db"
	"strings"
	"testing"
)

type accountFake struct {
	user            db.User
	member, deleted bool
	err             error
	room, caller    pgtype.UUID
}

func (f *accountFake) GetUser(context.Context, pgtype.UUID) (db.User, error) { return f.user, nil }
func (f *accountFake) ClearChat(_ context.Context, room, user pgtype.UUID) (bool, error) {
	f.room = room
	f.caller = user
	return f.member, f.err
}
func (f *accountFake) DeleteAccount(_ context.Context, user pgtype.UUID, _ storage.Storage) error {
	f.deleted = true
	f.caller = user
	return f.err
}
func TestAccountRoutes(t *testing.T) {
	var user, room pgtype.UUID
	_ = user.Scan("550e8400-e29b-41d4-a716-446655440000")
	_ = room.Scan("550e8400-e29b-41d4-a716-446655440001")
	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, method, path, body string
		authenticated, member    bool
		err                      error
		status                   int
		deleted                  bool
	}{
		{"unauthorized", "DELETE", "/auth/account", `{"password":"password123"}`, false, true, nil, 401, false},
		{"bad password", "DELETE", "/auth/account", `{"password":"wrong"}`, true, true, nil, 403, false},
		{"malformed", "DELETE", "/auth/account", `{`, true, true, nil, 400, false},
		{"delete account", "DELETE", "/auth/account", `{"password":"password123"}`, true, true, nil, 204, true},
		{"storage failure", "DELETE", "/auth/account", `{"password":"password123"}`, true, true, errors.New("failed"), 500, true},
		{"clear member chat", "DELETE", "/rooms/" + room.String(), "", true, true, nil, 204, false},
		{"reject nonmember", "DELETE", "/rooms/" + room.String(), "", true, false, nil, 403, false},
		{"invalid room", "DELETE", "/rooms/nope", "", true, true, nil, 400, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &accountFake{user: db.User{ID: user, PasswordHash: hash}, member: tt.member, err: tt.err}
			tokens, _ := NewTokens(strings.Repeat("s", 32))
			raw, _ := tokens.Issue(user.String())
			mux := http.NewServeMux()
			NewAccountHandler(repo, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), false).RegisterRoutes(mux, tokens)
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.authenticated {
				req.Header.Set("Authorization", "Bearer "+raw)
			}
			res := httptest.NewRecorder()
			mux.ServeHTTP(res, req)
			if res.Code != tt.status || repo.deleted != tt.deleted {
				t.Fatalf("status=%d deleted=%v", res.Code, repo.deleted)
			}
			if tt.deleted && tt.status == 204 {
				cookies := res.Result().Cookies()
				if len(cookies) != 1 || cookies[0].MaxAge != -1 || !cookies[0].HttpOnly || !cookies[0].Secure {
					t.Fatal("refresh cookie not cleared securely")
				}
			}
			if tt.status == 204 && repo.caller != user {
				t.Fatal("wrong user used")
			}
		})
	}
}
func TestDeletedAccountAccess(t *testing.T) {
	tokens, _ := NewTokens(strings.Repeat("s", 32))
	raw, _ := tokens.Issue("550e8400-e29b-41d4-a716-446655440000")
	for _, exists := range []bool{true, false} {
		t.Run(map[bool]string{true: "existing", false: "deleted"}[exists], func(t *testing.T) {
			tokens.WithUserLookup(func(context.Context, string) (bool, error) { return exists, nil })
			_, err := tokens.ValidateContext(context.Background(), raw)
			if (err == nil) != exists {
				t.Fatalf("exists=%v err=%v", exists, err)
			}
		})
	}
}
