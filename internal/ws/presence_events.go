package ws

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type presenceEvent struct {
	Type   string `json:"type"`
	UserID string `json:"user_id"`
	Online bool   `json:"online"`
}

// Serialize transitions and delivery so a reconnect cannot overtake an offline
// event. Shared-room peers receive a single event even with several shared rooms.
func (h *Handler) changePresence(ctx context.Context, userID string, online bool) error {
	h.presenceMu.Lock()
	defer h.presenceMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var changed bool
	var err error
	if online {
		changed, err = h.presence.Connect(ctx, userID)
	} else {
		changed, err = h.presence.Disconnect(ctx, userID)
	}
	if err != nil || !changed {
		return err
	}
	recipients, err := h.presenceRecipients(ctx, userID)
	if err != nil {
		if online {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cleanupCancel()
			_, rollbackErr := h.presence.Disconnect(cleanupCtx, userID)
			return errors.Join(err, rollbackErr)
		}
		return err
	}
	data, err := json.Marshal(presenceEvent{Type: "presence", UserID: userID, Online: online})
	if err != nil {
		return err
	}
	h.hub.BroadcastToUsers(recipients, data)
	return nil
}

func (h *Handler) presenceRecipients(ctx context.Context, userID string) ([]string, error) {
	var user pgtype.UUID
	if err := user.Scan(userID); err != nil {
		return nil, err
	}
	rooms, err := h.repo.ListUserRoomIDs(ctx, user)
	if err != nil {
		return nil, err
	}
	unique := make(map[string]struct{})
	for _, room := range rooms {
		members, err := h.repo.ListRoomMemberIDs(ctx, room)
		if err != nil {
			return nil, err
		}
		for _, member := range members {
			if id := member.String(); id != userID {
				unique[id] = struct{}{}
			}
		}
	}
	ids := make([]string, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	return ids, nil
}
