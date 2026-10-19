package ws

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/auth"
	"pulsechat/internal/chat"
)

type presenceStatus struct {
	UserID string `json:"user_id"`
	Online bool   `json:"online"`
}

func (h *Handler) listPresence(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var room, user pgtype.UUID
	if err := room.Scan(r.URL.Query().Get("room_id")); err != nil || !room.Valid {
		http.Error(w, "valid room_id is required", http.StatusBadRequest)
		return
	}
	userID, _ := auth.UserID(r.Context())
	if err := user.Scan(userID); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := chat.RequireMembership(r.Context(), h.repo, room, user); err != nil {
		if errors.Is(err, chat.ErrNotMember) {
			http.Error(w, "room membership required", http.StatusForbidden)
		} else {
			h.presenceFailure(w, r, err)
		}
		return
	}
	members, err := h.repo.ListRoomMemberIDs(r.Context(), room)
	if err != nil {
		h.presenceFailure(w, r, err)
		return
	}
	ids := make([]string, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.String())
	}
	counts, err := h.presence.Counts(r.Context(), ids)
	if err != nil {
		h.presenceFailure(w, r, err)
		return
	}
	result := make([]presenceStatus, 0, len(ids))
	for _, id := range ids {
		result = append(result, presenceStatus{UserID: id, Online: counts[id] > 0})
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		h.logger.ErrorContext(r.Context(), "write presence response", "error", err)
	}
}

func (h *Handler) presenceFailure(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.ErrorContext(r.Context(), "presence request failed", "error", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
