package ws

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func validateOrigin(origin string) error {
	if origin == "" {
		return nil
	}
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		strings.ContainsAny(u.Host, "*?[]\\") || origin != u.Scheme+"://"+u.Host {
		return fmt.Errorf("WS_ORIGIN must be an exact http(s) origin without a path or wildcard")
	}
	return nil
}

func (h *Handler) checkOrigin(r *http.Request) (string, bool) {
	origin := h.origin
	if origin == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		origin = scheme + "://" + r.Host
	}
	values := r.Header.Values("Origin")
	return origin, len(values) == 1 && values[0] == origin
}

// Browsers may pass ?access_token= because their WebSocket API cannot set an
// Authorization header. Other clients can use Authorization: Bearer <token>.
func accessToken(r *http.Request) string {
	if value := r.Header.Get("Authorization"); value != "" {
		fields := strings.Fields(value)
		if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") || len(fields[1]) > 4096 {
			return ""
		}
		return fields[1]
	}
	values := r.URL.Query()["access_token"]
	if len(values) != 1 || len(values[0]) > 4096 {
		return ""
	}
	return values[0]
}
