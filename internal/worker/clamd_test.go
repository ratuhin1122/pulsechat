package worker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func fakeClamd(t *testing.T, response string, stall bool) (string, <-chan []byte) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	received := make(chan []byte, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(3 * time.Second))
		reader := bufio.NewReader(conn)
		command, err := reader.ReadString(0)
		if err != nil {
			return
		}
		if command != "zINSTREAM\x00" {
			t.Errorf("wrong clamd command: %q", command)
			return
		}
		var data bytes.Buffer
		for {
			var header [4]byte
			if _, err := io.ReadFull(reader, header[:]); err != nil {
				return
			}
			size := binary.BigEndian.Uint32(header[:])
			if size == 0 {
				break
			}
			if size > 32<<10 {
				t.Errorf("unbounded chunk: %d", size)
				return
			}
			if _, err := io.CopyN(&data, reader, int64(size)); err != nil {
				return
			}
		}
		received <- data.Bytes()
		if stall {
			io.Copy(io.Discard, reader)
			return
		}
		writeAll(conn, []byte(response))
	}()
	t.Cleanup(func() {
		listener.Close()
		select {
		case <-done:
		case <-time.After(4 * time.Second):
			t.Error("fake clamd did not stop")
		}
	})
	return listener.Addr().String(), received
}

func TestClamdProtocol(t *testing.T) {
	for _, tt := range []struct {
		name, response, threat string
		wantErr                bool
	}{
		{name: "clean", response: "stream: OK\x00"},
		{name: "infected", response: "stream: Eicar-Signature FOUND\x00", threat: "Eicar-Signature"},
		{name: "daemon error", response: "stream: database unavailable ERROR\x00", wantErr: true},
		{name: "size error", response: "INSTREAM size limit exceeded. ERROR\x00", wantErr: true},
		{name: "ambiguous response", response: "garbage OK\x00", wantErr: true},
		{name: "empty signature", response: "stream:  FOUND\x00", wantErr: true},
		{name: "truncated response", response: "stream: OK", wantErr: true},
		{name: "oversized response", response: strings.Repeat("x", 5000) + "\x00", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			address, received := fakeClamd(t, tt.response, false)
			scanner, err := NewClamdScanner(address, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			content := bytes.Repeat([]byte("quarantined bytes"), 6000)
			result, err := scanner.Scan(context.Background(), bytes.NewReader(content))
			if (err != nil) != tt.wantErr || result.Threat != tt.threat {
				t.Fatalf("result = %+v, error = %v", result, err)
			}
			select {
			case data := <-received:
				if !bytes.Equal(data, content) {
					t.Error("file was not streamed intact")
				}
			case <-time.After(time.Second):
				t.Fatal("no streamed file")
			}
		})
	}
}

func TestClamdTimeoutAndCancellation(t *testing.T) {
	address, _ := fakeClamd(t, "", true)
	scanner, err := NewClamdScanner(address, 30*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := scanner.Scan(context.Background(), strings.NewReader("file")); err == nil {
		t.Fatal("stalled scanner returned clean")
	}
	if time.Since(start) > time.Second {
		t.Error("scan ignored timeout")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := scanner.Scan(ctx, strings.NewReader("file")); err == nil {
		t.Fatal("ignored canceled scan")
	}
	if _, err := NewClamdScanner("bad-address", time.Second); err == nil {
		t.Fatal("accepted invalid clamd address")
	}
}
