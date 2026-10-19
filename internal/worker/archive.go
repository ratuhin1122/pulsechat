package worker

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strings"
)

const (
	archiveMaxEntries = 1000
	archiveMaxBytes   = 100 << 20
	archiveMaxRatio   = 100
	archiveMaxDepth   = 2 // Root ZIP is depth zero; two nested ZIP levels are allowed.
)

type archiveBudget struct {
	entries int
	bytes   uint64
}

func checkZIP(ctx context.Context, data []byte, inspect bool) error {
	return (&archiveBudget{}).check(ctx, data, inspect, 0)
}

func (b *archiveBudget) check(ctx context.Context, data []byte, inspect bool, depth int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if depth > archiveMaxDepth {
		return fmt.Errorf("nesting exceeds %d", archiveMaxDepth)
	}
	entries, err := zipDirectoryEntries(data)
	if err != nil {
		return err
	}
	if inspect && b.entries+entries > archiveMaxEntries {
		return fmt.Errorf("total entry count exceeds %d", archiveMaxEntries)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("invalid ZIP: %w", err)
	}
	if len(reader.File) != entries || len(reader.File) > archiveMaxEntries {
		return fmt.Errorf("entry count exceeds limit or directory mismatch")
	}
	// Default mode only reads directory metadata; never opens or inflates members.
	if !inspect {
		return nil
	}
	b.entries += len(reader.File)
	if b.entries > archiveMaxEntries {
		return fmt.Errorf("total entry count exceeds %d", archiveMaxEntries)
	}
	for _, entry := range reader.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name
		if strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || strings.Contains(name, ":") ||
			path.Clean(name) != strings.TrimSuffix(name, "/") || strings.HasPrefix(name, "../") || entry.Flags&1 != 0 {
			return fmt.Errorf("unsafe or encrypted ZIP entry")
		}
		if entry.UncompressedSize64 > archiveMaxBytes-b.bytes {
			return fmt.Errorf("uncompressed size exceeds %d bytes", archiveMaxBytes)
		}
		if entry.UncompressedSize64 > 0 && (entry.CompressedSize64 == 0 ||
			float64(entry.UncompressedSize64)/float64(entry.CompressedSize64) > archiveMaxRatio) {
			return fmt.Errorf("compression ratio exceeds %d", archiveMaxRatio)
		}
		b.bytes += entry.UncompressedSize64
	}
	for _, entry := range reader.File {
		offset, err := entry.DataOffset()
		if err != nil || offset < 0 || offset > int64(len(data)) || entry.CompressedSize64 > uint64(int64(len(data))-offset) {
			return fmt.Errorf("ZIP compressed data exceeds archive bounds")
		}
		stream, err := entry.Open()
		if err != nil {
			return fmt.Errorf("open ZIP entry: %w", err)
		}
		content, readErr := io.ReadAll(io.LimitReader(contextInput{ctx, stream}, int64(entry.UncompressedSize64)+1))
		closeErr := stream.Close()
		if readErr != nil || closeErr != nil || uint64(len(content)) != entry.UncompressedSize64 {
			return fmt.Errorf("invalid ZIP entry data or checksum")
		}
		if bytes.HasPrefix(content, []byte("PK\x03\x04")) || bytes.HasPrefix(content, []byte("PK\x05\x06")) {
			if err := b.check(ctx, content, true, depth+1); err != nil {
				return err
			}
		} else if strings.EqualFold(path.Ext(entry.Name), ".zip") {
			return fmt.Errorf("invalid nested ZIP")
		}
	}
	return nil
}

type contextInput struct {
	ctx    context.Context
	source io.Reader
}

func (r contextInput) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(p)
}
