package worker

import (
	"context"
	"log/slog"
	"time"

	"pulsechat/internal/storage"
)

type CleanupRepository interface {
	CleanupDeletedFiles(context.Context, storage.Storage) error
}

// CleanDeletedFiles retries account file deletions, including failures before a restart.
func CleanDeletedFiles(ctx context.Context, repo CleanupRepository, disk storage.Storage, logger *slog.Logger) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		attempt, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := repo.CleanupDeletedFiles(attempt, disk)
		cancel()
		if err != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "cleanup deleted account files", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
