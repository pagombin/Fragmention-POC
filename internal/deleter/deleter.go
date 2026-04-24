package deleter

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/pagombin/fragmention-poc/internal/collector"
	mongoClient "github.com/pagombin/fragmention-poc/internal/mongo"
	"github.com/pagombin/fragmention-poc/internal/opevents"
	"github.com/pagombin/fragmention-poc/internal/storage"
)

// Deleter orchestrates one deletion operation.
type Deleter struct {
	id        string
	logger    zerolog.Logger
	mc        *mongoClient.Client
	ops       *storage.Operations
	events    *storage.Events
	collector *collector.Collector
	depsStore *storage.Store

	spec TargetSpec
	live *live

	mu       sync.Mutex
	state    storage.OperationState
	pauseCh  chan struct{}
	resumeCh chan struct{}
	stopCh   chan struct{}

	deleted     atomic.Int64
	batchesOK   atomic.Int64
	batchesKO   atomic.Int64
	perColl     sync.Map // map[string]*atomic.Int64
	startedAt   time.Time
}

// Deps carries the service-level dependencies shared across deleter
// instances.
type Deps struct {
	Logger    zerolog.Logger
	Mongo     *mongoClient.Client
	Ops       *storage.Operations
	Events    *storage.Events
	Collector *collector.Collector
	Store     *storage.Store // for delete_candidates table access
}

// New constructs a Deleter.
func New(id string, d Deps, spec TargetSpec, p Params) (*Deleter, error) {
	if id == "" {
		return nil, errors.New("deleter: id required")
	}
	if len(spec.Entries) == 0 {
		return nil, errors.New("deleter: spec must contain at least one target")
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	for _, e := range spec.Entries {
		if err := e.Pattern.Validate(p.MaxRatio); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Key(), err)
		}
	}
	if d.Store == nil {
		return nil, errors.New("deleter: Store required (for delete_candidates)")
	}
	return &Deleter{
		id:        id,
		logger:    d.Logger.With().Str("service", "deleter").Str("operation_id", id).Logger(),
		mc:        d.Mongo,
		ops:       d.Ops,
		events:    d.Events,
		collector: d.Collector,
		depsStore: d.Store,
		spec:      spec,
		live:      newLive(p),
		pauseCh:   make(chan struct{}),
		resumeCh:  closedChan(),
		stopCh:    make(chan struct{}),
		state:     storage.StateIdle,
	}, nil
}

// ID returns the operation id.
func (d *Deleter) ID() string { return d.id }

// State returns the current lifecycle state.
func (d *Deleter) State() storage.OperationState {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state
}

// Params returns the live parameters.
func (d *Deleter) Params() Params { return d.live.get() }

// SetParams swaps in a new param set. Only BatchSize and InterBatchJitter
// take effect mid-flight.
func (d *Deleter) SetParams(p Params) error {
	cur := d.live.get()
	p.MaxRatio = cur.MaxRatio
	if err := p.Validate(); err != nil {
		return err
	}
	d.live.set(p)
	return nil
}

// Pause asks the deleter to pause. Semantics mirror the loader: in-flight
// batches complete, then workers block until Resume is called.
func (d *Deleter) Pause(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.state != storage.StateRunning {
		return fmt.Errorf("cannot pause from state %s", d.state)
	}
	d.state = storage.StatePaused
	d.pauseCh = make(chan struct{})
	d.resumeCh = make(chan struct{})
	if err := d.ops.UpdateState(ctx, d.id, storage.StatePaused, ""); err != nil {
		return err
	}
	d.emit(ctx, storage.EventLevelInfo, "deleter_paused", "deleter paused")
	return nil
}

// Resume unblocks workers after a Pause.
func (d *Deleter) Resume(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.state != storage.StatePaused {
		return fmt.Errorf("cannot resume from state %s", d.state)
	}
	d.state = storage.StateRunning
	close(d.resumeCh)
	if err := d.ops.UpdateState(ctx, d.id, storage.StateRunning, ""); err != nil {
		return err
	}
	d.emit(ctx, storage.EventLevelInfo, "deleter_resumed", "deleter resumed")
	return nil
}

// Stop asks the deleter to exit after the current batch.
func (d *Deleter) Stop(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch d.state {
	case storage.StateStopping, storage.StateStopped, storage.StateCompleted, storage.StateFailed:
		return nil
	}
	d.state = storage.StateStopping
	close(d.stopCh)
	select {
	case <-d.resumeCh:
	default:
		close(d.resumeCh)
	}
	if err := d.ops.UpdateState(ctx, d.id, storage.StateStopping, ""); err != nil {
		return err
	}
	d.emit(ctx, storage.EventLevelInfo, "deleter_stopping", "deleter stop requested")
	return nil
}

