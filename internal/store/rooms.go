package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/store/db"
)

// CreateGroupRoom commits the room and its creator's membership together.
func (s *Store) CreateGroupRoom(ctx context.Context, name string, creator pgtype.UUID, public bool) (db.Room, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return db.Room{}, fmt.Errorf("begin group room: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)
	room, err := q.CreateRoom(ctx, db.CreateRoomParams{Type: "group", Name: name, IsPublic: public})
	if err != nil {
		return db.Room{}, fmt.Errorf("create group room: %w", err)
	}
	if _, err := q.AddRoomMember(ctx, db.AddRoomMemberParams{RoomID: room.ID, UserID: creator}); err != nil {
		return db.Room{}, fmt.Errorf("add creator membership: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Room{}, fmt.Errorf("commit group room: %w", err)
	}
	return room, nil
}

// GetOrCreateDirectRoom serializes both user orderings with a transaction lock.
// Direct-room memberships are fixed; join and leave only support group rooms.
func (s *Store) GetOrCreateDirectRoom(ctx context.Context, user, peer pgtype.UUID) (db.Room, error) {
	if user == peer {
		return db.Room{}, fmt.Errorf("direct room requires two distinct users")
	}
	first, second := user, peer
	if first.String() > second.String() {
		first, second = second, first
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return db.Room{}, fmt.Errorf("begin direct room: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)
	if err := q.LockDirectRoomPair(ctx, first.String()+":"+second.String()); err != nil {
		return db.Room{}, fmt.Errorf("lock direct room pair: %w", err)
	}
	if _, err := q.GetUser(ctx, peer); err != nil {
		return db.Room{}, fmt.Errorf("get direct room peer: %w", err)
	}
	room, err := q.FindDirectRoom(ctx, db.FindDirectRoomParams{FirstUser: first, SecondUser: second})
	if errors.Is(err, pgx.ErrNoRows) {
		room, err = q.CreateRoom(ctx, db.CreateRoomParams{Type: "dm"})
		if err != nil {
			return db.Room{}, fmt.Errorf("create direct room: %w", err)
		}
		for _, id := range []pgtype.UUID{first, second} {
			if _, err := q.AddRoomMember(ctx, db.AddRoomMemberParams{RoomID: room.ID, UserID: id}); err != nil {
				return db.Room{}, fmt.Errorf("add direct room member: %w", err)
			}
		}
	} else if err != nil {
		return db.Room{}, fmt.Errorf("find direct room: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Room{}, fmt.Errorf("commit direct room: %w", err)
	}
	return room, nil
}
