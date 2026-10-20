package tracing

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"

	"pulsechat/internal/config"
)

func TestOTLPHTTPAndPGX(t *testing.T) {
	requests := make(chan *collector.ExportTraceServiceRequest, 2)
	collectorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" {
			t.Errorf("export path=%s", r.URL.Path)
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		request := &collector.ExportTraceServiceRequest{}
		if err := proto.Unmarshal(data, request); err != nil {
			t.Error(err)
		}
		requests <- request
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(200)
	}))
	defer collectorServer.Close()
	runtime, err := New(context.Background(), config.Config{OTLPEndpoint: collectorServer.URL, TraceSampler: "parentbased_traceidratio", TraceSampleRatio: 1}, "api")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rooms/{id}", func(w http.ResponseWriter, r *http.Request) {
		tracer := runtime.PGX()
		ctx := tracer.TraceQueryStart(r.Context(), nil, pgx.TraceQueryStartData{SQL: "SELECT secret", Args: []any{"password"}})
		tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})
		w.WriteHeader(204)
	})
	runtime.HTTP(mux, mux).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/rooms/private-room?access_token=private-token", nil))
	runtime.Close(slog.New(slog.NewTextHandler(io.Discard, nil)))
	select {
	case exported := <-requests:
		var names []string
		var spans int
		for _, resource := range exported.ResourceSpans {
			for _, scope := range resource.ScopeSpans {
				for _, span := range scope.Spans {
					names = append(names, span.Name)
					spans++
					if strings.Contains(span.String(), "secret") || strings.Contains(span.String(), "password") || strings.Contains(span.String(), "private-") {
						t.Fatal("exported query/path secrets")
					}
				}
			}
		}
		if spans != 2 || !strings.Contains(strings.Join(names, ","), "GET /rooms/{id}") || !strings.Contains(strings.Join(names, ","), "postgres.query") {
			t.Fatalf("exported spans=%v", names)
		}
	default:
		t.Fatal("no OTLP export on shutdown")
	}
}

func TestSamplingPropagation(t *testing.T) {
	for _, sample := range []bool{false, true} {
		t.Run(map[bool]string{false: "unsampled", true: "sampled"}[sample], func(t *testing.T) {
			sampler := sdktrace.NeverSample()
			if sample {
				sampler = sdktrace.AlwaysSample()
			}
			api := sdktrace.NewTracerProvider(sdktrace.WithSampler(sampler))
			defer api.Shutdown(context.Background())
			worker := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())))
			defer worker.Shutdown(context.Background())
			parent, parentSpan := api.Tracer("test").Start(context.Background(), "upload")
			defer parentSpan.End()
			ctx := Extract(context.Background(), Inject(parent))
			_, span := worker.Tracer("test").Start(ctx, "worker")
			defer span.End()
			if span.SpanContext().TraceID() != parentSpan.SpanContext().TraceID() || span.SpanContext().IsSampled() != sample {
				t.Fatal("trace/sampling decision lost")
			}
		})
	}
}

func TestPGXErrorStatus(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer provider.Shutdown(context.Background())
	runtime := &Runtime{Provider: provider}
	ctx, span := runtime.Start(context.Background(), "root")
	defer span.End()
	query := runtime.PGX()
	ctx = query.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{})
	query.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: io.ErrUnexpectedEOF})
	if len(recorder.Ended()) != 1 || recorder.Ended()[0].Parent().SpanID() != span.SpanContext().SpanID() {
		t.Fatal("missing database child span")
	}
}