// Run executes the full delete flow: for each target, capture candidate
// IDs if needed, persist them, then delete in batches.
func (d *Deleter) Run(ctx context.Context) error {
	d.startedAt = time.Now().UTC()
	d.mu.Lock()
	d.state = storage.StateRunning
	d.mu.Unlock()
	if err := d.ops.UpdateState(ctx, d.id, storage.StateRunning, ""); err != nil {
		return err
	}
	d.emit(ctx, storage.EventLevelInfo, "deleter_started", "deleter started")
	if d.collector != nil {
		d.collector.MarkActive()
		defer d.collector.MarkIdle()
	}

	for _, t := range d.spec.Entries {
		if d.wasStopped() {
			break
		}
		if err := d.runTarget(ctx, t); err != nil {
			return d.fail(ctx, fmt.Errorf("%s: %w", t.Key(), err))
		}
	}

	final := storage.StateCompleted
	if d.wasStopped() {
		final = storage.StateStopped
	}

	// Auto-tag a post_delete snapshot if a collector is wired in
	// (spec § 5.2 "After deletions complete, automatically trigger a metrics
	// collection snapshot tagged as post_delete").
	if d.collector != nil && final == storage.StateCompleted {
		if _, err := d.collector.TakeSnapshot(ctx, "post_delete", "auto after deleter "+d.id, ""); err != nil {
			d.logger.Warn().Err(err).Msg("auto post_delete snapshot failed")
		}
	}
	return d.finalize(ctx, final, "")
}

// runTarget captures candidates if the pattern requires it, then deletes
// in batches with live BatchSize and jitter.
func (d *Deleter) runTarget(ctx context.Context, t Target) error {
	coll := d.mc.Raw().Database(t.Database).Collection(t.Collection)
	pattern, err := PatternFor(t.Pattern)
	if err != nil {
		return err
	}
	// If candidates were already persisted (resume), reuse them.
	candidates, err := d.loadCandidates(ctx, t.Key())
	if err != nil {
		return err
	}
	if candidates == nil {
		candidates, err = pattern.CaptureCandidates(ctx, coll, t.Pattern)
		if err != nil {
			return fmt.Errorf("capture candidates: %w", err)
		}
		if candidates != nil {
			if err := d.persistCandidates(ctx, t.Key(), candidates); err != nil {
				return err
			}
		}
	}

	if candidates != nil {
		return d.deleteFromCandidates(ctx, t, coll, pattern, candidates)
	}
	// No candidate list - apply the server-side filter in batches, bounded
	// by the pattern's filter plus _id > last_id cursor style.
	return d.deleteByFilter(ctx, t, coll, pattern)
}

// deleteFromCandidates consumes a persisted _id slice in BatchSize chunks.
func (d *Deleter) deleteFromCandidates(ctx context.Context, t Target, coll *mongo.Collection, pattern Pattern, candidates []any) error {
	offset, err := d.loadCandidateOffset(ctx, t.Key())
	if err != nil {
		return err
	}
	for offset < int64(len(candidates)) {
		if d.wasStopped() {
			return nil
		}
		if d.State() == storage.StatePaused {
			select {
			case <-d.resumeChSnapshot():
			case <-ctx.Done():
				return ctx.Err()
			case <-d.stopCh:
				return nil
			}
			continue
		}
		params := d.live.get()
		end := offset + int64(params.BatchSize)
		if end > int64(len(candidates)) {
			end = int64(len(candidates))
		}
		batch := candidates[offset:end]
		start := time.Now()
		res, err := coll.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": batch}})
		dur := time.Since(start)
		BatchDuration.WithLabelValues(t.Database, t.Collection, string(pattern.Name())).Observe(dur.Seconds())
		if err != nil {
			d.batchesKO.Add(1)
			Errors.WithLabelValues("delete_many").Inc()
			d.logger.Warn().Err(err).Str("target", t.Key()).Msg("delete batch failed")
			// brief backoff then continue
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
			continue
		}
		d.batchesOK.Add(1)
		d.deleted.Add(res.DeletedCount)
		d.bumpCollCount(t.Key(), res.DeletedCount)
		DocsDeleted.WithLabelValues(t.Database, t.Collection, string(pattern.Name())).Add(float64(res.DeletedCount))

		offset = end
		if err := d.saveCandidateOffset(ctx, t.Key(), offset); err != nil {
			d.logger.Warn().Err(err).Msg("save candidate offset failed")
		}
		if err := d.ops.IncProgress(ctx, d.id, t.Key(), res.DeletedCount, 0, int64(len(candidates))); err != nil {
			d.logger.Warn().Err(err).Msg("persist progress failed")
		}
		if params.InterBatchJitter > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(params.InterBatchJitter):
			}
		}
	}
	return nil
}

