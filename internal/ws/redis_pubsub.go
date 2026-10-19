package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const eventChannel = "pulsechat:events"

type RedisPubSub struct {
	client *redis.Client
	logger *slog.Logger
}

func NewRedisPubSub(client *redis.Client, logger *slog.Logger) *RedisPubSub {
	return &RedisPubSub{client: client, logger: logger}
}

func (b *RedisPubSub) Publish(ctx context.Context, event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode redis event: %w", err)
	}
	if err := b.client.Publish(ctx, eventChannel, data).Err(); err != nil {
		return fmt.Errorf("publish redis event: %w", err)
	}
	return nil
}

// Subscribe waits for the subscription acknowledgement before returning. The
// returned stop function closes the socket and joins the receive goroutine.
func (b *RedisPubSub) Subscribe(ctx context.Context, consume func(context.Context, Event) error) (func(), error) {
	ctx, cancel := context.WithCancel(ctx)
	sub := b.client.Subscribe(ctx, eventChannel)
	ackCtx, ackCancel := context.WithTimeout(ctx, 5*time.Second)
	_, err := sub.Receive(ackCtx)
	ackCancel()
	if err != nil {
		cancel()
		sub.Close()
		return nil, fmt.Errorf("subscribe redis events: %w", err)
	}
	done := make(chan struct{})
	stopOnCancel := context.AfterFunc(ctx, func() { sub.Close() })
	go func() {
		defer close(done)
		for {
			message, err := sub.ReceiveMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				b.logger.ErrorContext(ctx, "receive redis event", "error", err)
				timer := time.NewTimer(time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				continue
			}
			var event Event
			if err := json.Unmarshal([]byte(message.Payload), &event); err != nil {
				b.logger.ErrorContext(ctx, "decode redis event", "error", err)
				continue
			}
			deliveryCtx, deliveryCancel := context.WithTimeout(ctx, 5*time.Second)
			err = consume(deliveryCtx, event)
			deliveryCancel()
			if err != nil {
				b.logger.ErrorContext(ctx, "deliver redis event", "error", err)
			}
		}
	}()
	return func() { cancel(); stopOnCancel(); sub.Close(); <-done }, nil
}
