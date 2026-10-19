package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	TrustedProxyCIDRs string
	InspectArchives   bool
	ClamdAddr         string
	StorageDir        string
	RedisURL          string
	WSOrigin          string
	JWTSecret         string
	DatabaseURL       string
	HTTPAddr          string
	ShutdownTimeout   time.Duration
}

func Load() (Config, error) {
	cfg := Config{HTTPAddr: ":8080", ShutdownTimeout: 10 * time.Second, StorageDir: "data"}
	if directory := os.Getenv("STORAGE_DIR"); directory != "" {
		cfg.StorageDir = directory
	}
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	cfg.JWTSecret = os.Getenv("JWT_SECRET")
	cfg.WSOrigin = os.Getenv("WS_ORIGIN")
	cfg.RedisURL = os.Getenv("REDIS_URL")
	cfg.TrustedProxyCIDRs = os.Getenv("TRUSTED_PROXY_CIDRS")
	cfg.ClamdAddr = os.Getenv("CLAMD_ADDR")
	if value := os.Getenv("INSPECT_ARCHIVES"); value != "" {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse INSPECT_ARCHIVES: %w", err)
		}
		cfg.InspectArchives = enabled
	}
	if value := os.Getenv("HTTP_ADDR"); value != "" {
		cfg.HTTPAddr = value
	}
	if value := os.Getenv("SHUTDOWN_TIMEOUT"); value != "" {
		timeout, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse SHUTDOWN_TIMEOUT: %w", err)
		}
		if timeout <= 0 {
			return Config{}, fmt.Errorf("SHUTDOWN_TIMEOUT must be positive")
		}
		cfg.ShutdownTimeout = timeout
	}
	return cfg, nil
}
