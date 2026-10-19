package upload

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/auth"
	"pulsechat/internal/chat"
	"pulsechat/internal/storage"
	"pulsechat/internal/store/db"
)

type Repository interface {
	chat.MembershipReader
	CreatePendingUpload(context.Context, db.CreatePendingUploadParams) (db.Attachment, error)
}

type Handler struct {
	repo    Repository
	storage storage.Storage
	limiter RateLimiter
	logger  *slog.Logger
}

func New(repo Repository, disk storage.Storage, limiter RateLimiter, logger *slog.Logger) *Handler {
	return &Handler{repo: repo, storage: disk, limiter: limiter, logger: logger}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, tokens *auth.Tokens) {
	mux.Handle("POST /rooms/{roomID}/upload", tokens.Middleware(http.HandlerFunc(h.upload)))
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	var room, user pgtype.UUID
	if err := room.Scan(r.PathValue("roomID")); err != nil || !room.Valid {
		http.Error(w, "invalid room ID", http.StatusBadRequest)
		return
	}
	userID, _ := auth.UserID(r.Context())
	if err := user.Scan(userID); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := chat.RequireMembership(r.Context(), h.repo, room, user); err != nil {
		h.fail(w, r, err)
		return
	}
	allowed, retry, err := h.limiter.Allow(r.Context(), user.String())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", strconv.FormatInt(max(1, int64((retry+time.Second-1)/time.Second)), 10))
		http.Error(w, "upload rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	object, expected, original, err := h.receive(w, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
		defer cancel()
		if err := h.storage.Delete(ctx, storage.Quarantine, object.Name); err != nil {
			h.logger.ErrorContext(ctx, "remove rejected upload", "error", err)
		}
	}()
	meta, err := validate(r.Context(), h.storage, object, expected)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	attachment, err := h.repo.CreatePendingUpload(r.Context(), db.CreatePendingUploadParams{
		RoomID: room, SenderID: user, Kind: expected.kind, OriginalName: original,
		StoredName: object.Name, MimeType: expected.mime, SizeBytes: object.Size,
		Width: pgtype.Int4{Int32: meta.width, Valid: meta.width > 0}, Height: pgtype.Int4{Int32: meta.height, Valid: meta.height > 0},
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	committed = true
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(struct {
		ID        pgtype.UUID `json:"id"`
		MessageID pgtype.UUID `json:"message_id"`
		Kind      string      `json:"kind"`
		Status    string      `json:"status"`
	}{attachment.ID, attachment.MessageID, attachment.Kind, attachment.Status}); err != nil {
		h.logger.ErrorContext(r.Context(), "write upload response", "error", err)
	}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var invalidFile *validationError
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &invalidFile):
		http.Error(w, invalidFile.message, invalidFile.status)
	case errors.As(err, &tooLarge):
		http.Error(w, "upload exceeds size limit", http.StatusRequestEntityTooLarge)
	case errors.Is(err, io.ErrUnexpectedEOF):
		http.Error(w, "truncated multipart body", http.StatusBadRequest)
	case errors.Is(err, chat.ErrNotMember), errors.Is(err, pgx.ErrNoRows):
		http.Error(w, "room membership required", http.StatusForbidden)
	default:
		h.logger.ErrorContext(r.Context(), "upload failed", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func (h *Handler) receive(w http.ResponseWriter, r *http.Request) (object storage.Object, expected fileType, name string, err error) {
	r.Body = http.MaxBytesReader(w, r.Body, DocumentLimit+(64<<10))
	reader, err := r.MultipartReader()
	if err != nil {
		return object, expected, name, invalid(http.StatusBadRequest, "expected multipart file upload")
	}
	part, err := reader.NextRawPart()
	if err != nil {
		return object, expected, name, multipartError(err)
	}
	if part.FormName() != "file" || part.FileName() == "" || part.Header.Get("Content-Transfer-Encoding") != "" {
		return object, expected, name, invalid(http.StatusBadRequest, "expected one file field")
	}
	name = part.FileName()
	expected, err = allowedType(name)
	if err != nil {
		return object, expected, name, err
	}
	object, err = h.storage.Save(r.Context(), storage.Quarantine, http.MaxBytesReader(w, part, expected.limit))
	if err != nil {
		return object, expected, name, err
	}
	defer func() {
		if err != nil {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			defer cancel()
			if deleteErr := h.storage.Delete(ctx, storage.Quarantine, object.Name); deleteErr != nil {
				h.logger.ErrorContext(ctx, "remove invalid multipart upload", "error", deleteErr)
			}
		}
	}()
	_, err = reader.NextRawPart()
	if err != io.EOF {
		if err == nil {
			err = invalid(http.StatusBadRequest, "only one file is allowed")
		} else {
			err = multipartError(err)
		}
		return
	}
	_, err = io.Copy(io.Discard, r.Body)
	return
}

func multipartError(err error) error {
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		return err
	}
	return invalid(http.StatusBadRequest, "invalid multipart body")
}
