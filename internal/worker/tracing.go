package worker

import (
	"context"
	"encoding/json"
	"io"

	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/trace"

	"pulsechat/internal/tracing"
)

func TraceTasks(telemetry *tracing.Runtime, next asynq.Handler) asynq.Handler {
	return asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) (err error) {
		var payload attachmentPayload
		_ = json.Unmarshal(task.Payload(), &payload)
		ctx = tracing.Extract(ctx, payload.TraceContext)
		ctx, span := telemetry.Start(ctx, "attachment.process", trace.WithSpanKind(trace.SpanKindConsumer))
		defer func() { tracing.End(span, err) }()
		return next.ProcessTask(ctx, task)
	})
}

func (h *AttachmentHandler) WithTracing(telemetry *tracing.Runtime) *AttachmentHandler {
	h.telemetry = telemetry
	return h
}
func (h *AttachmentHandler) scan(ctx context.Context, file io.Reader) (result ScanResult, err error) {
	ctx, span := h.telemetry.Start(ctx, "clamd.scan", trace.WithSpanKind(trace.SpanKindClient))
	defer func() {
		if err != nil && h.metrics != nil {
			h.metrics.ScanErrors.Inc()
		}
		tracing.End(span, err)
	}()
	return h.scanner.Scan(ctx, file)
}
