package metrics

import (
	"net/http"
	"strconv"
	"time"
)

// Middleware resolves the ServeMux pattern before calling handlers, including rejected requests.
func (m *Metrics) Middleware(mux *http.ServeMux, next http.Handler) http.Handler {
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
		writer := &response{ResponseWriter: w}
		started := time.Now()
		defer func() {
			status := writer.status
			if status == 0 {
				status = http.StatusOK
			}
			m.duration.WithLabelValues(route, method, strconv.Itoa(status)).Observe(time.Since(started).Seconds())
		}()
		next.ServeHTTP(writer, r)
	})
}

type response struct {
	http.ResponseWriter
	status int
}

func (w *response) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *response) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	if status >= 200 || status == http.StatusSwitchingProtocols {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *response) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
