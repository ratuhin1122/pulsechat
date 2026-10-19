package store

import (
	"context"
	"testing"
)

func TestNewRejectsInvalidURL(t *testing.T) {
	for _, tt := range []struct {
		name string
		url  string
	}{
		{name: "empty"},
		{name: "whitespace", url: " \t"},
		{name: "malformed", url: "://invalid"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, err := New(context.Background(), tt.url)
			if s != nil {
				s.Close()
				t.Fatal("New() returned a store for an invalid URL")
			}
			if err == nil {
				t.Fatal("New() returned no error for an invalid URL")
			}
		})
	}
}
