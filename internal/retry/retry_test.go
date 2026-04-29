package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDo_SucceedsImmediately(t *testing.T) {
	calls := 0
	err := Do(context.Background(), Default(), nil, func(ctx context.Context, attempt int) error {
		calls++
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
}

func TestDo_RetriesUntilSuccess(t *testing.T) {
	calls := 0
	cfg := Config{Attempts: 5, Initial: time.Millisecond, Max: 4 * time.Millisecond}
	err := Do(context.Background(), cfg, nil, func(ctx context.Context, attempt int) error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, calls)
}

func TestDo_ExhaustsBudget(t *testing.T) {
	calls := 0
	cfg := Config{Attempts: 4, Initial: time.Millisecond, Max: 4 * time.Millisecond}
	err := Do(context.Background(), cfg, nil, func(ctx context.Context, attempt int) error {
		calls++
		return errors.New("always fails")
	})
	require.Error(t, err)
	require.Equal(t, 4, calls)
	require.Contains(t, err.Error(), "always fails")
}

func TestDo_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before first attempt
	err := Do(ctx, Default(), nil, func(ctx context.Context, attempt int) error {
		t.Fatal("should not be called when ctx is already cancelled")
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
}

func TestDo_NonRetryablePropagates(t *testing.T) {
	fatal := errors.New("auth failed")
	calls := 0
	cfg := Config{Attempts: 5, Initial: time.Millisecond}
	predicate := func(err error) bool { return !errors.Is(err, fatal) }
	err := Do(context.Background(), cfg, predicate, func(ctx context.Context, attempt int) error {
		calls++
		return fatal
	})
	require.ErrorIs(t, err, fatal)
	require.Equal(t, 1, calls)
}

func TestDefaultRetryable_RejectsCanceled(t *testing.T) {
	require.False(t, defaultRetryable(context.Canceled))
	require.True(t, defaultRetryable(errors.New("network")))
}
