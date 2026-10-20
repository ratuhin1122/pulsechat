package chat

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type messagePreview struct {
	ID        pgtype.UUID `json:"id"`
	SenderID  pgtype.UUID `json:"sender_id"`
	Type      string      `json:"type"`
	Body      string      `json:"body"`
	CreatedAt time.Time   `json:"created_at"`
}

type roomSummaryResponse struct {
	roomResponse
	UnreadCount int64           `json:"unread_count"`
	LastMessage *messagePreview `json:"last_message"`
}
