package storage

import (
	"context"
	"io"
)

type Area string

const (
	Quarantine Area = "quarantine"
	Clean      Area = "clean"
)

type Object struct {
	Name string
	Size int64
}

type File interface {
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Closer
}

type Storage interface {
	Save(context.Context, Area, io.Reader) (Object, error)
	Open(context.Context, Area, string) (File, error)
	Delete(context.Context, Area, string) error
}
