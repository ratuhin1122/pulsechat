package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/logging"
	"pulsechat/internal/store/db"
)

const refreshTTL = 30 * 24 * time.Hour

func newRefreshToken() (string, []byte, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", nil, err
	}
	raw := base64.RawURLEncoding.EncodeToString(value[:])
	return raw, refreshHash(raw), nil
}

func refreshHash(raw string) []byte {
	hash := sha256.Sum256([]byte(raw))
	return hash[:]
}

func (h *Handler) cookieName() string {
	if h.localHTTP {
		return "pulsechat_refresh_dev"
	}
	return "__Secure-pulsechat_refresh"
}

func (h *Handler) refreshCookie(raw string, expires time.Time, maxAge int) *http.Cookie {
	return &http.Cookie{Name: h.cookieName(), Value: raw, Path: "/auth",
		HttpOnly: true, Secure: !h.localHTTP, SameSite: http.SameSiteStrictMode,
		Expires: expires, MaxAge: maxAge}
}

func (h *Handler) startSession(w http.ResponseWriter, r *http.Request, id pgtype.UUID, status int) {
	logging.Set(r.Context(), logging.Fields{UserID: id.String()})
	access, err := h.tokens.Issue(id.String())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	raw, hash, err := newRefreshToken()
	if err != nil {
		h.fail(w, r, err)
		return
	}
	expires := time.Now().Add(refreshTTL)
	err = h.repo.CreateRefreshToken(r.Context(), db.CreateRefreshTokenParams{
		TokenHash: hash, UserID: id, ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.respond(w, r, access, raw, expires, status)
}

func (h *Handler) respond(w http.ResponseWriter, r *http.Request, access, refresh string, expires time.Time, status int) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	http.SetCookie(w, h.refreshCookie(refresh, expires, int(refreshTTL.Seconds())))
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}{access, "Bearer", int(AccessTokenTTL.Seconds())}); err != nil {
		h.logger.ErrorContext(r.Context(), "write token response", "error", err)
	}
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(h.cookieName())
	if err != nil || len(cookie.Value) != 43 {
		http.Error(w, "invalid refresh token", http.StatusUnauthorized)
		return
	}
	raw, hash, err := newRefreshToken()
	if err != nil {
		h.fail(w, r, err)
		return
	}
	expires := time.Now().Add(refreshTTL)
	id, err := h.repo.RotateRefreshToken(r.Context(), db.RotateRefreshTokenParams{
		NewHash: hash, OldHash: refreshHash(cookie.Value), ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "invalid refresh token", http.StatusUnauthorized)
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	logging.Set(r.Context(), logging.Fields{UserID: id.String()})
	access, err := h.tokens.Issue(id.String())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.respond(w, r, access, raw, expires, http.StatusOK)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(h.cookieName()); err == nil {
		if err := h.repo.DeleteRefreshToken(r.Context(), refreshHash(cookie.Value)); err != nil {
			h.fail(w, r, err)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	http.SetCookie(w, h.refreshCookie("", time.Unix(1, 0), -1))
	w.WriteHeader(http.StatusNoContent)
}
