package ws

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type readAudience struct {
	*testRepository
	err error
}

func (r *readAudience) ListRoomMemberIDs(ctx context.Context, id pgtype.UUID) ([]pgtype.UUID, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.testRepository.ListRoomMemberIDs(ctx, id)
}

func TestPublishRead(t *testing.T) {
	for _, mode := range []string{"memory", "redis", "audience error"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			repo := &readAudience{testRepository: &testRepository{}}
			if mode == "audience error" {
				repo.err = errors.New("database")
			}
			h, _ := newTestHandler(t, repo, "")
			defer h.hub.Close()
			if mode == "redis" {
				_, client := testRedis(t)
				bus := NewRedisPubSub(client, h.logger)
				h.hub = NewHubWithPublisher(bus)
				stop, err := bus.Subscribe(ctx, h.DeliverEvent)
				if err != nil {
					t.Fatal(err)
				}
				defer stop()
				defer h.hub.Close()
			}
			user := newClient("550e8400-e29b-41d4-a716-446655440000", 4)
			otherTab := newClient(user.userID, 4)
			outsider := newClient("outsider", 4)
			for _, c := range []*client{user, otherTab, outsider} {
				h.hub.register(c)
			}
			var room, actor, message pgtype.UUID
			_ = room.Scan("550e8400-e29b-41d4-a716-446655440001")
			_ = actor.Scan(user.userID)
			_ = message.Scan("550e8400-e29b-41d4-a716-446655440002")
			err := h.PublishRead(ctx, room, actor, message)
			if mode == "audience error" {
				if err == nil {
					t.Fatal("missing audience error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range []*client{user, otherTab} {
				select {
				case data := <-c.send:
					var event map[string]string
					if json.Unmarshal(data, &event) != nil || event["type"] != "read" || event["room_id"] != room.String() || event["user_id"] != actor.String() || event["message_id"] != message.String() {
						t.Fatalf("event=%s", data)
					}
				case <-ctx.Done():
					t.Fatal("read event not delivered")
				}
			}
			select {
			case <-outsider.send:
				t.Fatal("read event leaked to outsider")
			default:
			}
		})
	}
}
