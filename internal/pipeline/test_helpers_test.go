package pipeline

import "context"

// newTestCtx returns a background context for use in tests.
// It is intentionally minimal — tests that need cancellation or deadlines
// should create their own context directly.
func newTestCtx() context.Context {
	return context.Background()
}
