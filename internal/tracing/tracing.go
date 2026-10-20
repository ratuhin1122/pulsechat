package tracing

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"pulsechat/internal/config"
)

type Runtime struct {
	Provider trace.TracerProvider
	shutdown func(context.Context) error
}

func New(ctx context.Context, cfg config.Config, service string) (*Runtime, error) {
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(strings.TrimRight(cfg.OTLPEndpoint, "/")+"/v1/traces"), otlptracehttp.WithTimeout(5*time.Second))
	if err != nil {
		return nil, fmt.Errorf("create OTLP exporter: %w", err)
	}
	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.TraceSampleRatio))
	switch cfg.TraceSampler {
	case "always_on":
		sampler = sdktrace.AlwaysSample()
	case "always_off":
		sampler = sdktrace.NeverSample()
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", service))), sdktrace.WithSampler(sampler), sdktrace.WithBatcher(exporter))
	return &Runtime{Provider: provider, shutdown: provider.Shutdown}, nil
}

func (r *Runtime) Start(ctx context.Context, name string, options ...trace.SpanStartOption) (context.Context, trace.Span) {
	if r == nil {
		return ctx, trace.SpanFromContext(context.Background())
	}
	return r.Provider.Tracer("pulsechat").Start(ctx, name, options...)
}

func End(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "operation failed")
	}
	span.End()
}

func (r *Runtime) Close(logger *slog.Logger) {
	if r == nil || r.shutdown == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := r.shutdown(ctx); err != nil {
		logger.ErrorContext(ctx, "flush traces", "error", err)
	}
}

func Inject(ctx context.Context) propagation.MapCarrier {
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	return carrier
}
func Extract(ctx context.Context, carrier propagation.MapCarrier) context.Context {
	return propagation.TraceContext{}.Extract(ctx, carrier)
}
