package upload

import (
	"errors"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"pulsechat/internal/auth"
	"pulsechat/internal/chat"
	"pulsechat/internal/logging"
	"pulsechat/internal/storage"
)

func (h *Handler) file(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	var id, user pgtype.UUID
	if id.Scan(r.PathValue("id")) != nil || !id.Valid {
		http.Error(w, "invalid file ID", http.StatusBadRequest)
		return
	}
	logging.Set(r.Context(), logging.Fields{AttachmentID: id.String()})
	userID, _ := auth.UserID(r.Context())
	if user.Scan(userID) != nil || !user.Valid {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	a, err := h.repo.GetAttachment(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
		} else {
			h.fail(w, r, err)
		}
		return
	}
	logging.Set(r.Context(), logging.Fields{RoomID: a.RoomID.String()})
	if err := chat.RequireMembership(r.Context(), h.repo, a.RoomID, user); err != nil {
		h.fail(w, r, err)
		return
	}
	if a.Status != "ready" {
		http.NotFound(w, r)
		return
	}
	variant := "original"
	if values, ok := r.URL.Query()["variant"]; ok {
		if len(values) != 1 {
			http.Error(w, "invalid variant", http.StatusBadRequest)
			return
		}
		variant = values[0]
	}
	name := a.StoredName
	switch variant {
	case "original", "display": // Image originals are replaced by sanitized WebP.
	case "thumb":
		if a.Kind != "image" || !a.ThumbnailName.Valid {
			http.NotFound(w, r)
			return
		}
		name = a.ThumbnailName.String
	default:
		http.Error(w, "invalid variant", http.StatusBadRequest)
		return
	}
	file, err := h.storage.Open(r.Context(), storage.Clean, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
		} else {
			h.fail(w, r, err)
		}
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			h.logger.ErrorContext(r.Context(), "close download", "error", err)
		}
	}()
	disposition := "inline"
	if a.Kind == "document" {
		disposition = "attachment"
	}
	filename := a.OriginalName
	if a.Kind == "image" {
		filename = strings.TrimSuffix(filename, filepath.Ext(filename)) + ".webp"
	}
	w.Header().Set("Content-Type", a.MimeType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": filename}))
	// Storage supplies a ReadSeeker; ServeContent implements HEAD, byte ranges,
	// If-Range, and 416 responses without buffering the audio in memory.
	http.ServeContent(w, r, filename, a.CreatedAt.Time, file)
}
