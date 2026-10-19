package httpserver

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Monitor struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	active   prometheus.Gauge
	logger   *slog.Logger
}

func NewMonitor(logger *slog.Logger) (*Monitor, error) {
	m := &Monitor{
		registry: prometheus.NewRegistry(), logger: logger,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pulsechat_http_requests_total", Help: "Completed HTTP requests (WebSockets count on disconnect).",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "pulsechat_http_request_duration_seconds", Help: "HTTP request duration, including WebSocket lifetime.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
		active: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "pulsechat_http_requests_in_flight", Help: "Active HTTP requests, including WebSocket connections.",
		}),
	}
	for _, collector := range []prometheus.Collector{m.requests, m.duration, m.active, collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})} {
		if err := m.registry.Register(collector); err != nil {
			return nil, fmt.Errorf("register metrics: %w", err)
		}
	}
	return m, nil
}

func (m *Monitor) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{EnableOpenMetrics: true})
}

func (m *Monitor) Wrap(mux *http.ServeMux, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, route := mux.Handler(r)
		if route == "" {
			route = "unmatched"
		}
		method := r.Method
		switch method {
		case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "CONNECT", "TRACE":
		default:
			method = "OTHER"
		}
		started := time.Now()
		writer := &response{ResponseWriter: w}
		m.active.Inc()
		defer func() {
			m.active.Dec()
			status := writer.status
			if status == 0 {
				status = http.StatusOK
			}
			elapsed := time.Since(started)
			m.requests.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
			m.duration.WithLabelValues(method, route).Observe(elapsed.Seconds())
			// Never log queries, credentials, request bodies, or raw identifiers.
			m.logger.InfoContext(r.Context(), "http request", "method", method, "route", route,
				"status", status, "bytes", writer.bytes, "duration", elapsed)
		}()
		next.ServeHTTP(writer, r)
	})
}