// deleteByFilter runs DeleteMany-in-batches using a server-side filter.
func (d *Deleter) deleteByFilter(ctx context.Context, t Target, coll *mongo.Collection, pattern Pattern) error {
	base, err := pattern.BuildFilter(ctx, coll, t.Pattern)
	if err != nil {
		return err
	}
	target, err := pattern.EstimateMatchCount(ctx, coll, t.Pattern, base)
	if err != nil {
		return err
	}
	for {
		cont, err := d.waitIfPaused(ctx)
		if err != nil {
			return err
		}
		if !cont {
			return nil
		}
		if err := d.filterIteration(ctx, t, coll, pattern, base, target); err != nil {
			if errors.Is(err, errNoMore) {
				return nil
			}
			return err
		}
	}
}

// errNoMore signals deleteByFilter that the target has no remaining matches.
var errNoMore = errors.New("no more matches")

// waitIfPaused returns (continue, err). continue=false means the caller
// should exit cleanly (stopped or context cancelled).
func (d *Deleter) waitIfPaused(ctx context.Context) (bool, error) {
	if d.wasStopped() {
		return false, nil
	}
	if d.State() != storage.StatePaused {
		return true, nil
	}
	select {
	case <-d.resumeChSnapshot():
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	case <-d.stopCh:
		return false, nil
	}
}

// filterIteration performs one scan+delete batch. Returns errNoMore when
// the filter has no remaining matches.
func (d *Deleter) filterIteration(ctx context.Context, t Target, coll *mongo.Collection, pattern Pattern, base bson.M, target int64) error {
	params := d.live.get()
	batchIDs, err := d.collectIDs(ctx, coll, base, params.BatchSize)
	if err != nil {
		return err
	}
	if len(batchIDs) == 0 {
		return errNoMore
	}
	start := time.Now()
	res, err := coll.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": batchIDs}})
	BatchDuration.WithLabelValues(t.Database, t.Collection, string(pattern.Name())).Observe(time.Since(start).Seconds())
	if err != nil {
		d.batchesKO.Add(1)
		Errors.WithLabelValues("delete_many").Inc()
		d.logger.Warn().Err(err).Str("target", t.Key()).Msg("delete batch failed")
		return sleepOrCancel(ctx, 200*time.Millisecond)
	}
	d.batchesOK.Add(1)
	d.deleted.Add(res.DeletedCount)
	d.bumpCollCount(t.Key(), res.DeletedCount)
	DocsDeleted.WithLabelValues(t.Database, t.Collection, string(pattern.Name())).Add(float64(res.DeletedCount))
	if err := d.ops.IncProgress(ctx, d.id, t.Key(), res.DeletedCount, 0, target); err != nil {
		d.logger.Warn().Err(err).Msg("persist progress failed")
	}
	if params.InterBatchJitter > 0 {
		return sleepOrCancel(ctx, params.InterBatchJitter)
	}
	return nil
}

