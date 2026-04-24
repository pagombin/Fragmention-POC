// Package workload drives representative concurrent MongoDB traffic while
// reclaim operations run, so the report can correlate compact/initial-sync
// activity with customer-visible latency (spec § 5.5). One Workload
// instance runs one mix (reads / writes / aggregates) against one target
// scope with a live-adjustable rate.
package workload

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"

	"github.com/pagombin/fragmention-poc/internal/metrics"
	mongoClient "github.com/pagombin/fragmention-poc/internal/mongo"
	"github.com/pagombin/fragmention-poc/internal/storage"
)

// OpType enumerates the workload operation kinds.
type OpType string

// Op kinds.
const (
	OpRead      OpType = "read"
	OpWrite     OpType = "write"
	OpAggregate OpType = "aggregate"
)

// Target is one collection the workload drives traffic against.
type Target struct {
	Database   string `json:"database"`
	Collection string `json:"collection"`
}

// Key returns "database.collection" for the state store.
func (t Target) Key() string { return t.Database + "." + t.Collection }

// Spec describes a single workload invocation.
type Spec struct {
	Targets        []Target `json:"targets"`
	ReadWeight     float64  `json:"read_weight"`
	WriteWeight    float64  `json:"write_weight"`
	AggregateWeight float64 `json:"aggregate_weight"`
}

// Params carries the live-adjustable knobs (target rate, concurrency).
type Params struct {
	TargetOpsPerSec float64 `json:"target_ops_per_sec"`
	Workers         int     `json:"workers"`
}

// Validate reports common misconfigurations.
func (p Params) Validate() error {
	if p.TargetOpsPerSec < 0 {
		return errors.New("target_ops_per_sec must be >= 0")
	}
	if p.Workers < 0 {
		return errors.New("workers must be >= 0")
	}
	return nil
}

// Prometheus metrics. Histogram buckets span 100µs..10s to cover the full
// operational spectrum during compact/initial-sync pressure.
var (
	OpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "mfpoc", Subsystem: "workload",
		Name: "op_duration_seconds", Help: "Workload operation latency.",
		Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"op_type"})
	OpsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mfpoc", Subsystem: "workload",
		Name: "ops_total", Help: "Workload operations executed.",
	}, []string{"op_type", "outcome"})
	Errors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mfpoc", Subsystem: "workload",
		Name: "errors_total", Help: "Workload errors by reason.",
	}, []string{"reason"})
)

func init() {
	metrics.Registry.MustRegister(OpDuration, OpsTotal, Errors)
}

// Workload is one running workload instance.
type Workload struct {
	id     string
	logger zerolog.Logger
	mc     *mongoClient.Client
	ops    *storage.Operations
	events *storage.Events

	spec Spec
	live *liveParams

	mu       sync.Mutex
	state    storage.OperationState
	stopCh   chan struct{}

	opsDone   atomic.Int64
	errs      atomic.Int64
	startedAt time.Time
}

type liveParams struct {
	mu sync.RWMutex
	p  Params
	limiter *rate.Limiter
}

func newLive(p Params) *liveParams {
	return &liveParams{p: p, limiter: buildLimiter(p)}
}
func (l *liveParams) get() Params { l.mu.RLock(); defer l.mu.RUnlock(); return l.p }
func (l *liveParams) set(p Params) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.p = p
	l.limiter = buildLimiter(p)
}
func (l *liveParams) lim() *rate.Limiter {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.limiter
}

func buildLimiter(p Params) *rate.Limiter {
	if p.TargetOpsPerSec <= 0 {
		return nil
	}
	return rate.NewLimiter(rate.Limit(p.TargetOpsPerSec), maxInt(int(p.TargetOpsPerSec/5), 1))
}

// Deps is the shared dependency set.
type Deps struct {
	Logger zerolog.Logger
	Mongo  *mongoClient.Client
	Ops    *storage.Operations
	Events *storage.Events
}

