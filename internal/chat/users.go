package chat

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/store/db"
)

func (h *Handler) searchUsers(w http.ResponseWriter, r *http.Request) {
	viewer, err := requestUser(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	if !r.URL.Query().Has("q") {
		search = strings.TrimSpace(r.URL.Query().Get("search"))
	} // Existing frontend compatibility.
	if !utf8.ValidString(search) || utf8.RuneCountInString(search) < 2 || len(search) > 64 {
		http.Error(w, "search needs at least 2 characters and at most 64 bytes", http.StatusBadRequest)
		return
	}
	users, err := h.repo.SearchUsers(r.Context(), db.SearchUsersParams{ViewerID: viewer, Search: search})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	type userResponse struct {
		ID          pgtype.UUID `json:"id"`
		Username    string      `json:"username"`
		DisplayName string      `json:"display_name"`
	}
	result := make([]userResponse, 0, len(users))
	for _, user := range users {
		result = append(result, userResponse{user.ID, user.Username, user.DisplayName})
	}
	w.Header().Set("Cache-Control", "no-store")
	h.respond(w, r, http.StatusOK, result)
}
