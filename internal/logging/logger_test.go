package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"go.opentelemetry.io/otel/trace"
	"sync"
	"testing"
)

func TestScopesAreIsolatedAndConcurrent(t *testing.T) {
	ctx := WithFields(context.Background(), Fields{RequestID: "original"})
	child := WithFields(ctx, From(ctx))
	Set(child, Fields{RoomID: "room-1"})
	if From(ctx).RoomID != "" {
		t.Fatal("child scope changed parent")
	}
	var tasks sync.WaitGroup
	for range 10 {
		tasks.Go(func() { Set(ctx, Fields{UserID: "user-1"}); _ = From(ctx) })
	}
	tasks.Wait()
	var output bytes.Buffer
	New(&output, "worker").InfoContext(child, "done", "attachment_id", "explicit")
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["request_id"] != "original" || record["room_id"] != "room-1" || record["attachment_id"] != "explicit" {
		t.Fatal("incorrect correlation fields")
	}
	if bytes.Count(output.Bytes(), []byte(`"attachment_id"`)) != 1 {
		t.Fatal("duplicate correlation keys")
	}
}

func TestTraceIDInJSON(t *testing.T) {
	id, err := trace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	spanID, err := trace.SpanIDFromHex("0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: id, SpanID: spanID}))
	var output bytes.Buffer
	New(&output, "api").InfoContext(ctx, "traced")
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["trace_id"] != id.String() || record["span_id"] != spanID.String() {
		t.Fatal("log trace correlation missing")
	}
}
