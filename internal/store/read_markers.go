package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/store/db"
)

// MarkRoomRead serializes tabs/devices on the member row and never moves a
// marker backwards. The returned ID is the effective persisted marker.
func (s *Store) MarkRoomRead(ctx context.Context, p db.SetRoomReadMarkerParams) (pgtype.UUID, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return markRoomReadTx(ctx, tx, p)
}

func markRoomReadTx(ctx context.Context, tx pgx.Tx, p db.SetRoomReadMarkerParams) (pgtype.UUID, error) {
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	q := db.New(tx)
	member, err := q.LockRoomMember(ctx, db.LockRoomMemberParams{RoomID: p.RoomID, UserID: p.UserID})
	if err != nil {
		return pgtype.UUID{}, err
	}
	target, err := q.GetRoomMessage(ctx, db.GetRoomMessageParams{RoomID: p.RoomID, ID: p.MessageID})
	if err != nil {
		return pgtype.UUID{}, err
	}
	marker := target.ID
	if member.LastReadMessageID.Valid {
		previous, err := q.GetRoomMessage(ctx, db.GetRoomMessageParams{RoomID: p.RoomID, ID: member.LastReadMessageID})
		if err != nil {
			return pgtype.UUID{}, err
		}
		if !messageAfter(target, previous) {
			marker = previous.ID
		}
	}
	if marker != member.LastReadMessageID {
		p.MessageID = marker
		if err := q.SetRoomReadMarker(ctx, p); err != nil {
			return pgtype.UUID{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return pgtype.UUID{}, err
	}
	return marker, nil
}

func messageAfter(a, b db.Message) bool {
	return a.CreatedAt.Time.After(b.CreatedAt.Time) ||
		a.CreatedAt.Time.Equal(b.CreatedAt.Time) && a.ID.String() > b.ID.String()
}
