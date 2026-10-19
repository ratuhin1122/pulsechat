package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/store/db"
)

type Repository interface {
	GetAttachment(context.Context, pgtype.UUID) (db.Attachment, error)
	MarkAttachmentReady(context.Context, pgtype.UUID) (int64, error)
}

type AttachmentHandler struct {
	repo   Repository
	logger *slog.Logger
}

func NewAttachmentHandler(repo Repository, logger *slog.Logger) *AttachmentHandler {
	return &AttachmentHandler{repo: repo, logger: logger}
}

func (h *AttachmentHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload attachmentPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("decode attachment task: %v: %w", err, asynq.SkipRetry)
	}
	var id pgtype.UUID
	if err := id.Scan(payload.AttachmentID); err != nil || !id.Valid {
		return fmt.Errorf("invalid attachment task ID: %w", asynq.SkipRetry)
	}
	attachment, err := h.repo.GetAttachment(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("attachment not found: %w", asynq.SkipRetry)
	}
	if err != nil {
		return fmt.Errorf("load attachment: %w", err)
	}
	if attachment.Status == "ready" || attachment.Status == "rejected" {
		return nil
	}
	if attachment.Status != "pending" {
		return fmt.Errorf("invalid attachment status %q: %w", attachment.Status, asynq.SkipRetry)
	}
	updated, err := h.repo.MarkAttachmentReady(ctx, id)
	if err != nil {
		return fmt.Errorf("mark attachment ready: %w", err)
	}
	if updated == 0 {
		return nil
	}
	h.logger.InfoContext(ctx, "attachment processed", "attachment_id", id.String())
	return nil
}
