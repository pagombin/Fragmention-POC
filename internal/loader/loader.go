package loader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"

	"github.com/pagombin/fragmention-poc/internal/collector"
	"github.com/pagombin/fragmention-poc/internal/generator"
	mongoClient "github.com/pagombin/fragmention-poc/internal/mongo"
	"github.com/pagombin/fragmention-poc/internal/opevents"
	"github.com/pagombin/fragmention-poc/internal/retry"
	"github.com/pagombin/fragmention-poc/internal/storage"
)

// Loader orchestrates one data-load operation. It is owned by the Service
// (which may run multiple loaders concurrently against non-overlapping
// scopes) and holds all runtime state in-memory plus persisted progress in
// the state store.
type Loader struct {
	id         string
	logger     zerolog.Logger
	mc         *mongoClient.Client
	ops        *storage.Operations
	events     *storage.Events
	collector  *collector.Collector
	registry   *generator.Registry

	spec   TargetSpec
	live   *live

	// Control signals.
	pauseCh  chan struct{} // closed when paused (workers block)
	resumeCh chan struct{} // closed on resume to unblock
	stopCh   chan struct{} // closed to signal stop

	mu     sync.Mutex
	state  storage.OperationState

	stats struct {
		docs      atomic.Int64
		bytes     atomic.Int64
		batchesOK atomic.Int64
		batchesKO atomic.Int64
	}
	startedAt time.Time
}

// Deps aggregates the non-parameter dependencies. Services inject one Deps
// per process lifetime; each loader invocation creates its own Loader.
type Deps struct {
	Logger    zerolog.Logger
	Mongo     *mongoClient.Client
	Ops       *storage.Operations
	Events    *storage.Events
	Collector *collector.Collector
	Registry  *generator.Registry
}

