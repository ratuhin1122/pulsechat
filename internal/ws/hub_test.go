package ws

import (
	"sync"
	"testing"

	"github.com/coder/websocket"
)

func TestHubBroadcast(t *testing.T) {
	for _, tt := range []struct {
		name         string
		members      []string
		wantA, wantB bool
	}{
		{name: "only room members", members: []string{"alice"}, wantA: true},
		{name: "multiple members", members: []string{"alice", "bob"}, wantA: true, wantB: true},
		{name: "empty room"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hub := NewHub()
			defer hub.Close()
			a, aSecond, b, outsider := newClient("alice", 2), newClient("alice", 2), newClient("bob", 2), newClient("outsider", 2)
			for _, c := range []*client{a, aSecond, b, outsider} {
				if !hub.register(c) {
					t.Fatal("register failed")
				}
			}
			payload := []byte("message")
			hub.Broadcast("room", tt.members, payload)
			payload[0] = 'X'
			for _, check := range []struct {
				c    *client
				want bool
			}{{a, tt.wantA}, {aSecond, tt.wantA}, {b, tt.wantB}, {outsider, false}} {
				select {
				case got := <-check.c.send:
					if !check.want || string(got) != "message" {
						t.Errorf("unexpected delivery to %s: %s", check.c.userID, got)
					}
				default:
					if check.want {
						t.Errorf("no delivery to %s", check.c.userID)
					}
				}
			}
		})
	}
}

func TestHubMembershipChanges(t *testing.T) {
	hub := NewHub()
	defer hub.Close()
	a, b := newClient("alice", 2), newClient("bob", 2)
	hub.register(a)
	hub.register(b)
	hub.Broadcast("room", []string{"alice"}, []byte("first"))
	<-a.send
	hub.Broadcast("room", []string{"bob"}, []byte("second"))
	select {
	case <-a.send:
		t.Fatal("departed member received a message")
	default:
	}
	select {
	case got := <-b.send:
		if string(got) != "second" {
			t.Fatal("wrong message")
		}
	default:
		t.Fatal("new member did not receive message")
	}
	hub.unregister(b)
	hub.unregister(b)
	if len(hub.rooms) != 0 {
		t.Fatal("empty room retained after disconnect")
	}
	hub.Broadcast("room", []string{"bob"}, []byte("third"))
	if len(b.send) != 0 {
		t.Fatal("disconnected client received a message")
	}
}

func TestHubClosesSlowConsumer(t *testing.T) {
	hub := NewHub()
	defer hub.Close()
	slow, fast := newClient("slow", 1), newClient("fast", 1)
	hub.register(slow)
	hub.register(fast)
	members := []string{"slow", "fast"}
	hub.Broadcast("room", members, []byte("first"))
	<-fast.send
	hub.Broadcast("room", members, []byte("second"))
	select {
	case <-slow.done:
		if slow.closeCode != websocket.StatusPolicyViolation {
			t.Errorf("close code = %d", slow.closeCode)
		}
	default:
		t.Fatal("slow consumer was not closed")
	}
	if _, ok := hub.clients[slow]; ok {
		t.Fatal("slow consumer retained")
	}
	if got := <-fast.send; string(got) != "second" {
		t.Fatal("slow consumer blocked healthy client")
	}
	select {
	case <-fast.done:
		t.Fatal("healthy client closed")
	default:
	}
}

func TestHubCloseAndConcurrentDisconnect(t *testing.T) {
	hub := NewHub()
	c := newClient("alice", 128)
	hub.register(c)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); hub.Broadcast("room", []string{"alice"}, []byte("hello")); hub.unregister(c) }()
	}
	wg.Wait()
	hub.Close()
	hub.Close()
	if hub.register(newClient("bob", 1)) {
		t.Fatal("registered after shutdown")
	}
	if len(hub.clients) != 0 || len(hub.rooms) != 0 {
		t.Fatal("hub did not clean up")
	}
}
