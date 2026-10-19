package worker

import (
	"context"
	"io"
)

type ScanResult struct{ Threat string }

// Scan returns an empty Threat only after a successful clean scan. Any error is
// a scan failure, never a clean result.
type Scanner interface {
	Scan(context.Context, io.Reader) (ScanResult, error)
}
