package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"pulsechat/internal/storage"
	"pulsechat/internal/store/db"
	"pulsechat/internal/tracing"
	"pulsechat/internal/ws"
)

func TestUploadTraceThroughWorkerAndNotification(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	apiProvider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()), sdktrace.WithSpanProcessor(recorder))
	defer apiProvider.Shutdown(context.Background())
	workerProvider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())), sdktrace.WithSpanProcessor(recorder))
	defer workerProvider.Shutdown(context.Background())
	apiRuntime := &tracing.Runtime{Provider: apiProvider}
	workerRuntime := &tracing.Runtime{Provider: workerProvider}
	redis := miniredis.RunT(t)
	queue, err := NewQueue("redis://" + redis.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer queue.Close()
	queue.WithTracing(apiRuntime)
	h, completion, _ := completionFixture(t, "pending")
	local, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	original, err := local.Save(context.Background(), storage.Quarantine, bytes.NewReader(samplePNG(t, 8, 4)))
	if err != nil {
		t.Fatal(err)
	}
	completion.a.StoredName = original.Name
	h.repo = &testRepository{get: func(context.Context, pgtype.UUID) (db.Attachment, error) { return completion.a, nil }, complete: func(context.Context, db.CompleteImageAttachmentParams) (int64, error) {
		completion.a.Status = "ready"
		return 1, nil
	}}
	scanner := &fakeScanner{}
	h.scanner = scanner
	h.images = &fakeImages{scanner: scanner}
	h.storage = local
	h.WithTracing(workerRuntime)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	publisher := ws.NewRedisPubSub(queue.redis, logger)
	apiHandler, err := ws.New(context.Background(), nil, nil, ws.NewHub(), ws.NewMemoryPresence(), logger, "")
	if err != nil {
		t.Fatal(err)
	}
	apiHandler.WithTracing(apiRuntime)
	delivered := make(chan struct{}, 2)
	stop, err := publisher.Subscribe(context.Background(), func(ctx context.Context, event ws.Event) error {
		err := apiHandler.DeliverEvent(ctx, event)
		delivered <- struct{}{}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	h.ConfigureCompletion(completion, publisher)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rooms/{roomID}/upload", func(w http.ResponseWriter, r *http.Request) {
		if err := queue.EnqueueAttachment(r.Context(), completion.a.ID.String()); err != nil {
			t.Error(err)
		}
		w.WriteHeader(202)
	})
	apiRuntime.HTTP(mux, mux).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/rooms/test/upload", nil))
	inspector := asynq.NewInspectorFromRedisClient(queue.redis)
	pending, err := inspector.ListPendingTasks("default")
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending=%d,error=%v", len(pending), err)
	}
	var payload attachmentPayload
	if err := json.Unmarshal(pending[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.TraceID == "" || payload.TraceContext["traceparent"] == "" {
		t.Fatal("missing trace propagation in job")
	}
	task := asynq.NewTask(ProcessAttachment, pending[0].Payload)
	if err := TraceTasks(workerRuntime, h).ProcessTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		select {
		case <-delivered:
		case <-time.After(5 * time.Second):
			t.Fatal("notification not delivered")
		}
	}
	ended := recorder.Ended()
	byName := make(map[string]sdktrace.ReadOnlySpan)
	for _, span := range ended {
		if span.SpanContext().TraceID().String() != payload.TraceID {
			t.Fatalf("span %s belongs to another trace", span.Name())
		}
		byName[span.Name()] = span
	}
	for _, name := range []string{"POST /rooms/{roomID}/upload", "attachment.enqueue", "attachment.process", "clamd.scan", "attachment.image.process", "websocket.notify", "websocket.notify.deliver"} {
		if byName[name] == nil {
			t.Fatalf("missing %s span", name)
		}
	}
	if byName["attachment.process"].Parent().SpanID() != byName["attachment.enqueue"].SpanContext().SpanID() || !byName["attachment.process"].Parent().IsRemote() {
		t.Fatal("worker did not continue producer context")
	}
	if byName["websocket.notify.deliver"].Parent().SpanID() != byName["websocket.notify"].SpanContext().SpanID() {
		t.Fatal("Redis notification context lost")
	}
}

func TestLegacyJobGetsRootTrace(t *testing.T) {
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	defer provider.Shutdown(context.Background())
	runtime := &tracing.Runtime{Provider: provider}
	task, err := NewProcessAttachmentTask("550e8400-e29b-41d4-a716-446655440000")
	if err != nil {
		t.Fatal(err)
	}
	next := asynq.HandlerFunc(func(ctx context.Context, _ *asynq.Task) error {
		if !trace.SpanContextFromContext(ctx).IsValid() {
			t.Fatal("missing legacy root trace")
		}
		return nil
	})
	if err := TraceTasks(runtime, next).ProcessTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
}
