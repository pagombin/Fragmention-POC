package supervisor

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type fakeService struct {
	name   string
	ran    atomic.Bool
	errOut error
}

func (f *fakeService) Name() string { return f.name }
func (f *fakeService) Run(ctx context.Context) error {
	f.ran.Store(true)
	if f.errOut != nil {
		return f.errOut
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestSupervisor_StartAllAndCancel(t *testing.T) {
	sup := New(zerolog.Nop())
	a := &fakeService{name: "a"}
	b := &fakeService{name: "b"}
	sup.Register(a)
	sup.Register(b)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- sup.Start(ctx) }()

	// Give each service a tick to start.
	time.Sleep(50 * time.Millisecond)
	require.True(t, a.ran.Load())
	require.True(t, b.ran.Load())

	cancel()
	select {
	case err := <-done:
		require.True(t, err == nil || errors.Is(err, context.Canceled))
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not stop")
	}
}

func TestSupervisor_ServiceErrorPropagates(t *testing.T) {
	sup := New(zerolog.Nop())
	boom := errors.New("boom")
	sup.Register(&fakeService{name: "bad", errOut: boom})

	err := sup.Start(context.Background())
	require.ErrorIs(t, err, boom)
}

func TestSupervisor_NoServicesIsOK(t *testing.T) {
	sup := New(zerolog.Nop())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// An empty supervisor with a cancelled context should return nil
	// immediately (errgroup with no goroutines).
	require.NoError(t, sup.Start(ctx))
}
