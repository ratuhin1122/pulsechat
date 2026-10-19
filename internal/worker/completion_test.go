package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"pulsechat/internal/chat"
	"pulsechat/internal/store/db"
	"pulsechat/internal/ws"
)

type completionRepo struct {
	a                                db.Attachment
	message                          db.Message
	finalizeErr, membersErr, markErr error
	finalizes, creates, marks        int
}

func (r *completionRepo) FinalizeAttachment(_ context.Context, id pgtype.UUID) (db.Attachment, db.Message, error) {
	r.finalizes++
	if r.finalizeErr != nil {
		return r.a, r.message, r.finalizeErr
	}
	if r.a.Status == "ready" && !r.a.MessageID.Valid {
		r.creates++
		r.a.MessageID = id
		r.message = db.Message{ID: id, RoomID: r.a.RoomID, SenderID: r.a.SenderID, Type: r.a.Kind, CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}
	}
	return r.a, r.message, nil
}
func (r *completionRepo) ListRoomMemberIDs(context.Context, pgtype.UUID) ([]pgtype.UUID, error) {
	return []pgtype.UUID{r.a.SenderID, r.a.RoomID}, r.membersErr
}
func (r *completionRepo) MarkAttachmentNotified(context.Context, pgtype.UUID) error {
	r.marks++
	if r.markErr == nil {
		r.a.NotifiedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}
	return r.markErr
}

type eventPublisher struct {
	events []ws.Event
	failAt int
}

func (p *eventPublisher) Publish(_ context.Context, e ws.Event) error {
	if p.failAt == len(p.events)+1 {
		return errors.New("redis unavailable")
	}
	p.events = append(p.events, e)
	return nil
}

func completionFixture(t *testing.T, status string) (*AttachmentHandler, *completionRepo, *eventPublisher) {
	t.Helper()
	var id, room, sender pgtype.UUID
	_ = id.Scan("550e8400-e29b-41d4-a716-446655440010")
	_ = room.Scan("550e8400-e29b-41d4-a716-446655440001")
	_ = sender.Scan("550e8400-e29b-41d4-a716-446655440000")
	r := &completionRepo{a: db.Attachment{ID: id, RoomID: room, SenderID: sender, Status: status, Kind: "image", MimeType: "image/webp", OriginalName: "photo.jpg", StoredName: "private-storage-name", SizeBytes: 100}}
	p := &eventPublisher{}
	h := NewAttachmentHandler(nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.ConfigureCompletion(r, p)
	return h, r, p
}

func TestCompletionEvents(t *testing.T) {
	for _, status := range []string{"ready", "rejected"} {
		t.Run(status, func(t *testing.T) {
			h, r, p := completionFixture(t, status)
			if err := h.finish(context.Background(), r.a.ID); err != nil {
				t.Fatal(err)
			}
			want := 1
			if status == "ready" {
				want = 2
			}
			if len(p.events) != want || !r.a.NotifiedAt.Valid {
				t.Fatalf("events=%d, notified=%v", len(p.events), r.a.NotifiedAt.Valid)
			}
			private := p.events[0]
			if private.RoomID != "" || len(private.UserIDs) != 1 || private.UserIDs[0] != r.a.SenderID.String() {
				t.Fatal("status leaked beyond sender")
			}
			var notice struct {
				Type   string      `json:"type"`
				ID     pgtype.UUID `json:"attachment_id"`
				Status string      `json:"status"`
			}
			if json.Unmarshal(private.Payload, &notice) != nil || notice.Type != "attachment_status" || notice.ID != r.a.ID || notice.Status != status {
				t.Fatalf("wrong status event: %s", private.Payload)
			}
			if status == "ready" {
				room := p.events[1]
				if room.RoomID != r.a.RoomID.String() || len(room.UserIDs) != 2 {
					t.Fatal("wrong room audience")
				}
				var message struct {
					Type        string                    `json:"type"`
					ID          pgtype.UUID               `json:"id"`
					MessageType string                    `json:"message_type"`
					Attachments []chat.AttachmentResponse `json:"attachments"`
				}
				if json.Unmarshal(room.Payload, &message) != nil || message.Type != "message" || message.ID != r.message.ID || message.MessageType != "image" || len(message.Attachments) != 1 || message.Attachments[0].ID != r.a.ID {
					t.Fatalf("wrong media message: %s", room.Payload)
				}
				var raw map[string]any
				_ = json.Unmarshal(room.Payload, &raw)
				if _, leaked := raw["stored_name"]; leaked {
					t.Fatal("storage name leaked")
				}
			} else if r.creates != 0 {
				t.Fatal("rejected attachment created a message")
			}
			if err := h.finish(context.Background(), r.a.ID); err != nil {
				t.Fatal(err)
			}
			if len(p.events) != want || r.creates > 1 {
				t.Fatal("completed retry sent duplicate events or created another message")
			}
		})
	}
}

func TestCompletionRetries(t *testing.T) {
	for _, tt := range []struct {
		name                    string
		finalize, members, mark bool
		publish                 int
	}{
		{name: "database finalize", finalize: true}, {name: "sender event", publish: 1}, {name: "room event", publish: 2}, {name: "membership query", members: true}, {name: "delivery marker", mark: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h, r, p := completionFixture(t, "ready")
			failure := errors.New("unavailable")
			if tt.finalize {
				r.finalizeErr = failure
			}
			if tt.members {
				r.membersErr = failure
			}
			if tt.mark {
				r.markErr = failure
			}
			p.failAt = tt.publish
			if err := h.finish(context.Background(), r.a.ID); err == nil {
				t.Fatal("failure did not trigger retry")
			}
			if r.a.NotifiedAt.Valid {
				t.Fatal("marked notified before all events published")
			}
			r.finalizeErr = nil
			r.membersErr = nil
			r.markErr = nil
			p.failAt = 0
			if err := h.finish(context.Background(), r.a.ID); err != nil {
				t.Fatal(err)
			}
			if r.creates != 1 || !r.a.NotifiedAt.Valid {
				t.Fatal("retry did not reuse message and complete")
			}
		})
	}
}

func TestCleanupPrecedesCompletion(t *testing.T) {
	h, completion, p := completionFixture(t, "ready")
	disk := &fakeStorage{deleteErr: errors.New("cleanup failed")}
	a := completion.a
	a.QuarantineName = pgtype.Text{String: a.ID.String(), Valid: true}
	h.repo = &testRepository{get: func(context.Context, pgtype.UUID) (db.Attachment, error) { return a, nil }}
	h.storage = disk
	task, _ := NewProcessAttachmentTask(a.ID.String())
	if err := h.ProcessTask(context.Background(), task); err == nil {
		t.Fatal("expected cleanup retry")
	}
	if completion.finalizes != 0 || len(p.events) != 0 {
		t.Fatal("completion ran before processing finished")
	}
	disk.deleteErr = nil
	if err := h.ProcessTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if completion.creates != 1 || len(p.events) != 2 {
		t.Fatal("successful cleanup did not finalize")
	}
}
