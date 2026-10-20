package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

type DependencyCheck struct {
	Name string
	Ping func(context.Context) error
}

// HealthRoutes separates dependency readiness from cheap process liveness.
// Dependency checks must honor their context; the complete readiness probe has a 2s budget.
func HealthRoutes(mux *http.ServeMux, lifecycle context.Context, logger *slog.Logger, checks ...DependencyCheck) {
	respond := func(w http.ResponseWriter, r *http.Request, status int, body string) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body + "\n")); err != nil {
			logger.ErrorContext(r.Context(), "write health response", "error", err)
		}
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { respond(w, r, http.StatusOK, "ok") })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if lifecycle.Err() != nil {
			respond(w, r, http.StatusServiceUnavailable, "not ready")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		stop := context.AfterFunc(lifecycle, cancel)
		defer stop()
		for _, check := range checks {
			if err := check.Ping(ctx); err != nil {
				logger.WarnContext(r.Context(), "readiness dependency unavailable", "dependency", check.Name)
				respond(w, r, http.StatusServiceUnavailable, "not ready")
				return
			}
		}
		if ctx.Err() != nil || lifecycle.Err() != nil {
			respond(w, r, http.StatusServiceUnavailable, "not ready")
			return
		}
		respond(w, r, http.StatusOK, "ready")
	})
}
