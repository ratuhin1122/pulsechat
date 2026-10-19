package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
)

const (
	ProcessAttachment  = "process_attachment"
	AttachmentMaxRetry = 5
	AttachmentTimeout  = 2 * time.Minute
)

type attachmentPayload struct {
	AttachmentID string `json:"attachment_id"`
}

type Queue struct {
	client *asynq.Client
	redis  *redis.Client
}

func NewRedisClient(redisURL string) (*redis.Client, error) {
	if redisURL == "" {
		return nil, fmt.Errorf("REDIS_URL is required for the attachment worker")
	}
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse queue REDIS_URL: %w", err)
	}
	options.ContextTimeoutEnabled = true
	return redis.NewClient(options), nil
}

// Empty REDIS_URL preserves the API's in-memory realtime mode. Uploads then
// return an enqueue error rather than reporting success without a queued job.
func NewQueue(redisURL string) (*Queue, error) {
	if redisURL == "" {
		return &Queue{}, nil
	}
	client, err := NewRedisClient(redisURL)
	if err != nil {
		return nil, err
	}
	return &Queue{client: asynq.NewClientFromRedisClient(client), redis: client}, nil
}

func (q *Queue) Close() error {
	if q.redis == nil {
		return nil
	}
	return q.redis.Close()
}

func NewProcessAttachmentTask(attachmentID string) (*asynq.Task, error) {
	var id pgtype.UUID
	if err := id.Scan(attachmentID); err != nil || !id.Valid {
		return nil, fmt.Errorf("invalid attachment ID")
	}
	data, err := json.Marshal(attachmentPayload{AttachmentID: id.String()})
	if err != nil {
		return nil, fmt.Errorf("encode attachment task: %w", err)
	}
	return asynq.NewTask(ProcessAttachment, data), nil
}

func (q *Queue) EnqueueAttachment(ctx context.Context, attachmentID string) error {
	if q.client == nil {
		return fmt.Errorf("REDIS_URL is required to enqueue attachments")
	}
	task, err := NewProcessAttachmentTask(attachmentID)
	if err != nil {
		return err
	}
	var id pgtype.UUID
	if err := id.Scan(attachmentID); err != nil {
		return err
	}
	_, err = q.client.EnqueueContext(ctx, task, asynq.MaxRetry(AttachmentMaxRetry),
		asynq.Timeout(AttachmentTimeout), asynq.TaskID("attachment-"+id.String()))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("enqueue attachment: %w", err)
	}
	return nil
}
