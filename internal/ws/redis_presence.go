package ws

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const PresenceTTL = 2 * heartbeatInterval
const presenceUsersKey = "pulsechat:presence:users"

type RedisPresence struct {
	client *redis.Client
	script *redis.Script
}

func NewRedisPresence(client *redis.Client) *RedisPresence {
	return &RedisPresence{client: client, script: redis.NewScript(redisPresenceScript)}
}

func (p *RedisPresence) PublishesTransitions() bool { return true }

func (p *RedisPresence) apply(ctx context.Context, mode, userID, connectionID string) (int64, error) {
	keys := []string{"pulsechat:presence:user:" + userID, "pulsechat:presence:connection:" + connectionID, presenceUsersKey}
	value, err := p.script.Run(ctx, p.client, keys, mode, userID, connectionID, PresenceTTL.Milliseconds(), eventChannel).Int64()
	if err != nil {
		return 0, fmt.Errorf("redis presence %s: %w", mode, err)
	}
	return value, nil
}

func (p *RedisPresence) Connect(ctx context.Context, userID, connectionID string) (bool, error) {
	n, err := p.apply(ctx, "connect", userID, connectionID)
	return n == 1, err
}

func (p *RedisPresence) Disconnect(ctx context.Context, userID, connectionID string) (bool, error) {
	n, err := p.apply(ctx, "disconnect", userID, connectionID)
	return n == 1, err
}

func (p *RedisPresence) Refresh(ctx context.Context, userID, connectionID string) error {
	n, err := p.apply(ctx, "refresh", userID, connectionID)
	if err != nil {
		return err
	}
	if n < 0 {
		return fmt.Errorf("presence lease expired")
	}
	return nil
}

func (p *RedisPresence) Counts(ctx context.Context, userIDs []string) (map[string]int, error) {
	counts := make(map[string]int, len(userIDs))
	for _, id := range userIDs {
		n, err := p.apply(ctx, "count", id, "")
		if err != nil {
			return nil, err
		}
		counts[id] = int(n)
	}
	return counts, nil
}

// Reap expires stale indexes and publishes one offline transition even when an
// API process dies without disconnecting. Lua makes multiple reapers safe.
func (p *RedisPresence) Reap(ctx context.Context) error {
	now, err := p.client.Time(ctx).Result()
	if err != nil {
		return fmt.Errorf("redis presence time: %w", err)
	}
	users, err := p.client.ZRangeByScore(ctx, presenceUsersKey, &redis.ZRangeBy{Min: "-inf", Max: fmt.Sprint(now.UnixMilli()), Count: 128}).Result()
	if err != nil {
		return fmt.Errorf("redis presence expired users: %w", err)
	}
	_, err = p.Counts(ctx, users)
	return err
}

func (p *RedisPresence) Run(ctx context.Context, logger *slog.Logger) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reapCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := p.Reap(reapCtx)
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.ErrorContext(ctx, "reap redis presence", "error", err)
			}
		}
	}
}
