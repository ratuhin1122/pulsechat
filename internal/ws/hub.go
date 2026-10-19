package ws

import (
	"bytes"
	"sync"

	"github.com/coder/websocket"
)

type client struct {
	userID      string
	send        chan []byte
	done        chan struct{}
	closeCode   websocket.StatusCode
	closeReason string
}

func newClient(userID string, buffer int) *client {
	return &client{userID: userID, send: make(chan []byte, buffer), done: make(chan struct{})}
}

// Hub tracks local connections and room audiences. It never performs socket I/O
// while holding its lock; a full send queue disconnects only that consumer.
type Hub struct {
	mu      sync.Mutex
	clients map[*client]struct{}
	rooms   map[string]map[*client]struct{}
	closed  bool
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*client]struct{}), rooms: make(map[string]map[*client]struct{})}
}

func (h *Hub) register(c *client) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return false
	}
	h.clients[c] = struct{}{}
	return true
}

func (h *Hub) unregister(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.remove(c, websocket.StatusNormalClosure, "connection closed")
}

// remove must be called with mu held. Only the hub closes done; send stays open.
func (h *Hub) remove(c *client, code websocket.StatusCode, reason string) {
	if _, ok := h.clients[c]; !ok {
		return
	}
	delete(h.clients, c)
	for roomID, audience := range h.rooms {
		delete(audience, c)
		if len(audience) == 0 {
			delete(h.rooms, roomID)
		}
	}
	c.closeCode, c.closeReason = code, reason
	close(c.done)
}

// Broadcast replaces the room audience with a fresh database membership snapshot
// and sends to all active connections for those users, including the sender.
func (h *Hub) Broadcast(roomID string, memberIDs []string, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	allowed := make(map[string]struct{}, len(memberIDs))
	for _, id := range memberIDs {
		allowed[id] = struct{}{}
	}
	audience := make(map[*client]struct{})
	for c := range h.clients {
		if _, ok := allowed[c.userID]; ok {
			audience[c] = struct{}{}
		}
	}
	if len(audience) == 0 {
		delete(h.rooms, roomID)
		return
	}
	h.rooms[roomID] = audience
	data := bytes.Clone(payload)
	for c := range audience {
		select {
		case c.send <- data:
		default:
			h.remove(c, websocket.StatusPolicyViolation, "slow consumer")
		}
	}
}

func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for c := range h.clients {
		h.remove(c, websocket.StatusGoingAway, "server shutting down")
	}
}
