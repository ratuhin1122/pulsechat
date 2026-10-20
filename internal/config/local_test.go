package config

import "testing"

func TestLocalHTTP(t *testing.T) {
	for _, tt := range []struct {
		name, enabled, origin string
		bad                   bool
	}{
		{"production default", "", "https://chat.example.com", false},
		{"localhost", "true", "http://localhost:8080", false},
		{"ipv4", "true", "http://127.0.0.1:8080", false},
		{"ipv6", "true", "http://[::1]:8080", false},
		{"public host", "true", "http://chat.example.com", true},
		{"no origin", "true", "", true},
		{"wrong scheme", "true", "https://localhost", true},
		{"credentials", "true", "http://name@localhost", true},
		{"invalid boolean", "yes", "http://localhost", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DEV_HTTP", tt.enabled)
			t.Setenv("WS_ORIGIN", tt.origin)
			t.Setenv("INSPECT_ARCHIVES", "")
			t.Setenv("SHUTDOWN_TIMEOUT", "")
			cfg, err := Load()
			if (err != nil) != tt.bad {
				t.Fatalf("error=%v", err)
			}
			if err == nil && cfg.DevHTTP != (tt.enabled == "true") {
				t.Fatalf("DevHTTP=%v", cfg.DevHTTP)
			}
		})
	}
}
