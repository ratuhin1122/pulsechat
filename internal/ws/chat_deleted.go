package ws

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"
)

// PublishChatDeleted updates only this account's tabs, including other API instances.
func (h *Handler) PublishChatDeleted(ctx context.Context, user, room pgtype.UUID) error {
	payload, err := json.Marshal(struct {
		Type   string      `json:"type"`
		RoomID pgtype.UUID `json:"room_id"`
	}{"chat_deleted", room})
	if err != nil {
		return err
	}
	return h.hub.PublishToUsers(ctx, []string{user.String()}, payload)
}
