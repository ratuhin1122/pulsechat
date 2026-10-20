package httpserver

import (
	"crypto/rand"
	"net/http"

	"pulsechat/internal/logging"
)

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !logging.ValidID(id) {
			id = rand.Text()
		}
		ctx := logging.WithFields(r.Context(), logging.Fields{RequestID: id})
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
