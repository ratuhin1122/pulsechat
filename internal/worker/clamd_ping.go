package worker

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"time"
)

// Ping checks daemon availability without scanning a file or affecting scan metrics.
func (s *ClamdScanner) Ping(ctx context.Context) (err error) {
	ctx, cancel := context.WithTimeout(ctx, min(s.timeout, 2*time.Second))
	defer cancel()
	defer func() {
		if err != nil && ctx.Err() != nil {
			err = ctx.Err()
		}
	}()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", s.address)
	if err != nil {
		return fmt.Errorf("connect clamd: %w", err)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	if err := writeAll(conn, []byte("zPING\x00")); err != nil {
		return fmt.Errorf("ping clamd: %w", err)
	}
	response, err := bufio.NewReaderSize(conn, 256).ReadSlice(0)
	if err != nil {
		return fmt.Errorf("read clamd ping: %w", err)
	}
	if string(response) != "PONG\x00" {
		return fmt.Errorf("unexpected clamd ping response")
	}
	return nil
}
