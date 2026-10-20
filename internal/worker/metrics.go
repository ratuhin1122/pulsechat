package worker

import (
	"context"
	"time"

	"pulsechat/internal/metrics"
)

type processingKey struct{}
type processingStats struct {
	kind, result string
	skip         bool
}

// WithMetrics must be configured before workers start processing tasks.
func (h *AttachmentHandler) WithMetrics(m *metrics.Worker) *AttachmentHandler {
	h.metrics = m
	return h
}

func (h *AttachmentHandler) observeProcessing(ctx context.Context) (context.Context, *processingStats, func(error)) {
	stats := &processingStats{kind: "unknown", result: "ready"}
	started := time.Now()
	if h.metrics == nil {
		return ctx, stats, func(error) {}
	}
	ctx = context.WithValue(ctx, processingKey{}, stats)
	return ctx, stats, func(err error) {
		if stats.skip {
			return
		}
		if err != nil {
			stats.result = "error"
		}
		h.metrics.Processed.WithLabelValues(stats.kind, stats.result).Inc()
		h.metrics.Processing.WithLabelValues(stats.kind).Observe(time.Since(started).Seconds())
	}
}

func setProcessingResult(ctx context.Context, result string) {
	if stats, ok := ctx.Value(processingKey{}).(*processingStats); ok {
		stats.result = result
	}
}

func (h *AttachmentHandler) processedBytes(size int64) {
	if h.metrics != nil && size > 0 {
		h.metrics.Bytes.WithLabelValues("processed").Add(float64(size))
	}
}
