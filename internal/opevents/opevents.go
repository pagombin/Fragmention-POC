// Package opevents is a tiny dependency-free publisher seam. Services call
// Publish() to emit lifecycle frames; the api package plugs in a real hub
// at startup. This keeps the loader/deleter/compact/workload packages free
// of any HTTP/websocket imports.
package opevents

import (
	"sync"
	"time"
)

// Frame is the JSON payload emitted to subscribers.
type Frame struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Payload   any       `json:"payload"`
}

// Sink accepts frames produced by services.
type Sink interface {
	Publish(Frame)
}

var (
	mu   sync.RWMutex
	sink Sink
)

// SetSink installs the process-wide sink. Typically called once at startup
// from the api package. Pass nil to disable publishing.
func SetSink(s Sink) {
	mu.Lock()
	defer mu.Unlock()
	sink = s
}

// Publish forwards a frame to the installed sink, if any. Callers should
// treat this as best-effort; if no sink is set the frame is dropped.
func Publish(kind string, payload any) {
	mu.RLock()
	s := sink
	mu.RUnlock()
	if s == nil {
		return
	}
	s.Publish(Frame{Type: kind, Timestamp: time.Now().UTC(), Payload: payload})
}
