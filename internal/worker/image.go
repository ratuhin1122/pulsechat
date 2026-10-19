package worker

import (
	"context"
	"errors"
)

var ErrImageDecode = errors.New("image decode failed")

type ProcessedImage struct {
	Display, Thumbnail []byte
	Width, Height      int
	Blurhash           string
}

type ImageProcessor interface {
	Process(context.Context, []byte) (ProcessedImage, error)
}
