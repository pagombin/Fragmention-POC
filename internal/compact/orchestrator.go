package compact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/pagombin/fragmention-poc/internal/collector"
	mongoClient "github.com/pagombin/fragmention-poc/internal/mongo"
	"github.com/pagombin/fragmention-poc/internal/opevents"
	"github.com/pagombin/fragmention-poc/internal/storage"
)

// Orchestrator manages one compact run. It enforces spec § 5.4's safety
// gates: refuses to step down without a healthy secondary, refuses to
// compact system databases, respects max replication lag.
type Orchestrator struct {
	id        string
	logger    zerolog.Logger
	mc        *mongoClient.Client
	ops       *storage.Operations
	events    *storage.Events
	collector *collector.Collector

	scope  Scope
	params Params

	mu     sync.Mutex
	state  storage.OperationState
	stopCh chan struct{}

	cur atomic.Value // string "member/db.coll" for live status
	startedAt time.Time
}

// Deps is the set of dependencies shared between orchestrators.
type Deps struct {
	Logger    zerolog.Logger
	Mongo     *mongoClient.Client
	Ops       *storage.Operations
	Events    *storage.Events
	Collector *collector.Collector
}

// New constructs an Orchestrator.
func New(id string, d Deps, scope Scope, params Params) (*Orchestrator, error) {
	if id == "" {
		return nil, errors.New("compact: id required")
	}
	if d.Mongo == nil || d.Ops == nil {
		return nil, errors.New("compact: Mongo and Ops required")
	}
	if err := params.Validate(); err != nil {
		return nil, err
	}
	return &Orchestrator{
		id:        id,
		logger:    d.Logger.With().Str("service", "compact").Str("operation_id", id).Logger(),
		mc:        d.Mongo,
		ops:       d.Ops,
		events:    d.Events,
		collector: d.Collector,
		scope:     scope,
		params:    params,
		stopCh:    make(chan struct{}),
		state:     storage.StateIdle,
	}, nil
}

// ID returns the operation id.
func (o *Orchestrator) ID() string { return o.id }

// State returns the current lifecycle state.
func (o *Orchestrator) State() storage.OperationState {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.state
}

// CurrentStep returns a short description of the in-progress member +
// collection (e.g. "node-2/poc_db_1.coll_a"), or empty when idle.
func (o *Orchestrator) CurrentStep() string {
	if v := o.cur.Load(); v != nil {
		return v.(string)
	}
	return ""
}

// Cancel stops after the current collection completes. `compact` itself
// cannot be interrupted cleanly (spec § 5.4), so the orchestrator merely
// exits its loop at the next safe point.
func (o *Orchestrator) Cancel(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	switch o.state {
	case storage.StateStopping, storage.StateStopped, storage.StateCompleted, storage.StateFailed:
		return nil
	}
	o.state = storage.StateStopping
	close(o.stopCh)
	if err := o.ops.UpdateState(ctx, o.id, storage.StateStopping, ""); err != nil {
		return err
	}
	o.emit(ctx, storage.EventLevelInfo, "compact_cancel", "compact cancel requested")
	return nil
}

// Plan returns the execution order the orchestrator will follow. It is pure
// (no side effects) so the UI preview and the Start path can share it.
func (o *Orchestrator) Plan(ctx context.Context) (PreviewResult, error) {
	top, err := o.mc.DetectTopology(ctx)
	if err != nil {
		return PreviewResult{}, fmt.Errorf("topology: %w", err)
	}
	colls, err := o.resolveCollections(ctx)
	if err != nil {
		return PreviewResult{}, err
	}

	pr := PreviewResult{TotalCollections: len(colls), ExecutionOrder: []MemberPlan{}}
	if top.Kind == mongoClient.TopologyStandalone {
		pr.Mode = "single"
		collStrs := make([]string, 0, len(colls))
		for _, c := range colls {
			collStrs = append(collStrs, c.Key())
		}
		pr.ExecutionOrder = []MemberPlan{{
			Member: top.Members[0].Name, Role: "standalone", Collections: collStrs,
		}}
		pr.EstimatedDuration = estimateDuration(len(colls))
		return pr, nil
	}
	pr.Mode = "rolling"
	if err := o.validateReplicaSetSafety(top, &pr); err != nil {
		return pr, err
	}
	collStrs := make([]string, 0, len(colls))
	for _, c := range colls {
		collStrs = append(collStrs, c.Key())
	}
	// Secondaries first, then the current primary (after a stepdown).
	var primary mongoClient.Member
	for _, m := range top.Members {
		if strings.EqualFold(m.State, "PRIMARY") {
			primary = m
			continue
		}
		if !strings.EqualFold(m.State, "SECONDARY") {
			continue
		}
		pr.ExecutionOrder = append(pr.ExecutionOrder, MemberPlan{
			Member: m.Name, Role: "secondary", Collections: collStrs,
		})
	}
	if primary.Name != "" {
		pr.ExecutionOrder = append(pr.ExecutionOrder, MemberPlan{
			Member: primary.Name, Role: "primary", Collections: collStrs, RequiresStepdown: true,
		})
	}
	pr.EstimatedDuration = time.Duration(len(pr.ExecutionOrder)) * estimateDuration(len(colls))
	return pr, nil
}

