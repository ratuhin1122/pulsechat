package tracing

import (
	"context"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type pgxTracer struct{ runtime *Runtime }

func (r *Runtime) PGX() pgx.QueryTracer {
	if r == nil {
		return nil
	}
	return &pgxTracer{runtime: r}
}
func (t *pgxTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	// SQL and arguments can contain credentials or message content; export neither.
	ctx, _ = t.runtime.Start(ctx, "postgres.query", trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attribute.String("db.system.name", "postgresql")))
	return ctx
}
func (t *pgxTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	End(trace.SpanFromContext(ctx), data.Err)
}
