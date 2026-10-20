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
	result := make([]roomSummaryResponse, 0, len(rooms))
	roomIDs := make([]pgtype.UUID, 0, len(rooms))
	for _, room := range rooms {
		roomIDs = append(roomIDs, room.ID)
	}
	summaries := make(map[pgtype.UUID]db.ListRoomSummariesRow)
	if len(roomIDs) > 0 {
		rows, err := h.repo.ListRoomSummaries(r.Context(), db.ListRoomSummariesParams{UserID: user, RoomIds: roomIDs})
		if err != nil {
			h.fail(w, r, err)
			return
		}
		for _, row := range rows {
			summaries[row.RoomID] = row
		}
	}
	ids := make([]pgtype.UUID, 0)
	for _, room := range rooms {
		if room.Type == "dm" {
			ids = append(ids, room.ID)
		}
	}
	names := make(map[pgtype.UUID]string)
	if len(ids) > 0 {
		peers, err := h.repo.ListDirectRoomPeers(r.Context(), db.ListDirectRoomPeersParams{RoomIds: ids, ViewerID: user})
		if err != nil {
			h.fail(w, r, err)
			return
		}
		for _, peer := range peers {
			names[peer.RoomID] = peer.Username
		}
	}
	for _, room := range rooms {
		view := roomView(room)
		if room.Type == "dm" {
			view.Name = names[room.ID]
		}
		summary := summaries[room.ID]
		response := roomSummaryResponse{roomResponse: view, UnreadCount: summary.UnreadCount}
		if summary.LastMessageID.Valid {
			response.LastMessage = &messagePreview{ID: summary.LastMessageID, SenderID: summary.LastSenderID,
				Type: summary.LastType, Body: summary.LastBody, CreatedAt: summary.LastCreatedAt.Time}
		}
		result = append(result, response)
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
