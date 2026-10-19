package upload

import (
	"net/http"
	"path/filepath"
	"strings"
	"unicode"
)

type fileType struct {
	kind, mime string
	limit      int64
}
type validationError struct {
	status  int
	message string
}

func (e *validationError) Error() string { return e.message }
func invalid(status int, message string) error {
	return &validationError{status: status, message: message}
}

func allowedType(name string) (fileType, error) {
	if len(name) == 0 || len(name) > 255 || strings.ContainsAny(name, `/\`) || strings.ContainsFunc(name, unicode.IsControl) {
		return fileType{}, invalid(http.StatusBadRequest, "invalid filename")
	}
	var kind, mime string
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg":
		kind, mime = "image", "image/jpeg"
	case ".png":
		kind, mime = "image", "image/png"
	case ".gif":
		kind, mime = "image", "image/gif"
	case ".webp":
		kind, mime = "image", "image/webp"
	case ".pdf":
		kind, mime = "document", "application/pdf"
	case ".docx":
		kind, mime = "document", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		kind, mime = "document", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".pptx":
		kind, mime = "document", "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".mp3":
		kind, mime = "voice", "audio/mpeg"
	case ".wav":
		kind, mime = "voice", "audio/wave"
	case ".ogg", ".opus":
		kind, mime = "voice", "audio/ogg"
	case ".webm":
		kind, mime = "voice", "audio/webm"
	default:
		return fileType{}, invalid(http.StatusUnsupportedMediaType, "file extension is not allowed")
	}
	limit := DocumentLimit
	if kind == "image" {
		limit = ImageLimit
	} else if kind == "voice" {
		limit = VoiceLimit
	}
	return fileType{kind: kind, mime: mime, limit: limit}, nil
}

func matchesMagic(want fileType, prefix []byte) bool {
	actual := http.DetectContentType(prefix)
	switch want.mime {
	case "audio/ogg":
		return strings.HasPrefix(string(prefix), "OggS\x00") && (strings.Contains(string(prefix), "OpusHead") || strings.Contains(string(prefix), "\x01vorbis"))
	case "audio/webm":
		return actual == "video/webm" && strings.Contains(string(prefix), "webm") && (strings.Contains(string(prefix), "A_OPUS") || strings.Contains(string(prefix), "A_VORBIS"))
	case "audio/mpeg":
		return actual == "audio/mpeg" || len(prefix) > 2 && prefix[0] == 0xff && prefix[1]&0xe0 == 0xe0 && prefix[1]&0x06 != 0
	default:
		if strings.HasPrefix(want.mime, "application/vnd.openxmlformats") {
			return actual == "application/zip"
		}
		return actual == want.mime
	}
}

func typeMismatch() error {
	return invalid(http.StatusUnsupportedMediaType, "file bytes do not match the allowed extension")
}