// New constructs a Loader. ID is typically a fresh UUID.
func New(id string, d Deps, spec TargetSpec, p Params) (*Loader, error) {
	if id == "" {
		return nil, errors.New("loader: id required")
	}
	if len(spec.Entries) == 0 {
		return nil, errors.New("loader: target spec must contain at least one entry")
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if p.Workers == 0 {
		p.Workers = 2 * runtime.NumCPU()
	}
	if p.Seed == 0 {
		p.Seed = time.Now().UnixNano()
	}
	if p.LoadRunID == "" {
		p.LoadRunID = uuid.NewString()
	}
	return &Loader{
		id:        id,
		logger:    d.Logger.With().Str("service", "loader").Str("operation_id", id).Logger(),
		mc:        d.Mongo,
		ops:       d.Ops,
		events:    d.Events,
		collector: d.Collector,
		registry:  d.Registry,
		spec:      spec,
		live:      newLive(p),
		pauseCh:   make(chan struct{}),
		resumeCh:  closedChan(),
		stopCh:    make(chan struct{}),
		state:     storage.StateIdle,
	}, nil
}

// ID returns the operation id.
func (l *Loader) ID() string { return l.id }

// State returns the current lifecycle state.
func (l *Loader) State() storage.OperationState {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.state
}

// Params returns the current live params. Callers must not retain the
// returned struct across adjustments.
func (l *Loader) Params() Params { return l.live.get() }

// SetParams atomically updates the live params. Only Workers, BatchSize,
// and DocsPerSecond are meaningful mid-flight; others are validated and
// stored but do not retroactively change in-flight batches.
func (l *Loader) SetParams(p Params) error {
	cur := l.live.get()
	// Preserve immutable fields.
	p.Seed = cur.Seed
	p.LoadRunID = cur.LoadRunID
	p.WriteConcern = cur.WriteConcern
	if err := p.Validate(); err != nil {
		return err
	}
	l.live.set(p)
	l.logger.Info().
		Int("workers", p.Workers).
		Int("batch_size", p.BatchSize).
		Float64("docs_per_sec", p.DocsPerSecond).
		Msg("loader params adjusted")
	return nil
}

// Pause asks the loader to pause. In-flight batches complete; subsequent
// batches block on pauseCh until Resume is called. Safe to call twice.
func (l *Loader) Pause(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.state != storage.StateRunning {
		return fmt.Errorf("cannot pause from state %s", l.state)
	}
	// Swap the signal channels: close resumeCh (allow a pending pause wait to
	// exit), then create a new pauseCh/resumeCh pair. Workers re-read the
	// channels every batch.
	l.state = storage.StatePaused
	l.pauseCh = make(chan struct{})
	l.resumeCh = make(chan struct{})
	// When state is Paused, workers block on <-l.resumeCh.
	if err := l.ops.UpdateState(ctx, l.id, storage.StatePaused, ""); err != nil {
		return fmt.Errorf("persist paused: %w", err)
	}
	l.recordEvent(ctx, storage.EventLevelInfo, "loader_paused", "loader paused by operator", nil)
	return nil
}

// Resume unblocks workers after a Pause. Safe if never paused (no-op).
func (l *Loader) Resume(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.state != storage.StatePaused {
		return fmt.Errorf("cannot resume from state %s", l.state)
	}
	l.state = storage.StateRunning
	close(l.resumeCh)
	if err := l.ops.UpdateState(ctx, l.id, storage.StateRunning, ""); err != nil {
		return fmt.Errorf("persist resumed: %w", err)
	}
	l.recordEvent(ctx, storage.EventLevelInfo, "loader_resumed", "loader resumed", nil)
	return nil
}

// Stop asks the loader to exit. Workers drain their current batch, then
// exit the loop on the next iteration. The state store is updated to
// StateStopping; the final StateStopped / StateCompleted is written by Run.
func (l *Loader) Stop(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch l.state {
	case storage.StateStopping, storage.StateStopped, storage.StateCompleted, storage.StateFailed:
		return nil
	}
	l.state = storage.StateStopping
	// Closing stopCh is safe if the caller never calls Stop twice; we
	// defend against that via the state gate above.
	close(l.stopCh)
	// If paused, unblock workers so they can observe stopCh.
	select {
	case <-l.resumeCh:
	default:
		close(l.resumeCh)
	}
	if err := l.ops.UpdateState(ctx, l.id, storage.StateStopping, ""); err != nil {
		return fmt.Errorf("persist stopping: %w", err)
	}
	l.recordEvent(ctx, storage.EventLevelInfo, "loader_stopping", "loader stop requested", nil)
	return nil
}

// Run blocks until the loader completes, is stopped, or ctx is cancelled.
// It performs:
//  1. preflight (caller may opt out via ForceStart)
//  2. index creation per target using the default template set
//  3. resume from persisted progress, skipping completed targets
//  4. worker-pool loop writing batches and persisting progress
//  5. final state transition and stats persistence
func (l *Loader) Run(ctx context.Context) error {
	params := l.live.get()
	l.startedAt = time.Now().UTC()

	l.mu.Lock()
	l.state = storage.StateRunning
	l.mu.Unlock()

	if err := l.ops.UpdateState(ctx, l.id, storage.StateRunning, ""); err != nil {
		return fmt.Errorf("persist running: %w", err)
	}
	l.recordEvent(ctx, storage.EventLevelInfo, "loader_started", "loader started", map[string]any{
		"targets": len(l.spec.Entries), "workers": params.Workers, "batch_size": params.BatchSize,
	})
	if l.collector != nil {
		l.collector.MarkActive()
		defer l.collector.MarkIdle()
	}

	// Preflight storage headroom check.
	if !params.ForceStart {
		pf, err := Preflight(ctx, l.mc, l.spec, params.StorageHeadroom)
		if err != nil {
			return l.fail(ctx, fmt.Errorf("preflight: %w", err))
		}
		if !pf.Safe {
			return l.fail(ctx, fmt.Errorf("preflight unsafe: %s", pf.Reason))
		}
	}

	// Ensure indexes exist for every target so inserts incur realistic
	// index-maintenance cost (spec § 5.1).
	if err := l.ensureIndexes(ctx); err != nil {
		return l.fail(ctx, err)
	}

	// Initialize progress rows for every target (sets total_target).
	for _, t := range l.spec.Entries {
		if err := l.ops.UpsertProgress(ctx, storage.Progress{
			OperationID:   l.id,
			CollectionKey: t.Key(),
			TotalTarget:   t.BytesTarget,
			LastUpdated:   time.Now().UTC(),
		}); err != nil {
			return l.fail(ctx, fmt.Errorf("init progress: %w", err))
		}
	}
	// Seed cumulative counters from any persisted progress (resume).
	if err := l.loadPersistedCounters(ctx); err != nil {
		return l.fail(ctx, err)
	}

	if err := l.runWorkers(ctx); err != nil {
		return l.fail(ctx, err)
	}

	final := storage.StateCompleted
	if l.wasStopped() {
		final = storage.StateStopped
	}
	return l.finalize(ctx, final, "")
}

// ensureIndexes creates a set of universally-applicable indexes on each
// target so loads incur realistic index-maintenance cost without forcing a
// per-collection template choice. The two indexes here are guaranteed to
// match every template in DefaultTemplates() because every generator emits
// `created_at` and `_load_run_id` (see internal/generator.defaultCommonFields).
//
// Per-template index specs (e.g. UserProfile's unique-email or Order's
// customer-status compound) are intentionally NOT applied across the board:
// since the registry picks templates at random per document, applying
// UserProfile's unique-email index to a collection that mostly receives
// EventLog/Telemetry/Telemetry docs causes every batch to fail on
// `email: null` collisions. The per-template IndexSpecs metadata is still
// available for future "single-template-per-collection" loads.
func (l *Loader) ensureIndexes(ctx context.Context) error {
	universal := []generator.IndexSpec{
		{Name: "idx_created_at", Keys: map[string]int{"created_at": 1}},
		{Name: "idx_load_run_id", Keys: map[string]int{"_load_run_id": 1}},
	}
	for _, t := range l.spec.Entries {
		coll := l.mc.Raw().Database(t.Database).Collection(t.Collection)
		models := make([]mongo.IndexModel, 0, len(universal))
		for _, s := range universal {
			keys := bson.D{}
			for k, v := range s.Keys {
				keys = append(keys, bson.E{Key: k, Value: v})
			}
			models = append(models, mongo.IndexModel{
				Keys:    keys,
				Options: options.Index().SetName(s.Name),
			})
		}
		if _, err := coll.Indexes().CreateMany(ctx, models); err != nil {
			l.logger.Warn().Err(err).Str("target", t.Key()).Msg("index create failed; continuing")
		}
	}
	return nil
}

// loadPersistedCounters reads operation_progress into the in-memory counters
// so throughput + ETA math stays correct across pause/resume/restart.
func (l *Loader) loadPersistedCounters(ctx context.Context) error {
	rows, err := l.ops.ListProgress(ctx, l.id)
	if err != nil {
		return fmt.Errorf("load progress: %w", err)
	}
	for _, p := range rows {
		l.stats.docs.Add(p.CompletedCount)
		l.stats.bytes.Add(p.BytesProcessed)
	}
	return nil
}

// runWorkers is the heart of the loader: N goroutines pulling batches from a
// shared target queue and writing them into MongoDB.
func (l *Loader) runWorkers(ctx context.Context) error {
	params := l.live.get()
	g, gctx := errgroup.WithContext(ctx)

	// Shared queue of remaining byte targets per entry. Workers decrement
	// atomically; a target is complete when its remaining reaches 0.
	remaining := make([]*atomic.Int64, len(l.spec.Entries))
	for i, t := range l.spec.Entries {
		v := new(atomic.Int64)
		v.Store(t.BytesTarget - l.persistedBytes(ctx, t.Key()))
		remaining[i] = v
	}

	wc := writeConcernFor(params.WriteConcern)

	for w := 0; w < params.Workers; w++ {
		workerID := w
		g.Go(func() error {
			ActiveWorkers.Inc()
			defer ActiveWorkers.Dec()

			rng := rand.New(rand.NewSource(params.Seed + int64(workerID)))
			fk := gofakeit.NewFaker(rng, false)

			limiter := l.rateLimiter()
			return l.workerLoop(gctx, workerID, rng, fk, limiter, remaining, wc)
		})
	}
	return g.Wait()
}

func (l *Loader) persistedBytes(ctx context.Context, key string) int64 {
	rows, err := l.ops.ListProgress(ctx, l.id)
	if err != nil {
		return 0
	}
	for _, r := range rows {
		if r.CollectionKey == key {
			return r.BytesProcessed
		}
	}
	return 0
}

// workerLoop executes batches until every target is satisfied or the loader
// is stopped/paused. It honors the live rate limit and live batch size.
func (l *Loader) workerLoop(
	ctx context.Context, workerID int, rng *rand.Rand, fk *gofakeit.Faker,
	limiter *rate.Limiter, remaining []*atomic.Int64, wc *writeconcern.WriteConcern,
) error {
	for {
		// Check control signals first.
		if err := ctx.Err(); err != nil {
			return err
		}
		if l.wasStopped() {
			return nil
		}
		if l.State() == storage.StatePaused {
			select {
			case <-l.resumeChSnapshot():
			case <-ctx.Done():
				return ctx.Err()
			case <-l.stopCh:
				return nil
			}
			continue
		}

		// Pick a target that still has remaining bytes.
		targetIdx, ok := l.nextTarget(rng, remaining)
		if !ok {
			return nil // all targets complete
		}
		t := l.spec.Entries[targetIdx]

		params := l.live.get()
		batchSize := params.BatchSize
		docs := make([]any, 0, batchSize)
		var batchBytes int64
		for i := 0; i < batchSize; i++ {
			tpl := l.registry.Pick(rng)
			doc, size, err := tpl.Generate(rng, fk, params.LoadRunID)
			if err != nil {
				Errors.WithLabelValues("generate").Inc()
				continue
			}
			docs = append(docs, doc)
			batchBytes += int64(size)
		}
		if len(docs) == 0 {
			continue
		}

		// Rate limit on docs-per-second if configured.
		if limiter != nil {
			if err := limiter.WaitN(ctx, len(docs)); err != nil {
				return err
			}
		}

		start := time.Now()
		coll := l.mc.Raw().Database(t.Database).Collection(t.Collection)
		insertOpts := options.InsertMany().SetOrdered(false)
		_ = wc // per-op write concern is a future enhancement; URI-level WC applies.

		// Retry transient errors (managed cluster timeouts, primary
		// stepdown, network blips) up to 20 times with exponential
		// backoff. Persistent failures (auth, config) propagate after
		// the budget is exhausted so the operator sees the real error.
		err := retry.Do(ctx, retry.Default(), nil, func(ctx context.Context, attempt int) error {
			_, e := coll.InsertMany(ctx, docs, insertOpts)
			if e != nil && attempt > 0 {
				Errors.WithLabelValues("insert_retry").Inc()
				l.logger.Debug().Int("attempt", attempt).Err(e).Str("target", t.Key()).Msg("batch insert retry")
			}
			return e
		})
		dur := time.Since(start)
		BatchDuration.WithLabelValues(t.Database, t.Collection).Observe(dur.Seconds())
		if err != nil {
			l.stats.batchesKO.Add(1)
			Errors.WithLabelValues("insert").Inc()
			l.logger.Warn().Err(err).Str("target", t.Key()).Msg("batch insert failed (after retries)")
			// Move on to the next batch - a single chronically-failing
			// batch must not stall the whole loader.
			continue
		}

		// Progress bookkeeping.
		l.stats.batchesOK.Add(1)
		l.stats.docs.Add(int64(len(docs)))
		l.stats.bytes.Add(batchBytes)
		DocsInserted.WithLabelValues(t.Database, t.Collection).Add(float64(len(docs)))
		BytesInserted.WithLabelValues(t.Database, t.Collection).Add(float64(batchBytes))
		remaining[targetIdx].Add(-batchBytes)

		if err := l.ops.IncProgress(ctx, l.id, t.Key(), int64(len(docs)), batchBytes, t.BytesTarget); err != nil {
			l.logger.Warn().Err(err).Msg("persist progress failed")
		}
	}
}

// nextTarget selects an incomplete target pseudo-randomly so workers spread
// load across all configured collections.
func (l *Loader) nextTarget(rng *rand.Rand, remaining []*atomic.Int64) (int, bool) {
	indices := rng.Perm(len(remaining))
	for _, i := range indices {
		if remaining[i].Load() > 0 {
			return i, true
		}
	}
	return 0, false
}

// rateLimiter returns an x/time/rate limiter based on live params, or nil
// when DocsPerSecond is 0 (unlimited).
func (l *Loader) rateLimiter() *rate.Limiter {
	p := l.live.get()
	if p.DocsPerSecond <= 0 {
		return nil
	}
	// One limiter per worker so aggregate equals Workers * DocsPerSecond /
	// Workers == DocsPerSecond in aggregate when split evenly.
	per := p.DocsPerSecond / float64(maxInt(p.Workers, 1))
	return rate.NewLimiter(rate.Limit(per), maxInt(p.BatchSize, 1))
}

func (l *Loader) resumeChSnapshot() <-chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.resumeCh
}

