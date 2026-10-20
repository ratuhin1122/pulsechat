package chat

import (
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/store/db"
)

type messageResponse struct {
	ID          pgtype.UUID          `json:"id"`
	RoomID      pgtype.UUID          `json:"room_id"`
	SenderID    pgtype.UUID          `json:"sender_id"`
	Type        string               `json:"type"`
	Body        string               `json:"body"`
	CreatedAt   time.Time            `json:"created_at"`
	Attachments []AttachmentResponse `json:"attachments,omitempty"`
}

// History is newest first. The final message ID is the next page's before cursor.
func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
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
	params := db.ListRoomHistoryParams{RoomID: room, UserID: user, PageLimit: 50}
	query := r.URL.Query()
	if values, ok := query["limit"]; ok {
		limit, err := strconv.ParseInt(values[0], 10, 32)
		if len(values) != 1 || err != nil || limit < 1 || limit > 50 {
			http.Error(w, "limit must be between 1 and 50", http.StatusBadRequest)
			return
		}
		params.PageLimit = int32(limit)
	}
	if values, ok := query["before"]; ok {
		var id pgtype.UUID
		if len(values) != 1 || id.Scan(values[0]) != nil || !id.Valid {
			http.Error(w, "invalid before cursor", http.StatusBadRequest)
			return
		}
		cursor, err := h.repo.GetRoomMessage(r.Context(), db.GetRoomMessageParams{RoomID: room, ID: id})
		if err != nil {
			h.fail(w, r, err)
			return
		}
		params.BeforeCreatedAt, params.BeforeID = cursor.CreatedAt, cursor.ID
	}
	messages, err := h.repo.ListRoomHistory(r.Context(), params)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	result := make([]messageResponse, 0, len(messages))
	for _, message := range messages {
		response := messageResponse{ID: message.ID, RoomID: message.RoomID, SenderID: message.SenderID, Type: message.Type, Body: message.Body, CreatedAt: message.CreatedAt.Time}
		if message.Type != "text" && message.Type != "" {
			rows, err := h.repo.ListAttachmentsForMessage(r.Context(), message.ID)
			if err != nil {
				h.fail(w, r, err)
				return
			}
			for _, a := range rows {
				response.Attachments = append(response.Attachments, Attachment(a))
			}
		}
		result = append(result, response)
	}
	w.Header().Set("Cache-Control", "no-store")
	h.respond(w, r, http.StatusOK, result)
}
