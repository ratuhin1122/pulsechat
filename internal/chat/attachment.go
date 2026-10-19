package chat

import (
	"github.com/jackc/pgx/v5/pgtype"
	"pulsechat/internal/store/db"
)

// AttachmentResponse deliberately excludes storage names and quarantine paths.
type AttachmentResponse struct {
	ID           pgtype.UUID `json:"id"`
	Kind         string      `json:"kind"`
	OriginalName string      `json:"original_name"`
	MimeType     string      `json:"mime_type"`
	SizeBytes    int64       `json:"size_bytes"`
	Width        *int32      `json:"width,omitempty"`
	Height       *int32      `json:"height,omitempty"`
	DurationSec  *float64    `json:"duration_sec,omitempty"`
	Blurhash     string      `json:"blurhash,omitempty"`
}

func Attachment(a db.Attachment) AttachmentResponse {
	r := AttachmentResponse{ID: a.ID, Kind: a.Kind, OriginalName: a.OriginalName, MimeType: a.MimeType, SizeBytes: a.SizeBytes, Blurhash: a.Blurhash.String}
	if a.Width.Valid {
		r.Width = &a.Width.Int32
	}
	if a.Height.Valid {
		r.Height = &a.Height.Int32
	}
	if a.DurationSec.Valid {
		r.DurationSec = &a.DurationSec.Float64
	}
	return r
}
