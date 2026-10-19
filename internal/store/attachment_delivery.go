package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"pulsechat/internal/store/db"
)

// FinalizeAttachment serializes retries and creates a single message only after
// processing succeeds. Both the message and its attachment link commit together.
func (s *Store) FinalizeAttachment(ctx context.Context, id pgtype.UUID) (db.Attachment, db.Message, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return db.Attachment{}, db.Message{}, err
	}
	return finalizeAttachmentTx(ctx, tx, id)
}

func finalizeAttachmentTx(ctx context.Context, tx pgx.Tx, id pgtype.UUID) (db.Attachment, db.Message, error) {
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	q := db.New(tx)
	a, err := q.GetAttachmentForUpdate(ctx, id)
	if err != nil {
		return a, db.Message{}, err
	}
	var message db.Message
	switch a.Status {
	case "ready":
		if a.MessageID.Valid {
			message, err = q.GetMessage(ctx, a.MessageID)
		} else {
			message, err = q.CreateMessage(ctx, db.CreateMessageParams{RoomID: a.RoomID, SenderID: a.SenderID, Type: a.Kind})
			if err == nil {
				err = q.LinkAttachmentMessage(ctx, db.LinkAttachmentMessageParams{ID: a.ID, MessageID: message.ID})
				a.MessageID = message.ID
			}
		}
	case "rejected":
	default:
		return a, message, fmt.Errorf("attachment is not finished")
	}
	if err != nil {
		return a, message, err
	}
	if err := tx.Commit(ctx); err != nil {
		return a, message, err
	}
	return a, message, nil
}
