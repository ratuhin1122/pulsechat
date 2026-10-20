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
		{name: "defaults", want: Config{WorkerMetricsAddr: "127.0.0.1:9091", MetricsAddr: "127.0.0.1:9090", HTTPAddr: ":8080", ShutdownTimeout: 10 * time.Second, StorageDir: "data"}},
		{name: "overrides", addr: "127.0.0.1:9000", timeout: "5s", want: Config{WorkerMetricsAddr: "127.0.0.1:9091", MetricsAddr: "127.0.0.1:9090", HTTPAddr: "127.0.0.1:9000", ShutdownTimeout: 5 * time.Second, StorageDir: "data"}},
		{name: "invalid timeout", timeout: "invalid", wantErr: true},
		{name: "zero timeout", timeout: "0s", wantErr: true},
		{name: "negative timeout", timeout: "-1s", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "")
			t.Setenv("JWT_SECRET", "")
			t.Setenv("WS_ORIGIN", "")
			t.Setenv("DEV_HTTP", "")
			t.Setenv("TRUSTED_PROXY_CIDRS", "")
			t.Setenv("REDIS_URL", "")
			t.Setenv("STORAGE_DIR", "")
			t.Setenv("CLAMD_ADDR", "")
			t.Setenv("INSPECT_ARCHIVES", "")
			t.Setenv("METRICS_ADDR", "")
			t.Setenv("WORKER_METRICS_ADDR", "")
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

func TestArchiveInspectionConfig(t *testing.T) {
	t.Setenv("SHUTDOWN_TIMEOUT", "")
	for _, tt := range []struct {
		value        string
		enabled, bad bool
	}{
		{value: ""}, {value: "false"}, {value: "true", enabled: true}, {value: "invalid", bad: true},
	} {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv("INSPECT_ARCHIVES", tt.value)
			cfg, err := Load()
			if (err != nil) != tt.bad || err == nil && cfg.InspectArchives != tt.enabled {
				t.Fatalf("config=%+v,error=%v", cfg, err)
			}
		})
	}
}

func TestMetricsAddr(t *testing.T) {
	t.Setenv("SHUTDOWN_TIMEOUT", "")
	for _, tt := range []struct{ value, want string }{{"", "127.0.0.1:9090"}, {":9100", ":9100"}} {
		t.Run(tt.want, func(t *testing.T) {
			t.Setenv("METRICS_ADDR", tt.value)
			cfg, err := Load()
			if err != nil || cfg.MetricsAddr != tt.want {
				t.Fatalf("metrics addr=%q,error=%v", cfg.MetricsAddr, err)
			}
		})
	}
}

func TestWorkerMetricsAddr(t *testing.T) {
	t.Setenv("SHUTDOWN_TIMEOUT", "")
	for _, tt := range []struct{ env, want string }{{"", "127.0.0.1:9091"}, {":9191", ":9191"}} {
		t.Run(tt.want, func(t *testing.T) {
			t.Setenv("WORKER_METRICS_ADDR", tt.env)
			cfg, err := Load()
			if err != nil || cfg.WorkerMetricsAddr != tt.want {
				t.Fatalf("addr=%s,err=%v", cfg.WorkerMetricsAddr, err)
			}
		})
	}
}
