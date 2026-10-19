package upload

import (
	"bytes"
	"context"
	"errors"
	"mime"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"pulsechat/internal/storage"
	"pulsechat/internal/store/db"
)

func TestFiles(t *testing.T) {
	for _, tt := range []struct {
		name, kind, status, query, rangeHeader, method   string
		code                                             int
		body, disposition                                string
		noAuth, nonMember, missing, missingObject, badID bool
	}{
		{name: "document", kind: "document", code: 200, body: "0123456789", disposition: "attachment"},
		{name: "image display", kind: "image", query: "?variant=display", code: 200, body: "0123456789", disposition: "inline"},
		{name: "image sanitized original", kind: "image", query: "?variant=original", code: 200, body: "0123456789", disposition: "inline"},
		{name: "thumbnail", kind: "image", query: "?variant=thumb", code: 200, body: "thumb", disposition: "inline"},
		{name: "voice", kind: "voice", code: 200, body: "0123456789", disposition: "inline"},
		{name: "audio range", kind: "voice", rangeHeader: "bytes=2-5", code: 206, body: "2345", disposition: "inline"},
		{name: "audio suffix range", kind: "voice", rangeHeader: "bytes=-4", code: 206, body: "6789", disposition: "inline"},
		{name: "audio unsatisfiable range", kind: "voice", rangeHeader: "bytes=20-30", code: 416},
		{name: "HEAD", kind: "voice", method: "HEAD", code: 200, disposition: "inline"},
		{name: "authentication required", kind: "document", noAuth: true, code: 401},
		{name: "membership required", kind: "document", nonMember: true, code: 403},
		{name: "pending blocked", kind: "document", status: "pending", code: 404},
		{name: "rejected blocked", kind: "document", status: "rejected", code: 404},
		{name: "missing row", kind: "document", missing: true, code: 404},
		{name: "missing clean file", kind: "document", missingObject: true, code: 404},
		{name: "bad ID", kind: "document", badID: true, code: 400},
		{name: "invalid variant", kind: "image", query: "?variant=quarantine", code: 400},
		{name: "duplicate variant", kind: "image", query: "?variant=thumb&variant=original", code: 400},
		{name: "empty variant", kind: "image", query: "?variant=", code: 400},
		{name: "document has no thumb", kind: "document", query: "?variant=thumb", code: 404},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepository{}
			if tt.nonMember {
				repo.memberErr = pgx.ErrNoRows
			}
			if tt.missing {
				repo.fileErr = pgx.ErrNoRows
			}
			mux, token, directory := testHandler(t, repo, 1)
			disk, err := storage.NewLocal(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer disk.Close()
			object, err := disk.Save(context.Background(), storage.Clean, strings.NewReader("0123456789"))
			if err != nil {
				t.Fatal(err)
			}
			thumb, err := disk.Save(context.Background(), storage.Clean, strings.NewReader("thumb"))
			if err != nil {
				t.Fatal(err)
			}
			var id, room pgtype.UUID
			_ = id.Scan("550e8400-e29b-41d4-a716-446655440010")
			_ = room.Scan("550e8400-e29b-41d4-a716-446655440001")
			mimeType := "application/pdf"
			name := "résumé.pdf"
			if tt.kind == "image" {
				mimeType = "image/webp"
				name = "camera.jpg"
			}
			if tt.kind == "voice" {
				mimeType = "audio/ogg"
				name = "voice.ogg"
			}
			status := tt.status
			if status == "" {
				status = "ready"
			}
			repo.attachment = db.Attachment{ID: id, RoomID: room, Kind: tt.kind, Status: status, StoredName: object.Name,
				ThumbnailName: pgtype.Text{String: thumb.Name, Valid: true}, QuarantineName: pgtype.Text{String: "never-served", Valid: true}, MimeType: mimeType, OriginalName: name}
			if tt.missingObject {
				if err := disk.Delete(context.Background(), storage.Clean, object.Name); err != nil {
					t.Fatal(err)
				}
			}
			path := "/files/" + id.String()
			if tt.badID {
				path = "/files/bad"
			}
			method := tt.method
			if method == "" {
				method = "GET"
			}
			r := httptest.NewRequest(method, path+tt.query, nil)
			if !tt.noAuth {
				r.Header.Set("Authorization", "Bearer "+token)
			}
			if tt.rangeHeader != "" {
				r.Header.Set("Range", tt.rangeHeader)
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tt.code {
				t.Fatalf("status=%d,body=%s", w.Code, w.Body.String())
			}
			if w.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("missing nosniff")
			}
			if tt.code == 200 || tt.code == 206 {
				if !bytes.Equal(w.Body.Bytes(), []byte(tt.body)) || w.Header().Get("Content-Type") != mimeType {
					t.Fatalf("body=%q,type=%s", w.Body.String(), w.Header().Get("Content-Type"))
				}
				disposition, params, err := mime.ParseMediaType(w.Header().Get("Content-Disposition"))
				if err != nil || disposition != tt.disposition || params["filename"] == "" {
					t.Fatalf("disposition=%q,error=%v", disposition, err)
				}
				if tt.kind == "image" && !strings.HasSuffix(params["filename"], ".webp") {
					t.Fatal("image filename does not match cleaned MIME")
				}
				if w.Header().Get("Cache-Control") != "private, no-store" {
					t.Fatal("unsafe download caching")
				}
				if tt.code == 206 && w.Header().Get("Content-Range") == "" {
					t.Fatal("missing Content-Range")
				}
			}
		})
	}
}

func TestFileRepositoryFailure(t *testing.T) {
	repo := &testRepository{fileErr: errors.New("database unavailable")}
	mux, token, _ := testHandler(t, repo, 1)
	r := httptest.NewRequest("GET", "/files/550e8400-e29b-41d4-a716-446655440010", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 500 {
		t.Fatal(w.Code)
	}
}
