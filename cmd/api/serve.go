package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// A listener failure stops both servers; shutdown also runs after bind errors.
func serve(ctx context.Context, timeout time.Duration, servers ...*http.Server) error {
	results := make(chan error, len(servers))
	for _, server := range servers {
		go func() { results <- server.ListenAndServe() }()
	}
	var result error
	remaining := len(servers)
	select {
	case err := <-results:
		result = fmt.Errorf("serve http: %w", err)
		remaining--
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for _, server := range servers {
		if err := server.Shutdown(shutdownCtx); err != nil {
			result = errors.Join(result, err, server.Close())
		}
	}
	for range remaining {
		if err := <-results; !errors.Is(err, http.ErrServerClosed) {
			result = errors.Join(result, err)
		}
	}
	return result
}
