package reports

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pagombin/fragmention-poc/internal/storage"
)

func TestPickBaselineFinal_Labels(t *testing.T) {
	snaps := []storage.Snapshot{
		{ID: "1", Label: "pre_compact"},
		{ID: "2", Label: "something_else"},
		{ID: "3", Label: "post_compact"},
	}
	b, f := pickBaselineFinal(snaps)
	require.Equal(t, "1", b.ID)
	require.Equal(t, "3", f.ID)
}

func TestPickBaselineFinal_Fallback(t *testing.T) {
	snaps := []storage.Snapshot{{ID: "only", Label: "adhoc"}}
	b, f := pickBaselineFinal(snaps)
	require.Equal(t, b, f)
}

func TestFlattenCollectionStats(t *testing.T) {
	payload := map[string]any{
		"collections": map[string]any{
			"poc_db_1": []any{
				map[string]any{"name": "coll_a", "database": "poc_db_1", "storage_size": 1000, "free_storage_size": 100, "count": 42, "fragmentation_ratio": 0.1},
			},
		},
	}
	raw, _ := json.Marshal(payload)
	snap := &storage.Snapshot{RawStats: raw}
	out := flattenCollectionStats(snap)
	require.Len(t, out, 1)
	got := out["poc_db_1.coll_a"]
	require.Equal(t, int64(1000), got.StorageSize)
	require.Equal(t, int64(100), got.FreeStorageSize)
	require.Equal(t, int64(42), got.Count)
	require.Equal(t, 0.1, got.FragmentationRatio)
}

func TestWriteCSV(t *testing.T) {
	r := &Report{
		PerCollection: []CollectionReclaim{
			{Database: "db", Collection: "c", StorageBefore: 1000, StorageAfter: 500, BytesReclaimed: 500, FragmentationBefore: 0.4, FragmentationAfter: 0.05},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, WriteCSV(&buf, r))
	out := buf.String()
	require.Contains(t, out, "database,collection,storage_before")
	require.Contains(t, out, "db,c,1000,500,500")
	require.Contains(t, out, "0.400000")
}

func TestClusterFrag_EmptyIsZero(t *testing.T) {
	require.Equal(t, 0.0, clusterFrag(map[string]collStats{}))
}
