package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/store/db"
)

type memoryRepository struct {
	user     db.User
	sessions map[string]db.CreateRefreshTokenParams
}

func (m *memoryRepository) CreateUser(_ context.Context, p db.CreateUserParams) (db.User, error) {
	m.user = db.User{Username: p.Username, Email: p.Email, PasswordHash: p.PasswordHash}
	if err := m.user.ID.Scan("550e8400-e29b-41d4-a716-446655440000"); err != nil {
		return db.User{}, err
	}
	return m.user, nil
}

func (m *memoryRepository) GetUserByEmail(_ context.Context, email string) (db.User, error) {
	if email != m.user.Email {
		return db.User{}, pgx.ErrNoRows
	}
	return m.user, nil
}

func (m *memoryRepository) CreateRefreshToken(_ context.Context, p db.CreateRefreshTokenParams) error {
	m.sessions[string(p.TokenHash)] = p
	return nil
}

func (m *memoryRepository) RotateRefreshToken(_ context.Context, p db.RotateRefreshTokenParams) (pgtype.UUID, error) {
	old, ok := m.sessions[string(p.OldHash)]
	if !ok || !old.ExpiresAt.Time.After(time.Now()) {
		return pgtype.UUID{}, pgx.ErrNoRows
	}
	delete(m.sessions, string(p.OldHash))
	m.sessions[string(p.NewHash)] = db.CreateRefreshTokenParams{TokenHash: p.NewHash, UserID: old.UserID, ExpiresAt: p.ExpiresAt}
	return old.UserID, nil
}

func (m *memoryRepository) DeleteRefreshToken(_ context.Context, hash []byte) error {
	delete(m.sessions, string(hash))
	return nil
}

func TestSessions(t *testing.T) {
	repo := &memoryRepository{sessions: make(map[string]db.CreateRefreshTokenParams)}
	tokens, err := NewTokens(strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	New(repo, tokens, slog.New(slog.NewTextHandler(io.Discard, nil))).RegisterRoutes(mux)
	request := func(path, body string, cookie *http.Cookie, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "https://pulsechat.test"+path, strings.NewReader(body))
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s status = %d, want %d: %s", path, w.Code, want, w.Body.String())
		}
		return w
	}
	w := request("/auth/register", `{"username":"alice","email":"Alice@example.com","password":"correct-password"}`, nil, http.StatusCreated)
	if repo.user.PasswordHash == "correct-password" || !VerifyPassword("correct-password", repo.user.PasswordHash) {
		t.Fatal("registration did not hash password")
	}
	var response struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if _, err := tokens.Validate(response.AccessToken); err != nil {
		t.Fatal(err)
	}
	if response.ExpiresIn != 900 {
		t.Fatal("incorrect access lifetime")
	}
	checkCookie := func(w *httptest.ResponseRecorder) *http.Cookie {
		t.Helper()
		cookies := w.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatal("missing refresh cookie")
		}
		c := cookies[0]
		if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/auth" {
			t.Fatalf("unsafe cookie: %+v", c)
		}
		session, ok := repo.sessions[string(refreshHash(c.Value))]
		if !ok || len(session.TokenHash) != 32 || bytes.Equal(session.TokenHash, []byte(c.Value)) {
			t.Fatal("refresh token was not stored hashed")
		}
		return c
	}
	first := checkCookie(w)
	second := checkCookie(request("/auth/refresh", "", first, http.StatusOK))
	if first.Value == second.Value {
		t.Fatal("refresh token did not rotate")
	}
	request("/auth/refresh", "", first, http.StatusUnauthorized)
	request("/auth/refresh", "", nil, http.StatusUnauthorized)
	w = request("/auth/logout", "", second, http.StatusNoContent)
	if w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout did not clear cookie")
	}
	request("/auth/refresh", "", second, http.StatusUnauthorized)
	request("/auth/logout", "", nil, http.StatusNoContent)
	request("/auth/login", `{"email":"alice@example.com","password":"wrong-password"}`, nil, http.StatusUnauthorized)
	checkCookie(request("/auth/login", `{"email":"ALICE@example.com","password":"correct-password"}`, nil, http.StatusOK))
	request("/auth/login", `{"email":"missing@example.com","password":"correct-password"}`, nil, http.StatusUnauthorized)
	for _, body := range []string{`{}`, `{"username":"a","email":"bad","password":"correct-password"}`, `{"username":"a","email":"a@example.com","password":"short"}`, `{} {}`, `{"extra":1}`} {
		request("/auth/register", body, nil, http.StatusBadRequest)
	}
}
