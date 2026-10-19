package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/store/db"
)

type Repository interface {
	CreateUser(context.Context, db.CreateUserParams) (db.User, error)
	GetUserByEmail(context.Context, string) (db.User, error)
	CreateRefreshToken(context.Context, db.CreateRefreshTokenParams) error
	RotateRefreshToken(context.Context, db.RotateRefreshTokenParams) (pgtype.UUID, error)
	DeleteRefreshToken(context.Context, []byte) error
}

type Handler struct {
	repo   Repository
	tokens *Tokens
	logger *slog.Logger
}

func New(repo Repository, tokens *Tokens, logger *slog.Logger) *Handler {
	return &Handler{repo: repo, tokens: tokens, logger: logger}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/register", h.register)
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("POST /auth/refresh", h.refresh)
	mux.HandleFunc("POST /auth/logout", h.logout)
}

func decode(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("expected one JSON object")
	}
	return nil
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.ErrorContext(r.Context(), "authentication failed", "error", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
