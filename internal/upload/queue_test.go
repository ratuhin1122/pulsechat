package upload

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestQueueFailurePreservesPendingUpload(t *testing.T) {
	repo := &testRepository{queueErr: errors.New("redis unavailable")}
	mux, token, dir := testHandler(t, repo, 10)
	r := fileRequest(t, "document.pdf", bytes.NewReader(append([]byte("%PDF-1.7\n"), make([]byte, 100)...)))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	files, err := os.ReadDir(filepath.Join(dir, "quarantine"))
	if err != nil || len(files) != 1 || len(repo.inserted) != 1 || len(repo.enqueued) != 1 {
		t.Fatalf("pending upload lost after enqueue failure: files=%d, inserts=%d, attempts=%d, error=%v", len(files), len(repo.inserted), len(repo.enqueued), err)
	}
}
