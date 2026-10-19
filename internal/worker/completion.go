package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"pulsechat/internal/chat"
	"pulsechat/internal/store/db"
	"pulsechat/internal/ws"
)

type CompletionRepository interface {
	FinalizeAttachment(context.Context, pgtype.UUID) (db.Attachment, db.Message, error)
	ListRoomMemberIDs(context.Context, pgtype.UUID) ([]pgtype.UUID, error)
	MarkAttachmentNotified(context.Context, pgtype.UUID) error
}

func (h *AttachmentHandler) ConfigureCompletion(repo CompletionRepository, publisher ws.Publisher) {
	h.completion, h.publisher = repo, publisher
}

func (h *AttachmentHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	if err := h.processTask(ctx, task); err != nil {
		return err
	}
	if h.completion == nil {
		return nil
	}
	var payload attachmentPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	var id pgtype.UUID
	if err := id.Scan(payload.AttachmentID); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return h.finish(ctx, id)
}

func (h *AttachmentHandler) finish(ctx context.Context, id pgtype.UUID) error {
	if h.publisher == nil {
		return fmt.Errorf("attachment event publisher is not configured")
	}
	a, message, err := h.completion.FinalizeAttachment(ctx, id)
	if err != nil {
		return fmt.Errorf("finalize attachment: %w", err)
	}
	if a.NotifiedAt.Valid {
		return nil
	}
	status, err := json.Marshal(struct {
		Type         string      `json:"type"`
		AttachmentID pgtype.UUID `json:"attachment_id"`
		Status       string      `json:"status"`
	}{"attachment_status", a.ID, a.Status})
	if err != nil {
		return err
	}
	if err := h.publisher.Publish(ctx, ws.Event{UserIDs: []string{a.SenderID.String()}, Payload: status}); err != nil {
		return err
	}
	if a.Status == "ready" {
		members, err := h.completion.ListRoomMemberIDs(ctx, a.RoomID)
		if err != nil {
			return err
		}
		ids := make([]string, 0, len(members))
		for _, member := range members {
			ids = append(ids, member.String())
		}
		data, err := ws.MarshalAttachmentMessage(message, []chat.AttachmentResponse{chat.Attachment(a)})
		if err != nil {
			return err
		}
		if err := h.publisher.Publish(ctx, ws.Event{RoomID: a.RoomID.String(), UserIDs: ids, Payload: data}); err != nil {
			return err
		}
	}
	return h.completion.MarkAttachmentNotified(ctx, a.ID)
}
