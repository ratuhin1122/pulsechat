package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/hibiken/asynq"

	"pulsechat/internal/config"
	"pulsechat/internal/metrics"
	"pulsechat/internal/worker"
)

func serveWorker(ctx context.Context, cfg config.Config, queue *asynq.Server, handler asynq.Handler, inspector worker.QueueInspector, meter *metrics.Worker, logger *slog.Logger) error {
	listener, err := net.Listen("tcp", cfg.WorkerMetricsAddr)
	if err != nil {
		return fmt.Errorf("listen worker metrics: %w", err)
	}
	defer listener.Close()
	if err := queue.Start(handler); err != nil {
		return fmt.Errorf("start worker: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", meter.Handler())
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, WriteTimeout: 10 * time.Second}
	results := make(chan error, 1)
	go func() { results <- server.Serve(listener) }()
	pollCtx, cancelPoll := context.WithCancel(ctx)
	pollDone := make(chan struct{})
	go func() {
		defer close(pollDone)
		worker.RunQueueMetrics(pollCtx, inspector, meter, []string{"default"}, logger)
	}()
	logger.InfoContext(ctx, "worker started", "metrics_addr", cfg.WorkerMetricsAddr)
	var result error
	finished := false
	select {
	case <-ctx.Done():
	case err := <-results:
		finished = true
		result = fmt.Errorf("serve worker metrics: %w", err)
	}
	// Retain metrics during task draining and stop the inspector before Redis is closed.
	queue.Shutdown()
	cancelPoll()
	<-pollDone
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		result = errors.Join(result, err, server.Close())
	}
	if !finished {
		if err := <-results; !errors.Is(err, http.ErrServerClosed) {
			result = errors.Join(result, err)
		}
	}
	return result
}
