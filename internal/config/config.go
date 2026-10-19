package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	DatabaseURL     string
	HTTPAddr        string
	ShutdownTimeout time.Duration
}

func Load() (Config, error) {
	cfg := Config{HTTPAddr: ":8080", ShutdownTimeout: 10 * time.Second}
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
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
