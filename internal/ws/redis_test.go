package ws

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func testRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	server.SetTime(time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC))
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), ContextTimeoutEnabled: true})
	t.Cleanup(func() { client.Close() })
	return server, client
}

func TestRedisPubSubAcrossHubs(t *testing.T) {
	_, redisClient := testRedis(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	firstBus, secondBus := NewRedisPubSub(redisClient, logger), NewRedisPubSub(redisClient, logger)
	first, second := NewHubWithPublisher(firstBus), NewHubWithPublisher(secondBus)
	defer first.Close()
	defer second.Close()
	a, b, outsider := newClient("alice", 8), newClient("bob", 8), newClient("outsider", 8)
	first.register(a)
	second.register(b)
	second.register(outsider)
	for _, pair := range []struct {
		bus *RedisPubSub
		hub *Hub
	}{{firstBus, first}, {secondBus, second}} {
		stop, err := pair.bus.Subscribe(ctx, func(_ context.Context, event Event) error { pair.hub.deliver(event); return nil })
		if err != nil {
			t.Fatal(err)
		}
		defer stop()
	}
	for _, tt := range []struct {
		name    string
		hub     *Hub
		payload string
	}{
		{"first instance", first, `{"type":"message","body":"first"}`},
		{"second instance", second, `{"type":"message","body":"second"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.hub.PublishRoom(ctx, "room", []string{"alice", "bob"}, []byte(tt.payload)); err != nil {
				t.Fatal(err)
			}
			for _, c := range []*client{a, b} {
				select {
				case data := <-c.send:
					if string(data) != tt.payload {
						t.Fatalf("unexpected/duplicate event: %s", data)
					}
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			if len(outsider.send) != 0 {
				t.Fatal("nonmember received room event")
			}
		})
	}
	// A subsequent targeted event verifies neither instance got a local+Redis echo.
	if err := first.PublishToUsers(ctx, []string{"alice", "bob"}, []byte(`{"type":"barrier"}`)); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*client{a, b} {
		select {
		case data := <-c.send:
			if string(data) != `{"type":"barrier"}` {
				t.Fatalf("duplicate Redis delivery: %s", data)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}

func presenceObserver(t *testing.T, client *redis.Client) func(*bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	sub := client.Subscribe(ctx, eventChannel)
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sub.Close() })
	return func(online *bool) {
		t.Helper()
		if online == nil {
			if err := client.Publish(ctx, eventChannel, `{"payload":{"type":"barrier"}}`).Err(); err != nil {
				t.Fatal(err)
			}
		}
		message, err := sub.ReceiveMessage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var event Event
		if err := json.Unmarshal([]byte(message.Payload), &event); err != nil {
			t.Fatal(err)
		}
		if online == nil {
			if event.Presence != nil {
				t.Fatalf("unexpected transition: %s", message.Payload)
			}
		} else if event.Presence == nil || event.Presence.UserID != "alice" || event.Presence.Online != *online {
			t.Fatalf("wrong transition: %s", message.Payload)
		}
	}
}
