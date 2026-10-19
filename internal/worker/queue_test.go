package worker

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/store/db"
)

func TestAttachmentQueue(t *testing.T) {
	redis := miniredis.RunT(t)
	queue, err := NewQueue("redis://" + redis.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer queue.Close()
	id := "550e8400-e29b-41d4-a716-446655440000"
	for i := 0; i < 2; i++ {
		if err := queue.EnqueueAttachment(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	inspector := asynq.NewInspectorFromRedisClient(queue.redis)
	tasks, err := inspector.ListPendingTasks("default")
	if err != nil || len(tasks) != 1 {
		t.Fatalf("pending tasks = %d, error = %v", len(tasks), err)
	}
	task := tasks[0]
	var payload attachmentPayload
	if json.Unmarshal(task.Payload, &payload) != nil || payload.AttachmentID != id {
		t.Fatal("wrong task payload")
	}
	if task.Type != ProcessAttachment || task.MaxRetry != 5 || task.Timeout != 2*time.Minute {
		t.Fatalf("incorrect task options: %+v", task)
	}
	if err := queue.EnqueueAttachment(context.Background(), "bad"); err == nil {
		t.Fatal("enqueued invalid attachment ID")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := queue.EnqueueAttachment(ctx, "550e8400-e29b-41d4-a716-446655440001"); err == nil {
		t.Fatal("ignored canceled enqueue context")
	}
}

func TestQueueConfiguration(t *testing.T) {
	queue, err := NewQueue("")
	if err != nil {
		t.Fatal(err)
	}
	defer queue.Close()
	if err := queue.EnqueueAttachment(context.Background(), "550e8400-e29b-41d4-a716-446655440000"); err == nil {
		t.Fatal("disabled queue silently accepted task")
	}
	if _, err := NewQueue("invalid://redis"); err == nil {
		t.Fatal("accepted invalid Redis URL")
	}
	if _, err := NewRedisClient(""); err == nil {
		t.Fatal("worker allowed missing Redis URL")
	}
}

func TestWorkerConsumesAttachment(t *testing.T) {
	redis := miniredis.RunT(t)
	queue, err := NewQueue("redis://" + redis.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer queue.Close()
	processed := make(chan pgtype.UUID, 1)
	repo := &testRepository{
		get: func(_ context.Context, id pgtype.UUID) (db.Attachment, error) {
			return db.Attachment{ID: id, Status: "pending"}, nil
		},
		mark: func(ctx context.Context, id pgtype.UUID) (int64, error) {
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > AttachmentTimeout {
				t.Error("worker task has no two-minute timeout")
			}
			processed <- id
			return 1, nil
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := asynq.NewServerFromRedisClient(queue.redis, asynq.Config{Concurrency: 1, TaskCheckInterval: 10 * time.Millisecond, ShutdownTimeout: time.Second, Logger: Logger{Log: logger}})
	mux := asynq.NewServeMux()
	mux.Handle(ProcessAttachment, NewAttachmentHandler(repo, logger))
	if err := server.Start(mux); err != nil {
		t.Fatal(err)
	}
	defer server.Shutdown()
	id := "550e8400-e29b-41d4-a716-446655440000"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := queue.EnqueueAttachment(ctx, id); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-processed:
		if got.String() != id {
			t.Error("processed wrong attachment")
		}
	case <-ctx.Done():
		t.Fatal("worker did not consume attachment task")
	}
}
