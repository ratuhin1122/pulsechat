package upload

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFileSizeLimits(t *testing.T) {
	for _, tt := range []struct {
		name   string
		prefix []byte
		limit  int64
	}{
		{"image.gif", imageFixture(t, "gif", 1, 1), ImageLimit},
		{"document.pdf", []byte("%PDF-1.7\n"), DocumentLimit},
		{"voice.wav", []byte("RIFF\x00\x00\x00\x00WAVE"), VoiceLimit},
	} {
		for _, extra := range []int64{0, 1} {
			t.Run(tt.name+"/"+string(rune('0'+extra)), func(t *testing.T) {
				repo := &testRepository{}
				mux, token, dir := testHandler(t, repo, 10)
				r := fileRequest(t, tt.name, sizedFile(tt.prefix, tt.limit+extra))
				r.Header.Set("Authorization", "Bearer "+token)
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				want := 202
				if extra != 0 {
					want = 413
				}
				if w.Code != want {
					t.Fatalf("status = %d: %s", w.Code, w.Body.String())
				}
				files, err := os.ReadDir(filepath.Join(dir, "quarantine"))
				if err != nil {
					t.Fatal(err)
				}
				if extra != 0 && (len(files) != 0 || len(repo.inserted) != 0) {
					t.Fatal("oversized file was retained")
				}
			})
		}
	}
}

func TestMalformedMultipart(t *testing.T) {
	for _, mode := range []string{"multiple files", "wrong field", "truncated", "not multipart"} {
		t.Run(mode, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			field := "file"
			if mode == "wrong field" {
				field = "unexpected"
			}
			part, err := writer.CreateFormFile(field, "document.pdf")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(part, "%PDF-1.7\n"+string(bytes.Repeat([]byte("x"), 100))); err != nil {
				t.Fatal(err)
			}
			if mode == "multiple files" {
				if _, err := writer.CreateFormFile("file", "second.pdf"); err != nil {
					t.Fatal(err)
				}
			}
			if mode != "truncated" {
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
			}
			repo := &testRepository{}
			mux, token, dir := testHandler(t, repo, 10)
			r := httptest.NewRequest("POST", "/rooms/550e8400-e29b-41d4-a716-446655440001/upload", &body)
			if mode != "not multipart" {
				r.Header.Set("Content-Type", writer.FormDataContentType())
			}
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != 400 {
				t.Fatalf("status = %d: %s", w.Code, w.Body.String())
			}
			files, err := os.ReadDir(filepath.Join(dir, "quarantine"))
			if err != nil || len(files) != 0 || len(repo.inserted) != 0 {
				t.Fatal("malformed upload retained state")
			}
		})
	}
}
