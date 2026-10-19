package upload

import (
	"context"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"

	_ "golang.org/x/image/webp"

	"pulsechat/internal/storage"
)

type metadata struct{ width, height int32 }

func validate(ctx context.Context, disk storage.Storage, object storage.Object, expected fileType) (metadata, error) {
	if object.Size < 100 {
		return metadata{}, invalid(http.StatusBadRequest, "file must contain at least 100 bytes")
	}
	file, err := disk.Open(ctx, storage.Quarantine, object.Name)
	if err != nil {
		return metadata{}, err
	}
	defer file.Close()
	prefix := make([]byte, 64<<10)
	n, err := io.ReadFull(file, prefix)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return metadata{}, err
	}
	prefix = prefix[:n]
	if !matchesMagic(expected, prefix) {
		return metadata{}, typeMismatch()
	}
	if expected.kind == "image" {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return metadata{}, err
		}
		// DecodeConfig reads only format headers, never the image's pixel buffer.
		config, _, err := image.DecodeConfig(io.LimitReader(file, 1<<20))
		if err != nil || config.Width <= 0 || config.Height <= 0 {
			return metadata{}, invalid(http.StatusBadRequest, "invalid image header")
		}
		if uint64(config.Width)*uint64(config.Height) > MaxPixels {
			return metadata{}, invalid(http.StatusRequestEntityTooLarge, "image exceeds 50 megapixels")
		}
		return metadata{width: int32(config.Width), height: int32(config.Height)}, nil
	}
	if strings.HasPrefix(expected.mime, "application/vnd.openxmlformats") {
		if err := validateOffice(file, object.Size, expected.mime); err != nil {
			return metadata{}, err
		}
	}
	return metadata{}, nil
}
