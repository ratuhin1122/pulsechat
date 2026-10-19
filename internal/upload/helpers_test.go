package upload

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/jackc/pgx/v5/pgtype"
	"image"
	"image/color/palette"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"pulsechat/internal/auth"
	"pulsechat/internal/storage"
	"pulsechat/internal/store/db"
)

type testRepository struct {
	memberErr, insertErr error
	inserted             []db.CreatePendingUploadParams
	enqueued             []string
	queueErr             error
	attachment           db.Attachment
	fileErr              error
}

func (s *testRepository) GetAttachment(_ context.Context, _ pgtype.UUID) (db.Attachment, error) {
	return s.attachment, s.fileErr
}

func (s *testRepository) EnqueueAttachment(_ context.Context, id string) error {
	if len(s.inserted) == 0 {
		return fmt.Errorf("enqueued before attachment insert")
	}
	s.enqueued = append(s.enqueued, id)
	return s.queueErr
}

func (s *testRepository) GetRoomMember(_ context.Context, p db.GetRoomMemberParams) (db.RoomMember, error) {
	return db.RoomMember{RoomID: p.RoomID, UserID: p.UserID}, s.memberErr
}
func (s *testRepository) CreatePendingUpload(_ context.Context, p db.CreatePendingUploadParams) (db.Attachment, error) {
	s.inserted = append(s.inserted, p)
	return db.Attachment{ID: p.RoomID, RoomID: p.RoomID, SenderID: p.SenderID, Kind: p.Kind, Status: "pending"}, s.insertErr
}

func testHandler(t *testing.T, repo *testRepository, limit int) (*http.ServeMux, string, string) {
	t.Helper()
	dir := t.TempDir()
	disk, err := storage.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { disk.Close() })
	limiter, err := NewMemoryLimiter(limit, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := auth.NewTokens(strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	token, err := tokens.Issue("550e8400-e29b-41d4-a716-446655440000")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	New(repo, disk, limiter, repo, slog.New(slog.NewTextHandler(io.Discard, nil))).RegisterRoutes(mux, tokens)
	return mux, token, dir
}

func fileRequest(t *testing.T, name string, data io.Reader) *http.Request {
	t.Helper()
	var framing bytes.Buffer
	writer := multipart.NewWriter(&framing)
	_, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="file"; filename="` + name + `"`},
		"Content-Type":        {"application/x-msdownload"}, // Never trust this value.
	})
	if err != nil {
		t.Fatal(err)
	}
	header := bytes.Clone(framing.Bytes())
	framing.Reset()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	body := io.MultiReader(bytes.NewReader(header), data, bytes.NewReader(framing.Bytes()))
	r := httptest.NewRequest("POST", "/rooms/550e8400-e29b-41d4-a716-446655440001/upload", body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	return r
}

func imageFixture(t *testing.T, format string, width, height int) []byte {
	t.Helper()
	var buffer bytes.Buffer
	var err error
	switch format {
	case "gif":
		err = gif.Encode(&buffer, image.NewPaletted(image.Rect(0, 0, 1, 1), palette.Plan9), nil)
	case "jpeg":
		err = jpeg.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, width, height)), nil)
	default:
		err = png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, width, height)))
	}
	if err != nil {
		t.Fatal(err)
	}
	data := buffer.Bytes()
	if format == "gif" {
		binary.LittleEndian.PutUint16(data[6:8], uint16(width))
		binary.LittleEndian.PutUint16(data[8:10], uint16(height))
	}
	if len(data) < 100 {
		data = append(data, make([]byte, 100-len(data))...)
	}
	return data
}

func officeFixture(t *testing.T, extras map[string]string) []byte {
	t.Helper()
	files := map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"word/document.xml":   `<document/>`,
	}
	for name, value := range extras {
		files[name] = value
	}
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, value := range files {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

type paddingReader struct{}

func (paddingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}
func sizedFile(prefix []byte, size int64) io.Reader {
	return io.MultiReader(bytes.NewReader(prefix), io.LimitReader(paddingReader{}, size-int64(len(prefix))))
}
