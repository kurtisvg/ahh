package harness

import "context"

// Harness owns the lifecycle of an underlying coding-agent harness process.
type Harness interface {
	Wait(context.Context) error
	Done() bool
	Close()
}
