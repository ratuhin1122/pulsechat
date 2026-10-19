package chat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"pulsechat/internal/store/db"
)

func TestHistoryAttachments(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "ready media", true: "metadata query failure"}[failure], func(t *testing.T) {
			id := testUUID(t, "550e8400-e29b-41d4-a716-446655440001")
			repo := &historyRepository{fakeRepository: &fakeRepository{
				member: func(context.Context, db.GetRoomMemberParams) (db.RoomMember, error) { return db.RoomMember{}, nil },
				attachments: func(_ context.Context, messageID pgtype.UUID) ([]db.Attachment, error) {
					if messageID != id {
						t.Fatal("wrong message ID")
					}
					if failure {
						return nil, errors.New("unavailable")
					}
					return []db.Attachment{{ID: id, Kind: "image", MimeType: "image/webp", StoredName: "private-clean-name", Width: pgtype.Int4{Int32: 320, Valid: true}}}, nil
				},
			}, history: func(context.Context, db.ListRoomHistoryParams) ([]db.Message, error) {
				return []db.Message{{ID: id, RoomID: id, Type: "image"}}, nil
			}}
			mux, token := testRoutes(t, repo)
			r := httptest.NewRequest("GET", "/rooms/"+id.String()+"/messages", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if failure {
				if w.Code != 500 {
					t.Fatal(w.Code)
				}
				return
			}
			var messages []messageResponse
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &messages) != nil || len(messages) != 1 || len(messages[0].Attachments) != 1 || messages[0].Attachments[0].ID != id {
				t.Fatalf("response=%s", w.Body.String())
			}
			if strings.Contains(w.Body.String(), "private-clean-name") || strings.Contains(w.Body.String(), "stored_name") {
				t.Fatal("storage names leaked")
			}
		})
	}
}
