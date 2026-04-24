// Package collector polls the target MongoDB cluster on an adaptive cadence
// and persists a time series of storage, fragmentation, and health metrics
// into the SQLite state store. It never panics: transient errors back off
// exponentially, and the collector is expected to run for the life of the
// application.
package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/pagombin/fragmention-poc/internal/metrics"
	mongoClient "github.com/pagombin/fragmention-poc/internal/mongo"
	"github.com/pagombin/fragmention-poc/internal/storage"
)

// Config tunes polling cadence and backoff. Zero values get sane defaults.
type Config struct {
	IdleInterval   time.Duration
	ActiveInterval time.Duration
	BackoffInitial time.Duration
	BackoffMax     time.Duration
}

// Collector samples the target cluster on an adaptive cadence. It is safe
// for concurrent use but only one Run loop should be active at a time.
type Collector struct {
	cfg       Config
	client    *mongoClient.Client
	samples   *storage.Samples
	snapshots *storage.Snapshots
	events    *storage.Events
	logger    zerolog.Logger

	mu         sync.RWMutex
	topology   mongoClient.Topology
	lastError  error

	activeCount atomic.Int32
}

// New constructs a Collector. Pass activeCount==nil to let the collector own
// its own active-flag; the supervisor increments it when loader/deleter/
// compact services are running so the polling cadence speeds up.
func New(cfg Config, c *mongoClient.Client, samples *storage.Samples, snaps *storage.Snapshots, events *storage.Events, logger zerolog.Logger) *Collector {
	if cfg.IdleInterval <= 0 {
		cfg.IdleInterval = 10 * time.Second
	}
	if cfg.ActiveInterval <= 0 {
		cfg.ActiveInterval = 2 * time.Second
	}
	if cfg.BackoffInitial <= 0 {
		cfg.BackoffInitial = time.Second
	}
	if cfg.BackoffMax <= 0 {
		cfg.BackoffMax = 30 * time.Second
	}
	return &Collector{
		cfg:       cfg,
		client:    c,
		samples:   samples,
		snapshots: snaps,
		events:    events,
		logger:    logger.With().Str("service", "collector").Logger(),
	}
}

// MarkActive informs the collector that an operation is in progress, so it
// should switch to the faster ActiveInterval cadence. Each MarkActive must be
// paired with a MarkIdle. Nested calls are safe.
func (c *Collector) MarkActive() { c.activeCount.Add(1) }

// MarkIdle undoes a prior MarkActive. Pair each MarkActive with exactly one
// MarkIdle; deferring MarkIdle at the caller is the idiomatic usage.
func (c *Collector) MarkIdle() { c.activeCount.Add(-1) }

// Topology returns the most recently observed topology. Zero value is
// returned before the first tick succeeds.
func (c *Collector) Topology() mongoClient.Topology {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.topology
}

// LastError returns the most recent tick error. Used by /ready and the UI
// connectivity indicator.
func (c *Collector) LastError() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastError
}

