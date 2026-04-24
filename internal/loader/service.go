package loader

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/pagombin/fragmention-poc/internal/collector"
	"github.com/pagombin/fragmention-poc/internal/generator"
	mongoClient "github.com/pagombin/fragmention-poc/internal/mongo"
	"github.com/pagombin/fragmention-poc/internal/storage"
)

// Service owns active Loader instances. It is the single entry point for
// API handlers: Start/Pause/Resume/Stop/Get/List all route through here.
// Multiple loaders may run concurrently if their target scopes do not
// overlap (spec § 21.3); the conflict check lives in Start.
type Service struct {
	deps Deps
	mu   sync.Mutex
	act  map[string]*Loader
}

// NewService constructs the Service. Callers must provide a fully-initialized
// Deps (Mongo client, ops repo, events, collector) - nil fields are allowed
// only for collector (degraded mode) and events (logging disabled).
func NewService(d Deps) (*Service, error) {
	if d.Mongo == nil {
		return nil, errors.New("loader: Mongo client required")
	}
	if d.Ops == nil {
		return nil, errors.New("loader: operations repo required")
	}
	if d.Registry == nil {
		tpls, weights := generator.DefaultTemplates()
		reg, err := generator.NewRegistry(tpls, weights)
		if err != nil {
			return nil, err
		}
		d.Registry = reg
	}
	if d.Logger.GetLevel() == zerolog.Disabled {
		d.Logger = zerolog.Nop()
	}
	return &Service{deps: d, act: map[string]*Loader{}}, nil
}

// Start creates a persisted operation, constructs a Loader, launches its
// Run() loop in a goroutine, and returns its id. Overlap conflicts are
// detected against every currently-active operation of any kind the service
// can see; callers in other services (deleter, compact) also register
// themselves so the check is authoritative.
func (s *Service) Start(ctx context.Context, spec TargetSpec, params Params, runID string) (*Loader, error) {
	s.mu.Lock()
	// Detect overlap with other in-memory loaders.
	for _, other := range s.act {
		if targetsOverlap(other.spec, spec) {
			s.mu.Unlock()
			return nil, fmt.Errorf("target scope overlaps with active loader %s", other.id)
		}
	}
	s.mu.Unlock()

	opID := uuid.NewString()
	l, err := New(opID, s.deps, spec, params)
	if err != nil {
		return nil, err
	}

	// Persist the operation BEFORE launching the worker loop so that a
	// crash between Register and the first progress row is still recoverable
	// by RecoverOrphans.
	targetJSON, _ := encodeJSON(spec)
	paramsJSON, _ := encodeJSON(l.live.get())
	runPtr := nullableStr(runID)
	if err := s.deps.Ops.Create(ctx, storage.Operation{
		ID:     opID,
		RunID:  runPtr,
		Kind:   storage.OpKindLoader,
		Target: targetJSON,
		Params: paramsJSON,
		State:  storage.StateIdle,
	}); err != nil {
		return nil, fmt.Errorf("persist operation: %w", err)
	}

	s.mu.Lock()
	s.act[opID] = l
	s.mu.Unlock()

	go func() {
		if err := l.Run(context.Background()); err != nil {
			l.logger.Error().Err(err).Msg("loader run exited with error")
		}
		s.mu.Lock()
		delete(s.act, opID)
		s.mu.Unlock()
	}()
	return l, nil
}

// Get returns the in-memory Loader by id, or nil when absent. Completed
// loaders are removed from the active map; API handlers fall back to the
// ops repo for historical lookup.
func (s *Service) Get(id string) *Loader {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.act[id]
}

// ListActive returns a snapshot of currently-active Loader instances.
func (s *Service) ListActive() []*Loader {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Loader, 0, len(s.act))
	for _, v := range s.act {
		out = append(out, v)
	}
	return out
}

// targetsOverlap reports whether two specs touch any common
// (database, collection) pair.
func targetsOverlap(a, b TargetSpec) bool {
	set := make(map[string]struct{}, len(a.Entries))
	for _, e := range a.Entries {
		set[e.Key()] = struct{}{}
	}
	for _, e := range b.Entries {
		if _, ok := set[e.Key()]; ok {
			return true
		}
	}
	return false
}

// EnsureRegistry lets callers override the default 6-template mix. Passing
// nil reinstates the default.
func (s *Service) EnsureRegistry(reg *generator.Registry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if reg == nil {
		tpls, weights := generator.DefaultTemplates()
		reg, _ = generator.NewRegistry(tpls, weights)
	}
	s.deps.Registry = reg
}

func encodeJSON(v any) ([]byte, error) {
	return jsonMarshal(v)
}

func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ServiceDeps is an alias kept to document intent at call sites.
type ServiceDeps = Deps

// Compile-time check that *Loader exposes everything the handlers need.
var _ interface {
	ID() string
	State() storage.OperationState
	Params() Params
	SetParams(Params) error
} = (*Loader)(nil)

// Silence unused warnings when collector is nil.
var _ = collector.Collector{}
var _ = (*mongoClient.Client)(nil)
