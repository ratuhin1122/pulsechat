package config

import (
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		timeout string
		want    Config
		wantErr bool
	}{
		{name: "defaults", want: Config{HTTPAddr: ":8080", ShutdownTimeout: 10 * time.Second}},
		{name: "overrides", addr: "127.0.0.1:9000", timeout: "5s", want: Config{HTTPAddr: "127.0.0.1:9000", ShutdownTimeout: 5 * time.Second}},
		{name: "invalid timeout", timeout: "invalid", wantErr: true},
		{name: "zero timeout", timeout: "0s", wantErr: true},
		{name: "negative timeout", timeout: "-1s", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HTTP_ADDR", tt.addr)
			t.Setenv("SHUTDOWN_TIMEOUT", tt.timeout)
			got, err := Load()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
