package ws

import (
	"context"
	"sync"
)

// Presence tracks connection counts. Connect and Disconnect return true only
// for the first connection and the last disconnection, respectively.
type Presence interface {
	Connect(context.Context, string) (bool, error)
	Disconnect(context.Context, string) (bool, error)
	Counts(context.Context, []string) (map[string]int, error)
}

type MemoryPresence struct {
	mu     sync.Mutex
	counts map[string]int
}

func NewMemoryPresence() *MemoryPresence {
	return &MemoryPresence{counts: make(map[string]int)}
}

func (p *MemoryPresence) Connect(ctx context.Context, userID string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	p.counts[userID]++
	return p.counts[userID] == 1, nil
}

func (p *MemoryPresence) Disconnect(ctx context.Context, userID string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	switch p.counts[userID] {
	case 0:
		return false, nil
	case 1:
		delete(p.counts, userID)
		return true, nil
	default:
		p.counts[userID]--
		return false, nil
	}
}

func (p *MemoryPresence) Counts(ctx context.Context, userIDs []string) (map[string]int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(userIDs))
	for _, id := range userIDs {
		counts[id] = p.counts[id]
	}
	return counts, nil
}
