package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"pulsechat/internal/auth"
	"pulsechat/internal/config"
	"pulsechat/internal/ws"
)

func newRealtime(ctx context.Context, cfg config.Config, repo ws.Repository, tokens *auth.Tokens, logger *slog.Logger) (*ws.Handler, func(), error) {
	ctx, cancel := context.WithCancel(ctx)
	var client *redis.Client
	var publisher *ws.RedisPubSub
	var presence ws.Presence = ws.NewMemoryPresence()
	var workers sync.WaitGroup
	stopSubscription := func() {}
	if cfg.RedisURL != "" {
		options, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			cancel()
			return nil, nil, fmt.Errorf("parse REDIS_URL: %w", err)
		}
		options.ContextTimeoutEnabled = true
		client = redis.NewClient(options)
		pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
		err = client.Ping(pingCtx).Err()
		pingCancel()
		if err != nil {
			cancel()
			client.Close()
			return nil, nil, fmt.Errorf("ping redis: %w", err)
		}
		publisher = ws.NewRedisPubSub(client, logger)
		presence = ws.NewRedisPresence(client)
	}
	hub := ws.NewHub()
	if publisher != nil {
		hub = ws.NewHubWithPublisher(publisher)
	}
	cleanup := func() {
		cancel()
		hub.Close()
		stopSubscription()
		workers.Wait()
		if client != nil {
			client.Close()
		}
	}
	handler, err := ws.New(ctx, repo, tokens, hub, presence, logger, cfg.WSOrigin)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	if publisher != nil {
		stopSubscription, err = publisher.Subscribe(ctx, handler.DeliverEvent)
		if err != nil {
			stopSubscription = func() {}
			cleanup()
			return nil, nil, err
		}
		redisPresence := presence.(*ws.RedisPresence)
		workers.Add(1)
		go func() { defer workers.Done(); redisPresence.Run(ctx, logger) }()
	}
	return handler, cleanup, nil
}
