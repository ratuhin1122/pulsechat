package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"pulsechat/internal/storage"
	"pulsechat/internal/store/db"
)

func (h *AttachmentHandler) reject(ctx context.Context, attachment db.Attachment, reason string) error {
	if stats, ok := ctx.Value(processingKey{}).(*processingStats); ok && stats.result != "error" {
		stats.result = "rejected"
		if strings.HasPrefix(reason, "malware detected:") {
			stats.result = "infected"
		}
	}
	// Record the denial even if the scan failed because the task timed out.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if len(reason) > 1024 {
		reason = reason[:1024]
	}
	reason = strings.ToValidUTF8(reason, "?")
	_, rejectErr := h.repo.RejectAttachment(ctx, db.RejectAttachmentParams{ID: attachment.ID, Reason: reason})
	deleteErr := h.storage.Delete(ctx, storage.Quarantine, attachment.StoredName)
	if rejectErr != nil || deleteErr != nil {
		return fmt.Errorf("reject attachment (%s): %w", reason, errors.Join(rejectErr, deleteErr))
	}
	h.logger.WarnContext(ctx, "attachment rejected", "attachment_id", attachment.ID.String(), "reason", reason)
	return nil
}

// Retry cleanup without rescanning or replacing the recorded rejection reason.
func (h *AttachmentHandler) deleteRejected(ctx context.Context, attachment db.Attachment) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := h.storage.Delete(ctx, storage.Quarantine, attachment.StoredName); err != nil {
		return fmt.Errorf("delete rejected attachment: %w", err)
	}
	return nil
}
