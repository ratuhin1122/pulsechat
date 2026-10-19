package ws

import (
	"context"
	"time"

	"github.com/coder/websocket"
)

const (
	heartbeatInterval = 25 * time.Second
	heartbeatTimeout  = 10 * time.Second
)

type socketWriter interface {
	Write(context.Context, websocket.MessageType, []byte) error
	Ping(context.Context) error
	Close(websocket.StatusCode, string) error
	CloseNow() error
}

func (h *Handler) writeLoop(ctx context.Context, conn socketWriter, c *client, raw string) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	h.writePump(ctx, conn, c, raw, ticker.C)
}

func (h *Handler) writePump(ctx context.Context, conn socketWriter, c *client, raw string, ticks <-chan time.Time) {
	for {
		// Prioritize eviction over any messages still buffered for this client.
		select {
		case <-c.done:
			conn.Close(c.closeCode, c.closeReason)
			return
		default:
		}
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			conn.Close(c.closeCode, c.closeReason)
			return
		case data := <-c.send:
			if _, err := h.tokens.Validate(raw); err != nil {
				conn.Close(websocket.StatusPolicyViolation, "access token expired")
				return
			}
			writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Write(writeCtx, websocket.MessageText, data)
			cancel()
			if err != nil {
				return
			}
		case <-ticks:
			if _, err := h.tokens.Validate(raw); err != nil {
				conn.Close(websocket.StatusPolicyViolation, "access token expired")
				return
			}
			pingCtx, cancel := context.WithTimeout(ctx, heartbeatTimeout)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				conn.CloseNow()
				return
			}
		}
	}
}
