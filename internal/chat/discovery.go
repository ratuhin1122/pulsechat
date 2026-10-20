package chat

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"pulsechat/internal/store/db"
)

func (h *Handler) publicRooms(w http.ResponseWriter, r *http.Request) {
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	if !utf8.ValidString(search) || len(search) > 100 {
		http.Error(w, "q must be at most 100 bytes", 400)
		return
	}
	limit, offset, err := pagination(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	rooms, err := h.repo.ListPublicRooms(r.Context(), db.ListPublicRoomsParams{Search: search, PageLimit: limit, PageOffset: offset})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	result := make([]roomResponse, 0, len(rooms))
	for _, room := range rooms {
		result = append(result, roomView(room))
	}
	h.respond(w, r, http.StatusOK, result)
}
