package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestLocalAreas(t *testing.T) {
	disk, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	var previous string
	for _, area := range []Area{Quarantine, Clean} {
		t.Run(string(area), func(t *testing.T) {
			object, err := disk.Save(context.Background(), area, strings.NewReader("payload"))
			if err != nil {
				t.Fatal(err)
			}
			var id pgtype.UUID
			if id.Scan(object.Name) != nil || object.Name == previous || object.Size != 7 {
				t.Fatalf("invalid object: %+v", object)
			}
			previous = object.Name
			file, err := disk.Open(context.Background(), area, object.Name)
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(file)
			file.Close()
			if err != nil || string(data) != "payload" {
				t.Fatal("object content mismatch")
			}
			if err := disk.Delete(context.Background(), area, object.Name); err != nil {
				t.Fatal(err)
			}
			if err := disk.Delete(context.Background(), area, object.Name); err != nil {
				t.Fatal("delete is not idempotent")
			}
		})
	}
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func TestLocalFailureCleanup(t *testing.T) {
	dir := t.TempDir()
	disk, err := NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	sentinel := errors.New("read failed")
	_, err = disk.Save(context.Background(), Quarantine, io.MultiReader(strings.NewReader("partial"), failingReader{err: sentinel}))
	if !errors.Is(err, sentinel) {
		t.Fatalf("Save error = %v", err)
	}
	files, err := os.ReadDir(filepath.Join(dir, string(Quarantine)))
	if err != nil || len(files) != 0 {
		t.Fatal("partial file not removed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := disk.Save(ctx, Quarantine, strings.NewReader("data")); !errors.Is(err, context.Canceled) {
		t.Error("ignored canceled save")
	}
}

func TestLocalRejectsTraversal(t *testing.T) {
	disk, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	for _, name := range []string{"../secret", "/tmp/file", `..\secret`, "not-a-uuid", "550e8400-e29b-41d4-a716-44665544000/"} {
		if _, err := disk.Open(context.Background(), Quarantine, name); err == nil {
			t.Errorf("opened invalid name %q", name)
		}
		if err := disk.Delete(context.Background(), Quarantine, name); err == nil {
			t.Errorf("deleted invalid name %q", name)
		}
	}
	if _, err := disk.Save(context.Background(), Area("../outside"), strings.NewReader("data")); err == nil {
		t.Fatal("accepted invalid area")
	}
}
