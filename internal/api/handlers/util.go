package handlers

import (
	"context"
	"time"
)

// ctxWithTimeout is a small shorthand used by every handler. It inherits
// cancellation from the request context and adds a per-handler deadline.
func ctxWithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, d)
}
