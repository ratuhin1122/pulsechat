package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pulsechat/internal/auth"
	"pulsechat/internal/chat"
	"pulsechat/internal/config"
	"pulsechat/internal/httpserver"
	"pulsechat/internal/metrics"
	"pulsechat/internal/storage"
	"pulsechat/internal/store"
	"pulsechat/internal/upload"
	"pulsechat/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("api stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	limits, err := httpserver.NewRateLimits(cfg.TrustedProxyCIDRs)
	if err != nil {
		return err
	}
	meter := metrics.New()
	monitor, err := httpserver.NewMonitor(logger, meter.Registry)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	tokens, err := auth.NewTokens(cfg.JWTSecret)
	if err != nil {
		return err
	}
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	database, err := store.New(connectCtx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer database.Close()
	websocketHandler, closeRealtime, err := newRealtime(ctx, cfg, database, tokens, logger, meter)
	if err != nil {
		return err
	}
	defer closeRealtime()
	disk, err := storage.NewLocal(cfg.StorageDir)
	if err != nil {
		return err
	}
	defer disk.Close()
	attachmentQueue, err := worker.NewQueue(cfg.RedisURL)
	if err != nil {
		return err
	}
	defer attachmentQueue.Close()
	uploadLimiter, err := upload.NewMemoryLimiter(10, time.Minute)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	auth.NewWithOptions(database, tokens, logger, auth.Options{LocalHTTP: cfg.DevHTTP}).RegisterRoutes(mux)
	chat.New(database, logger).WithReadPublisher(websocketHandler).RegisterRoutes(mux, tokens)
	websocketHandler.RegisterRoutes(mux)
	upload.New(database, disk, uploadLimiter, attachmentQueue, logger).WithMetrics(meter).RegisterRoutes(mux, tokens)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if _, err := w.Write([]byte("ok\n")); err != nil {
			logger.ErrorContext(r.Context(), "write health response", "error", err)
		}
	})
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           meter.Middleware(mux, monitor.Wrap(mux, limits.Wrap(mux))),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	metricsMux := http.NewServeMux()
	metricsMux.Handle("GET /metrics", monitor.Handler())
	internal := &http.Server{Addr: cfg.MetricsAddr, Handler: metricsMux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	logger.InfoContext(ctx, "api starting", "addr", cfg.HTTPAddr, "metrics_addr", cfg.MetricsAddr)
	return serve(ctx, cfg.ShutdownTimeout, server, internal)
}
