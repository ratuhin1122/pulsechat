package storage

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Local struct{ root *os.Root }

func NewLocal(directory string) (*Local, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, fmt.Errorf("create storage root: %w", err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("open storage root: %w", err)
	}
	for _, area := range []Area{Quarantine, Clean} {
		if err := root.MkdirAll(string(area), 0700); err != nil {
			root.Close()
			return nil, fmt.Errorf("create storage area: %w", err)
		}
	}
	return &Local{root: root}, nil
}

func (s *Local) Close() error { return s.root.Close() }

func objectPath(area Area, name string) (string, error) {
	if area != Quarantine && area != Clean {
		return "", fmt.Errorf("invalid storage area")
	}
	if len(name) != 36 {
		return "", fmt.Errorf("invalid object name")
	}
	for i, c := range name {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return "", fmt.Errorf("invalid object name")
			}
		} else if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return "", fmt.Errorf("invalid object name")
		}
	}
	return filepath.Join(string(area), name), nil
}

func (s *Local) Save(ctx context.Context, area Area, source io.Reader) (Object, error) {
	if err := ctx.Err(); err != nil {
		return Object{}, err
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return Object{}, fmt.Errorf("generate storage name: %w", err)
	}
	id[6], id[8] = id[6]&0x0f|0x40, id[8]&0x3f|0x80
	name := fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
	path, err := objectPath(area, name)
	if err != nil {
		return Object{}, err
	}
	file, err := s.root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Object{}, fmt.Errorf("create storage object: %w", err)
	}
	n, copyErr := io.Copy(file, contextReader{ctx: ctx, source: source})
	err = errors.Join(copyErr, file.Close())
	if err != nil {
		return Object{}, errors.Join(fmt.Errorf("save storage object: %w", err), s.root.Remove(path))
	}
	return Object{Name: name, Size: n}, nil
}

func (s *Local) Open(ctx context.Context, area Area, name string) (File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := objectPath(area, name)
	if err != nil {
		return nil, err
	}
	file, err := s.root.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open storage object: %w", err)
	}
	return file, nil
}

func (s *Local) Delete(ctx context.Context, area Area, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := objectPath(area, name)
	if err != nil {
		return err
	}
	if err := s.root.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete storage object: %w", err)
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	source io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(p)
}
