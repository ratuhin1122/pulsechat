package ws

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"pulsechat/internal/metrics"
)

func metricValue(t *testing.T, m prometheus.Metric) float64 {
	t.Helper()
	value := &dto.Metric{}
	if err := m.Write(value); err != nil {
		t.Fatal(err)
	}
	if value.Gauge != nil {
		return value.Gauge.GetValue()
	}
	return value.Counter.GetValue()
}

func TestHubMetrics(t *testing.T) {
	for _, tt := range []struct {
		reason string
		code   int32
	}{{"error", 0}, {"client_close", 1}, {"heartbeat_timeout", 2}} {
		t.Run(tt.reason, func(t *testing.T) {
			m := metrics.New()
			hub := NewHub().WithMetrics(m)
			defer hub.Close()
			a, b := newClient("alice", 2), newClient("alice", 2)
			hub.register(a)
			hub.register(a)
			hub.register(b)
			if metricValue(t, m.ActiveConnections) != 2 || metricValue(t, m.OnlineUsers) != 1 {
				t.Fatal("multi-tab connection/user counts")
			}
			hub.Broadcast("room", []string{"alice"}, []byte(`{"type":"message","message_type":"image"}`))
			if metricValue(t, m.Messages.WithLabelValues("image")) != 1 {
				t.Fatal("message counted per recipient")
			}
			a.disconnect.Store(tt.code)
			hub.unregister(a)
			hub.unregister(a)
			if metricValue(t, m.ActiveConnections) != 1 || metricValue(t, m.OnlineUsers) != 1 {
				t.Fatal("first tab disconnected user")
			}
			if metricValue(t, m.Disconnects.WithLabelValues(tt.reason)) != 1 {
				t.Fatal("disconnect duplicated or misclassified")
			}
			hub.unregister(b)
			if metricValue(t, m.ActiveConnections) != 0 || metricValue(t, m.OnlineUsers) != 0 {
				t.Fatal("gauge leak")
			}
		})
	}
}

func TestHeartbeatMetricReason(t *testing.T) {
	h, _ := newTestHandler(t, &testRepository{}, "")
	c := newClient("alice", 1)
	socket := &heartbeatSocket{ping: func(context.Context) error { return context.DeadlineExceeded }}
	if err := h.heartbeat(context.Background(), socket, c); err == nil || c.disconnect.Load() != 2 {
		t.Fatal("heartbeat timeout not classified")
	}
}
