package main

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"pulsechat/internal/auth"
	"pulsechat/internal/config"
	"pulsechat/internal/metrics"
)

func TestRealtimeSelection(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	tokens, err := auth.NewTokens(strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, url       string
		wantSubscribers int64
		wantErr         bool
	}{
		{name: "empty uses memory"},
		{name: "configured uses redis", url: "redis://" + server.Addr(), wantSubscribers: 1},
		{name: "invalid URL fails", url: "invalid://redis", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			handler, cleanup, err := newRealtime(ctx, config.Config{RedisURL: tt.url}, nil, tokens, slog.New(slog.NewTextHandler(io.Discard, nil)), metrics.New())
			if (err != nil) != tt.wantErr {
				t.Fatalf("newRealtime() error = %v", err)
			}
			if tt.wantErr {
				return
			}
			defer cleanup()
			if handler == nil {
				t.Fatal("missing handler")
			}
			counts, err := client.PubSubNumSub(ctx, "pulsechat:events").Result()
			if err != nil || counts["pulsechat:events"] != tt.wantSubscribers {
				t.Fatalf("subscribers = %v, error = %v", counts, err)
			}
		})
	}
}
