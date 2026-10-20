package ws

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgtype"
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

func (h *Handler) writeLoop(ctx context.Context, conn socketWriter, c *client, raw string, plan *replayPlan) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	if plan != nil {
		if err := h.replay(ctx, conn, c, raw, plan, ticker.C); err != nil {
			h.logger.ErrorContext(ctx, "replay messages", "error", err)
			conn.CloseNow()
			return
		}
	}
	h.writePump(ctx, conn, c, raw, ticker.C)
}

func ping(ctx context.Context, conn socketWriter) error {
	pingCtx, cancel := context.WithTimeout(ctx, heartbeatTimeout)
	defer cancel()
	return conn.Ping(pingCtx)
}

func (h *Handler) heartbeat(ctx context.Context, conn socketWriter, c *client) error {
	if err := ping(ctx, conn); err != nil {
		if ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
			c.disconnect.Store(2)
		}
		return err
	}
	refreshCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return h.presence.Refresh(refreshCtx, c.userID, c.connectionID)
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
			if c.replayed != nil {
				var event struct {
					ID pgtype.UUID `json:"id"`
				}
				if json.Unmarshal(data, &event) == nil && event.ID.Valid {
					if _, duplicate := c.replayed[event.ID]; duplicate {
						continue
					}
				}
			}
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
			err := h.heartbeat(ctx, conn, c)
			if err != nil {
				conn.CloseNow()
				return
			}
		}
	}
}
