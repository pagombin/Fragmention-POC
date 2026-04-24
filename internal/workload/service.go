package workload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/rs/zerolog"

	"github.com/pagombin/fragmention-poc/internal/storage"
)

// Service owns active Workload instances.
type Service struct {
	deps Deps
	mu   sync.Mutex
	act  map[string]*Workload
}

// NewService constructs a Service.
func NewService(d Deps) (*Service, error) {
	if d.Mongo == nil || d.Ops == nil {
		return nil, errors.New("workload: Mongo and Ops required")
	}
	if d.Logger.GetLevel() == zerolog.Disabled {
		d.Logger = zerolog.Nop()
	}
	return &Service{deps: d, act: map[string]*Workload{}}, nil
}

// Start persists an operation and launches a workload in a goroutine.
func (s *Service) Start(ctx context.Context, spec Spec, params Params, runID string) (*Workload, error) {
	id := newID()
	w, err := New(id, s.deps, spec, params)
	if err != nil {
		return nil, err
	}
	target, _ := json.Marshal(spec)
	paramsJSON, _ := json.Marshal(params)
	var runPtr *string
	if runID != "" {
		v := runID
		runPtr = &v
	}
	if err := s.deps.Ops.Create(ctx, storage.Operation{
		ID: id, RunID: runPtr, Kind: storage.OpKindWorkload,
		Target: target, Params: paramsJSON, State: storage.StateIdle,
	}); err != nil {
		return nil, fmt.Errorf("persist operation: %w", err)
	}
	s.mu.Lock()
	s.act[id] = w
	s.mu.Unlock()
	go func() {
		if err := w.Run(context.Background()); err != nil {
			w.logger.Error().Err(err).Msg("workload exited with error")
		}
		s.mu.Lock()
		delete(s.act, id)
		s.mu.Unlock()
	}()
	return w, nil
}

// Get returns an active workload by id, or nil.
func (s *Service) Get(id string) *Workload {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.act[id]
}

// ListActive returns a snapshot of running workloads.
func (s *Service) ListActive() []*Workload {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Workload, 0, len(s.act))
	for _, v := range s.act {
		out = append(out, v)
	}
	return out
}
