package main

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServeStopsBothOnListenerFailure(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	public := &http.Server{Addr: "127.0.0.1:0"}
	internal := &http.Server{Addr: occupied.Addr().String()}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := serve(ctx, time.Second, public, internal); err == nil {
		t.Fatal("metrics bind failure must stop the API")
	}
	if ctx.Err() != nil {
		t.Fatal("listener failure did not return promptly")
	}
}
