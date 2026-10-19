package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"pulsechat/internal/store/db"
)

type deliveryTx struct {
	pgx.Tx
	a                             db.Attachment
	message                       db.Message
	inserts                       int
	linkErr, commitErr            error
	committed, rolledBack, locked bool
}
type valueRow struct {
	value any
	err   error
}

func (r valueRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	v := reflect.ValueOf(r.value)
	for i, target := range dest {
		reflect.ValueOf(target).Elem().Set(v.Field(i))
	}
	return nil
}
func (tx *deliveryTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	switch {
	case strings.Contains(sql, "FROM attachments"):
		tx.locked = strings.Contains(sql, "FOR UPDATE")
		return valueRow{value: tx.a}
	case strings.Contains(sql, "INSERT INTO messages"):
		tx.inserts++
		tx.message = db.Message{ID: tx.a.ID, RoomID: args[0].(pgtype.UUID), SenderID: args[1].(pgtype.UUID), Type: args[2].(string), CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}
		return valueRow{value: tx.message}
	case strings.Contains(sql, "FROM messages"):
		return valueRow{value: tx.message}
	default:
		return valueRow{err: errors.New("unexpected query")}
	}
}
func (tx *deliveryTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if tx.linkErr != nil {
		return pgconn.CommandTag{}, tx.linkErr
	}
	if !strings.Contains(sql, "SET message_id") {
		return pgconn.CommandTag{}, errors.New("unexpected exec")
	}
	tx.a.MessageID = args[1].(pgtype.UUID)
	return pgconn.NewCommandTag("UPDATE 1"), nil
}
func (tx *deliveryTx) Commit(context.Context) error {
	tx.committed = tx.commitErr == nil
	return tx.commitErr
}
func (tx *deliveryTx) Rollback(context.Context) error { tx.rolledBack = true; return nil }

func TestFinalizeAttachmentTransaction(t *testing.T) {
	for _, tt := range []struct {
		name, status                        string
		existing, linkFail, commitFail, bad bool
		inserts                             int
	}{
		{name: "ready creates message", status: "ready", inserts: 1},
		{name: "ready retry reuses message", status: "ready", existing: true},
		{name: "rejected never creates message", status: "rejected"},
		{name: "pending cannot publish", status: "pending", bad: true},
		{name: "link failure rolls back", status: "ready", linkFail: true, bad: true, inserts: 1},
		{name: "commit failure retries", status: "ready", commitFail: true, bad: true, inserts: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var id pgtype.UUID
			_ = id.Scan("550e8400-e29b-41d4-a716-446655440001")
			tx := &deliveryTx{a: db.Attachment{ID: id, RoomID: id, SenderID: id, Status: tt.status, Kind: "voice"}}
			if tt.existing {
				tx.a.MessageID = id
				tx.message = db.Message{ID: id}
			}
			if tt.linkFail {
				tx.linkErr = errors.New("link unavailable")
			}
			if tt.commitFail {
				tx.commitErr = errors.New("commit unavailable")
			}
			a, m, err := finalizeAttachmentTx(context.Background(), tx, id)
			if (err != nil) != tt.bad || tx.inserts != tt.inserts || !tx.locked || !tx.rolledBack || tx.committed == tt.bad {
				t.Fatalf("error=%v,inserts=%d,locked=%v,committed=%v", err, tx.inserts, tx.locked, tx.committed)
			}
			if !tt.bad && tt.status == "ready" && (!a.MessageID.Valid || a.MessageID != m.ID) {
				t.Fatal("attachment did not link to message")
			}
		})
	}
}
