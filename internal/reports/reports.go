// Package reports renders structured comparison reports from the state
// store. Reports combine a run's metadata, snapshot diffs, metric
// trajectories, and per-collection reclaim so the Phase-14 Run Detail view
// and offline analysis tools share one canonical shape.
package reports

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"github.com/pagombin/fragmention-poc/internal/storage"
)

// Report is the JSON representation a `/runs/{id}/report` endpoint returns.
type Report struct {
	Run               storage.Run             `json:"run"`
	GeneratedAt       time.Time               `json:"generated_at"`
	Summary           Summary                 `json:"summary"`
	Snapshots         []storage.Snapshot      `json:"snapshots"`
	Operations        []storage.Operation     `json:"operations"`
	PerCollection     []CollectionReclaim     `json:"per_collection_reclaim,omitempty"`
	FragmentationLine []MetricPoint           `json:"cluster_fragmentation,omitempty"`
}

// Summary captures the headline numbers every report displays.
type Summary struct {
	BaselineStorage int64   `json:"baseline_storage_bytes,omitempty"`
	FinalStorage    int64   `json:"final_storage_bytes,omitempty"`
	BytesReclaimed  int64   `json:"bytes_reclaimed"`
	PercentReclaim  float64 `json:"percent_reclaim"`
	BaselineFrag    float64 `json:"baseline_fragmentation"`
	FinalFrag       float64 `json:"final_fragmentation"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
	OperationCount  int     `json:"operation_count"`
	SnapshotCount   int     `json:"snapshot_count"`
}

// CollectionReclaim is a per-collection reclaim line (mirrors the
// snapshot-diff payload but survives retention purges because it is frozen
// into the report).
type CollectionReclaim struct {
	Database            string  `json:"database"`
	Collection          string  `json:"collection"`
	StorageBefore       int64   `json:"storage_before"`
	StorageAfter        int64   `json:"storage_after"`
	FreeBefore          int64   `json:"free_before"`
	FreeAfter           int64   `json:"free_after"`
	BytesReclaimed      int64   `json:"bytes_reclaimed"`
	FragmentationBefore float64 `json:"fragmentation_before"`
	FragmentationAfter  float64 `json:"fragmentation_after"`
}

// MetricPoint is one (timestamp, value) pair.
type MetricPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// Generator builds reports from storage repositories. Keep it narrow so
// tests can swap in fakes.
type Generator struct {
	Runs      *storage.Runs
	Snaps     *storage.Snapshots
	Ops       *storage.Operations
	Samples   *storage.Samples
}

// GenerateRun builds a Report for the given run. Baseline snapshot is
// picked as the earliest snapshot associated with the run; final snapshot
// is the latest. Reclaim lines are derived from collection-level entries
// in the baseline vs final snapshot payloads.
func (g *Generator) GenerateRun(ctx context.Context, runID string) (*Report, error) {
	run, err := g.Runs.Get(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, fmt.Errorf("run %s not found", runID)
	}

	allSnaps, err := g.Snaps.List(ctx, "", 1000)
	if err != nil {
		return nil, err
	}
	var snaps []storage.Snapshot
	for _, s := range allSnaps {
		if s.RunID != nil && *s.RunID == runID {
			snaps = append(snaps, s)
		}
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].TakenAt.Before(snaps[j].TakenAt) })

	baseline, final := pickBaselineFinal(snaps)
	baseColls := flattenCollectionStats(baseline)
	finalColls := flattenCollectionStats(final)

	var reclaim []CollectionReclaim
	for key, b := range baseColls {
		f, ok := finalColls[key]
		if !ok {
			continue
		}
		reclaim = append(reclaim, CollectionReclaim{
			Database: b.Database, Collection: b.Collection,
			StorageBefore: b.StorageSize, StorageAfter: f.StorageSize,
			FreeBefore: b.FreeStorageSize, FreeAfter: f.FreeStorageSize,
			BytesReclaimed:      b.StorageSize - f.StorageSize,
			FragmentationBefore: b.FragmentationRatio,
			FragmentationAfter:  f.FragmentationRatio,
		})
	}
	sort.Slice(reclaim, func(i, j int) bool { return reclaim[i].BytesReclaimed > reclaim[j].BytesReclaimed })

	line, err := g.Samples.Query(ctx, storage.Query{
		RunID: runID, Scope: storage.ScopeCluster, MetricName: "fragmentation_ratio", Limit: 10000,
	})
	if err != nil {
		return nil, err
	}
	trajectory := make([]MetricPoint, 0, len(line))
	for _, s := range line {
		trajectory = append(trajectory, MetricPoint{Timestamp: s.Timestamp, Value: s.Value})
	}

	opsAll, err := g.Ops.ListByKind(ctx, "", 500)
	if err != nil {
		// ListByKind requires a kind; fallback to empty set when none supplied.
		opsAll = nil
	}
	_ = opsAll // we don't currently filter by run_id; future phases can.

	var baseSize, finalSize int64
	var baseFrag, finalFrag float64
	for _, r := range reclaim {
		baseSize += r.StorageBefore
		finalSize += r.StorageAfter
	}
	if baseline != nil {
		baseFrag = clusterFrag(baseColls)
	}
	if final != nil {
		finalFrag = clusterFrag(finalColls)
	}
	reclaimed := baseSize - finalSize
	var pct float64
	if baseSize > 0 {
		pct = float64(reclaimed) / float64(baseSize)
	}

	var duration float64
	if run.StartedAt != nil && run.CompletedAt != nil {
		duration = run.CompletedAt.Sub(*run.StartedAt).Seconds()
	}

	return &Report{
		Run:               *run,
		GeneratedAt:       time.Now().UTC(),
		Snapshots:         snaps,
		PerCollection:     reclaim,
		FragmentationLine: trajectory,
		Summary: Summary{
			BaselineStorage: baseSize,
			FinalStorage:    finalSize,
			BytesReclaimed:  reclaimed,
			PercentReclaim:  pct,
			BaselineFrag:    baseFrag,
			FinalFrag:       finalFrag,
			DurationSeconds: duration,
			SnapshotCount:   len(snaps),
		},
	}, nil
}

// GenerateCompareSnapshots builds a minimal report from two snapshot IDs
// (used by the Initial Sync Companion and any ad-hoc external-flow reports).
func (g *Generator) GenerateCompareSnapshots(ctx context.Context, aID, bID string) (*Report, error) {
	a, err := g.Snaps.Get(ctx, aID)
	if err != nil || a == nil {
		return nil, fmt.Errorf("snapshot a not found")
	}
	b, err := g.Snaps.Get(ctx, bID)
	if err != nil || b == nil {
		return nil, fmt.Errorf("snapshot b not found")
	}
	baseColls := flattenCollectionStats(a)
	finalColls := flattenCollectionStats(b)
	var reclaim []CollectionReclaim
	var baseSize, finalSize int64
	for key, bm := range baseColls {
		fm, ok := finalColls[key]
		if !ok {
			continue
		}
		baseSize += bm.StorageSize
		finalSize += fm.StorageSize
		reclaim = append(reclaim, CollectionReclaim{
			Database: bm.Database, Collection: bm.Collection,
			StorageBefore: bm.StorageSize, StorageAfter: fm.StorageSize,
			FreeBefore: bm.FreeStorageSize, FreeAfter: fm.FreeStorageSize,
			BytesReclaimed:      bm.StorageSize - fm.StorageSize,
			FragmentationBefore: bm.FragmentationRatio,
			FragmentationAfter:  fm.FragmentationRatio,
		})
	}
	sort.Slice(reclaim, func(i, j int) bool { return reclaim[i].BytesReclaimed > reclaim[j].BytesReclaimed })
	var pct float64
	if baseSize > 0 {
		pct = float64(baseSize-finalSize) / float64(baseSize)
	}
	return &Report{
		GeneratedAt:   time.Now().UTC(),
		Snapshots:     []storage.Snapshot{*a, *b},
		PerCollection: reclaim,
		Summary: Summary{
			BaselineStorage: baseSize,
			FinalStorage:    finalSize,
			BytesReclaimed:  baseSize - finalSize,
			PercentReclaim:  pct,
			BaselineFrag:    clusterFrag(baseColls),
			FinalFrag:       clusterFrag(finalColls),
			SnapshotCount:   2,
		},
	}, nil
}

func pickBaselineFinal(snaps []storage.Snapshot) (baseline, final *storage.Snapshot) {
	for i := range snaps {
		if snaps[i].Label == "baseline" || snaps[i].Label == "pre_compact" ||
			snaps[i].Label == "pre_initial_sync" || snaps[i].Label == "post_load" {
			baseline = &snaps[i]
			break
		}
	}
	if baseline == nil && len(snaps) > 0 {
		baseline = &snaps[0]
	}
	for i := len(snaps) - 1; i >= 0; i-- {
		if snaps[i].Label == "post_compact" || snaps[i].Label == "post_initial_sync" ||
			snaps[i].Label == "final" {
			final = &snaps[i]
			break
		}
	}
	if final == nil && len(snaps) > 0 {
		final = &snaps[len(snaps)-1]
	}
	return
}

// collStats mirrors the Collector.TakeSnapshot payload shape for the
// fields we care about.
type collStats struct {
	Database            string
	Collection          string
	StorageSize         int64
	FreeStorageSize     int64
	Count               int64
	FragmentationRatio  float64
}

// flattenCollectionStats walks the raw_stats JSON and returns a map keyed by
// "db.collection". Unknown shapes are tolerated silently.
func flattenCollectionStats(s *storage.Snapshot) map[string]collStats {
	out := map[string]collStats{}
	if s == nil {
		return out
	}
	var envelope struct {
		Collections map[string][]struct {
			Name               string  `json:"name"`
			Database           string  `json:"database"`
			StorageSize        int64   `json:"storage_size"`
			FreeStorageSize    int64   `json:"free_storage_size"`
			Count              int64   `json:"count"`
			FragmentationRatio float64 `json:"fragmentation_ratio"`
		} `json:"collections"`
	}
	if err := json.Unmarshal(s.RawStats, &envelope); err != nil {
		return out
	}
	for db, list := range envelope.Collections {
		for _, c := range list {
			dbName := c.Database
			if dbName == "" {
				dbName = db
			}
			key := dbName + "." + c.Name
			out[key] = collStats{
				Database: dbName, Collection: c.Name,
				StorageSize: c.StorageSize, FreeStorageSize: c.FreeStorageSize,
				Count: c.Count, FragmentationRatio: c.FragmentationRatio,
			}
		}
	}
	return out
}

func clusterFrag(m map[string]collStats) float64 {
	var storage, free int64
	for _, c := range m {
		storage += c.StorageSize
		free += c.FreeStorageSize
	}
	if storage == 0 {
		return 0
	}
	return float64(free) / float64(storage)
}

// WriteCSV emits a per-collection reclaim CSV suitable for spreadsheets.
// Header: database, collection, storage_before, storage_after,
// bytes_reclaimed, frag_before, frag_after.
func WriteCSV(w io.Writer, r *Report) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()
	if err := cw.Write([]string{
		"database", "collection", "storage_before", "storage_after",
		"bytes_reclaimed", "free_before", "free_after",
		"fragmentation_before", "fragmentation_after",
	}); err != nil {
		return err
	}
	for _, c := range r.PerCollection {
		if err := cw.Write([]string{
			c.Database, c.Collection,
			strconv.FormatInt(c.StorageBefore, 10),
			strconv.FormatInt(c.StorageAfter, 10),
			strconv.FormatInt(c.BytesReclaimed, 10),
			strconv.FormatInt(c.FreeBefore, 10),
			strconv.FormatInt(c.FreeAfter, 10),
			strconv.FormatFloat(c.FragmentationBefore, 'f', 6, 64),
			strconv.FormatFloat(c.FragmentationAfter, 'f', 6, 64),
		}); err != nil {
			return err
		}
	}
	return nil
}
