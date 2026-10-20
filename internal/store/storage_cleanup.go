package store

import (
	"context"
	"fmt"

	"pulsechat/internal/storage"
)

// CleanupDeletedFiles is safe across worker replicas: Delete is idempotent and
// outbox rows are removed only after storage confirms deletion.
func (s *Store) CleanupDeletedFiles(ctx context.Context, disk storage.Storage) error {
	rows, err := s.Pool.Query(ctx, "SELECT area,name FROM storage_deletions ORDER BY area,name LIMIT 500")
	if err != nil {
		return err
	}
	type object struct {
		area storage.Area
		name string
	}
	var objects []object
	for rows.Next() {
		var item object
		if err = rows.Scan(&item.area, &item.name); err != nil {
			rows.Close()
			return err
		}
		objects = append(objects, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, item := range objects {
		if err = disk.Delete(ctx, item.area, item.name); err != nil {
			return fmt.Errorf("clean deleted file: %w", err)
		}
		if _, err = s.Pool.Exec(ctx, "DELETE FROM storage_deletions WHERE area=$1 AND name=$2", string(item.area), item.name); err != nil {
			return err
		}
	}
	return nil
}
