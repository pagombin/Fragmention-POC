package opevents

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type recorder struct {
	mu     sync.Mutex
	frames []Frame
}

func (r *recorder) Publish(f Frame) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frames = append(r.frames, f)
}

func TestPublish_Forwards(t *testing.T) {
	r := &recorder{}
	SetSink(r)
	t.Cleanup(func() { SetSink(nil) })

	Publish("loader_started", map[string]any{"operation_id": "abc"})
	r.mu.Lock()
	defer r.mu.Unlock()
	require.Len(t, r.frames, 1)
	require.Equal(t, "loader_started", r.frames[0].Type)
	require.False(t, r.frames[0].Timestamp.IsZero())
}

func TestPublish_NoSinkIsNoop(t *testing.T) {
	SetSink(nil)
	// Must not panic.
	Publish("compact_done", nil)
}
