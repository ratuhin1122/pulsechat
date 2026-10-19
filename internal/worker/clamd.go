package worker

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

type ClamdScanner struct {
	address string
	timeout time.Duration
}

func NewClamdScanner(address string, timeout time.Duration) (*ClamdScanner, error) {
	if _, _, err := net.SplitHostPort(address); err != nil {
		return nil, fmt.Errorf("invalid CLAMD_ADDR: %w", err)
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("clamd timeout must be positive")
	}
	return &ClamdScanner{address: address, timeout: timeout}, nil
}

// Scan uses clamd's NUL-framed INSTREAM protocol with bounded chunks and reply.
// https://docs.clamav.net/manual/Usage/ClamdProtocol.html
func (s *ClamdScanner) Scan(ctx context.Context, source io.Reader) (result ScanResult, err error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	defer func() {
		if err != nil && ctx.Err() != nil {
			err = fmt.Errorf("clamd scan: %w", ctx.Err())
		}
	}()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", s.address)
	if err != nil {
		return result, fmt.Errorf("connect clamd: %w", err)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return result, fmt.Errorf("set clamd deadline: %w", err)
	}
	if err := writeAll(conn, []byte("zINSTREAM\x00")); err != nil {
		return result, fmt.Errorf("send clamd command: %w", err)
	}
	var header [4]byte
	buffer := make([]byte, 32<<10)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		n, readErr := source.Read(buffer)
		if n > 0 {
			total += int64(n)
			if total > 30<<20 {
				return result, fmt.Errorf("clamd stream exceeds 30 MiB")
			}
			binary.BigEndian.PutUint32(header[:], uint32(n))
			if err := writeAll(conn, header[:]); err != nil {
				return result, fmt.Errorf("send clamd chunk length: %w", err)
			}
			if err := writeAll(conn, buffer[:n]); err != nil {
				return result, fmt.Errorf("send clamd chunk: %w", err)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return result, fmt.Errorf("read scan input: %w", readErr)
		}
	}
	binary.BigEndian.PutUint32(header[:], 0)
	if err := writeAll(conn, header[:]); err != nil {
		return result, fmt.Errorf("finish clamd stream: %w", err)
	}
	response, err := bufio.NewReaderSize(conn, 4096).ReadSlice(0)
	if err != nil {
		return result, fmt.Errorf("read clamd result: %w", err)
	}
	text := string(response[:len(response)-1])
	if text == "stream: OK" {
		return result, nil
	}
	if strings.HasPrefix(text, "stream: ") && strings.HasSuffix(text, " FOUND") && !strings.ContainsAny(text, "\r\n") {
		threat := strings.TrimSuffix(strings.TrimPrefix(text, "stream: "), " FOUND")
		if threat != "" {
			return ScanResult{Threat: threat}, nil
		}
	}
	return result, fmt.Errorf("clamd returned an unsuccessful result: %s", text)
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
