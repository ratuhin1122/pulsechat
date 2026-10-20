package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"pulsechat/internal/metrics"
	"pulsechat/internal/store/db"
)

func value(t *testing.T, m prometheus.Metric) *dto.Metric {
	t.Helper()
	out := &dto.Metric{}
	if err := m.Write(out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestProcessingMetrics(t *testing.T) {
	for _, tt := range []struct {
		name, kind, status, result string
		threat                     string
		scanErr, loadErr           error
		wantScan, wantBytes        bool
	}{
		{name: "ready", kind: "document", status: "pending", result: "ready", wantScan: true, wantBytes: true},
		{name: "infected", kind: "document", status: "pending", result: "infected", threat: "Eicar", wantScan: true},
		{name: "scan failure", kind: "document", status: "pending", result: "error", scanErr: errors.New("clamd unavailable"), wantScan: true},
		{name: "unsupported kind rejected", kind: "script", status: "pending", result: "rejected", wantScan: true},
		{name: "load failure", result: "error", loadErr: errors.New("database unavailable")},
		{name: "ready notification retry", kind: "document", status: "ready"},
		{name: "rejected cleanup retry", kind: "document", status: "rejected"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := metrics.NewWorker()
			disk := &fakeStorage{}
			scanner := &fakeScanner{result: ScanResult{Threat: tt.threat}, err: tt.scanErr}
			repo := &testRepository{
				get: func(_ context.Context, id pgtype.UUID) (db.Attachment, error) {
					return db.Attachment{ID: id, Kind: tt.kind, Status: tt.status, StoredName: id.String(), OriginalName: "sample.pdf", MimeType: "application/pdf", SizeBytes: 129}, tt.loadErr
				},
				mark:   func(context.Context, pgtype.UUID) (int64, error) { return 1, nil },
				reject: func(context.Context, db.RejectAttachmentParams) (int64, error) { return 1, nil },
			}
			h := NewAttachmentHandler(repo, disk, scanner, slog.New(slog.NewTextHandler(io.Discard, nil))).WithMetrics(m)
			task, err := NewProcessAttachmentTask("550e8400-e29b-41d4-a716-446655440000")
			if err != nil {
				t.Fatal(err)
			}
			err = h.ProcessTask(context.Background(), task)
			if (err != nil) != (tt.loadErr != nil) {
				t.Fatalf("process error=%v", err)
			}
			kind := tt.kind
			if kind != "image" && kind != "document" && kind != "voice" {
				kind = "unknown"
			}
			for _, result := range []string{"ready", "infected", "rejected", "error"} {
				want := float64(0)
				if result == tt.result {
					want = 1
				}
				if got := value(t, m.Processed.WithLabelValues(kind, result)).Counter.GetValue(); got != want {
					t.Fatalf("%s=%v,want %v", result, got, want)
				}
			}
			scanCount := value(t, m.Scan).Histogram.GetSampleCount()
			if (scanCount == 1) != tt.wantScan {
				t.Fatalf("scan samples=%d", scanCount)
			}
			original := value(t, m.Bytes.WithLabelValues("original")).Counter.GetValue()
			if tt.status == "pending" && original != 129 {
				t.Fatalf("original bytes=%v", original)
			}
			processed := value(t, m.Bytes.WithLabelValues("processed")).Counter.GetValue()
			if tt.wantBytes && processed != 129 || !tt.wantBytes && processed != 0 {
				t.Fatalf("processed bytes=%v", processed)
			}
		})
	}
}

type inspectorFake struct {
	info  *asynq.QueueInfo
	err   error
	calls int
}

func (i *inspectorFake) GetQueueInfo(string) (*asynq.QueueInfo, error) {
	i.calls++
	return i.info, i.err
}

func TestQueueDepth(t *testing.T) {
	m := metrics.NewWorker()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	inspector := &inspectorFake{info: &asynq.QueueInfo{Pending: 2, Scheduled: 3, Retry: 4, Aggregating: 1, Active: 9, Archived: 20}}
	for _, tt := range []struct {
		name string
		err  error
		want float64
	}{
		{"waiting backlog", nil, 10}, {"redis unavailable preserves sample", errors.New("offline"), 10}, {"removed queue", asynq.ErrQueueNotFound, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			inspector.err = tt.err
			RefreshQueueDepth(context.Background(), inspector, m, []string{"default"}, logger)
			if got := value(t, m.QueueDepth.WithLabelValues("default")).Gauge.GetValue(); got != tt.want {
				t.Fatalf("depth=%v,want %v", got, tt.want)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := inspector.calls
	RunQueueMetrics(ctx, inspector, m, []string{"default"}, logger)
	if inspector.calls != calls {
		t.Fatal("polled after shutdown")
	}
}
