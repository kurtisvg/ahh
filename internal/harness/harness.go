package harness

import (
	"context"
	"io"
)

// Harness owns the lifecycle of an underlying coding-agent harness process.
type Harness interface {
	io.Reader
	io.Writer
	Wait(context.Context) error
	Done() bool
	Resize(rows, cols uint16) error
	Close()
}
