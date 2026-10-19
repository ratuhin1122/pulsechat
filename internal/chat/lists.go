package chat

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/store/db"
)

func pagination(r *http.Request) (int32, int32, error) {
	limit, offset := int64(50), int64(0)
	var err error
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, err = strconv.ParseInt(value, 10, 32)
		if err != nil || limit < 1 || limit > 100 {
			return 0, 0, fmt.Errorf("limit must be between 1 and 100")
		}
	}
	if value := r.URL.Query().Get("offset"); value != "" {
		offset, err = strconv.ParseInt(value, 10, 32)
		if err != nil || offset < 0 {
			return 0, 0, fmt.Errorf("offset must be a nonnegative integer")
		}
	}
	return int32(limit), int32(offset), nil
}

func (h *Handler) listRooms(w http.ResponseWriter, r *http.Request) {
	user, err := requestUser(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	limit, offset, err := pagination(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rooms, err := h.repo.ListRoomsForUser(r.Context(), db.ListRoomsForUserParams{UserID: user, Limit: limit, Offset: offset})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	result := make([]roomResponse, 0, len(rooms))
	for _, room := range rooms {
		result = append(result, roomView(room))
	}
	h.respond(w, r, http.StatusOK, result)
}

func (h *Handler) listMembers(w http.ResponseWriter, r *http.Request) {
	user, err := requestUser(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	id, ok := roomID(w, r)
	if !ok {
		return
	}
	if err := RequireMembership(r.Context(), h.repo, id, user); err != nil {
		h.fail(w, r, err)
		return
	}
	limit, offset, err := pagination(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	members, err := h.repo.ListRoomMembers(r.Context(), db.ListRoomMembersParams{RoomID: id, Limit: limit, Offset: offset})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	type memberResponse struct {
		UserID   pgtype.UUID `json:"user_id"`
		JoinedAt time.Time   `json:"joined_at"`
	}
	result := make([]memberResponse, 0, len(members))
	for _, member := range members {
		result = append(result, memberResponse{member.UserID, member.JoinedAt.Time})
	}
	h.respond(w, r, http.StatusOK, result)
}
