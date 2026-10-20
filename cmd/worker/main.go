package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"

	"pulsechat/internal/config"
	"pulsechat/internal/httpserver"
	"pulsechat/internal/logging"
	"pulsechat/internal/metrics"
	"pulsechat/internal/storage"
	"pulsechat/internal/store"
	"pulsechat/internal/tracing"
	"pulsechat/internal/worker"
	"pulsechat/internal/ws"
)

func main() {
	logger := logging.New(os.Stdout, "worker")
	if err := run(logger); err != nil {
		logger.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	telemetry, err := tracing.New(ctx, cfg, "worker")
	if err != nil {
		return err
	}
	defer telemetry.Close(logger)
	redis, err := worker.NewRedisClient(cfg.RedisURL)
	if err != nil {
		return err
	}
	defer redis.Close()
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := redis.Ping(connectCtx).Err(); err != nil {
		return fmt.Errorf("connect worker redis: %w", err)
	}
	database, err := store.New(connectCtx, cfg.DatabaseURL, telemetry.PGX())
	if err != nil {
		return err
	}
	defer database.Close()
	cancel()
	disk, err := storage.NewLocal(cfg.StorageDir)
	if err != nil {
		return err
	}
	defer disk.Close()
	cleanupCtx, stopCleanup := context.WithCancel(ctx)
	cleanupDone := make(chan struct{})
	go func() { defer close(cleanupDone); worker.CleanDeletedFiles(cleanupCtx, database, disk, logger) }()
	defer func() { stopCleanup(); <-cleanupDone }()
	images, closeImages, err := worker.NewImageProcessor()
	if err != nil {
		return err
	}
	defer closeImages()
	address := cfg.ClamdAddr
	if address == "" {
		address = "localhost:3310"
	}
	scanner, err := worker.NewClamdScanner(address, 90*time.Second)
	if err != nil {
		return err
	}
	server := asynq.NewServerFromRedisClient(redis, asynq.Config{
		Concurrency:     4,
		ShutdownTimeout: cfg.ShutdownTimeout,
		Logger:          worker.Logger{Log: logger},
	})
	meter := metrics.NewWorker()
	mux := asynq.NewServeMux()
	handler := worker.NewAttachmentHandler(database, disk, scanner, logger, images).WithMetrics(meter).WithTracing(telemetry)
	handler.ConfigureMedia(worker.FFProbe{}, cfg.InspectArchives)
	handler.ConfigureCompletion(database, ws.NewRedisPubSub(redis, logger))
	mux.Handle(worker.ProcessAttachment, worker.TraceTasks(telemetry, worker.LogTasks(logger, handler)))
	inspector := asynq.NewInspectorFromRedisClient(redis)
	return serveWorker(ctx, cfg, server, mux, inspector, meter, logger,
		httpserver.DependencyCheck{Name: "postgres", Ping: database.Pool.Ping},
		httpserver.DependencyCheck{Name: "redis", Ping: func(ctx context.Context) error { return redis.Ping(ctx).Err() }},
		httpserver.DependencyCheck{Name: "clamav", Ping: scanner.Ping})
}