// Run blocks, polling the cluster until ctx is cancelled. On error the loop
// backs off with exponential jitter rather than exiting.
func (c *Collector) Run(ctx context.Context) error {
	backoff := c.cfg.BackoffInitial
	for {
		start := time.Now()
		err := c.tick(ctx)
		took := time.Since(start)
		metrics.CollectorDuration.WithLabelValues("all").Observe(took.Seconds())

		c.mu.Lock()
		c.lastError = err
		c.mu.Unlock()

		outcome := "ok"
		if err != nil {
			outcome = "error"
			c.logger.Warn().Err(err).Dur("took", took).Msg("collector tick failed")
		}
		metrics.CollectorTick.WithLabelValues(outcome).Inc()

		// Sleep until the next tick, honoring cancellation.
		var wait time.Duration
		if err != nil {
			wait = backoff
			backoff *= 2
			if backoff > c.cfg.BackoffMax {
				backoff = c.cfg.BackoffMax
			}
		} else {
			backoff = c.cfg.BackoffInitial
			if c.activeCount.Load() > 0 {
				wait = c.cfg.ActiveInterval
			} else {
				wait = c.cfg.IdleInterval
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

// tick runs one sampling pass: topology, database stats, collection stats,
// member info. Samples are written in one transactional batch.
func (c *Collector) tick(ctx context.Context) error {
	top, err := c.client.DetectTopology(ctx)
	if err != nil {
		return fmt.Errorf("detect topology: %w", err)
	}
	c.mu.Lock()
	c.topology = top
	c.mu.Unlock()

	dbs, err := c.client.ListDatabases(ctx)
	if err != nil {
		return fmt.Errorf("list dbs: %w", err)
	}

	var batch []storage.Sample
	now := time.Now().UTC()

	// Cluster rollup.
	var totalStorage, totalData, totalIndex int64
	for _, db := range dbs {
		totalStorage += db.StorageSize
		totalData += db.DataSize
		totalIndex += db.IndexSize
	}
	if totalStorage > 0 {
		clusterFrag := float64(totalStorage-totalData) / float64(totalStorage)
		batch = append(batch,
			storage.Sample{Timestamp: now, Scope: storage.ScopeCluster, ScopeID: "cluster", MetricName: "fragmentation_ratio", Value: clusterFrag},
			storage.Sample{Timestamp: now, Scope: storage.ScopeCluster, ScopeID: "cluster", MetricName: "storage_size_bytes", Value: float64(totalStorage)},
			storage.Sample{Timestamp: now, Scope: storage.ScopeCluster, ScopeID: "cluster", MetricName: "data_size_bytes", Value: float64(totalData)},
			storage.Sample{Timestamp: now, Scope: storage.ScopeCluster, ScopeID: "cluster", MetricName: "index_size_bytes", Value: float64(totalIndex)},
		)
		metrics.CollectorFragRatio.WithLabelValues("cluster", "cluster").Set(clusterFrag)
		metrics.CollectorStorageBytes.WithLabelValues("cluster", "cluster").Set(float64(totalStorage))
	}

	// Per-database and per-collection samples.
	for _, db := range dbs {
		if db.StorageSize > 0 {
			dbFrag := float64(db.StorageSize-db.DataSize) / float64(db.StorageSize)
			batch = append(batch,
				storage.Sample{Timestamp: now, Scope: storage.ScopeDatabase, ScopeID: db.Name, MetricName: "fragmentation_ratio", Value: dbFrag},
				storage.Sample{Timestamp: now, Scope: storage.ScopeDatabase, ScopeID: db.Name, MetricName: "storage_size_bytes", Value: float64(db.StorageSize)},
				storage.Sample{Timestamp: now, Scope: storage.ScopeDatabase, ScopeID: db.Name, MetricName: "data_size_bytes", Value: float64(db.DataSize)},
				storage.Sample{Timestamp: now, Scope: storage.ScopeDatabase, ScopeID: db.Name, MetricName: "index_size_bytes", Value: float64(db.IndexSize)},
			)
			metrics.CollectorFragRatio.WithLabelValues("database", db.Name).Set(dbFrag)
			metrics.CollectorStorageBytes.WithLabelValues("database", db.Name).Set(float64(db.StorageSize))
		}

		// Only sample user databases for per-collection detail to bound cost.
		if db.Name == "admin" || db.Name == "config" || db.Name == "local" {
			continue
		}
		colls, err := c.client.ListCollections(ctx, db.Name)
		if err != nil {
			c.logger.Warn().Err(err).Str("db", db.Name).Msg("list collections failed")
			continue
		}
		for _, coll := range colls {
			scopeID := db.Name + "." + coll.Name
			if coll.StorageSize > 0 {
				metrics.CollectorFragRatio.WithLabelValues("collection", scopeID).Set(coll.FragmentationRatio)
				metrics.CollectorStorageBytes.WithLabelValues("collection", scopeID).Set(float64(coll.StorageSize))
				metrics.CollectorFreeStorageBytes.WithLabelValues("collection", scopeID).Set(float64(coll.FreeStorageSize))
			}
			batch = append(batch,
				storage.Sample{Timestamp: now, Scope: storage.ScopeCollection, ScopeID: scopeID, MetricName: "fragmentation_ratio", Value: coll.FragmentationRatio},
				storage.Sample{Timestamp: now, Scope: storage.ScopeCollection, ScopeID: scopeID, MetricName: "storage_size_bytes", Value: float64(coll.StorageSize)},
				storage.Sample{Timestamp: now, Scope: storage.ScopeCollection, ScopeID: scopeID, MetricName: "free_storage_bytes", Value: float64(coll.FreeStorageSize)},
				storage.Sample{Timestamp: now, Scope: storage.ScopeCollection, ScopeID: scopeID, MetricName: "size_bytes", Value: float64(coll.Size)},
				storage.Sample{Timestamp: now, Scope: storage.ScopeCollection, ScopeID: scopeID, MetricName: "index_size_bytes", Value: float64(coll.TotalIndexSize)},
				storage.Sample{Timestamp: now, Scope: storage.ScopeCollection, ScopeID: scopeID, MetricName: "doc_count", Value: float64(coll.Count)},
			)
		}
	}

	// Per-member health.
	for _, m := range top.Members {
		batch = append(batch,
			storage.Sample{Timestamp: now, Scope: storage.ScopeMember, ScopeID: m.Name, MetricName: "health", Value: m.Health},
			storage.Sample{Timestamp: now, Scope: storage.ScopeMember, ScopeID: m.Name, MetricName: "lag_seconds", Value: m.LagSeconds},
		)
	}

	if _, err := c.samples.WriteBatch(ctx, batch); err != nil {
		return fmt.Errorf("persist samples: %w", err)
	}
	return nil
}

// TakeSnapshot captures a point-in-time reading labelled for humans. When
// runID is empty the snapshot is stored with NULL run_id (ad-hoc snapshot).
// The raw stats payload is a JSON object containing cluster-wide rollups
// plus per-database and per-collection summaries so the UI can render diffs
// even if the metrics_samples table has been purged.
func (c *Collector) TakeSnapshot(ctx context.Context, label, note, runID string) (string, error) {
	if label == "" {
		return "", errors.New("snapshot label required")
	}
	dbs, err := c.client.ListDatabases(ctx)
	if err != nil {
		return "", fmt.Errorf("list dbs: %w", err)
	}
	payload := map[string]any{"databases": dbs}
	collsByDB := make(map[string][]mongoClient.CollectionSummary)
	for _, db := range dbs {
		if db.Name == "admin" || db.Name == "config" || db.Name == "local" {
			continue
		}
		cs, err := c.client.ListCollections(ctx, db.Name)
		if err != nil {
			return "", fmt.Errorf("list colls %s: %w", db.Name, err)
		}
		collsByDB[db.Name] = cs
	}
	payload["collections"] = collsByDB
	payload["topology"] = c.Topology()

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal snapshot: %w", err)
	}
	id := uuid.NewString()
	var runIDPtr *string
	if runID != "" {
		runIDPtr = &runID
	}
	if _, err := c.snapshots.Record(ctx, storage.Snapshot{
		ID:       id,
		RunID:    runIDPtr,
		Label:    label,
		Scope:    storage.ScopeCluster,
		ScopeID:  "cluster",
		RawStats: raw,
		Note:     note,
	}); err != nil {
		return "", err
	}
	if c.events != nil {
		_, _ = c.events.Record(ctx, storage.Event{
			RunID:    runIDPtr,
			Level:    storage.EventLevelInfo,
			Category: "snapshot",
			Message:  "snapshot " + label,
			Context:  map[string]any{"snapshot_id": id, "note": note},
		})
	}
	return id, nil
}
