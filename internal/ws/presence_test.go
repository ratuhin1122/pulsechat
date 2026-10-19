package ws

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestMemoryPresence(t *testing.T) {
	p := NewMemoryPresence()
	ctx := context.Background()
	for _, tt := range []struct {
		name         string
		connectionID string
		connect      bool
		changed      bool
		count        int
	}{
		{"first tab", "a", true, true, 1},
		{"second tab", "b", true, false, 2},
		{"one tab leaves", "a", false, false, 1},
		{"last tab leaves", "b", false, true, 0},
		{"duplicate disconnect", "b", false, false, 0},
		{"reconnect", "a", true, true, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var changed bool
			var err error
			if tt.connect {
				changed, err = p.Connect(ctx, "alice", tt.connectionID)
			} else {
				changed, err = p.Disconnect(ctx, "alice", tt.connectionID)
			}
			if err != nil || changed != tt.changed {
				t.Fatalf("changed = %v, error = %v", changed, err)
			}
			counts, err := p.Counts(ctx, []string{"alice", "bob"})
			if err != nil || counts["alice"] != tt.count || counts["bob"] != 0 {
				t.Fatalf("counts = %v, error = %v", counts, err)
			}
			counts["alice"] = 100 // Snapshots cannot mutate stored counts.
		})
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := p.Connect(canceled, "alice", "a"); !errors.Is(err, context.Canceled) {
		t.Errorf("Connect error = %v", err)
	}
	if _, err := p.Disconnect(canceled, "alice", "a"); !errors.Is(err, context.Canceled) {
		t.Errorf("Disconnect error = %v", err)
	}
	if _, err := p.Counts(canceled, []string{"alice"}); !errors.Is(err, context.Canceled) {
		t.Errorf("Counts error = %v", err)
	}
}

func TestConcurrentPresence(t *testing.T) {
	p := NewMemoryPresence()
	var first, last atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			changed, err := p.Connect(context.Background(), "alice", fmt.Sprint(i))
			if err != nil {
				t.Error(err)
			}
			if changed {
				first.Add(1)
			}
		}()
	}
	wg.Wait()
	counts, err := p.Counts(context.Background(), []string{"alice"})
	if err != nil || counts["alice"] != 32 || first.Load() != 1 {
		t.Fatalf("counts = %v, online transitions = %d, error = %v", counts, first.Load(), err)
	}
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			changed, err := p.Disconnect(context.Background(), "alice", fmt.Sprint(i))
			if err != nil {
				t.Error(err)
			}
			if changed {
				last.Add(1)
			}
		}()
	}
	wg.Wait()
	if last.Load() != 1 {
		t.Errorf("offline transitions = %d", last.Load())
	}
}
