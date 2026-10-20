package tracing

import (
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func (r *Runtime) HTTP(mux *http.ServeMux, next http.Handler) http.Handler {
	pattern := func(request *http.Request) string {
		_, route := mux.Handler(request)
		if route == "" {
			return "unmatched"
		}
		return route
	}
	safe := http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		route := pattern(request)
		_, path, found := strings.Cut(route, " ")
		if !found {
			path = route
		}
		// Retain route patterns rather than identifiers in HTTP span metadata.
		trace.SpanFromContext(request.Context()).SetAttributes(attribute.String("http.route", path), attribute.String("url.path", path), attribute.String("url.query", ""))
		next.ServeHTTP(w, request)
	})
	return otelhttp.NewHandler(safe, "http request", otelhttp.WithTracerProvider(r.Provider), otelhttp.WithPropagators(propagation.TraceContext{}), otelhttp.WithSpanNameFormatter(func(_ string, request *http.Request) string { return pattern(request) }))
}