// New constructs a Workload.
func New(id string, d Deps, spec Spec, p Params) (*Workload, error) {
	if id == "" {
		return nil, errors.New("workload: id required")
	}
	if len(spec.Targets) == 0 {
		return nil, errors.New("workload: at least one target required")
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if p.Workers == 0 {
		p.Workers = 8
	}
	if spec.ReadWeight+spec.WriteWeight+spec.AggregateWeight == 0 {
		spec.ReadWeight = 0.7
		spec.WriteWeight = 0.2
		spec.AggregateWeight = 0.1
	}
	return &Workload{
		id:     id,
		logger: d.Logger.With().Str("service", "workload").Str("operation_id", id).Logger(),
		mc:     d.Mongo,
		ops:    d.Ops,
		events: d.Events,
		spec:   spec,
		live:   newLive(p),
		stopCh: make(chan struct{}),
		state:  storage.StateIdle,
	}, nil
}

// ID returns the operation id.
func (w *Workload) ID() string { return w.id }

// State returns the current lifecycle state.
func (w *Workload) State() storage.OperationState {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.state
}

// Params returns live params.
func (w *Workload) Params() Params { return w.live.get() }

// SetParams atomically swaps the live params. Changes take effect within
// one op (the token bucket is rebuilt).
func (w *Workload) SetParams(p Params) error {
	cur := w.live.get()
	if p.Workers == 0 {
		p.Workers = cur.Workers
	}
	if err := p.Validate(); err != nil {
		return err
	}
	w.live.set(p)
	return nil
}

// Stop requests termination. Workers exit at the next op boundary.
func (w *Workload) Stop(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch w.state {
	case storage.StateStopping, storage.StateStopped, storage.StateCompleted, storage.StateFailed:
		return nil
	}
	w.state = storage.StateStopping
	close(w.stopCh)
	if err := w.ops.UpdateState(ctx, w.id, storage.StateStopping, ""); err != nil {
		return err
	}
	w.emit(ctx, storage.EventLevelInfo, "workload_stopped", "workload stop requested")
	return nil
}

// Run blocks until Stop or ctx cancellation. Workers run indefinitely.
func (w *Workload) Run(ctx context.Context) error {
	w.startedAt = time.Now().UTC()
	w.mu.Lock()
	w.state = storage.StateRunning
	w.mu.Unlock()
	if err := w.ops.UpdateState(ctx, w.id, storage.StateRunning, ""); err != nil {
		return err
	}
	w.emit(ctx, storage.EventLevelInfo, "workload_started", "workload started")

	params := w.live.get()
	g, gctx := errgroup.WithContext(ctx)
	for i := 0; i < params.Workers; i++ {
		wid := i
		g.Go(func() error {
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(wid)))
			return w.workerLoop(gctx, rng)
		})
	}
	err := g.Wait()
	return w.finalize(ctx, err)
}

