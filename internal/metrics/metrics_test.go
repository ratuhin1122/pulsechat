package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPPatterns(t *testing.T) {
	m := New()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rooms/{id}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	for _, tt := range []struct{ path, route, status string }{
		{"/rooms/private-one?token=secret", "GET /rooms/{id}", "204"},
		{"/rooms/private-two", "GET /rooms/{id}", "204"},
		{"/unknown-private", "unmatched", "404"},
	} {
		t.Run(tt.path, func(t *testing.T) {
			m.Middleware(mux, mux).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", tt.path, nil))
			w := httptest.NewRecorder()
			m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
			body := w.Body.String()
			if !strings.Contains(body, `http_request_duration_seconds_count{method="GET",route="`+tt.route+`",status="`+tt.status+`"}`) {
				t.Fatalf("missing route/status metric: %s", body)
			}
			for _, secret := range []string{"private-one", "private-two", "unknown-private", "secret"} {
				if strings.Contains(body, secret) {
					t.Fatalf("leaked path/query %q", secret)
				}
			}
		})
	}
	// No global registration or observations shared across instances.
	other := New()
	families, err := other.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() == "http_request_duration_seconds" {
			t.Fatal("shared histogram")
		}
	}
}
