package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/chat"
	"pulsechat/internal/store/db"
)

type incomingMessage struct {
	Type   string `json:"type"`
	RoomID string `json:"room_id"`
	Body   string `json:"body"`
}

type outgoingMessage struct {
	Type      string      `json:"type"`
	ID        pgtype.UUID `json:"id"`
	RoomID    pgtype.UUID `json:"room_id"`
	SenderID  pgtype.UUID `json:"sender_id"`
	Body      string      `json:"body"`
	CreatedAt time.Time   `json:"created_at"`
}

func parseMessage(data []byte) (incomingMessage, pgtype.UUID, error) {
	var input incomingMessage
	var room pgtype.UUID
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&input); err != nil {
		return input, room, err
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return input, room, errors.New("expected one JSON object")
	}
	if !utf8.Valid(data) {
		return input, room, errors.New("invalid UTF-8")
	}
	if input.Type == "ping" && input.RoomID == "" && input.Body == "" {
		return input, room, nil
	}
	if input.Type != "message" || strings.TrimSpace(input.Body) == "" || len(input.Body) > MaxBodySize {
		return input, room, errors.New("invalid message")
	}
	if err := room.Scan(input.RoomID); err != nil || !room.Valid {
		return input, room, errors.New("invalid room ID")
	}
	return input, room, nil
}

func (h *Handler) readLoop(ctx context.Context, conn *websocket.Conn, c *client, raw string) {
	var user pgtype.UUID
	if err := user.Scan(c.userID); err != nil {
		return
	}
	for {
		kind, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if _, err := h.tokens.Validate(raw); err != nil {
			conn.Close(websocket.StatusPolicyViolation, "access token expired")
			return
		}
		input, room, err := parseMessage(data)
		if kind != websocket.MessageText || err != nil {
			conn.Close(websocket.StatusPolicyViolation, "invalid message")
			return
		}
		if input.Type == "ping" {
			if !h.hub.send(c, []byte(`{"type":"pong"}`)) {
				return
			}
			continue
		}
		if err := h.persistAndBroadcast(ctx, room, user, input.Body); err != nil {
			if errors.Is(err, chat.ErrNotMember) || errors.Is(err, pgx.ErrNoRows) {
				conn.Close(websocket.StatusPolicyViolation, "room membership required")
			} else {
				h.logger.ErrorContext(ctx, "process websocket message", "error", err)
				conn.Close(websocket.StatusInternalError, "message could not be delivered")
			}
			return
		}
	}
}

func (h *Handler) persistAndBroadcast(ctx context.Context, room, user pgtype.UUID, body string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := chat.RequireMembership(ctx, h.repo, room, user); err != nil {
		return err
	}
	// The insert also checks membership, covering a leave after the helper check.
	message, err := h.repo.CreateMemberMessage(ctx, db.CreateMemberMessageParams{RoomID: room, SenderID: user, Body: body})
	if err != nil {
		return err
	}
	members, err := h.repo.ListRoomMemberIDs(ctx, room)
	if err != nil {
		return err
	}
	data, err := json.Marshal(outgoingMessage{Type: "message", ID: message.ID, RoomID: message.RoomID,
		SenderID: message.SenderID, Body: message.Body, CreatedAt: message.CreatedAt.Time})
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.String())
	}
	h.hub.Broadcast(room.String(), ids, data)
	return nil
}
