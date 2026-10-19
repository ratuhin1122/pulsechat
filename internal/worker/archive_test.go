package worker

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
)

type zipMember struct {
	name   string
	data   []byte
	method uint16
}

func sampleZIP(t *testing.T, members ...zipMember) []byte {
	t.Helper()
	var buffer bytes.Buffer
	w := zip.NewWriter(&buffer)
	for _, member := range members {
		f, err := w.CreateHeader(&zip.FileHeader{Name: member.name, Method: member.method})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(member.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestArchiveInspection(t *testing.T) {
	plain := sampleZIP(t, zipMember{name: "note.txt", data: []byte("hello")})
	nested := func(data []byte) []byte { return sampleZIP(t, zipMember{name: "nested.bin", data: data}) }
	deep := nested(nested(nested(plain)))
	ratio := sampleZIP(t, zipMember{name: "large.txt", data: bytes.Repeat([]byte("A"), 1<<20), method: zip.Deflate})
	corrupt := bytes.Clone(plain)
	pos := bytes.Index(corrupt, []byte("hello"))
	corrupt[pos] = 'X'
	oversized := bytes.Clone(plain)
	central := bytes.Index(oversized, []byte("PK\x01\x02"))
	binary.LittleEndian.PutUint32(oversized[central+24:], archiveMaxBytes+1)
	forged := bytes.Clone(plain)
	footer := bytes.LastIndex(forged, []byte("PK\x05\x06"))
	binary.LittleEndian.PutUint16(forged[footer+8:], 0)
	binary.LittleEndian.PutUint16(forged[footer+10:], 0)
	manyMembers := make([]zipMember, archiveMaxEntries+1)
	for i := range manyMembers {
		manyMembers[i].name = fmt.Sprintf("%d.txt", i)
	}
	many := sampleZIP(t, manyMembers...)
	for _, tt := range []struct {
		name    string
		data    []byte
		inspect bool
		reason  string
	}{
		{name: "normal", data: plain, inspect: true},
		{name: "empty", data: sampleZIP(t), inspect: true},
		{name: "default does not inflate damaged member", data: corrupt},
		{name: "default does not inflate bomb", data: ratio},
		{name: "default does not recurse", data: deep},
		{name: "CRC failure", data: corrupt, inspect: true, reason: "checksum"},
		{name: "compression ratio", data: ratio, inspect: true, reason: "compression ratio"},
		{name: "uncompressed size", data: oversized, inspect: true, reason: "uncompressed size"},
		{name: "entries", data: many, inspect: true, reason: "directory exceeds"},
		{name: "allowed nesting", data: nested(nested(plain)), inspect: true},
		{name: "excess nesting identified by bytes", data: deep, inspect: true, reason: "nesting"},
		{name: "path traversal", data: sampleZIP(t, zipMember{name: "../bad", data: []byte("x")}), inspect: true, reason: "unsafe"},
		{name: "invalid nested ZIP", data: sampleZIP(t, zipMember{name: "bad.zip", data: []byte("bad")}), inspect: true, reason: "nested ZIP"},
		{name: "invalid ZIP", data: []byte("PK\x03\x04bad"), inspect: true, reason: "directory"},
		{name: "forged count", data: forged, inspect: true, reason: "entry count"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := checkZIP(context.Background(), tt.data, tt.inspect)
			if tt.reason == "" && err != nil || tt.reason != "" && (err == nil || !strings.Contains(err.Error(), tt.reason)) {
				t.Fatalf("error=%v, expected %q", err, tt.reason)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if checkZIP(ctx, plain, true) == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestArchiveSharedBudgets(t *testing.T) {
	childMembers := make([]zipMember, 501)
	for i := range childMembers {
		childMembers[i].name = fmt.Sprintf("%d", i)
	}
	child := sampleZIP(t, childMembers...)
	root := sampleZIP(t, zipMember{name: "one.zip", data: child}, zipMember{name: "two.zip", data: child})
	if err := checkZIP(context.Background(), root, true); err == nil || !strings.Contains(err.Error(), "entry count") {
		t.Fatalf("shared entries: %v", err)
	}
	// Preflight all member sizes before inflating any content.
	data := sampleZIP(t, zipMember{name: "a", data: []byte("a")}, zipMember{name: "b", data: []byte("b")})
	for pos := 0; ; {
		next := bytes.Index(data[pos:], []byte("PK\x01\x02"))
		if next < 0 {
			break
		}
		pos += next
		binary.LittleEndian.PutUint32(data[pos+20:], 1<<20)
		binary.LittleEndian.PutUint32(data[pos+24:], 60<<20)
		pos += 46
	}
	if err := checkZIP(context.Background(), data, true); err == nil || !strings.Contains(err.Error(), "uncompressed size") {
		t.Fatalf("total bytes: %v", err)
	}
	child = sampleZIP(t, zipMember{name: "a", data: []byte("a")})
	central := bytes.Index(child, []byte("PK\x01\x02"))
	binary.LittleEndian.PutUint32(child[central+20:], 2<<20)
	binary.LittleEndian.PutUint32(child[central+24:], archiveMaxBytes)
	root = sampleZIP(t, zipMember{name: "nested.zip", data: child})
	if err := checkZIP(context.Background(), root, true); err == nil || !strings.Contains(err.Error(), "uncompressed size") {
		t.Fatalf("nested byte budget: %v", err)
	}
}
