package logging

import (
	"context"
	"io"
	"log/slog"
)

func New(output io.Writer, service string) *slog.Logger {
	return slog.New(&handler{next: slog.NewJSONHandler(output, nil)}).With("service", service)
}

type handler struct{ next slog.Handler }

func (h *handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}
func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &handler{next: h.next.WithAttrs(attrs)}
}
func (h *handler) WithGroup(name string) slog.Handler { return &handler{next: h.next.WithGroup(name)} }
func (h *handler) Handle(ctx context.Context, record slog.Record) error {
	fields := From(ctx)
	explicit := make(map[string]bool)
	record.Attrs(func(attr slog.Attr) bool { explicit[attr.Key] = true; return true })
	for _, attr := range []slog.Attr{slog.String("request_id", fields.RequestID), slog.String("user_id", fields.UserID), slog.String("room_id", fields.RoomID), slog.String("attachment_id", fields.AttachmentID)} {
		if !explicit[attr.Key] {
			record.AddAttrs(attr)
		}
	}
	return h.next.Handle(ctx, record)
}
