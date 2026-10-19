package chat

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/store/db"
)

var ErrNotMember = errors.New("user is not a room member")

type MembershipReader interface {
	GetRoomMember(context.Context, db.GetRoomMemberParams) (db.RoomMember, error)
}

// RequireMembership returns ErrNotMember when access to a room should be denied.
func RequireMembership(ctx context.Context, reader MembershipReader, roomID, userID pgtype.UUID) error {
	_, err := reader.GetRoomMember(ctx, db.GetRoomMemberParams{RoomID: roomID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotMember
	}
	if err != nil {
		return fmt.Errorf("check room membership: %w", err)
	}
	return nil
}
