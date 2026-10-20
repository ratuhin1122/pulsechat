package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Worker owns a registry independent of the API registry.
type Worker struct {
	Registry   *prometheus.Registry
	Processed  *prometheus.CounterVec
	Processing *prometheus.HistogramVec
	Scan       prometheus.Histogram
	ScanErrors prometheus.Counter
	QueueDepth *prometheus.GaugeVec
	Bytes      *prometheus.CounterVec
}

func NewWorker() *Worker {
	m := &Worker{
		Registry:   prometheus.NewRegistry(),
		Processed:  prometheus.NewCounterVec(prometheus.CounterOpts{Name: "attachments_processed_total", Help: "Attachment processing attempts by kind and outcome; terminal cleanup/notification retries are excluded."}, []string{"kind", "result"}),
		Processing: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "attachment_processing_seconds", Help: "Attachment processing attempt duration, excluding completion notifications.", Buckets: prometheus.ExponentialBuckets(0.05, 2, 13)}, []string{"kind"}),
		ScanErrors: prometheus.NewCounter(prometheus.CounterOpts{Name: "clamav_scan_errors_total", Help: "Failed ClamAV scan attempts; excludes clean scans, infections and non-scan processing errors."}),
		Scan:       prometheus.NewHistogram(prometheus.HistogramOpts{Name: "clamav_scan_seconds", Help: "ClamAV scan duration, including failed scans.", Buckets: prometheus.ExponentialBuckets(0.05, 2, 13)}),
		QueueDepth: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "queue_depth", Help: "Tasks waiting for execution (pending, scheduled, retry and aggregating); excludes active and archived tasks."}, []string{"queue"}),
		Bytes:      prometheus.NewCounterVec(prometheus.CounterOpts{Name: "attachment_bytes_total", Help: "Original bytes entering processing attempts and committed clean output bytes (including image thumbnails)."}, []string{"stage"}),
	}
	m.Registry.MustRegister(m.Processed, m.Processing, m.Scan, m.ScanErrors, m.QueueDepth, m.Bytes)
	for _, kind := range []string{"image", "document", "voice", "unknown"} {
		for _, result := range []string{"ready", "rejected", "infected", "error"} {
			m.Processed.WithLabelValues(kind, result)
		}
	}
	for _, stage := range []string{"original", "processed"} {
		m.Bytes.WithLabelValues(stage)
	}
	return m
}

func (m *Worker) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}