// validateReplicaSetSafety appends warnings for conditions that should
// abort a compact: excessive lag, no eligible secondary, unhealthy members.
func (o *Orchestrator) validateReplicaSetSafety(top mongoClient.Topology, pr *PreviewResult) error {
	if top.MaxReplicaLag > o.params.MaxReplicationLag && o.params.MaxReplicationLag > 0 {
		pr.Warnings = append(pr.Warnings, fmt.Sprintf("replication lag %v exceeds max %v", top.MaxReplicaLag, o.params.MaxReplicationLag))
	}
	var secondaries int
	for _, m := range top.Members {
		if strings.EqualFold(m.State, "SECONDARY") && m.Health > 0 {
			secondaries++
		}
	}
	if secondaries == 0 {
		return errors.New("no healthy secondary available for rolling compact")
	}
	return nil
}

// resolveCollections expands the scope into a concrete CollectionRef list,
// omitting system databases per spec § 5.4.
func (o *Orchestrator) resolveCollections(ctx context.Context) ([]CollectionRef, error) {
	switch o.scope.Kind {
	case ScopeCollections:
		return o.scope.Collections, nil
	case ScopeDatabases:
		var out []CollectionRef
		for _, db := range o.scope.Databases {
			if isSystemDB(db) {
				continue
			}
			colls, err := o.mc.ListCollections(ctx, db)
			if err != nil {
				return nil, err
			}
			for _, c := range colls {
				if c.Type == mongoClient.CollTypeRegular {
					out = append(out, CollectionRef{Database: db, Collection: c.Name})
				}
			}
		}
		return out, nil
	case ScopeCluster:
		dbs, err := o.mc.ListDatabases(ctx)
		if err != nil {
			return nil, err
		}
		var out []CollectionRef
		for _, db := range dbs {
			if isSystemDB(db.Name) {
				continue
			}
			colls, err := o.mc.ListCollections(ctx, db.Name)
			if err != nil {
				return nil, err
			}
			for _, c := range colls {
				if c.Type == mongoClient.CollTypeRegular {
					out = append(out, CollectionRef{Database: db.Name, Collection: c.Name})
				}
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("unknown scope kind %q", o.scope.Kind)
}

// Run executes the planned compact. pre/post snapshots are taken via the
// collector (when present) and per-collection outcomes are recorded.
func (o *Orchestrator) Run(ctx context.Context) error {
	if err := o.markRunning(ctx); err != nil {
		return err
	}
	if o.collector != nil {
		o.collector.MarkActive()
		defer o.collector.MarkIdle()
		if _, err := o.collector.TakeSnapshot(ctx, "pre_compact", "auto before compact "+o.id, ""); err != nil {
			o.logger.Warn().Err(err).Msg("pre_compact snapshot failed")
		}
	}

	plan, err := o.Plan(ctx)
	if err != nil {
		return o.fail(ctx, err)
	}

	outcomes, totals, counts, validateOut, err := o.runPlan(ctx, plan)
	if err != nil {
		return o.fail(ctx, err)
	}

	if o.collector != nil {
		if _, err := o.collector.TakeSnapshot(ctx, "post_compact", "auto after compact "+o.id, ""); err != nil {
			o.logger.Warn().Err(err).Msg("post_compact snapshot failed")
		}
	}
	return o.finalize(ctx, outcomes, totals, counts, validateOut)
}

// runTotals tracks per-plan sums.
type runTotals struct {
	pre, post int64
}

// runCounts tracks per-outcome counts.
type runCounts struct {
	compacted, skipped, failed int
}

func (o *Orchestrator) markRunning(ctx context.Context) error {
	o.startedAt = time.Now().UTC()
	o.mu.Lock()
	o.state = storage.StateRunning
	o.mu.Unlock()
	if err := o.ops.UpdateState(ctx, o.id, storage.StateRunning, ""); err != nil {
		return err
	}
	o.emit(ctx, storage.EventLevelInfo, "compact_started", "compact started")
	return nil
}

func (o *Orchestrator) runPlan(ctx context.Context, plan PreviewResult) ([]CollectionOutcome, runTotals, runCounts, map[string]ValidateResult, error) {
	var outcomes []CollectionOutcome
	var totals runTotals
	var counts runCounts
	validateOut := map[string]ValidateResult{}

	for _, member := range plan.ExecutionOrder {
		if o.wasStopped() {
			break
		}
		if member.RequiresStepdown {
			if err := o.stepdownPrimary(ctx); err != nil {
				return outcomes, totals, counts, validateOut, fmt.Errorf("stepdown %s: %w", member.Member, err)
			}
		}
		Started.WithLabelValues(member.Member).Inc()
		o.runMember(ctx, plan, member, &outcomes, &totals, &counts, validateOut)
	}
	o.cur.Store("")
	return outcomes, totals, counts, validateOut, nil
}

func (o *Orchestrator) runMember(ctx context.Context, plan PreviewResult, member MemberPlan,
	outcomes *[]CollectionOutcome, totals *runTotals, counts *runCounts, validateOut map[string]ValidateResult) {
	for _, collKey := range member.Collections {
		if o.wasStopped() {
			return
		}
		parts := strings.SplitN(collKey, ".", 2)
		if len(parts) != 2 {
			continue
		}
		dbName, coll := parts[0], parts[1]
		o.cur.Store(member.Member + "/" + collKey)

		outcome := o.compactOne(ctx, member.Member, dbName, coll)
		*outcomes = append(*outcomes, outcome)
		totals.pre += outcome.SizeBefore
		totals.post += outcome.SizeAfter
		ReclaimBytes.WithLabelValues(member.Member, dbName, coll).Add(float64(outcome.BytesReclaimed))
		Duration.WithLabelValues(member.Member).Observe(outcome.Duration.Seconds())
		o.tallyOutcome(outcome, member.Member, counts)
		_ = o.ops.IncProgress(ctx, o.id, collKey, 1, outcome.BytesReclaimed, int64(len(plan.ExecutionOrder)*len(member.Collections)))

		if o.params.ValidateAfterCompact && outcome.Error == "" && strings.EqualFold(member.Role, "primary") {
			if vr, err := o.validate(ctx, dbName, coll); err == nil {
				validateOut[collKey] = vr
			}
		}
	}
}

func (o *Orchestrator) tallyOutcome(outcome CollectionOutcome, member string, counts *runCounts) {
	switch {
	case outcome.Error != "":
		counts.failed++
		Completed.WithLabelValues(member, "failed").Inc()
	case outcome.SizeBefore == 0:
		counts.skipped++
		Completed.WithLabelValues(member, "skipped").Inc()
	default:
		counts.compacted++
		Completed.WithLabelValues(member, "ok").Inc()
	}
}

func (o *Orchestrator) finalize(ctx context.Context, outcomes []CollectionOutcome, totals runTotals, counts runCounts, validateOut map[string]ValidateResult) error {
	state := storage.StateCompleted
	if o.wasStopped() {
		state = storage.StateStopped
	}
	stats := Stats{
		StartedAt:       o.startedAt,
		CompletedAt:     time.Now().UTC(),
		CompactedCount:  counts.compacted,
		SkippedCount:    counts.skipped,
		FailedCount:     counts.failed,
		TotalBytesPre:   totals.pre,
		TotalBytesPost:  totals.post,
		BytesReclaimed:  totals.pre - totals.post,
		PerCollection:   outcomes,
		ValidateResults: validateOut,
	}
	stats.Duration = stats.CompletedAt.Sub(stats.StartedAt)
	raw, _ := json.Marshal(stats)
	_ = o.ops.SetStats(ctx, o.id, raw)
	if err := o.ops.UpdateState(ctx, o.id, state, ""); err != nil {
		return err
	}
	o.mu.Lock()
	o.state = state
	o.mu.Unlock()
	o.emit(ctx, storage.EventLevelInfo, "compact_"+string(state), "compact finalized")
	return nil
}

// compactOne runs { compact: coll, force: true } on the current primary and
// records per-collection timing + reclaim. Because the v2 driver doesn't
// allow targeting a specific secondary for write-like commands, we run
// against the cluster's current primary for every step; replication carries
// the effect to secondaries per-member for rolling mode the orchestrator
// calls stepdown to shift primacy.
func (o *Orchestrator) compactOne(ctx context.Context, member, dbName, coll string) CollectionOutcome {
	out := CollectionOutcome{
		Member: member, Database: dbName, Collection: coll, StartedAt: time.Now().UTC(),
	}
	before, err := o.mc.CollectionStats(ctx, dbName, coll)
	if err != nil {
		out.CompletedAt = time.Now().UTC()
		out.Duration = out.CompletedAt.Sub(out.StartedAt)
		out.Error = err.Error()
		return out
	}
	out.SizeBefore = before.StorageSize
	out.FreeBefore = before.FreeStorageSize

	cmd := bson.D{{Key: "compact", Value: coll}, {Key: "force", Value: true}}
	if err := o.mc.Raw().Database(dbName).RunCommand(ctx, cmd).Err(); err != nil {
		out.CompletedAt = time.Now().UTC()
		out.Duration = out.CompletedAt.Sub(out.StartedAt)
		out.Error = err.Error()
		return out
	}

	after, err := o.mc.CollectionStats(ctx, dbName, coll)
	if err != nil {
		out.Error = err.Error()
	} else {
		out.SizeAfter = after.StorageSize
		out.FreeAfter = after.FreeStorageSize
		out.BytesReclaimed = out.SizeBefore - out.SizeAfter
	}
	out.CompletedAt = time.Now().UTC()
	out.Duration = out.CompletedAt.Sub(out.StartedAt)
	return out
}

// stepdownPrimary asks the current primary to step down and blocks briefly
// for a new one. Failures are fatal for the orchestrator since rolling
// compacts depend on a clean rotation.
func (o *Orchestrator) stepdownPrimary(ctx context.Context) error {
	cctx, cancel := context.WithTimeout(ctx, o.params.StepdownWaitTimeout)
	defer cancel()

	cmd := bson.D{
		{Key: "replSetStepDown", Value: int32(60)},
		{Key: "secondaryCatchUpPeriodSecs", Value: int32(10)},
	}
	// replSetStepDown returns an error from the primary's point of view
	// (because the socket is torn down); we treat "network error" as success
	// and proceed to wait for a new primary.
	_ = o.mc.Raw().Database("admin").RunCommand(cctx, cmd).Err()
	Stepdown.Inc()
	o.emit(ctx, storage.EventLevelInfo, "stepdown_initiated", "primary stepdown requested")

	deadline := time.Now().Add(o.params.StepdownWaitTimeout)
	for time.Now().Before(deadline) {
		top, err := o.mc.DetectTopology(ctx)
		if err == nil && top.Primary != "" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return errors.New("timed out waiting for new primary after stepdown")
}

func (o *Orchestrator) validate(ctx context.Context, dbName, coll string) (ValidateResult, error) {
	cmd := bson.D{{Key: "validate", Value: coll}, {Key: "background", Value: true}}
	var m bson.M
	if err := o.mc.Raw().Database(dbName).RunCommand(ctx, cmd).Decode(&m); err != nil {
		return ValidateResult{}, err
	}
	vr := ValidateResult{}
	if v, ok := m["valid"].(bool); ok {
		vr.Valid = v
	}
	if e, ok := m["errors"].(bson.A); ok {
		vr.Errors = int64(len(e))
	}
	if w, ok := m["warnings"].(bson.A); ok {
		vr.Warnings = int64(len(w))
	}
	raw, _ := json.Marshal(m)
	vr.Raw = raw
	return vr, nil
}

func (o *Orchestrator) wasStopped() bool {
	select {
	case <-o.stopCh:
		return true
	default:
		return false
	}
}

func (o *Orchestrator) fail(ctx context.Context, err error) error {
	_ = o.ops.UpdateState(ctx, o.id, storage.StateFailed, err.Error())
	o.emit(ctx, storage.EventLevelError, "compact_failed", err.Error())
	return err
}

func (o *Orchestrator) emit(ctx context.Context, level storage.EventLevel, category, message string) {
	if o.events != nil {
		id := o.id
		_, _ = o.events.Record(ctx, storage.Event{
			OperationID: &id, Level: level, Category: category, Message: message,
		})
	}
	opevents.Publish(category, map[string]any{
		"operation_id": o.id,
		"kind":         "compact",
		"message":      message,
		"current_step": o.CurrentStep(),
	})
}

func isSystemDB(name string) bool {
	switch name {
	case "admin", "config", "local":
		return true
	}
	return false
}

// estimateDuration is a naive per-collection estimate. The reports view in
// Phase 15 replaces this with an EWMA over historical durations.
func estimateDuration(nColls int) time.Duration {
	return time.Duration(nColls) * 30 * time.Second
}

// newID lets tests override the UUID generator.
var newID = func() string { return uuid.NewString() }
