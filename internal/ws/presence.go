package ws

import (
	"context"
	"sync"
)

// Presence tracks connection counts. Connect and Disconnect return true only
// for the first connection and the last disconnection, respectively.
type Presence interface {
	Connect(context.Context, string, string) (bool, error)
	Disconnect(context.Context, string, string) (bool, error)
	Refresh(context.Context, string, string) error
	Counts(context.Context, []string) (map[string]int, error)
	// Redis publishes transitions atomically with counts; memory uses the handler.
	PublishesTransitions() bool
}

type MemoryPresence struct {
	mu          sync.Mutex
	connections map[string]map[string]struct{}
}

func NewMemoryPresence() *MemoryPresence {
	return &MemoryPresence{connections: make(map[string]map[string]struct{})}
}

func (p *MemoryPresence) PublishesTransitions() bool { return false }

func (p *MemoryPresence) Connect(ctx context.Context, userID, connectionID string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	connections := p.connections[userID]
	if connections == nil {
		connections = make(map[string]struct{})
		p.connections[userID] = connections
	}
	_, exists := connections[connectionID]
	connections[connectionID] = struct{}{}
	return !exists && len(connections) == 1, nil
}

func (p *MemoryPresence) Disconnect(ctx context.Context, userID, connectionID string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	connections := p.connections[userID]
	if _, exists := connections[connectionID]; !exists {
		return false, nil
	}
	delete(connections, connectionID)
	if len(connections) == 0 {
		delete(p.connections, userID)
		return true, nil
	}
	return false, nil
}

func (p *MemoryPresence) Refresh(ctx context.Context, userID, connectionID string) error {
	return ctx.Err()
}

func (p *MemoryPresence) Counts(ctx context.Context, userIDs []string) (map[string]int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(userIDs))
	for _, id := range userIDs {
		counts[id] = len(p.connections[id])
	}
	return counts, nil
}
