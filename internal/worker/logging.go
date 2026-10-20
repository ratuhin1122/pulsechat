package worker

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"log/slog"

	"github.com/hibiken/asynq"

	"pulsechat/internal/logging"
)

// LogTasks preserves the upload request ID across Redis retries and enriches all task logs.
func LogTasks(logger *slog.Logger, next asynq.Handler) asynq.Handler {
	return asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
		var payload attachmentPayload
		_ = json.Unmarshal(task.Payload(), &payload)
		id := payload.RequestID
		if !logging.ValidID(id) {
			id, _ = asynq.GetTaskID(ctx)
			if !logging.ValidID(id) {
				id = rand.Text()
			}
		}
		ctx = logging.WithFields(ctx, logging.Fields{RequestID: id, AttachmentID: payload.AttachmentID})
		logger.InfoContext(ctx, "attachment task started", "task_type", task.Type())
		err := next.ProcessTask(ctx, task)
		if err != nil {
			logger.ErrorContext(ctx, "attachment task failed", "task_type", task.Type(), "error", err)
		} else {
			logger.InfoContext(ctx, "attachment task finished", "task_type", task.Type())
		}
		return err
	})
}
