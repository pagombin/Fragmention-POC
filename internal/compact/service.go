package compact

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/rs/zerolog"

	"github.com/pagombin/fragmention-poc/internal/storage"
)

// Service owns active compact orchestrators.
type Service struct {
	deps Deps
	mu   sync.Mutex
	act  map[string]*Orchestrator
}

// NewService constructs a Service.
func NewService(d Deps) (*Service, error) {
	if d.Mongo == nil || d.Ops == nil {
		return nil, errors.New("compact: Mongo and Ops required")
	}
	if d.Logger.GetLevel() == zerolog.Disabled {
		d.Logger = zerolog.Nop()
	}
	return &Service{deps: d, act: map[string]*Orchestrator{}}, nil
}

// Preview plans the compact without side effects. UIs call this before
// Start to show the user the exact execution order and warnings.
func (s *Service) Preview(ctx context.Context, scope Scope, params Params) (PreviewResult, error) {
	if err := params.Validate(); err != nil {
		return PreviewResult{}, err
	}
	o, err := New(newID(), s.deps, scope, params)
	if err != nil {
		return PreviewResult{}, err
	}
	return o.Plan(ctx)
}

// Start persists the operation row and launches the orchestrator in a
// goroutine. Overlap with active loaders/deleters at the collection level
// is rejected here.
func (s *Service) Start(ctx context.Context, scope Scope, params Params, runID string) (*Orchestrator, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}
	id := newID()
	o, err := New(id, s.deps, scope, params)
	if err != nil {
		return nil, err
	}
	// Persist operation up front.
	target, _ := encodeJSON(scope)
	paramsJSON, _ := encodeJSON(params)
	var runPtr *string
	if runID != "" {
		v := runID
		runPtr = &v
	}
	if err := s.deps.Ops.Create(ctx, storage.Operation{
		ID: id, RunID: runPtr, Kind: storage.OpKindCompact,
		Target: target, Params: paramsJSON, State: storage.StateIdle,
	}); err != nil {
		return nil, fmt.Errorf("persist operation: %w", err)
	}
	s.mu.Lock()
	s.act[id] = o
	s.mu.Unlock()
	go func() {
		if err := o.Run(context.Background()); err != nil {
			o.logger.Error().Err(err).Msg("compact orchestrator exited with error")
		}
		s.mu.Lock()
		delete(s.act, id)
		s.mu.Unlock()
	}()
	return o, nil
}

// Get returns an active orchestrator by id, or nil.
func (s *Service) Get(id string) *Orchestrator {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.act[id]
}

// ListActive returns a snapshot of running orchestrators.
func (s *Service) ListActive() []*Orchestrator {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Orchestrator, 0, len(s.act))
	for _, v := range s.act {
		out = append(out, v)
	}
	return out
}

func encodeJSON(v any) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	return jsonMarshal(v)
}
