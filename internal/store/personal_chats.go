package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/store/db"
)

// ClearChat hides only this user's conversation and records a permanent history cutoff.
func (s *Store) ClearChat(ctx context.Context, room, user pgtype.UUID) (bool, error) {
	result, err := s.Pool.Exec(ctx, `INSERT INTO personal_chat_deletions (room_id,user_id)
 SELECT room_id,user_id FROM room_members WHERE room_id=$1 AND user_id=$2
 ON CONFLICT (user_id,room_id) DO UPDATE SET cleared_at=clock_timestamp(),hidden=true`, room, user)
	return result.RowsAffected() > 0, err
}

func (s *Store) JoinGroupRoom(ctx context.Context, arg db.JoinGroupRoomParams) (int64, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	n, err := s.Queries.WithTx(tx).JoinGroupRoom(ctx, arg)
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(ctx, `UPDATE personal_chat_deletions SET hidden=false WHERE room_id=$1 AND user_id=$2`, arg.RoomID, arg.UserID)
	if err != nil {
		return 0, err
	}
	return n, tx.Commit(ctx)
}

// AttachmentVisible enforces deletion cutoffs even after the conversation is reopened.
func (s *Store) AttachmentVisible(ctx context.Context, id, user pgtype.UUID) (bool, error) {
	var visible bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM attachments a
 JOIN room_members m ON m.room_id=a.room_id AND m.user_id=$2
 LEFT JOIN messages msg ON msg.id=a.message_id
 WHERE a.id=$1 AND NOT EXISTS (SELECT 1 FROM personal_chat_deletions d
 WHERE d.user_id=$2 AND d.room_id=a.room_id AND (d.hidden OR (a.created_at<=d.cleared_at OR msg.created_at<=d.cleared_at))))`, id, user).Scan(&visible)
	return visible, err
}

// MessageVisible also filters already queued WebSocket events after deletion.
func (s *Store) MessageVisible(ctx context.Context, id, user pgtype.UUID) (bool, error) {
	var visible bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM messages m
 JOIN room_members member ON member.room_id=m.room_id AND member.user_id=$2
 WHERE m.id=$1 AND NOT EXISTS (SELECT 1 FROM personal_chat_deletions d
 WHERE d.room_id=m.room_id AND d.user_id=$2 AND (d.hidden OR (m.created_at<=d.cleared_at OR EXISTS (SELECT 1 FROM attachments a WHERE a.message_id=m.id AND a.created_at<=d.cleared_at)))))`, id, user).Scan(&visible)
	return visible, err
}
