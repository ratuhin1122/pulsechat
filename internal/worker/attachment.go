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

	"pulsechat/internal/storage"
	"pulsechat/internal/store/db"
)

type Repository interface {
	GetAttachment(context.Context, pgtype.UUID) (db.Attachment, error)
	MarkAttachmentReady(context.Context, pgtype.UUID) (int64, error)
	RejectAttachment(context.Context, db.RejectAttachmentParams) (int64, error)
	CompleteImageAttachment(context.Context, db.CompleteImageAttachmentParams) (int64, error)
}

type AttachmentHandler struct {
	repo    Repository
	logger  *slog.Logger
	storage storage.Storage
	scanner Scanner
	images  ImageProcessor
}

func NewAttachmentHandler(repo Repository, disk storage.Storage, scanner Scanner, logger *slog.Logger, images ...ImageProcessor) *AttachmentHandler {
	h := &AttachmentHandler{repo: repo, storage: disk, scanner: scanner, logger: logger}
	if len(images) > 0 {
		h.images = images[0]
	}
	return h
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
	if attachment.Status == "ready" {
		if attachment.QuarantineName.Valid {
			return h.deleteOriginal(ctx, attachment.QuarantineName.String)
		}
		return nil
	}
	if attachment.Status == "rejected" {
		return h.deleteRejected(ctx, attachment)
	}
	if attachment.Status != "pending" {
		return fmt.Errorf("invalid attachment status %q: %w", attachment.Status, asynq.SkipRetry)
	}
	file, err := h.storage.Open(ctx, storage.Quarantine, attachment.StoredName)
	if err != nil {
		return h.reject(ctx, attachment, "scan failed: open quarantine: "+err.Error())
	}
	result, scanErr := h.scanner.Scan(ctx, file)
	closeErr := file.Close()
	if err := errors.Join(scanErr, closeErr); err != nil {
		return h.reject(ctx, attachment, "scan failed: "+err.Error())
	}
	if result.Threat != "" {
		return h.reject(ctx, attachment, "malware detected: "+result.Threat)
	}
	if attachment.Kind == "image" {
		return h.processImage(ctx, attachment)
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
