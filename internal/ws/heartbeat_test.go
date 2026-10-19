package ws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type heartbeatSocket struct {
	ping   func(context.Context) error
	closed bool
}

type heartbeatPresence struct {
	*MemoryPresence
	refreshed func()
}

func (p *heartbeatPresence) Refresh(context.Context, string, string) error {
	p.refreshed()
	return nil
}

func (s *heartbeatSocket) Ping(ctx context.Context) error                             { return s.ping(ctx) }
func (s *heartbeatSocket) Write(context.Context, websocket.MessageType, []byte) error { return nil }
func (s *heartbeatSocket) Close(websocket.StatusCode, string) error                   { s.closed = true; return nil }
func (s *heartbeatSocket) CloseNow() error                                            { s.closed = true; return nil }

func TestHeartbeat(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
	}{
		{"alive", nil}, {"timeout", context.DeadlineExceeded}, {"dead connection", errors.New("connection lost")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h, raw := newTestHandler(t, &testRepository{}, "")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			h.presence = &heartbeatPresence{MemoryPresence: NewMemoryPresence(), refreshed: cancel}
			var pingCtx context.Context
			socket := &heartbeatSocket{ping: func(ctx context.Context) error {
				pingCtx = ctx
				deadline, ok := ctx.Deadline()
				remaining := time.Until(deadline)
				if !ok || remaining > 10*time.Second || remaining < 9*time.Second {
					t.Errorf("ping timeout = %v", remaining)
				}
				if tt.err != nil {
					cancel()
				}
				return tt.err
			}}
			ticks := make(chan time.Time, 1)
			ticks <- time.Now()
			h.writePump(ctx, socket, newClient("alice", 1), raw, ticks)
			if pingCtx == nil {
				t.Fatal("no server ping")
			}
			if pingCtx.Err() == nil {
				t.Fatal("ping context not canceled")
			}
			if socket.closed != (tt.err != nil) {
				t.Errorf("socket closed = %v", socket.closed)
			}
		})
	}
}
