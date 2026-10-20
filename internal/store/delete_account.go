package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/storage"
)

// DeleteAccount locks the user and attachment rows against concurrent uploads and
// processing. File cleanup is committed atomically in a durable outbox.
func (s *Store) DeleteAccount(ctx context.Context, user pgtype.UUID, disk storage.Storage) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	var id pgtype.UUID
	if err = tx.QueryRow(ctx, "SELECT id FROM users WHERE id=$1 FOR UPDATE", user).Scan(&id); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT stored_name,COALESCE(thumbnail_name,''),COALESCE(quarantine_name,'') FROM attachments WHERE sender_id=$1 FOR UPDATE`, user)
	if err != nil {
		return err
	}
	var objects [][3]string
	for rows.Next() {
		var names [3]string
		if err = rows.Scan(&names[0], &names[1], &names[2]); err != nil {
			rows.Close()
			return err
		}
		objects = append(objects, names)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, names := range objects {
		for _, area := range []storage.Area{storage.Quarantine, storage.Clean} {
			for _, name := range names {
				if name != "" {
					if _, err = tx.Exec(ctx, "INSERT INTO storage_deletions(area,name) VALUES($1,$2) ON CONFLICT DO NOTHING", string(area), name); err != nil {
						return err
					}
				}
			}
		}
	}
	// Delete messages explicitly: sender_id intentionally has no cascading FK.
	if _, err = tx.Exec(ctx, "DELETE FROM messages WHERE sender_id=$1", user); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM users WHERE id=$1", user); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit account deletion: %w", err)
	}
	// Attempt immediate cleanup; the worker retries the durable outbox on failure.
	_ = s.CleanupDeletedFiles(ctx, disk)
	return nil
}