func (l *Loader) wasStopped() bool {
	select {
	case <-l.stopCh:
		return true
	default:
		return false
	}
}

func (l *Loader) fail(ctx context.Context, err error) error {
	_ = l.ops.UpdateState(ctx, l.id, storage.StateFailed, err.Error())
	l.recordEvent(ctx, storage.EventLevelError, "loader_failed", err.Error(), nil)
	return err
}

func (l *Loader) finalize(ctx context.Context, state storage.OperationState, errMsg string) error {
	s := Stats{
		DocsInserted:  l.stats.docs.Load(),
		BytesInserted: l.stats.bytes.Load(),
		BatchesOK:     l.stats.batchesOK.Load(),
		BatchesFailed: l.stats.batchesKO.Load(),
		StartedAt:     l.startedAt,
		CompletedAt:   time.Now().UTC(),
	}
	s.Duration = s.CompletedAt.Sub(s.StartedAt)
	raw, _ := json.Marshal(s)
	_ = l.ops.SetStats(ctx, l.id, raw)
	if err := l.ops.UpdateState(ctx, l.id, state, errMsg); err != nil {
		return fmt.Errorf("finalize: %w", err)
	}
	l.recordEvent(ctx, storage.EventLevelInfo, "loader_"+string(state), "loader finalized", map[string]any{
		"docs": s.DocsInserted, "bytes": s.BytesInserted,
	})
	l.mu.Lock()
	l.state = state
	l.mu.Unlock()
	return nil
}

func (l *Loader) recordEvent(ctx context.Context, level storage.EventLevel, category, message string, ctxMap map[string]any) {
	if l.events == nil {
		return
	}
	id := l.id
	_, _ = l.events.Record(ctx, storage.Event{
		OperationID: &id,
		Level:       level,
		Category:    category,
		Message:     message,
		Context:     ctxMap,
	})
	opevents.Publish(category, map[string]any{
		"operation_id": l.id,
		"kind":         "loader",
		"message":      message,
		"context":      ctxMap,
	})
}

func writeConcernFor(s string) *writeconcern.WriteConcern {
	switch s {
	case "", "1":
		return writeconcern.W1()
	case "majority":
		return writeconcern.Majority()
	}
	return nil
}

func closedChan() chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// SnapshotStats returns a live copy of in-flight counters. Used by the API
// handler to render progress responses without touching the state store.
func (l *Loader) SnapshotStats() Stats {
	return Stats{
		DocsInserted:  l.stats.docs.Load(),
		BytesInserted: l.stats.bytes.Load(),
		BatchesOK:     l.stats.batchesOK.Load(),
		BatchesFailed: l.stats.batchesKO.Load(),
		StartedAt:     l.startedAt,
	}
}
