package worker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"pulsechat/internal/storage"
	"pulsechat/internal/store/db"
)

type fakeScanner struct {
	result    ScanResult
	err       error
	calls     int
	content   []byte
	afterRead func()
}

func (s *fakeScanner) Scan(_ context.Context, reader io.Reader) (ScanResult, error) {
	s.calls++
	s.content, _ = io.ReadAll(reader)
	if s.afterRead != nil {
		s.afterRead()
	}
	return s.result, s.err
}

type fakeFile struct {
	*bytes.Reader
	closeErr error
}

func (f fakeFile) Close() error { return f.closeErr }

type fakeStorage struct {
	storage.Storage
	openErr, closeErr, deleteErr error
	opens, deletes               int
}

func (s *fakeStorage) Open(_ context.Context, area storage.Area, _ string) (storage.File, error) {
	s.opens++
	if area != storage.Quarantine {
		return nil, errors.New("only quarantine may be scanned")
	}
	if s.openErr != nil {
		return nil, s.openErr
	}
	return fakeFile{Reader: bytes.NewReader([]byte("%PDF-1.7\n" + strings.Repeat("x", 120))), closeErr: s.closeErr}, nil
}

func (s *fakeStorage) Save(_ context.Context, area storage.Area, source io.Reader) (storage.Object, error) {
	if area != storage.Clean {
		return storage.Object{}, errors.New("expected clean destination")
	}
	n, err := io.Copy(io.Discard, source)
	return storage.Object{Name: "550e8400-e29b-41d4-a716-446655440001", Size: n}, err
}

func (s *fakeStorage) Delete(ctx context.Context, area storage.Area, _ string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if area != storage.Quarantine {
		return errors.New("only quarantine may be deleted")
	}
	s.deletes++
	return s.deleteErr
}

func TestAttachmentScanning(t *testing.T) {
	for _, tt := range []struct {
		name, threat, reason                             string
		scanErr, openErr, closeErr, rejectErr, deleteErr error
		wantReady, wantErr                               bool
	}{
		{name: "clean file", wantReady: true},
		{name: "infected", threat: "Eicar-Signature", reason: "malware detected: Eicar-Signature"},
		{name: "scan failure", scanErr: errors.New("clamd unavailable"), reason: "scan failed: clamd unavailable"},
		{name: "scan timeout", scanErr: context.DeadlineExceeded, reason: "scan failed: context deadline exceeded"},
		{name: "missing file", openErr: errors.New("file missing"), reason: "scan failed: open quarantine: file missing"},
		{name: "file close failure", closeErr: errors.New("close failed"), reason: "scan failed: close failed"},
		{name: "rejection database failure retries", threat: "Eicar-Signature", reason: "malware detected:", rejectErr: errors.New("database unavailable"), wantErr: true},
		{name: "deletion failure retries", threat: "Eicar-Signature", reason: "malware detected:", deleteErr: errors.New("disk unavailable"), wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scanner := &fakeScanner{result: ScanResult{Threat: tt.threat}, err: tt.scanErr}
			disk := &fakeStorage{openErr: tt.openErr, closeErr: tt.closeErr, deleteErr: tt.deleteErr}
			ready, rejected := false, false
			repo := &testRepository{
				get: func(_ context.Context, id pgtype.UUID) (db.Attachment, error) {
					return db.Attachment{ID: id, Status: "pending", StoredName: id.String(), Kind: "document", OriginalName: "sample.pdf", MimeType: "application/pdf"}, nil
				},
				mark: func(context.Context, pgtype.UUID) (int64, error) {
					if scanner.calls != 1 || scanner.result.Threat != "" || scanner.err != nil {
						t.Error("processing happened before a clean scan")
					}
					ready = true
					return 1, nil
				},
				reject: func(ctx context.Context, p db.RejectAttachmentParams) (int64, error) {
					rejected = true
					if ctx.Err() != nil || !strings.Contains(p.Reason, tt.reason) {
						t.Errorf("rejection reason = %q, context error = %v", p.Reason, ctx.Err())
					}
					return 1, tt.rejectErr
				},
			}
			task, err := NewProcessAttachmentTask("550e8400-e29b-41d4-a716-446655440000")
			if err != nil {
				t.Fatal(err)
			}
			h := NewAttachmentHandler(repo, disk, scanner, slog.New(slog.NewTextHandler(io.Discard, nil)))
			err = h.ProcessTask(context.Background(), task)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ProcessTask error = %v", err)
			}
			if ready != tt.wantReady || rejected == tt.wantReady {
				t.Fatalf("ready=%v, rejected=%v", ready, rejected)
			}
			if !tt.wantReady && disk.deletes != 1 {
				t.Error("rejected file was not deleted")
			}
			if tt.wantReady && disk.deletes != 1 {
				t.Error("quarantine original was not deleted after promotion")
			}
			if tt.openErr == nil && string(scanner.content) != "%PDF-1.7\n"+strings.Repeat("x", 120) {
				t.Error("scanner did not receive quarantined content")
			}
		})
	}
}

func TestRejectedAttachmentRetriesDeletion(t *testing.T) {
	disk, scanner := &fakeStorage{}, &fakeScanner{}
	repo := &testRepository{get: func(_ context.Context, id pgtype.UUID) (db.Attachment, error) {
		return db.Attachment{ID: id, Status: "rejected", StoredName: id.String()}, nil
	}}
	h := NewAttachmentHandler(repo, disk, scanner, slog.New(slog.NewTextHandler(io.Discard, nil)))
	task, err := NewProcessAttachmentTask("550e8400-e29b-41d4-a716-446655440000")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.ProcessTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if disk.deletes != 1 || scanner.calls != 0 {
		t.Fatal("rejected retry must only perform deletion")
	}
}

func TestScanCancellationStillRejects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	disk := &fakeStorage{}
	scanner := &fakeScanner{afterRead: cancel, err: context.Canceled}
	rejected := false
	repo := &testRepository{
		get: func(_ context.Context, id pgtype.UUID) (db.Attachment, error) {
			return db.Attachment{ID: id, Status: "pending", StoredName: id.String()}, nil
		},
		reject: func(cleanup context.Context, p db.RejectAttachmentParams) (int64, error) {
			if cleanup.Err() != nil || !strings.Contains(p.Reason, "context canceled") {
				t.Error("scan cancellation prevented rejection recording")
			}
			rejected = true
			return 1, nil
		},
	}
	task, err := NewProcessAttachmentTask("550e8400-e29b-41d4-a716-446655440000")
	if err != nil {
		t.Fatal(err)
	}
	h := NewAttachmentHandler(repo, disk, scanner, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := h.ProcessTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if !rejected || disk.deletes != 1 {
		t.Fatal("canceled scan did not reject and delete the file")
	}
}
