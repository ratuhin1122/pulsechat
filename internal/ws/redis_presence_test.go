package ws

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisPresenceAcrossInstances(t *testing.T) {
	server, client := testRedis(t)
	options := *client.Options()
	otherClient := redis.NewClient(&options)
	defer otherClient.Close()
	first, second := NewRedisPresence(client), NewRedisPresence(otherClient)
	observe := presenceObserver(t, client)
	ctx := context.Background()
	online, offline := true, false
	for _, tt := range []struct {
		name, connection string
		presence         *RedisPresence
		connect, changed bool
		count            int
	}{
		{"first tab", "one", first, true, true, 1},
		{"idempotent connect", "one", first, true, false, 1},
		{"other instance tab", "two", second, true, false, 2},
		{"first tab closes", "one", first, false, false, 1},
		{"idempotent disconnect", "one", first, false, false, 1},
		{"last tab closes", "two", second, false, true, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var changed bool
			var err error
			if tt.connect {
				changed, err = tt.presence.Connect(ctx, "alice", tt.connection)
			} else {
				changed, err = tt.presence.Disconnect(ctx, "alice", tt.connection)
			}
			if err != nil || changed != tt.changed {
				t.Fatalf("changed = %v, error = %v", changed, err)
			}
			counts, err := second.Counts(ctx, []string{"alice", "bob"})
			if err != nil || counts["alice"] != tt.count || counts["bob"] != 0 {
				t.Fatalf("counts = %v, error = %v", counts, err)
			}
			if tt.connect && server.TTL("pulsechat:presence:connection:"+tt.connection) != 50*time.Second {
				t.Fatal("connection TTL must be 2 x heartbeat = 50s")
			}
			if tt.changed {
				if tt.connect {
					observe(&online)
				} else {
					observe(&offline)
				}
			}
			observe(nil)
		})
	}
}

func TestRedisPresenceRefreshAndExpiry(t *testing.T) {
	server, client := testRedis(t)
	p := NewRedisPresence(client)
	observe := presenceObserver(t, client)
	ctx := context.Background()
	online, offline := true, false
	if _, err := p.Connect(ctx, "alice", "one"); err != nil {
		t.Fatal(err)
	}
	observe(&online)
	now := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	advance := func(d time.Duration) { now = now.Add(d); server.FastForward(d); server.SetTime(now) }
	advance(25 * time.Second)
	if err := p.Refresh(ctx, "alice", "one"); err != nil {
		t.Fatal(err)
	}
	if server.TTL("pulsechat:presence:connection:one") != 50*time.Second {
		t.Fatal("heartbeat did not renew the connection TTL")
	}
	advance(30 * time.Second)
	counts, err := p.Counts(ctx, []string{"alice"})
	if err != nil || counts["alice"] != 1 {
		t.Fatalf("refreshed connection expired: %v, %v", counts, err)
	}
	observe(nil)
	advance(21 * time.Second)
	if server.Exists("pulsechat:presence:connection:one") {
		t.Fatal("dead connection key did not expire")
	}
	if err := p.Reap(ctx); err != nil {
		t.Fatal(err)
	}
	observe(&offline)
	if err := p.Reap(ctx); err != nil {
		t.Fatal(err)
	}
	if err := p.Refresh(ctx, "alice", "one"); err == nil {
		t.Fatal("expired lease was resurrected by heartbeat")
	}
	counts, err = p.Counts(ctx, []string{"alice"})
	if err != nil || counts["alice"] != 0 {
		t.Fatalf("dead connection still online: %v, %v", counts, err)
	}
	observe(nil)
}