func (w *Workload) workerLoop(ctx context.Context, rng *rand.Rand) error {
	for {
		if w.stopped() {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if lim := w.live.lim(); lim != nil {
			if err := lim.Wait(ctx); err != nil {
				return err
			}
		}
		target := w.spec.Targets[rng.Intn(len(w.spec.Targets))]
		opKind := pickOp(rng, w.spec)
		w.runOne(ctx, rng, target, opKind)
	}
}

func (w *Workload) runOne(ctx context.Context, rng *rand.Rand, t Target, op OpType) {
	coll := w.mc.Raw().Database(t.Database).Collection(t.Collection)
	start := time.Now()
	var err error
	switch op {
	case OpRead:
		err = w.doRead(ctx, coll, rng)
	case OpWrite:
		err = w.doWrite(ctx, coll, rng)
	case OpAggregate:
		err = w.doAggregate(ctx, coll)
	}
	dur := time.Since(start)
	OpDuration.WithLabelValues(string(op)).Observe(dur.Seconds())
	outcome := "ok"
	if err != nil {
		outcome = "err"
		w.errs.Add(1)
		Errors.WithLabelValues(reasonFor(err)).Inc()
	}
	OpsTotal.WithLabelValues(string(op), outcome).Inc()
	w.opsDone.Add(1)
}

func (w *Workload) doRead(ctx context.Context, c *mongo.Collection, rng *rand.Rand) error {
	// Find by a random offset using skip+limit; cheap and indexed via _id.
	cur, err := c.Find(ctx, bson.M{}, nil)
	if err != nil {
		return err
	}
	defer func() { _ = cur.Close(ctx) }()
	// Advance a handful of docs to simulate a typical query result shape.
	for i := 0; i < 5 && cur.Next(ctx); i++ {
	}
	_ = rng // kept for signature symmetry
	return cur.Err()
}

func (w *Workload) doWrite(ctx context.Context, c *mongo.Collection, rng *rand.Rand) error {
	_, err := c.InsertOne(ctx, bson.M{
		"_w":         "workload",
		"v":          rng.Int63(),
		"created_at": time.Now(),
	})
	return err
}

func (w *Workload) doAggregate(ctx context.Context, c *mongo.Collection) error {
	cur, err := c.Aggregate(ctx, bson.A{
		bson.D{{Key: "$match", Value: bson.M{}}},
		bson.D{{Key: "$group", Value: bson.M{"_id": nil, "n": bson.M{"$sum": 1}}}},
	})
	if err != nil {
		return err
	}
	defer func() { _ = cur.Close(ctx) }()
	for cur.Next(ctx) {
	}
	return cur.Err()
}

func (w *Workload) finalize(ctx context.Context, runErr error) error {
	state := storage.StateCompleted
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		state = storage.StateFailed
	} else if w.stopped() {
		state = storage.StateStopped
	}
	s := struct {
		OpsDone   int64         `json:"ops_done"`
		Errors    int64         `json:"errors"`
		StartedAt time.Time     `json:"started_at"`
		StoppedAt time.Time     `json:"stopped_at"`
		Duration  time.Duration `json:"duration_ns"`
	}{
		OpsDone:   w.opsDone.Load(),
		Errors:    w.errs.Load(),
		StartedAt: w.startedAt,
		StoppedAt: time.Now().UTC(),
	}
	s.Duration = s.StoppedAt.Sub(s.StartedAt)
	raw, _ := json.Marshal(s)
	_ = w.ops.SetStats(ctx, w.id, raw)
	errMsg := ""
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		errMsg = runErr.Error()
	}
	if err := w.ops.UpdateState(ctx, w.id, state, errMsg); err != nil {
		return err
	}
	w.emit(ctx, storage.EventLevelInfo, "workload_"+string(state), "workload finalized")
	w.mu.Lock()
	w.state = state
	w.mu.Unlock()
	return nil
}

func (w *Workload) stopped() bool {
	select {
	case <-w.stopCh:
		return true
	default:
		return false
	}
}

func (w *Workload) emit(ctx context.Context, level storage.EventLevel, category, message string) {
	if w.events == nil {
		return
	}
	id := w.id
	_, _ = w.events.Record(ctx, storage.Event{
		OperationID: &id, Level: level, Category: category, Message: message,
	})
}

// SnapshotStats returns live counters.
func (w *Workload) SnapshotStats() (opsDone, errs int64) {
	return w.opsDone.Load(), w.errs.Load()
}

func pickOp(rng *rand.Rand, s Spec) OpType {
	total := s.ReadWeight + s.WriteWeight + s.AggregateWeight
	x := rng.Float64() * total
	if x < s.ReadWeight {
		return OpRead
	}
	if x < s.ReadWeight+s.WriteWeight {
		return OpWrite
	}
	return OpAggregate
}

func reasonFor(err error) string {
	// Keep labels low-cardinality; anything beyond a handful of buckets
	// explodes Prometheus cardinality.
	if err == nil {
		return "none"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "mongo"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// newID is a tiny factory for testability.
var newID = func() string { return uuid.NewString() }
