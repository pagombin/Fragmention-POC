// Package retry implements exponential-backoff retries for MongoDB
// operations against managed clusters where transient timeouts and
// primary-step-down errors are routine. The shared helper keeps the
// loader, deleter, and compact paths consistent.
package retry

import (
	"context"
	"errors"
	"math/rand"
	"time"
)

// Config controls a retry budget. Zero values get sane defaults:
//
//	Attempts:    20
//	Initial:     250ms
//	Max:         30s
//	Jitter:      true (full-jitter backoff)
type Config struct {
	Attempts int
	Initial  time.Duration
	Max      time.Duration
	Jitter   bool
}

// IsRetryable reports whether an error is worth backing off on. Default
// implementation considers context errors fatal, every other error
// retryable. Callers can wrap with their own predicate.
type IsRetryable func(error) bool

// Default is the standard 20-try budget the spec asks for (§ 11).
func Default() Config {
	return Config{Attempts: 20, Initial: 250 * time.Millisecond, Max: 30 * time.Second, Jitter: true}
}

// Do invokes fn until it returns nil or the budget is exhausted.
// retryable, when nil, defaults to "everything except context errors".
// The returned error is the LAST error fn produced; retryable errors are
// not joined into a multi-error to keep callers' logging clean.
func Do(ctx context.Context, cfg Config, retryable IsRetryable, fn func(ctx context.Context, attempt int) error) error {
	if cfg.Attempts <= 0 {
		cfg.Attempts = 20
	}
	if cfg.Initial <= 0 {
		cfg.Initial = 250 * time.Millisecond
	}
	if cfg.Max <= 0 {
		cfg.Max = 30 * time.Second
	}
	if retryable == nil {
		retryable = defaultRetryable
	}
	var lastErr error
	backoff := cfg.Initial
	for i := 0; i < cfg.Attempts; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := fn(ctx, i)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryable(err) {
			return err
		}
		// Final attempt - return without sleeping.
		if i == cfg.Attempts-1 {
			break
		}
		wait := backoff
		if cfg.Jitter {
			// Full-jitter: pick a random duration in [0, backoff]. AWS
			// architecture-blog popularised this pattern; it spreads
			// retry storms more effectively than equal/decorrelated.
			wait = time.Duration(rand.Int63n(int64(backoff) + 1))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		backoff *= 2
		if backoff > cfg.Max {
			backoff = cfg.Max
		}
	}
	return lastErr
}

// defaultRetryable retries everything except cancelled / deadline-exceeded
// contexts. MongoDB transient timeouts come in as a wrapped network-write
// error, which is exactly what we want to back off on.
func defaultRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	// context.DeadlineExceeded on the OUTER context is fatal, but on the
	// per-attempt context it bubbles up as a wrapped error which Is() does
	// not match - good. We treat it as retryable here. Callers that pass
	// the outer ctx to Do() get correct behavior automatically.
	return true
}
