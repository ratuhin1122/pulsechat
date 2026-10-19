package auth

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"pulsechat/internal/store/db"
)

type credentials struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if err := decode(w, r, &input); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	input.Username = strings.TrimSpace(input.Username)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	address, err := mail.ParseAddress(input.Email)
	if err != nil || address.Address != input.Email || len(input.Email) > 254 || len(input.Username) < 1 || len(input.Username) > 64 {
		http.Error(w, "invalid username or email", http.StatusBadRequest)
		return
	}
	if len(input.Password) < 8 || len(input.Password) > 1024 {
		http.Error(w, "password must be between 8 and 1024 bytes", http.StatusBadRequest)
		return
	}
	hash, err := HashPassword(input.Password)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	user, err := h.repo.CreateUser(r.Context(), db.CreateUserParams{Username: input.Username, Email: input.Email, PasswordHash: hash})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			http.Error(w, "username or email already exists", http.StatusConflict)
			return
		}
		h.fail(w, r, err)
		return
	}
	h.startSession(w, r, user.ID, http.StatusCreated)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(w, r, &input); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	user, err := h.repo.GetUserByEmail(r.Context(), strings.ToLower(strings.TrimSpace(input.Email)))
	if errors.Is(err, pgx.ErrNoRows) {
		// Match the password work performed for an existing account.
		_, hashErr := HashPassword("invalid-account-password")
		if hashErr != nil {
			h.fail(w, r, hashErr)
			return
		}
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if !VerifyPassword(input.Password, user.PasswordHash) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	h.startSession(w, r, user.ID, http.StatusOK)
}
