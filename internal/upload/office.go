package upload

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"io"
	"path"
	"strings"
)

func validateOffice(file io.ReaderAt, size int64, mime string) error {
	// Reject oversized directory metadata before zip.NewReader allocates entries.
	tail := make([]byte, min(size, 22+65535))
	if _, err := file.ReadAt(tail, size-int64(len(tail))); err != nil {
		return typeMismatch()
	}
	end := bytes.LastIndex(tail, []byte("PK\x05\x06"))
	if end < 0 || len(tail)-end < 22 {
		return typeMismatch()
	}
	footer := tail[end:]
	if binary.LittleEndian.Uint16(footer[10:12]) > 1000 || binary.LittleEndian.Uint32(footer[12:16]) > 1<<20 {
		return typeMismatch()
	}
	archive, err := zip.NewReader(file, size)
	if err != nil || len(archive.File) > 1000 {
		return typeMismatch()
	}
	mainPart := "word/document.xml"
	if strings.Contains(mime, "spreadsheetml") {
		mainPart = "xl/workbook.xml"
	}
	if strings.Contains(mime, "presentationml") {
		mainPart = "ppt/presentation.xml"
	}
	var total uint64
	foundMain, foundTypes := false, false
	seen := make(map[string]struct{})
	for _, entry := range archive.File {
		name := strings.ToLower(entry.Name)
		if _, exists := seen[name]; exists {
			return typeMismatch()
		}
		seen[name] = struct{}{}
		if strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || path.Clean(name) != strings.TrimSuffix(name, "/") || strings.HasPrefix(name, "../") || entry.Flags&1 != 0 {
			return typeMismatch()
		}
		if entry.UncompressedSize64 > 50<<20 {
			return typeMismatch()
		}
		total += entry.UncompressedSize64
		if total > 100<<20 {
			return typeMismatch()
		}
		switch path.Ext(name) {
		case "", ".xml", ".rels", ".png", ".jpg", ".jpeg", ".gif", ".webp":
		default:
			return typeMismatch() // No scripts, executables, macros or nested archives.
		}
		if name == mainPart {
			foundMain = true
		}
		if name == "[content_types].xml" {
			if entry.UncompressedSize64 > 1<<20 {
				return typeMismatch()
			}
			reader, err := entry.Open()
			if err != nil {
				return typeMismatch()
			}
			content, readErr := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
			closeErr := reader.Close()
			lower := strings.ToLower(string(content))
			if readErr != nil || closeErr != nil || len(content) > 1<<20 || strings.Contains(lower, "macroenabled") || strings.Contains(lower, "vbaproject") {
				return typeMismatch()
			}
			var types struct {
				XMLName   xml.Name
				Overrides []struct {
					Part string `xml:"PartName,attr"`
					Type string `xml:"ContentType,attr"`
				} `xml:"Override"`
			}
			if xml.Unmarshal(content, &types) != nil || types.XMLName.Local != "Types" {
				return typeMismatch()
			}
			for _, override := range types.Overrides {
				if override.Part == "/"+mainPart && override.Type == mime+".main+xml" {
					foundTypes = true
				}
			}
		}
	}
	if !foundMain || !foundTypes {
		return typeMismatch()
	}
	return nil
}
