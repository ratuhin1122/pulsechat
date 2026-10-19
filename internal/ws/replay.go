package ws

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/store/db"
)

type replayPlan struct{ after, through db.Message }

func (h *Handler) replayCursor(w http.ResponseWriter, r *http.Request, user pgtype.UUID) (*db.Message, bool) {
	values, present := r.URL.Query()["last_message_id"]
	if !present {
		return nil, true
	}
	var id pgtype.UUID
	if len(values) != 1 || id.Scan(values[0]) != nil || !id.Valid {
		http.Error(w, "invalid last_message_id", http.StatusBadRequest)
		return nil, false
	}
	cursor, err := h.repo.GetUserMessage(r.Context(), db.GetUserMessageParams{MessageID: id, UserID: user})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "cursor not found", http.StatusNotFound)
		} else {
			h.presenceFailure(w, r, err)
		}
		return nil, false
	}
	return &cursor, true
}

// Capture a fixed replay boundary and register for live messages under the same
// lock used by local message insertion/broadcast. This leaves no handoff gap.
func (h *Handler) registerForReplay(ctx context.Context, c *client, cursor *db.Message, user pgtype.UUID) (*replayPlan, error) {
	h.messageMu.Lock()
	defer h.messageMu.Unlock()
	var plan *replayPlan
	if cursor != nil {
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		latest, err := h.repo.GetLatestUserMessage(queryCtx, user)
		if err != nil {
			return nil, err
		}
		plan = &replayPlan{after: *cursor, through: latest}
	}
	if !h.hub.register(c) {
		return nil, errors.New("hub closed")
	}
	return plan, nil
}

// Replay uses bounded database pages and the sole socket writer. Live events
// remain in the buffered queue until replay completes; overflow still evicts.
func (h *Handler) replay(ctx context.Context, conn socketWriter, c *client, raw string, plan *replayPlan, ticks <-chan time.Time) error {
	var user pgtype.UUID
	if err := user.Scan(c.userID); err != nil {
		return err
	}
	after := plan.after
	for {
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		messages, err := h.repo.ListReplayMessages(queryCtx, db.ListReplayMessagesParams{
			UserID: user, AfterCreatedAt: after.CreatedAt, AfterID: after.ID,
			ThroughCreatedAt: plan.through.CreatedAt, ThroughID: plan.through.ID, PageLimit: 50,
		})
		cancel()
		if err != nil {
			return err
		}
		for _, message := range messages {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-c.done:
				return errors.New("connection evicted")
			case <-ticks:
				if err := ping(ctx, conn); err != nil {
					return err
				}
			default:
			}
			if _, err := h.tokens.Validate(raw); err != nil {
				return err
			}
			data, err := marshalMessage(message)
			if err != nil {
				return err
			}
			writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err = conn.Write(writeCtx, websocket.MessageText, data)
			cancel()
			if err != nil {
				return err
			}
			after = message
		}
		if len(messages) < 50 {
			return nil
		}
	}
}
