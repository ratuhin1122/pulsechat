package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/store/db"
)

type testRepository struct {
	get      func(context.Context, pgtype.UUID) (db.Attachment, error)
	mark     func(context.Context, pgtype.UUID) (int64, error)
	reject   func(context.Context, db.RejectAttachmentParams) (int64, error)
	complete func(context.Context, db.CompleteImageAttachmentParams) (int64, error)
	media    func(context.Context, db.CompleteMediaAttachmentParams) (int64, error)
}

func (r *testRepository) CompleteMediaAttachment(ctx context.Context, p db.CompleteMediaAttachmentParams) (int64, error) {
	if r.media != nil {
		return r.media(ctx, p)
	}
	return r.mark(ctx, p.ID)
}

func (r *testRepository) CompleteImageAttachment(ctx context.Context, p db.CompleteImageAttachmentParams) (int64, error) {
	return r.complete(ctx, p)
}

func (r *testRepository) GetAttachment(ctx context.Context, id pgtype.UUID) (db.Attachment, error) {
	return r.get(ctx, id)
}
func (r *testRepository) MarkAttachmentReady(ctx context.Context, id pgtype.UUID) (int64, error) {
	return r.mark(ctx, id)
}

func (r *testRepository) RejectAttachment(ctx context.Context, p db.RejectAttachmentParams) (int64, error) {
	return r.reject(ctx, p)
}

func TestProcessAttachment(t *testing.T) {
	validID := "550e8400-e29b-41d4-a716-446655440000"
	for _, tt := range []struct {
		name, payload, status   string
		getErr, markErr         error
		wantErr, skip, wantMark bool
	}{
		{name: "pending becomes ready", status: "pending", wantMark: true},
		{name: "already ready", status: "ready"},
		{name: "rejected remains rejected", status: "rejected"},
		{name: "malformed payload", payload: "{", wantErr: true, skip: true},
		{name: "invalid UUID", payload: `{"attachment_id":"bad"}`, wantErr: true, skip: true},
		{name: "missing UUID", payload: `{}`, wantErr: true, skip: true},
		{name: "deleted attachment", getErr: pgx.ErrNoRows, wantErr: true, skip: true},
		{name: "transient load failure retries", getErr: errors.New("database unavailable"), wantErr: true},
		{name: "transient update failure retries", status: "pending", markErr: errors.New("database unavailable"), wantErr: true, wantMark: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			marked := 0
			status := tt.status
			repo := &testRepository{
				get: func(got context.Context, id pgtype.UUID) (db.Attachment, error) {
					if got != ctx || id.String() != validID {
						t.Error("load arguments not forwarded")
					}
					return db.Attachment{ID: id, Status: status, Kind: "document", OriginalName: "sample.pdf", MimeType: "application/pdf"}, tt.getErr
				},
				mark: func(got context.Context, id pgtype.UUID) (int64, error) {
					marked++
					if got != ctx || id.String() != validID {
						t.Error("update arguments not forwarded")
					}
					if tt.markErr == nil {
						status = "ready"
					}
					return 1, tt.markErr
				},
			}
			task, err := NewProcessAttachmentTask(validID)
			if err != nil {
				t.Fatal(err)
			}
			if tt.payload != "" {
				task = asynq.NewTask(ProcessAttachment, []byte(tt.payload))
			}
			handler := NewAttachmentHandler(repo, &fakeStorage{}, &fakeScanner{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			err = handler.ProcessTask(ctx, task)
			if (err != nil) != tt.wantErr || errors.Is(err, asynq.SkipRetry) != tt.skip {
				t.Fatalf("error = %v, wantErr = %v, skip = %v", err, tt.wantErr, tt.skip)
			}
			if (marked > 0) != tt.wantMark {
				t.Errorf("ready updates = %d", marked)
			}
			if tt.name == "pending becomes ready" {
				if err := handler.ProcessTask(ctx, task); err != nil || marked != 1 {
					t.Fatalf("task is not idempotent: %d updates, %v", marked, err)
				}
			}
		})
	}
}
