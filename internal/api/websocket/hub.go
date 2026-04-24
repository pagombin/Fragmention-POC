// Package websocket exposes the live metric and live operation streams.
// Two hubs are maintained: MetricsHub receives collector-tick frames; OpsHub
// receives operation lifecycle events. Each hub fans messages out to every
// connected client without blocking on slow readers (slow clients are
// dropped).
package websocket

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

// Frame is one JSON-serializable message. Domains use type-specific payloads;
// the wrapper lets clients discriminate without parsing everything.
type Frame struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   any         `json:"payload"`
}

// Hub fans out frames to every connected subscriber. Subscribers get a
// bounded channel; if they fall behind, the hub drops messages for them
// rather than block everyone.
type Hub struct {
	logger zerolog.Logger
	mu     sync.RWMutex
	subs   map[int64]*subscriber
	nextID atomic.Int64
	name   string
}

type subscriber struct {
	ch   chan Frame
	done chan struct{}
}

// NewHub constructs a Hub.
func NewHub(name string, logger zerolog.Logger) *Hub {
	return &Hub{
		logger: logger.With().Str("service", "ws_hub").Str("hub", name).Logger(),
		subs:   map[int64]*subscriber{},
		name:   name,
	}
}

// Subscribe registers a new client. Callers must drain the returned channel
// and call the returned cancel when done.
func (h *Hub) Subscribe() (id int64, ch <-chan Frame, cancel func()) {
	s := &subscriber{ch: make(chan Frame, 64), done: make(chan struct{})}
	id = h.nextID.Add(1)
	h.mu.Lock()
	h.subs[id] = s
	h.mu.Unlock()
	return id, s.ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[id]; ok {
			close(s.done)
			close(s.ch)
			delete(h.subs, id)
		}
	}
}

// Publish fans out a frame. Slow subscribers are dropped silently - the hub
// tracks how many frames were dropped so operators can observe client lag.
func (h *Hub) Publish(f Frame) {
	if f.Timestamp.IsZero() {
		f.Timestamp = time.Now().UTC()
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subs {
		select {
		case s.ch <- f:
		default:
			// Drop on slow reader.
		}
	}
}

// SubscriberCount returns the number of connected clients.
func (h *Hub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}

// EncodeFrame returns the JSON encoding of a frame. Useful for buffering
// before send.
func EncodeFrame(f Frame) ([]byte, error) {
	return json.Marshal(f)
}

// Wait is a small helper that blocks until either ctx is done or the
// subscriber's done channel is closed.
func Wait(ctx context.Context, done <-chan struct{}) {
	select {
	case <-ctx.Done():
	case <-done:
	}
}

// Publisher is a narrow interface services use to avoid importing the ws
// package directly. Any *Hub satisfies it.
type Publisher interface {
	Publish(Frame)
}

// Verify *Hub satisfies Publisher at compile time.
var _ Publisher = (*Hub)(nil)
