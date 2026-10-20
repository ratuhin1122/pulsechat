package upload

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"testing"

	dto "github.com/prometheus/client_model/go"

	"pulsechat/internal/metrics"
)

func TestUploadMetrics(t *testing.T) {
	for _, tt := range []struct {
		name, filename, result string
		queueErr               error
	}{
		{name: "accepted", filename: "document.pdf", result: "accepted"},
		{name: "invalid extension", filename: "script.exe", result: "rejected_stage1"},
		{name: "saved despite queue failure", filename: "document.pdf", result: "accepted", queueErr: errors.New("redis down")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := metrics.New()
			mux, token, _ := testHandler(t, &testRepository{queueErr: tt.queueErr}, 10, m)
			r := fileRequest(t, tt.filename, bytes.NewReader(append([]byte("%PDF-1.7\n"), make([]byte, 100)...)))
			r.Header.Set("Authorization", "Bearer "+token)
			mux.ServeHTTP(httptest.NewRecorder(), r)
			for _, result := range []string{"accepted", "rejected_stage1"} {
				value := &dto.Metric{}
				if err := m.Uploads.WithLabelValues(result).Write(value); err != nil {
					t.Fatal(err)
				}
				want := float64(0)
				if result == tt.result {
					want = 1
				}
				if value.Counter.GetValue() != want {
					t.Fatalf("%s=%v, want %v", result, value.Counter.GetValue(), want)
				}
			}
		})
	}
}
