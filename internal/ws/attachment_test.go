package ws

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"pulsechat/internal/store/db"
)

func TestReplayAttachmentMetadata(t *testing.T) {
	cursor, missed := replayMessage(t, 1), replayMessage(t, 2)
	missed.Type = "voice"
	repo := &replayRepository{testRepository: &testRepository{
		attachments: func(_ context.Context, id pgtype.UUID) ([]db.Attachment, error) {
			if id != missed.ID {
				t.Error("wrong message ID")
			}
			return []db.Attachment{{ID: id, Kind: "voice", MimeType: "audio/ogg", DurationSec: pgtype.Float8{Float64: 12, Valid: true}, StoredName: "private-storage"}}, nil
		},
	}, cursor: func(context.Context, db.GetUserMessageParams) (db.Message, error) { return cursor, nil },
		latest: func(context.Context, pgtype.UUID) (db.Message, error) { return missed, nil },
		page: func(context.Context, db.ListReplayMessagesParams) ([]db.Message, error) {
			return []db.Message{missed}, nil
		}}
	h, token := newTestHandler(t, repo, "http://frontend.test")
	ctx, conn := replaySocket(t, h, token, cursor.ID.String())
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var message outgoingMessage
	if json.Unmarshal(data, &message) != nil || message.ID != missed.ID || message.MessageType != "voice" || len(message.Attachments) != 1 || message.Attachments[0].DurationSec == nil || *message.Attachments[0].DurationSec != 12 {
		t.Fatalf("unexpected media replay: %s", data)
	}
	if strings.Contains(string(data), "private-storage") {
		t.Fatal("storage name leaked")
	}
}
