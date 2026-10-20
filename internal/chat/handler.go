package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/auth"
	"pulsechat/internal/store/db"
)

type Repository interface {
	MembershipReader
	SearchUsers(context.Context, db.SearchUsersParams) ([]db.SearchUsersRow, error)
	GetUser(context.Context, pgtype.UUID) (db.User, error)
	ListDirectRoomPeers(context.Context, db.ListDirectRoomPeersParams) ([]db.ListDirectRoomPeersRow, error)
	CreateGroupRoom(context.Context, string, pgtype.UUID, bool) (db.Room, error)
	ListPublicRooms(context.Context, db.ListPublicRoomsParams) ([]db.Room, error)
	ListRoomSummaries(context.Context, db.ListRoomSummariesParams) ([]db.ListRoomSummariesRow, error)
	MarkRoomRead(context.Context, db.SetRoomReadMarkerParams) (pgtype.UUID, error)
	GetOrCreateDirectRoom(context.Context, pgtype.UUID, pgtype.UUID) (db.Room, error)
	GetRoom(context.Context, pgtype.UUID) (db.Room, error)
	JoinGroupRoom(context.Context, db.JoinGroupRoomParams) (int64, error)
	RemoveRoomMember(context.Context, db.RemoveRoomMemberParams) (int64, error)
	ListRoomsForUser(context.Context, db.ListRoomsForUserParams) ([]db.Room, error)
	ListRoomMembers(context.Context, db.ListRoomMembersParams) ([]db.RoomMember, error)
	GetRoomMessage(context.Context, db.GetRoomMessageParams) (db.Message, error)
	ListRoomHistory(context.Context, db.ListRoomHistoryParams) ([]db.Message, error)
	ListAttachmentsForMessage(context.Context, pgtype.UUID) ([]db.Attachment, error)
}

type Handler struct {
	repo   Repository
	logger *slog.Logger
	reads  ReadPublisher
}

type ReadPublisher interface {
	PublishRead(context.Context, pgtype.UUID, pgtype.UUID, pgtype.UUID) error
}

func (h *Handler) WithReadPublisher(p ReadPublisher) *Handler { h.reads = p; return h }

func New(repo Repository, logger *slog.Logger) *Handler {
	return &Handler{repo: repo, logger: logger}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, tokens *auth.Tokens) {
	mux.Handle("GET /users", tokens.Middleware(http.HandlerFunc(h.searchUsers)))
	mux.HandleFunc("GET /rooms/public", h.publicRooms)
	mux.Handle("POST /rooms/{roomID}/read", tokens.Middleware(http.HandlerFunc(h.markRead)))
	mux.Handle("POST /rooms", tokens.Middleware(http.HandlerFunc(h.createGroup)))
	mux.Handle("POST /rooms/{roomID}/join", tokens.Middleware(http.HandlerFunc(h.join)))
	mux.Handle("POST /rooms/{roomID}/leave", tokens.Middleware(http.HandlerFunc(h.leave)))
	mux.Handle("POST /rooms/dm", tokens.Middleware(http.HandlerFunc(h.direct)))
	mux.Handle("GET /rooms", tokens.Middleware(http.HandlerFunc(h.listRooms)))
	mux.Handle("GET /rooms/{roomID}/members", tokens.Middleware(http.HandlerFunc(h.listMembers)))
	mux.Handle("GET /rooms/{roomID}/messages", tokens.Middleware(http.HandlerFunc(h.history)))
}

func requestUser(r *http.Request) (pgtype.UUID, error) {
	var id pgtype.UUID
	value, ok := auth.UserID(r.Context())
	if !ok {
		return id, errors.New("missing authenticated user")
	}
	err := id.Scan(value)
	return id, err
}

func roomID(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(r.PathValue("roomID")); err != nil || !id.Valid {
		http.Error(w, "invalid room ID", http.StatusBadRequest)
		return id, false
	}
	return id, true
}

func decodeBody(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return false
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		http.Error(w, "expected one JSON object", http.StatusBadRequest)
		return false
	}
	return true
}

func (h *Handler) respond(w http.ResponseWriter, r *http.Request, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		h.logger.ErrorContext(r.Context(), "write room response", "error", err)
	}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotMember):
		http.Error(w, "room membership required", http.StatusForbidden)
	case errors.Is(err, pgx.ErrNoRows):
		http.Error(w, "room or user not found", http.StatusNotFound)
	default:
		h.logger.ErrorContext(r.Context(), "room request failed", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
