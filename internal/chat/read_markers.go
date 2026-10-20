package chat

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"pulsechat/internal/store/db"
)

// POST /rooms/{roomID}/read accepts {"message_id":"uuid"}.
func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) {
	user, err := requestUser(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	room, ok := roomID(w, r)
	if !ok {
		return
	}
	if err := RequireMembership(r.Context(), h.repo, room, user); err != nil {
		h.fail(w, r, err)
		return
	}
	var input struct {
		MessageID string `json:"message_id"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	var message pgtype.UUID
	if err := message.Scan(input.MessageID); err != nil || !message.Valid {
		http.Error(w, "valid message_id required", 400)
		return
	}
	marker, err := h.repo.MarkRoomRead(r.Context(), db.SetRoomReadMarkerParams{RoomID: room, UserID: user, MessageID: message})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if h.reads != nil {
		if err := h.reads.PublishRead(r.Context(), room, user, marker); err != nil {
			h.logger.ErrorContext(r.Context(), "publish read marker", "error", err)
			http.Error(w, "read marker saved; notification unavailable, retry safely", http.StatusServiceUnavailable)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
