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
		{name: "defaults", want: Config{HTTPAddr: ":8080", ShutdownTimeout: 10 * time.Second, StorageDir: "data"}},
		{name: "overrides", addr: "127.0.0.1:9000", timeout: "5s", want: Config{HTTPAddr: "127.0.0.1:9000", ShutdownTimeout: 5 * time.Second, StorageDir: "data"}},
		{name: "invalid timeout", timeout: "invalid", wantErr: true},
		{name: "zero timeout", timeout: "0s", wantErr: true},
		{name: "negative timeout", timeout: "-1s", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "")
			t.Setenv("JWT_SECRET", "")
			t.Setenv("WS_ORIGIN", "")
			t.Setenv("REDIS_URL", "")
			t.Setenv("STORAGE_DIR", "")
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

func TestLoadDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/pulsechat")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("SHUTDOWN_TIMEOUT", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if cfg.DatabaseURL != "postgres://localhost/pulsechat" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
}

func TestLoadRedisURL(t *testing.T) {
	t.Setenv("SHUTDOWN_TIMEOUT", "")
	for _, value := range []string{"", "redis://localhost:6379/0"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("REDIS_URL", value)
			cfg, err := Load()
			if err != nil || cfg.RedisURL != value {
				t.Fatalf("RedisURL = %q, error = %v", cfg.RedisURL, err)
			}
		})
	}
}
