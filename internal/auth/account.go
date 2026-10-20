package auth

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/storage"
	"pulsechat/internal/store/db"
)

type AccountRepository interface {
	GetUser(context.Context, pgtype.UUID) (db.User, error)
	ClearChat(context.Context, pgtype.UUID, pgtype.UUID) (bool, error)
	DeleteAccount(context.Context, pgtype.UUID, storage.Storage) error
}

type ChatDeletionPublisher interface {
	PublishChatDeleted(context.Context, pgtype.UUID, pgtype.UUID) error
}

func (h *AccountHandler) WithChatPublisher(p ChatDeletionPublisher) *AccountHandler {
	h.chats = p
	return h
}

type AccountHandler struct {
	repo      AccountRepository
	disk      storage.Storage
	logger    *slog.Logger
	localHTTP bool
	chats     ChatDeletionPublisher
}

func NewAccountHandler(repo AccountRepository, disk storage.Storage, logger *slog.Logger, localHTTP bool) *AccountHandler {
	return &AccountHandler{repo: repo, disk: disk, logger: logger, localHTTP: localHTTP}
}
func (h *AccountHandler) RegisterRoutes(mux *http.ServeMux, tokens *Tokens) {
	mux.Handle("DELETE /auth/account", tokens.Middleware(http.HandlerFunc(h.deleteAccount)))
	mux.Handle("DELETE /rooms/{roomID}", tokens.Middleware(http.HandlerFunc(h.clearChat)))
}
func accountUser(r *http.Request) pgtype.UUID {
	raw, _ := UserID(r.Context())
	var id pgtype.UUID
	_ = id.Scan(raw)
	return id
}
func (h *AccountHandler) clearChat(w http.ResponseWriter, r *http.Request) {
	var room pgtype.UUID
	if room.Scan(r.PathValue("roomID")) != nil || !room.Valid {
		http.Error(w, "invalid room ID", 400)
		return
	}
	ok, err := h.repo.ClearChat(r.Context(), room, accountUser(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if !ok {
		http.Error(w, "room membership required", 403)
		return
	}
	if h.chats != nil {
		if err := h.chats.PublishChatDeleted(r.Context(), accountUser(r), room); err != nil {
			h.logger.ErrorContext(r.Context(), "publish personal chat deletion", "error", err)
		}
	}
	w.WriteHeader(204)
}
func (h *AccountHandler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Password string `json:"password"`
	}
	if decode(w, r, &input) != nil {
		http.Error(w, "invalid JSON body", 400)
		return
	}
	user, err := h.repo.GetUser(r.Context(), accountUser(r))
	if err != nil {
		http.Error(w, "invalid account", 401)
		return
	}
	if !VerifyPassword(input.Password, user.PasswordHash) {
		http.Error(w, "password is incorrect", 403)
		return
	}
	if err = h.repo.DeleteAccount(r.Context(), user.ID, h.disk); err != nil {
		h.fail(w, r, err)
		return
	}
	cookieHandler := Handler{localHTTP: h.localHTTP}
	http.SetCookie(w, cookieHandler.refreshCookie("", time.Unix(1, 0), -1))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(204)
}
func (h *AccountHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.ErrorContext(r.Context(), "account request failed", "error", err)
	http.Error(w, "internal server error", 500)
}
