package worker

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"pulsechat/internal/storage"
	"pulsechat/internal/store/db"
)

func samplePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type fakeImages struct {
	scanner *fakeScanner
	err     error
	calls   int
}

func (p *fakeImages) Process(ctx context.Context, input []byte) (ProcessedImage, error) {
	p.calls++
	if p.scanner.calls != 1 || !bytes.Equal(input, p.scanner.content) {
		return ProcessedImage{}, errors.New("image processed before scan")
	}
	if _, err := png.Decode(bytes.NewReader(input)); err != nil {
		return ProcessedImage{}, err
	}
	return ProcessedImage{Display: []byte("display"), Thumbnail: []byte("thumbnail"), Width: 8, Height: 4, Blurhash: "test-hash"}, p.err
}

func TestImageAttachment(t *testing.T) {
	for _, tt := range []struct {
		name                   string
		decodeErr, databaseErr error
		threat                 string
		deleteFails            bool
	}{
		{name: "clean image"},
		{name: "decode rejection", decodeErr: ErrImageDecode},
		{name: "infected image", threat: "test-virus"},
		{name: "database error retries", databaseErr: errors.New("database unavailable")},
		{name: "quarantine cleanup retries", deleteFails: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			local, err := storage.NewLocal(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer local.Close()
			original, err := local.Save(ctx, storage.Quarantine, bytes.NewReader(samplePNG(t, 8, 4)))
			if err != nil {
				t.Fatal(err)
			}
			disk := &cleanupStorage{Storage: local, fail: tt.deleteFails}
			scanner := &fakeScanner{result: ScanResult{Threat: tt.threat}}
			processor := &fakeImages{scanner: scanner, err: tt.decodeErr}
			var id pgtype.UUID
			_ = id.Scan("550e8400-e29b-41d4-a716-446655440000")
			a := db.Attachment{ID: id, Kind: "image", StoredName: original.Name, Status: "pending"}
			repo := &testRepository{
				get: func(context.Context, pgtype.UUID) (db.Attachment, error) { return a, nil },
				reject: func(_ context.Context, p db.RejectAttachmentParams) (int64, error) {
					a.Status = "rejected"
					return 1, nil
				},
				complete: func(_ context.Context, p db.CompleteImageAttachmentParams) (int64, error) {
					if tt.databaseErr != nil {
						return 0, tt.databaseErr
					}
					if p.Width != 8 || p.Height != 4 || p.SizeBytes != 7 || p.Blurhash != "test-hash" {
						t.Errorf("bad metadata: %+v", p)
					}
					for name, want := range map[string]string{p.DisplayName: "display", p.ThumbnailName: "thumbnail"} {
						f, err := local.Open(ctx, storage.Clean, name)
						if err != nil {
							t.Fatal(err)
						}
						data, _ := io.ReadAll(f)
						f.Close()
						if string(data) != want {
							t.Fatal("wrong clean content")
						}
					}
					a.QuarantineName = pgtype.Text{String: a.StoredName, Valid: true}
					a.StoredName = p.DisplayName
					a.Status = "ready"
					return 1, nil
				},
			}
			h := NewAttachmentHandler(repo, disk, scanner, slog.New(slog.NewTextHandler(io.Discard, nil)), processor)
			task, _ := NewProcessAttachmentTask(id.String())
			err = h.ProcessTask(ctx, task)
			if (err != nil) != (tt.databaseErr != nil || tt.deleteFails) {
				t.Fatalf("error = %v", err)
			}
			if tt.threat != "" && processor.calls != 0 {
				t.Fatal("infected image processed")
			}
			if tt.decodeErr != nil || tt.threat != "" {
				if a.Status != "rejected" {
					t.Fatal("not rejected")
				}
			}
			if tt.deleteFails {
				disk.fail = false
				if err := h.ProcessTask(ctx, task); err != nil {
					t.Fatal(err)
				}
				if processor.calls != 1 {
					t.Fatal("cleanup retry reprocessed image")
				}
			}
			f, openErr := local.Open(ctx, storage.Quarantine, original.Name)
			if openErr == nil {
				f.Close()
			}
			if tt.databaseErr == nil && openErr == nil {
				t.Fatal("quarantine original retained")
			}
			if tt.databaseErr != nil && openErr != nil {
				t.Fatal("retry lost original")
			}
		})
	}
}

type cleanupStorage struct {
	storage.Storage
	fail bool
}

func (s *cleanupStorage) Delete(ctx context.Context, area storage.Area, name string) error {
	if area == storage.Quarantine && s.fail {
		return errors.New("delete unavailable")
	}
	return s.Storage.Delete(ctx, area, name)
}
