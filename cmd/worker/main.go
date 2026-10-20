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
	"pulsechat/internal/metrics"
	"pulsechat/internal/storage"
	"pulsechat/internal/store"
	"pulsechat/internal/worker"
	"pulsechat/internal/ws"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
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
	database, err := store.New(connectCtx, cfg.DatabaseURL)
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
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			logger.ErrorContext(ctx, "attachment task failed", "type", task.Type(), "error", err)
		}),
	})
	meter := metrics.NewWorker()
	mux := asynq.NewServeMux()
	handler := worker.NewAttachmentHandler(database, disk, scanner, logger, images).WithMetrics(meter)
	handler.ConfigureMedia(worker.FFProbe{}, cfg.InspectArchives)
	handler.ConfigureCompletion(database, ws.NewRedisPubSub(redis, logger))
	mux.Handle(worker.ProcessAttachment, handler)
	inspector := asynq.NewInspectorFromRedisClient(redis)
	return serveWorker(ctx, cfg, server, mux, inspector, meter, logger)
}
