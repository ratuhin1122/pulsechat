package worker

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestClamdPing(t *testing.T) {
	for _, tt := range []struct {
		name, reply string
		stall, bad  bool
	}{
		{name: "pong", reply: "PONG\x00"}, {name: "bad response", reply: "ERROR\x00", bad: true},
		{name: "truncated", reply: "PONG", bad: true}, {name: "oversized", reply: strings.Repeat("x", 1000) + "\x00", bad: true},
		{name: "deadline", stall: true, bad: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(time.Second))
				command, err := bufio.NewReader(conn).ReadString(0)
				if err != nil {
					return
				}
				if command != "zPING\x00" {
					t.Errorf("expected PING,got %q", command)
					return
				}
				if tt.stall {
					io.Copy(io.Discard, conn)
					return
				}
				_ = writeAll(conn, []byte(tt.reply))
			}()
			defer func() {
				listener.Close()
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					t.Error("fake clamd did not close")
				}
			}()
			scanner, err := NewClamdScanner(listener.Addr().String(), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			err = scanner.Ping(ctx)
			if (err != nil) != tt.bad {
				t.Fatalf("ping error=%v,want bad=%v", err, tt.bad)
			}
			if tt.stall && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expected deadline,got %v", err)
			}
		})
	}
}
