package chat

import (
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/store/db"
)

type roomResponse struct {
	ID        pgtype.UUID `json:"id"`
	Type      string      `json:"type"`
	Name      string      `json:"name"`
	CreatedAt time.Time   `json:"created_at"`
}

func roomView(room db.Room) roomResponse {
	return roomResponse{room.ID, room.Type, room.Name, room.CreatedAt.Time}
}

func (h *Handler) createGroup(w http.ResponseWriter, r *http.Request) {
	user, err := requestUser(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 100 {
		http.Error(w, "name must be between 1 and 100 bytes", http.StatusBadRequest)
		return
	}
	room, err := h.repo.CreateGroupRoom(r.Context(), input.Name, user)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.respond(w, r, http.StatusCreated, roomView(room))
}

func (h *Handler) direct(w http.ResponseWriter, r *http.Request) {
	user, err := requestUser(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var input struct {
		UserID string `json:"user_id"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	var peer pgtype.UUID
	if err := peer.Scan(input.UserID); err != nil || !peer.Valid || peer == user {
		http.Error(w, "a different valid user ID is required", http.StatusBadRequest)
		return
	}
	room, err := h.repo.GetOrCreateDirectRoom(r.Context(), user, peer)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.respond(w, r, http.StatusOK, roomView(room))
}

func (h *Handler) groupRoom(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	id, ok := roomID(w, r)
	if !ok {
		return id, false
	}
	room, err := h.repo.GetRoom(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return id, false
	}
	if room.Type != "group" {
		http.Error(w, "only group rooms support joining and leaving", http.StatusForbidden)
		return id, false
	}
	return id, true
}

func (h *Handler) join(w http.ResponseWriter, r *http.Request) {
	user, err := requestUser(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	id, ok := h.groupRoom(w, r)
	if !ok {
		return
	}
	if _, err := h.repo.JoinGroupRoom(r.Context(), db.JoinGroupRoomParams{RoomID: id, UserID: user}); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) leave(w http.ResponseWriter, r *http.Request) {
	user, err := requestUser(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	id, ok := h.groupRoom(w, r)
	if !ok {
		return
	}
	if _, err := h.repo.RemoveRoomMember(r.Context(), db.RemoveRoomMemberParams{RoomID: id, UserID: user}); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
