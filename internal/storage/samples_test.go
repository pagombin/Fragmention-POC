package storage

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSamples_WriteBatchAndQuery(t *testing.T) {
	s := openTempStore(t)
	repo := NewSamples(s)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	rid := "run-1"
	_, err := s.db.Exec(`INSERT INTO runs (id, name, config_json, status, created_at) VALUES (?, ?, ?, ?, ?)`,
		rid, "unit", "{}", "running", FormatTime(now))
	require.NoError(t, err)

	samples := []Sample{
		{RunID: &rid, Timestamp: now.Add(-2 * time.Minute), Scope: ScopeCluster, ScopeID: "cluster", MetricName: "fragmentation_ratio", Value: 0.1},
		{RunID: &rid, Timestamp: now.Add(-1 * time.Minute), Scope: ScopeCluster, ScopeID: "cluster", MetricName: "fragmentation_ratio", Value: 0.25},
		{RunID: &rid, Timestamp: now, Scope: ScopeCollection, ScopeID: "poc_db_1.coll", MetricName: "storage_size_bytes", Value: 1024},
	}
	n, err := repo.WriteBatch(ctx, samples)
	require.NoError(t, err)
	require.Equal(t, 3, n)

	// Filter by metric + scope.
	out, err := repo.Query(ctx, Query{RunID: rid, MetricName: "fragmentation_ratio"})
	require.NoError(t, err)
	require.Len(t, out, 2)
	require.Equal(t, 0.1, out[0].Value)
	require.Equal(t, 0.25, out[1].Value)

	// Since filter.
	out, err = repo.Query(ctx, Query{RunID: rid, Since: now.Add(-30 * time.Second)})
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, "poc_db_1.coll", out[0].ScopeID)
}

func TestSamples_EmptyBatchIsOK(t *testing.T) {
	s := openTempStore(t)
	repo := NewSamples(s)
	n, err := repo.WriteBatch(context.Background(), nil)
	require.NoError(t, err)
	require.Zero(t, n)
}

func TestSnapshots_RecordAndGet(t *testing.T) {
	s := openTempStore(t)
	repo := NewSnapshots(s)
	raw, _ := json.Marshal(map[string]any{"total_storage": 1024})

	id, err := repo.Record(context.Background(), Snapshot{
		ID:       "snap-1",
		Label:    "baseline",
		Scope:    ScopeCluster,
		ScopeID:  "cluster",
		RawStats: raw,
		Note:     "first baseline",
	})
	require.NoError(t, err)
	require.Equal(t, "snap-1", id)

	got, err := repo.Get(context.Background(), "snap-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "baseline", got.Label)
	require.Equal(t, "first baseline", got.Note)

	list, err := repo.List(context.Background(), "baseline", 10)
	require.NoError(t, err)
	require.Len(t, list, 1)
}

func TestSnapshots_MissingIsNil(t *testing.T) {
	s := openTempStore(t)
	repo := NewSnapshots(s)
	got, err := repo.Get(context.Background(), "nope")
	require.NoError(t, err)
	require.Nil(t, got)
}
