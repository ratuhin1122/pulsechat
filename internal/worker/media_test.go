package worker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"pulsechat/internal/storage"
	"pulsechat/internal/store/db"
)

type fakeVoiceProbe struct {
	seconds float64
	err     error
	scanner *fakeScanner
	calls   int
}

func (p *fakeVoiceProbe) Duration(_ context.Context, data []byte, mime string) (float64, error) {
	p.calls++
	if p.scanner.calls != 1 || !bytes.Equal(data, p.scanner.content) {
		return 0, errors.New("probed before malware scan")
	}
	return p.seconds, p.err
}

func TestMediaAttachments(t *testing.T) {
	pdf := []byte("%PDF-1.7\n" + strings.Repeat("x", 120))
	wav := sampleWAV(1)
	bomb := sampleZIP(t, zipMember{name: "a", data: bytes.Repeat([]byte("x"), 10000), method: 8})
	for _, tt := range []struct {
		name, kind, filename, mime, threat, reason          string
		data                                                []byte
		seconds                                             float64
		probeErr                                            error
		inspect, cleanupFailure, saveFailure, updateFailure bool
	}{
		{name: "PDF moves to clean", kind: "document", filename: "a.pdf", mime: "application/pdf", data: pdf},
		{name: "ZIP stays intact by default", kind: "document", filename: "a.zip", mime: "application/zip", data: bomb},
		{name: "inspection breach rejects", kind: "document", filename: "a.zip", mime: "application/zip", data: bomb, inspect: true, reason: "compression ratio"},
		{name: "wrong document bytes", kind: "document", filename: "a.pdf", mime: "application/pdf", data: bytes.Repeat([]byte("MZ"), 100), reason: "type re-check"},
		{name: "spoofed extension", kind: "document", filename: "a.exe", mime: "application/pdf", data: pdf, reason: "type re-check"},
		{name: "wrong MIME", kind: "document", filename: "a.pdf", mime: "application/zip", data: pdf, reason: "type re-check"},
		{name: "voice moves to clean", kind: "voice", filename: "a.wav", mime: "audio/wave", data: wav, seconds: 1},
		{name: "five minute boundary", kind: "voice", filename: "a.wav", mime: "audio/wave", data: wav, seconds: 300},
		{name: "voice too long", kind: "voice", filename: "a.wav", mime: "audio/wave", data: wav, seconds: 300.01, reason: "5 minutes"},
		{name: "voice invalid duration", kind: "voice", filename: "a.wav", mime: "audio/wave", data: wav, seconds: math.NaN(), reason: "duration"},
		{name: "probe failure", kind: "voice", filename: "a.wav", mime: "audio/wave", data: wav, probeErr: errors.New("invalid audio"), reason: "probe failed"},
		{name: "voice wrong type", kind: "voice", filename: "a.wav", mime: "audio/wave", data: pdf, reason: "type re-check"},
		{name: "infected voice never probed", kind: "voice", filename: "a.wav", mime: "audio/wave", data: wav, threat: "test-virus", reason: "malware"},
		{name: "cleanup retries", kind: "document", filename: "a.pdf", mime: "application/pdf", data: pdf, cleanupFailure: true},
		{name: "save retries", kind: "document", filename: "a.pdf", mime: "application/pdf", data: pdf, saveFailure: true},
		{name: "database retries", kind: "document", filename: "a.pdf", mime: "application/pdf", data: pdf, updateFailure: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			local, err := storage.NewLocal(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer local.Close()
			original, err := local.Save(ctx, storage.Quarantine, bytes.NewReader(tt.data))
			if err != nil {
				t.Fatal(err)
			}
			disk := &mediaStorage{Storage: local, failSave: tt.saveFailure, failDelete: tt.cleanupFailure}
			scanner := &fakeScanner{result: ScanResult{Threat: tt.threat}}
			probe := &fakeVoiceProbe{seconds: tt.seconds, err: tt.probeErr, scanner: scanner}
			var id pgtype.UUID
			_ = id.Scan("550e8400-e29b-41d4-a716-446655440000")
			a := db.Attachment{ID: id, Kind: tt.kind, OriginalName: tt.filename, MimeType: tt.mime, StoredName: original.Name, Status: "pending"}
			var rejection string
			updates := 0
			repo := &testRepository{
				get: func(context.Context, pgtype.UUID) (db.Attachment, error) { return a, nil },
				reject: func(_ context.Context, p db.RejectAttachmentParams) (int64, error) {
					a.Status = "rejected"
					rejection = p.Reason
					return 1, nil
				},
				media: func(_ context.Context, p db.CompleteMediaAttachmentParams) (int64, error) {
					updates++
					if tt.updateFailure {
						return 0, errors.New("database unavailable")
					}
					if scanner.calls != 1 || p.SizeBytes != int64(len(tt.data)) {
						t.Fatal("incorrect promotion")
					}
					if tt.kind == "voice" && (!p.DurationSec.Valid || p.DurationSec.Float64 != tt.seconds) || tt.kind == "document" && p.DurationSec.Valid {
						t.Fatal("incorrect duration metadata")
					}
					f, err := local.Open(ctx, storage.Clean, p.CleanName)
					if err != nil {
						t.Fatal(err)
					}
					got, _ := io.ReadAll(f)
					f.Close()
					if !bytes.Equal(got, tt.data) {
						t.Fatal("promotion changed original content")
					}
					a.QuarantineName = pgtype.Text{String: a.StoredName, Valid: true}
					a.StoredName = p.CleanName
					a.Status = "ready"
					return 1, nil
				},
			}
			h := NewAttachmentHandler(repo, disk, scanner, slog.New(slog.NewTextHandler(io.Discard, nil)))
			h.ConfigureMedia(probe, tt.inspect)
			task, _ := NewProcessAttachmentTask(id.String())
			err = h.ProcessTask(ctx, task)
			wantRetry := tt.cleanupFailure || tt.saveFailure || tt.updateFailure
			if (err != nil) != wantRetry {
				t.Fatalf("error=%v", err)
			}
			if tt.reason != "" {
				if a.Status != "rejected" || !strings.Contains(rejection, tt.reason) || updates != 0 {
					t.Fatalf("status=%s, reason=%q, updates=%d", a.Status, rejection, updates)
				}
			} else if !wantRetry && a.Status != "ready" {
				t.Fatal("not ready")
			}
			if tt.threat != "" && probe.calls != 0 {
				t.Fatal("infected voice probed")
			}
			if tt.cleanupFailure {
				disk.failDelete = false
				if err := h.ProcessTask(ctx, task); err != nil {
					t.Fatal(err)
				}
				if updates != 1 || scanner.calls != 1 {
					t.Fatal("cleanup retry reprocessed media")
				}
			}
			f, openErr := local.Open(ctx, storage.Quarantine, original.Name)
			if openErr == nil {
				f.Close()
			}
			if tt.saveFailure || tt.updateFailure {
				if openErr != nil {
					t.Fatal("retry lost original")
				}
			} else if openErr == nil {
				t.Fatal("quarantine file retained")
			}
		})
	}
}

type mediaStorage struct {
	storage.Storage
	failSave, failDelete bool
}

func (s *mediaStorage) Save(ctx context.Context, area storage.Area, r io.Reader) (storage.Object, error) {
	if s.failSave {
		return storage.Object{}, errors.New("storage unavailable")
	}
	return s.Storage.Save(ctx, area, r)
}
func (s *mediaStorage) Delete(ctx context.Context, area storage.Area, name string) error {
	if s.failDelete && area == storage.Quarantine {
		return errors.New("cleanup unavailable")
	}
	return s.Storage.Delete(ctx, area, name)
}
