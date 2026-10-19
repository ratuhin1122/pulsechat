//go:build libvips

package worker

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
	"testing"

	"github.com/bbrks/go-blurhash"
	"golang.org/x/image/webp"
)

func TestVipsImages(t *testing.T) {
	processor, close, err := NewImageProcessor()
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	for _, tt := range []struct {
		name                  string
		input                 []byte
		width, height, tw, th int
		bad                   bool
	}{
		{name: "small PNG", input: samplePNG(t, 24, 12), width: 24, height: 12, tw: 24, th: 12},
		{name: "wide PNG", input: samplePNG(t, 2000, 100), width: 1600, height: 80, tw: 320, th: 16},
		{name: "rotated JPEG", input: orientedJPEG(t), width: 12, height: 24, tw: 12, th: 24},
		{name: "invalid", input: []byte("not an image"), bad: true},
		{name: "truncated PNG", input: samplePNG(t, 24, 12)[:40], bad: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out, err := processor.Process(context.Background(), tt.input)
			if tt.bad {
				if !errors.Is(err, ErrImageDecode) {
					t.Fatalf("error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if out.Width != tt.width || out.Height != tt.height {
				t.Fatalf("dimensions = %dx%d", out.Width, out.Height)
			}
			for _, variant := range []struct {
				data []byte
				w, h int
			}{{out.Display, tt.width, tt.height}, {out.Thumbnail, tt.tw, tt.th}} {
				img, err := webp.Decode(bytes.NewReader(variant.data))
				if err != nil {
					t.Fatal(err)
				}
				if img.Bounds().Dx() != variant.w || img.Bounds().Dy() != variant.h {
					t.Fatal("variant size incorrect")
				}
				// Walk RIFF chunks rather than matching compressed pixel bytes.
				for pos := 12; pos+8 <= len(variant.data); {
					tag := string(variant.data[pos : pos+4])
					if tag == "EXIF" || tag == "XMP " || tag == "ICCP" {
						t.Fatalf("metadata chunk %s retained", tag)
					}
					size := int(binary.LittleEndian.Uint32(variant.data[pos+4 : pos+8]))
					pos += 8 + size + (size % 2)
				}
			}
			if _, err := blurhash.Decode(out.Blurhash, 8, 8, 1); err != nil {
				t.Fatalf("invalid blurhash: %v", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := processor.Process(ctx, samplePNG(t, 8, 8)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func orientedJPEG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 24, 12)), nil); err != nil {
		t.Fatal(err)
	}
	// Little-endian TIFF IFD with orientation=6 (90 degrees clockwise).
	exif := []byte{'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
	output := append([]byte{}, buf.Bytes()[:2]...)
	output = append(output, 0xff, 0xe1, byte((len(exif)+2)>>8), byte(len(exif)+2))
	output = append(output, exif...)
	return append(output, buf.Bytes()[2:]...)
}
