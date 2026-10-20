package logging

import (
	"context"
	"sync"
)

type Fields struct{ RequestID, UserID, RoomID, AttachmentID string }
type scopeKey struct{}
type scope struct {
	mu     sync.RWMutex
	fields Fields
}

// ValidID bounds client-supplied correlation values before headers, logs or queue payloads use them.
func ValidID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

func WithFields(ctx context.Context, fields Fields) context.Context {
	ctx = context.WithValue(ctx, scopeKey{}, &scope{})
	Set(ctx, fields)
	return ctx
}

// Set updates only this scope. The outer request logger can observe IDs learned by inner handlers.
func Set(ctx context.Context, fields Fields) {
	s, ok := ctx.Value(scopeKey{}).(*scope)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, pair := range []struct {
		to    *string
		value string
	}{{&s.fields.RequestID, fields.RequestID}, {&s.fields.UserID, fields.UserID}, {&s.fields.RoomID, fields.RoomID}, {&s.fields.AttachmentID, fields.AttachmentID}} {
		if ValidID(pair.value) {
			*pair.to = pair.value
		}
	}
}

func From(ctx context.Context) Fields {
	s, ok := ctx.Value(scopeKey{}).(*scope)
	if !ok {
		return Fields{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fields
}
