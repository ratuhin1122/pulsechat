package ws

import (
	"context"
	"time"

	"github.com/coder/websocket"
)

func (h *Handler) writeLoop(ctx context.Context, conn *websocket.Conn, c *client, raw string) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
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
		case <-ticker.C:
			if _, err := h.tokens.Validate(raw); err != nil {
				conn.Close(websocket.StatusPolicyViolation, "access token expired")
				return
			}
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
