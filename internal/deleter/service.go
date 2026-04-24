package deleter

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/pagombin/fragmention-poc/internal/storage"
)

// Service owns active Deleters and manages the preview → confirmation-token
// → execute flow required by spec § 5.2.
type Service struct {
	deps       Deps
	previewTTL time.Duration
	defaultParams Params

	mu       sync.Mutex
	active   map[string]*Deleter
	previews map[string]storedPreview
}

type storedPreview struct {
	spec      TargetSpec
	params    Params
	result    PreviewResult
	expiresAt time.Time
}

// NewService constructs the Service.
func NewService(d Deps, defaults Params, previewTTL time.Duration) (*Service, error) {
	if d.Mongo == nil || d.Ops == nil || d.Store == nil {
		return nil, errors.New("deleter: Mongo, Ops, and Store required")
	}
	if d.Logger.GetLevel() == zerolog.Disabled {
		d.Logger = zerolog.Nop()
	}
	if previewTTL <= 0 {
		previewTTL = 2 * time.Minute
	}
	return &Service{
		deps:          d,
		previewTTL:    previewTTL,
		defaultParams: defaults,
		active:        map[string]*Deleter{},
		previews:      map[string]storedPreview{},
	}, nil
}

// Preview estimates the impact of a delete and returns a one-time
// confirmation token. Tokens expire after previewTTL. Large-impact
// previews (ratio > 0.5 or many collections) flag RequiresTypedConfirmation
// so the UI enforces an extra typed-confirmation input.
func (s *Service) Preview(ctx context.Context, spec TargetSpec, params Params) (PreviewResult, error) {
	if len(spec.Entries) == 0 {
		return PreviewResult{}, errors.New("spec must contain at least one target")
	}
	if err := params.Validate(); err != nil {
		return PreviewResult{}, err
	}

	res := PreviewResult{
		Token:     generateToken(),
		ExpiresAt: time.Now().Add(s.previewTTL).UTC(),
	}
	var total int64
	var maxRatio float64
	for _, t := range spec.Entries {
		if err := t.Pattern.Validate(params.MaxRatio); err != nil {
			return PreviewResult{}, fmt.Errorf("%s: %w", t.Key(), err)
		}
		coll := s.deps.Mongo.Raw().Database(t.Database).Collection(t.Collection)
		pattern, err := PatternFor(t.Pattern)
		if err != nil {
			return PreviewResult{}, err
		}
		filter, err := pattern.BuildFilter(ctx, coll, t.Pattern)
		if err != nil {
			return PreviewResult{}, err
		}
		matched, err := pattern.EstimateMatchCount(ctx, coll, t.Pattern, filter)
		if err != nil {
			return PreviewResult{}, fmt.Errorf("%s count: %w", t.Key(), err)
		}
		totalDocs, err := coll.EstimatedDocumentCount(ctx)
		if err != nil {
			return PreviewResult{}, fmt.Errorf("%s total: %w", t.Key(), err)
		}
		ratio := 0.0
		if totalDocs > 0 {
			ratio = float64(matched) / float64(totalDocs)
		}
		if ratio > maxRatio {
			maxRatio = ratio
		}
		res.PerCollection = append(res.PerCollection, PreviewPerColl{
			Database: t.Database, Collection: t.Collection,
			MatchedCount: matched, TotalDocuments: totalDocs,
		})
		total += matched
	}
	res.TotalMatches = total
	res.RequiresTyped = maxRatio > 0.5 || len(spec.Entries) > 5
	res.MaxRatioBreached = maxRatio > params.MaxRatio

	s.mu.Lock()
	s.previews[res.Token] = storedPreview{
		spec: spec, params: params, result: res, expiresAt: res.ExpiresAt,
	}
	s.mu.Unlock()
	s.reapExpired()
	return res, nil
}

// Start consumes a preview token and launches the deleter. The token is
// rejected if absent, already used, or expired. Overlap with other active
// deletes is rejected with a descriptive error.
func (s *Service) Start(ctx context.Context, token, runID string) (*Deleter, error) {
	s.mu.Lock()
	p, ok := s.previews[token]
	if ok {
		delete(s.previews, token) // consume the token - single use
	}
	s.mu.Unlock()
	if !ok {
		return nil, errors.New("invalid or consumed confirmation token")
	}
	if time.Now().After(p.expiresAt) {
		return nil, errors.New("confirmation token expired")
	}
	if p.result.MaxRatioBreached {
		return nil, errors.New("preview exceeds configured max_ratio; refine and re-preview")
	}

	s.mu.Lock()
	for _, other := range s.active {
		if targetsOverlap(other.spec, p.spec) {
			s.mu.Unlock()
			return nil, fmt.Errorf("target scope overlaps with active deleter %s", other.id)
		}
	}
	s.mu.Unlock()

	d, err := New(newID(), s.deps, p.spec, p.params)
	if err != nil {
		return nil, err
	}
	target, _ := encodeJSON(p.spec)
	paramsJSON, _ := encodeJSON(p.params)
	var runPtr *string
	if runID != "" {
		v := runID
		runPtr = &v
	}
	if err := s.deps.Ops.Create(ctx, storage.Operation{
		ID: d.id, RunID: runPtr, Kind: storage.OpKindDeleter,
		Target: target, Params: paramsJSON, State: storage.StateIdle,
	}); err != nil {
		return nil, fmt.Errorf("persist operation: %w", err)
	}

	s.mu.Lock()
	s.active[d.id] = d
	s.mu.Unlock()

	go func() {
		if err := d.Run(context.Background()); err != nil {
			d.logger.Error().Err(err).Msg("deleter run exited with error")
		}
		s.mu.Lock()
		delete(s.active, d.id)
		s.mu.Unlock()
	}()
	return d, nil
}

// Get returns an active Deleter by id, or nil.
func (s *Service) Get(id string) *Deleter {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active[id]
}

// ListActive returns a snapshot of running deleters.
func (s *Service) ListActive() []*Deleter {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Deleter, 0, len(s.active))
	for _, v := range s.active {
		out = append(out, v)
	}
	return out
}

func (s *Service) reapExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for tok, p := range s.previews {
		if now.After(p.expiresAt) {
			delete(s.previews, tok)
		}
	}
}

// targetsOverlap mirrors the loader implementation: two spec envelopes
// conflict if they share any (db, collection) pair.
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
