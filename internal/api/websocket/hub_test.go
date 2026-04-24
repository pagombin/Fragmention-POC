package websocket

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestHub_FanOut(t *testing.T) {
	h := NewHub("t", zerolog.Nop())
	_, aCh, aCancel := h.Subscribe()
	defer aCancel()
	_, bCh, bCancel := h.Subscribe()
	defer bCancel()

	require.Equal(t, 2, h.SubscriberCount())

	h.Publish(Frame{Type: "hello", Payload: 1})
	select {
	case f := <-aCh:
		require.Equal(t, "hello", f.Type)
	default:
		t.Fatal("subscriber a did not receive")
	}
	select {
	case f := <-bCh:
		require.Equal(t, "hello", f.Type)
	default:
		t.Fatal("subscriber b did not receive")
	}
}

func TestHub_DropOnSlowReader(t *testing.T) {
	h := NewHub("t", zerolog.Nop())
	_, _, cancel := h.Subscribe()
	defer cancel()

	// Flood past the 64-frame buffer; should not block nor panic.
	for i := 0; i < 1000; i++ {
		h.Publish(Frame{Type: "x"})
	}
}

func TestHub_CancelRemovesSubscriber(t *testing.T) {
	h := NewHub("t", zerolog.Nop())
	_, _, cancel := h.Subscribe()
	require.Equal(t, 1, h.SubscriberCount())
	cancel()
	require.Equal(t, 0, h.SubscriberCount())
}
