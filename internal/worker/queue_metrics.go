package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"

	"pulsechat/internal/metrics"
)

type QueueInspector interface {
	GetQueueInfo(string) (*asynq.QueueInfo, error)
}

// RefreshQueueDepth preserves the previous sample on Redis failures, rather than reporting a false zero.
func RefreshQueueDepth(ctx context.Context, inspector QueueInspector, m *metrics.Worker, queues []string, logger *slog.Logger) {
	for _, queue := range queues {
		if ctx.Err() != nil {
			return
		}
		info, err := inspector.GetQueueInfo(queue)
		if errors.Is(err, asynq.ErrQueueNotFound) {
			m.QueueDepth.WithLabelValues(queue).Set(0)
			continue
		}
		if err != nil {
			logger.ErrorContext(ctx, "inspect queue depth", "queue", queue, "error", err)
			continue
		}
		m.QueueDepth.WithLabelValues(queue).Set(float64(info.Pending + info.Scheduled + info.Retry + info.Aggregating))
	}
}

func RunQueueMetrics(ctx context.Context, inspector QueueInspector, m *metrics.Worker, queues []string, logger *slog.Logger) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		RefreshQueueDepth(ctx, inspector, m, queues, logger)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
