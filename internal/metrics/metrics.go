package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics owns an isolated registry. Gauges describe this API instance, not Redis-wide presence.
type Metrics struct {
	Registry          *prometheus.Registry
	ActiveConnections prometheus.Gauge
	OnlineUsers       prometheus.Gauge
	Messages          *prometheus.CounterVec
	Disconnects       *prometheus.CounterVec
	Uploads           *prometheus.CounterVec
	duration          *prometheus.HistogramVec
}

func New() *Metrics {
	m := &Metrics{
		Registry:          prometheus.NewRegistry(),
		ActiveConnections: prometheus.NewGauge(prometheus.GaugeOpts{Name: "ws_active_connections", Help: "Registered local WebSocket connections."}),
		OnlineUsers:       prometheus.NewGauge(prometheus.GaugeOpts{Name: "ws_online_users", Help: "Distinct users with local WebSocket connections."}),
		Messages:          prometheus.NewCounterVec(prometheus.CounterOpts{Name: "chat_messages_total", Help: "Live chat message events delivered to this API hub, excluding history replay."}, []string{"type"}),
		Disconnects:       prometheus.NewCounterVec(prometheus.CounterOpts{Name: "ws_disconnects_total", Help: "Local WebSocket disconnects by cause."}, []string{"reason"}),
		Uploads:           prometheus.NewCounterVec(prometheus.CounterOpts{Name: "uploads_total", Help: "Uploads accepted for processing or rejected during stage-one validation."}, []string{"result"}),
		duration:          prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "http_request_duration_seconds", Help: "HTTP request duration including WebSocket lifetime.", Buckets: prometheus.DefBuckets}, []string{"route", "method", "status"}),
	}
	m.Registry.MustRegister(m.ActiveConnections, m.OnlineUsers, m.Messages, m.Disconnects, m.Uploads, m.duration)
	for _, kind := range []string{"text", "image", "document", "voice"} {
		m.Messages.WithLabelValues(kind)
	}
	for _, reason := range []string{"heartbeat_timeout", "client_close", "error"} {
		m.Disconnects.WithLabelValues(reason)
	}
	for _, result := range []string{"accepted", "rejected_stage1"} {
		m.Uploads.WithLabelValues(result)
	}
	return m
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}
