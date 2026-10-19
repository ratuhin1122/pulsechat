package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"pulsechat/internal/storage"
	"pulsechat/internal/store/db"
	"pulsechat/internal/upload"
)

func (h *AttachmentHandler) processMedia(ctx context.Context, a db.Attachment) error {
	limit := int64(25 << 20)
	if a.Kind == "voice" {
		limit = 10 << 20
	}
	file, err := h.storage.Open(ctx, storage.Quarantine, a.StoredName)
	if err != nil {
		return fmt.Errorf("open media: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return fmt.Errorf("read media: %w", err)
	}
	if err := upload.Recheck(a.Kind, a.OriginalName, a.MimeType, data); err != nil {
		return h.reject(ctx, a, "type re-check failed: "+err.Error())
	}
	if a.MimeType == "application/zip" || strings.HasPrefix(a.MimeType, "application/vnd.openxmlformats") {
		if err := checkZIP(ctx, data, h.inspectArchives); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return h.reject(ctx, a, "archive rejected: "+err.Error())
		}
	}
	var duration pgtype.Float8
	if a.Kind == "voice" {
		seconds, err := h.voice.Duration(ctx, data, a.MimeType)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return h.reject(ctx, a, "voice probe failed: "+err.Error())
		}
		if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds > 300 {
			return h.reject(ctx, a, "voice duration must be positive and at most 5 minutes")
		}
		duration = pgtype.Float8{Float64: seconds, Valid: true}
	}
	clean, err := h.storage.Save(ctx, storage.Clean, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("save clean media: %w", err)
	}
	updated, err := h.repo.CompleteMediaAttachment(ctx, db.CompleteMediaAttachmentParams{
		ID: a.ID, CleanName: clean.Name, SizeBytes: clean.Size, DurationSec: duration,
	})
	// Retain outputs on ambiguous database errors, as the update may have committed.
	if err != nil {
		return fmt.Errorf("complete media attachment: %w", err)
	}
	if updated == 0 {
		return h.deleteClean(ctx, clean.Name)
	}
	h.logger.InfoContext(ctx, "attachment processed", "attachment_id", a.ID.String(), "kind", a.Kind)
	return h.deleteOriginal(ctx, a.StoredName)
}
