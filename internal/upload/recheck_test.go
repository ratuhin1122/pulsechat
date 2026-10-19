package upload

import (
	"strings"
	"testing"
)

func TestRecheck(t *testing.T) {
	pdf := []byte("%PDF-1.7\n" + strings.Repeat("x", 120))
	for _, tt := range []struct {
		name, kind, file, mime string
		data                   []byte
		bad                    bool
	}{
		{name: "PDF", kind: "document", file: "a.pdf", mime: "application/pdf", data: pdf},
		{name: "wrong magic", kind: "document", file: "a.pdf", mime: "application/pdf", data: []byte(strings.Repeat("MZ", 100)), bad: true},
		{name: "wrong MIME", kind: "document", file: "a.pdf", mime: "application/zip", data: pdf, bad: true},
		{name: "wrong kind", kind: "voice", file: "a.pdf", mime: "application/pdf", data: pdf, bad: true},
		{name: "executable", kind: "document", file: "a.exe", mime: "application/pdf", data: pdf, bad: true},
		{name: "too small", kind: "document", file: "a.pdf", mime: "application/pdf", data: pdf[:10], bad: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := Recheck(tt.kind, tt.file, tt.mime, tt.data); (err != nil) != tt.bad {
				t.Fatal(err)
			}
		})
	}
	if typ, err := allowedType("a.zip"); err != nil || typ.kind != "document" || typ.mime != "application/zip" {
		t.Fatalf("ZIP allow-list: %+v,%v", typ, err)
	}
}
