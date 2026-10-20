package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"pulsechat/internal/storage"
	"pulsechat/internal/store/db"
)

func (h *AttachmentHandler) processImage(ctx context.Context, a db.Attachment) error {
	if h.images == nil {
		return fmt.Errorf("image processor is not configured")
	}
	file, err := h.storage.Open(ctx, storage.Quarantine, a.StoredName)
	if err != nil {
		return fmt.Errorf("open image: %w", err)
	}
	input, readErr := io.ReadAll(io.LimitReader(file, (15<<20)+1))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return fmt.Errorf("read image: %w", err)
	}
	if len(input) > 15<<20 {
		return h.reject(ctx, a, "image exceeds 15 MiB")
	}
	image, err := h.images.Process(ctx, input)
	if errors.Is(err, ErrImageDecode) {
		return h.reject(ctx, a, "image decode failed: "+err.Error())
	}
	if err != nil {
		return fmt.Errorf("process image: %w", err)
	}
	display, err := h.storage.Save(ctx, storage.Clean, bytes.NewReader(image.Display))
	if err != nil {
		return err
	}
	thumb, err := h.storage.Save(ctx, storage.Clean, bytes.NewReader(image.Thumbnail))
	if err != nil {
		return errors.Join(err, h.deleteClean(ctx, display.Name))
	}
	updated, err := h.repo.CompleteImageAttachment(ctx, db.CompleteImageAttachmentParams{
		ID: a.ID, DisplayName: display.Name, ThumbnailName: thumb.Name,
		SizeBytes: display.Size, Width: int32(image.Width), Height: int32(image.Height), Blurhash: image.Blurhash,
	})
	// A failed commit may have succeeded remotely; retain outputs for safety.
	if err != nil {
		return fmt.Errorf("complete image attachment: %w", err)
	}
	if updated == 0 {
		return errors.Join(h.deleteClean(ctx, display.Name), h.deleteClean(ctx, thumb.Name))
	}
	h.processedBytes(display.Size + thumb.Size)
	return h.deleteOriginal(ctx, a.StoredName)
}

func (h *AttachmentHandler) deleteOriginal(ctx context.Context, name string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	return h.storage.Delete(ctx, storage.Quarantine, name)
}

func (h *AttachmentHandler) deleteClean(ctx context.Context, name string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	return h.storage.Delete(ctx, storage.Clean, name)
}
