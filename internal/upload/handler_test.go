package upload

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestUploads(t *testing.T) {
	pdf := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte("x"), 91)...)
	for _, tt := range []struct {
		name, filename, kind, mime string
		data                       []byte
		status                     int
		memberErr, insertErr       error
		noAuth                     bool
	}{
		{name: "PNG ignores claimed content type", filename: "photo.png", kind: "image", mime: "image/png", data: imageFixture(t, "png", 16, 16), status: 202},
		{name: "JPEG uppercase extension", filename: "photo.JPG", kind: "image", mime: "image/jpeg", data: imageFixture(t, "jpeg", 16, 16), status: 202},
		{name: "PDF exactly 100 bytes", filename: "document.pdf", kind: "document", mime: "application/pdf", data: pdf, status: 202},
		{name: "DOCX structure", filename: "document.docx", kind: "document", mime: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", data: officeFixture(t, nil), status: 202},
		{name: "wave voice", filename: "voice.wav", kind: "voice", mime: "audio/wave", data: append([]byte("RIFF\x64\x00\x00\x00WAVE"), make([]byte, 100)...), status: 202},
		{name: "too small", filename: "small.pdf", data: pdf[:99], status: 400},
		{name: "spoofed extension", filename: "photo.png", data: pdf, status: 415},
		{name: "renamed executable", filename: "photo.png", data: append([]byte("MZ"), make([]byte, 100)...), status: 415},
		{name: "ELF executable", filename: "document.pdf", data: append([]byte("\x7fELF"), make([]byte, 100)...), status: 415},
		{name: "renamed shell script", filename: "document.pdf", data: append([]byte("#!/bin/sh\n"), make([]byte, 100)...), status: 415},
		{name: "script extension", filename: "photo.js", data: imageFixture(t, "png", 16, 16), status: 415},
		{name: "SVG blocked", filename: "image.svg", data: []byte(`<svg><script>alert(1)</script></svg>`), status: 415},
		{name: "executable extension", filename: "document.exe", data: pdf, status: 415},
		{name: "oversized image header", filename: "image.gif", data: imageFixture(t, "gif", 10000, 5001), status: 413},
		{name: "exactly 50 megapixels header", filename: "image.gif", kind: "image", mime: "image/gif", data: imageFixture(t, "gif", 10000, 5000), status: 202},
		{name: "invalid image header", filename: "image.png", data: append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 100)...), status: 400},
		{name: "office embedded script", filename: "document.docx", data: officeFixture(t, map[string]string{"word/evil.js": "alert(1)"}), status: 415},
		{name: "office wrong extension", filename: "document.xlsx", data: officeFixture(t, nil), status: 415},
		{name: "office embedded executable", filename: "document.docx", data: officeFixture(t, map[string]string{"word/evil.exe": "MZ"}), status: 415},
		{name: "membership denied", filename: "document.pdf", data: pdf, memberErr: pgx.ErrNoRows, status: 403},
		{name: "unauthenticated", filename: "document.pdf", data: pdf, noAuth: true, status: 401},
		{name: "database error cleans quarantine", filename: "document.pdf", data: pdf, insertErr: errors.New("database unavailable"), status: 500},
		{name: "membership lost before insert", filename: "document.pdf", data: pdf, insertErr: pgx.ErrNoRows, status: 403},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepository{memberErr: tt.memberErr, insertErr: tt.insertErr}
			mux, token, dir := testHandler(t, repo, 10)
			r := fileRequest(t, tt.filename, bytes.NewReader(tt.data))
			if !tt.noAuth {
				r.Header.Set("Authorization", "Bearer "+token)
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.status, w.Body.String())
			}
			files, err := os.ReadDir(filepath.Join(dir, "quarantine"))
			if err != nil {
				t.Fatal(err)
			}
			if tt.status != 202 {
				if len(repo.enqueued) != 0 {
					t.Error("invalid upload was enqueued")
				}
				if len(files) != 0 {
					t.Error("rejected upload left files behind")
				}
				if tt.insertErr == nil && len(repo.inserted) != 0 {
					t.Error("invalid upload reached database insert")
				}
				return
			}
			if len(files) != 1 || len(repo.inserted) != 1 {
				t.Fatalf("files = %d, inserts = %d", len(files), len(repo.inserted))
			}
			p := repo.inserted[0]
			if len(repo.enqueued) != 1 || repo.enqueued[0] != p.RoomID.String() {
				t.Error("attachment ID was not enqueued")
			}
			var storedID pgtype.UUID
			if storedID.Scan(p.StoredName) != nil || p.StoredName == tt.filename || p.StoredName != files[0].Name() {
				t.Error("stored name is not a random UUID")
			}
			if p.Kind != tt.kind || p.MimeType != tt.mime || p.SizeBytes != int64(len(tt.data)) || p.OriginalName != tt.filename {
				t.Fatalf("wrong attachment metadata: %+v", p)
			}
			if p.SenderID.String() != "550e8400-e29b-41d4-a716-446655440000" || p.RoomID.String() != "550e8400-e29b-41d4-a716-446655440001" {
				t.Error("wrong room or sender")
			}
			if p.Kind == "image" && (!p.Width.Valid || !p.Height.Valid) {
				t.Error("image dimensions not stored")
			}
			var response struct {
				Status    string `json:"status"`
				MessageID string `json:"message_id"`
			}
			if json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Status != "pending" || response.MessageID != "" {
				t.Fatalf("wrong response: %s", w.Body.String())
			}
			clean, err := os.ReadDir(filepath.Join(dir, "clean"))
			if err != nil || len(clean) != 0 {
				t.Error("upload escaped quarantine")
			}
		})
	}
}
