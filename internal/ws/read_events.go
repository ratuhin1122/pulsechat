package ws

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"
)

func (h *Handler) PublishRead(ctx context.Context, room, user, message pgtype.UUID) error {
	members, err := h.repo.ListRoomMemberIDs(ctx, room)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.String())
	}
	payload, err := json.Marshal(struct {
		Type      string      `json:"type"`
		RoomID    pgtype.UUID `json:"room_id"`
		UserID    pgtype.UUID `json:"user_id"`
		MessageID pgtype.UUID `json:"message_id"`
	}{"read", room, user, message})
	if err != nil {
		return err
	}
	return h.hub.PublishRoom(ctx, room.String(), ids, payload)
}
