//go:build libvips

package worker

import (
	"context"
	"fmt"
	"math"
	"runtime"

	"github.com/bbrks/go-blurhash"
	"github.com/davidbyttow/govips/v2/vips"
)

type vipsProcessor struct{}

// The worker owns libvips' process lifetime; close after all jobs have stopped.
func NewImageProcessor() (ImageProcessor, func(), error) {
	if err := vips.Startup(nil); err != nil {
		return nil, nil, err
	}
	return vipsProcessor{}, vips.Shutdown, nil
}

func (vipsProcessor) Process(ctx context.Context, input []byte) (out ProcessedImage, err error) {
	if err := ctx.Err(); err != nil {
		return out, err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer vips.ShutdownThread()
	image, err := vips.LoadImageFromBuffer(input, vips.NewImportParams())
	if err != nil {
		return out, fmt.Errorf("%w: %v", ErrImageDecode, err)
	}
	defer image.Close()
	if int64(image.Width())*int64(image.Height()) > 50_000_000 {
		return out, fmt.Errorf("%w: exceeds 50 megapixels", ErrImageDecode)
	}
	if err := image.AutoRotate(); err != nil {
		return out, fmt.Errorf("%w: %v", ErrImageDecode, err)
	}
	if err := fitImage(image, 1600); err != nil {
		return out, fmt.Errorf("%w: %v", ErrImageDecode, err)
	}
	params := vips.NewWebpExportParams()
	params.StripMetadata = true
	out.Display, _, err = image.ExportWebp(params)
	// Export forces libvips' lazy decode, including corrupt pixel data.
	if err != nil {
		return out, fmt.Errorf("%w: %v", ErrImageDecode, err)
	}
	out.Width, out.Height = image.Width(), image.Height()
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if err := fitImage(image, 320); err != nil {
		return out, err
	}
	out.Thumbnail, _, err = image.ExportWebp(params)
	if err != nil {
		return out, err
	}
	if err := fitImage(image, 32); err != nil {
		return out, err
	}
	pixels, err := image.ToGoImage()
	if err != nil {
		return out, err
	}
	out.Blurhash, err = blurhash.Encode(4, 3, pixels)
	if err != nil {
		return out, err
	}
	return out, ctx.Err()
}

func fitImage(image *vips.ImageRef, max int) error {
	scale := math.Min(float64(max)/float64(image.Width()), float64(max)/float64(image.Height()))
	if scale >= 1 {
		return nil
	}
	return image.Resize(scale, vips.KernelLanczos3)
}
