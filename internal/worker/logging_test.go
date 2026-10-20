package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hibiken/asynq"

	"pulsechat/internal/logging"
)

func TestTaskLogCorrelation(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[failed], func(t *testing.T) {
			var output bytes.Buffer
			task, err := newProcessAttachmentTask("550e8400-e29b-41d4-a716-446655440000", "http-request-123")
			if err != nil {
				t.Fatal(err)
			}
			wantErr := errors.New("test failure")
			next := asynq.HandlerFunc(func(ctx context.Context, _ *asynq.Task) error {
				if logging.From(ctx).RequestID != "http-request-123" {
					t.Fatal("lost upload request ID")
				}
				logging.Set(ctx, logging.Fields{UserID: "user-1", RoomID: "room-1"})
				if failed {
					return wantErr
				}
				return nil
			})
			err = LogTasks(logging.New(&output, "worker"), next).ProcessTask(context.Background(), task)
			if failed && !errors.Is(err, wantErr) || !failed && err != nil {
				t.Fatalf("task error=%v", err)
			}
			records := bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n"))
			var last map[string]any
			if err := json.Unmarshal(records[len(records)-1], &last); err != nil {
				t.Fatal(err)
			}
			for key, want := range map[string]string{"request_id": "http-request-123", "user_id": "user-1", "room_id": "room-1", "attachment_id": "550e8400-e29b-41d4-a716-446655440000"} {
				if last[key] != want {
					t.Errorf("%s=%v,want %s", key, last[key], want)
				}
			}
			if failed && last["level"] != "ERROR" {
				t.Fatal("failure was not logged at ERROR")
			}
		})
	}
}
