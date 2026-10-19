package main

import (
	"context"
	"errors"
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
	"pulsechat/internal/store"
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
	websocketHandler, closeRealtime, err := newRealtime(ctx, cfg, database, tokens, logger)
	if err != nil {
		return err
	}
	defer closeRealtime()

	mux := http.NewServeMux()
	auth.New(database, tokens, logger).RegisterRoutes(mux)
	chat.New(database, logger).RegisterRoutes(mux, tokens)
	websocketHandler.RegisterRoutes(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if _, err := w.Write([]byte("ok\n")); err != nil {
			logger.ErrorContext(r.Context(), "write health response", "error", err)
		}
	})
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	logger.InfoContext(ctx, "api starting", "addr", cfg.HTTPAddr)

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve http: %w", err)
	case <-ctx.Done():
	}
	stop()
	logger.Info("api shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		closeErr := server.Close()
		return fmt.Errorf("shutdown http: %w", errors.Join(err, closeErr))
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve http: %w", err)
	}
	return nil
}
