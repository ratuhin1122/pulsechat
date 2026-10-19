//go:build !libvips

package worker

import "fmt"

func NewImageProcessor() (ImageProcessor, func(), error) {
	return nil, nil, fmt.Errorf("worker image processing requires a build with -tags libvips and libvips installed")
}
