package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"

	"pulsechat/internal/config"
	"pulsechat/internal/metrics"
)

func TestMetricsBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	err = serveWorker(context.Background(), config.Config{WorkerMetricsAddr: listener.Addr().String()}, nil, nil, nil, metrics.NewWorker(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "listen worker metrics") {
		t.Fatalf("expected listener error before worker starts,got %v", err)
	}
}