func (d *Deleter) collectIDs(ctx context.Context, coll *mongo.Collection, base bson.M, batchSize int) ([]any, error) {
	cur, err := coll.Find(ctx, base, options.Find().SetProjection(bson.M{"_id": 1}).SetLimit(int64(batchSize)))
	if err != nil {
		return nil, fmt.Errorf("find candidates: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()
	var ids []any
	for cur.Next(ctx) {
		var doc struct {
			ID any `bson:"_id"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		ids = append(ids, doc.ID)
	}
	return ids, cur.Err()
}

func sleepOrCancel(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// persistCandidates gob-encodes the slice into delete_candidates so
// pause/resume sees the same set.
func (d *Deleter) persistCandidates(ctx context.Context, collKey string, ids []any) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(ids); err != nil {
		return fmt.Errorf("encode candidates: %w", err)
	}
	_, err := d.storeDB().ExecContext(ctx, `
		INSERT INTO delete_candidates (operation_id, collection_key, candidate_ids_blob)
		VALUES (?, ?, ?)
		ON CONFLICT(operation_id, collection_key) DO UPDATE SET candidate_ids_blob = excluded.candidate_ids_blob
	`, d.id, collKey, buf.Bytes())
	if err != nil {
		return fmt.Errorf("persist candidates: %w", err)
	}
	return nil
}

func (d *Deleter) loadCandidates(ctx context.Context, collKey string) ([]any, error) {
	row := d.storeDB().QueryRowContext(ctx, `
		SELECT candidate_ids_blob FROM delete_candidates
		WHERE operation_id = ? AND collection_key = ?
	`, d.id, collKey)
	var blob []byte
	err := row.Scan(&blob)
	if err != nil {
		if errors.Is(err, sqlErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var ids []any
	if err := gob.NewDecoder(bytes.NewReader(blob)).Decode(&ids); err != nil {
		return nil, fmt.Errorf("decode candidates: %w", err)
	}
	return ids, nil
}

// candidateOffset is stored alongside the blob in a lightweight events
// context so resume can skip the already-deleted prefix of the list. We
// piggyback on operation_progress.completed_count for that math.
func (d *Deleter) loadCandidateOffset(ctx context.Context, collKey string) (int64, error) {
	rows, err := d.ops.ListProgress(ctx, d.id)
	if err != nil {
		return 0, err
	}
	for _, r := range rows {
		if r.CollectionKey == collKey {
			return r.CompletedCount, nil
		}
	}
	return 0, nil
}

func (d *Deleter) saveCandidateOffset(ctx context.Context, collKey string, offset int64) error {
	return d.ops.UpsertProgress(ctx, storage.Progress{
		OperationID:    d.id,
		CollectionKey:  collKey,
		CompletedCount: offset,
		LastUpdated:    time.Now().UTC(),
	})
}

func (d *Deleter) wasStopped() bool {
	select {
	case <-d.stopCh:
		return true
	default:
		return false
	}
}

func (d *Deleter) resumeChSnapshot() <-chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.resumeCh
}

func (d *Deleter) bumpCollCount(key string, n int64) {
	v, _ := d.perColl.LoadOrStore(key, new(atomic.Int64))
	v.(*atomic.Int64).Add(n)
}

func (d *Deleter) fail(ctx context.Context, err error) error {
	_ = d.ops.UpdateState(ctx, d.id, storage.StateFailed, err.Error())
	d.emit(ctx, storage.EventLevelError, "deleter_failed", err.Error())
	return err
}

func (d *Deleter) finalize(ctx context.Context, state storage.OperationState, errMsg string) error {
	per := map[string]int64{}
	d.perColl.Range(func(k, v any) bool {
		per[k.(string)] = v.(*atomic.Int64).Load()
		return true
	})
	s := Stats{
		Deleted:       d.deleted.Load(),
		BatchesOK:     d.batchesOK.Load(),
		BatchesFailed: d.batchesKO.Load(),
		PerCollection: per,
		StartedAt:     d.startedAt,
		CompletedAt:   time.Now().UTC(),
	}
	s.Duration = s.CompletedAt.Sub(s.StartedAt)
	raw, _ := json.Marshal(s)
	_ = d.ops.SetStats(ctx, d.id, raw)
	if err := d.ops.UpdateState(ctx, d.id, state, errMsg); err != nil {
		return err
	}
	d.emit(ctx, storage.EventLevelInfo, "deleter_"+string(state), "deleter finalized")
	d.mu.Lock()
	d.state = state
	d.mu.Unlock()
	return nil
}

func (d *Deleter) emit(ctx context.Context, level storage.EventLevel, category, message string) {
	if d.events != nil {
		id := d.id
		_, _ = d.events.Record(ctx, storage.Event{
			OperationID: &id, Level: level, Category: category, Message: message,
		})
	}
	opevents.Publish(category, map[string]any{
		"operation_id": d.id,
		"kind":         "deleter",
		"message":      message,
	})
}

// SnapshotStats returns live counters for the API.
func (d *Deleter) SnapshotStats() Stats {
	per := map[string]int64{}
	d.perColl.Range(func(k, v any) bool {
		per[k.(string)] = v.(*atomic.Int64).Load()
		return true
	})
	return Stats{
		Deleted:       d.deleted.Load(),
		BatchesOK:     d.batchesOK.Load(),
		BatchesFailed: d.batchesKO.Load(),
		PerCollection: per,
		StartedAt:     d.startedAt,
	}
}

// generateToken returns a cryptographically-random hex token for preview
// confirmations. 32 bytes = 64 hex chars; a token cannot be guessed.
func generateToken() string {
	var b [32]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// newID is a tiny factory so tests can override.
var newID = func() string { return uuid.NewString() }

func closedChan() chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}
