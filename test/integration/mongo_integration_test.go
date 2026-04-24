//go:build integration

package integration

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/pagombin/fragmention-poc/internal/collector"
	cfgpkg "github.com/pagombin/fragmention-poc/internal/config"
	mongoClient "github.com/pagombin/fragmention-poc/internal/mongo"
	"github.com/pagombin/fragmention-poc/internal/storage"
)

func startMongo(t *testing.T) *mongoClient.Client {
	t.Helper()
	ctx := context.Background()
	ctr, err := mongodb.Run(ctx, "mongo:7.0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	uri, err := ctr.ConnectionString(ctx)
	require.NoError(t, err)

	cfg := cfgpkg.MongoConfig{
		URI:             uri,
		ResolvedURI:     uri,
		ConnectTimeout:  30 * time.Second,
		OperationTimeout: 30 * time.Second,
		AppName:         "mfpoc-test",
	}
	mc, err := mongoClient.Connect(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = mc.Close(context.Background()) })
	return mc
}

func TestConnect_VersionAndTopology(t *testing.T) {
	mc := startMongo(t)
	info := mc.ServerInfo()
	require.GreaterOrEqual(t, info.Major, mongoClient.MinMajor)

	top, err := mc.DetectTopology(context.Background())
	require.NoError(t, err)
	require.Equal(t, mongoClient.TopologyStandalone, top.Kind)
	require.False(t, top.Sharded)
	require.NotEmpty(t, top.Members)
}

func TestCollectionAndDatabaseStats(t *testing.T) {
	mc := startMongo(t)
	ctx := context.Background()
	coll := mc.Raw().Database("poc_db_test").Collection("widgets")
	for i := 0; i < 50; i++ {
		_, err := coll.InsertOne(ctx, bson.M{"i": i, "payload": "hello"})
		require.NoError(t, err)
	}
	stats, err := mc.CollectionStats(ctx, "poc_db_test", "widgets")
	require.NoError(t, err)
	require.Equal(t, int64(50), stats.Count)
	require.Greater(t, stats.StorageSize, int64(0))
	require.GreaterOrEqual(t, stats.FragmentationRatio, 0.0)

	dbs, err := mc.ListDatabases(ctx)
	require.NoError(t, err)
	var found bool
	for _, d := range dbs {
		if d.Name == "poc_db_test" {
			found = true
		}
	}
	require.True(t, found)

	colls, err := mc.ListCollections(ctx, "poc_db_test")
	require.NoError(t, err)
	require.NotEmpty(t, colls)
}

func TestCollectorTickPersistsSamples(t *testing.T) {
	mc := startMongo(t)
	ctx := context.Background()

	// Seed some data so the collector sees a non-empty cluster.
	coll := mc.Raw().Database("poc_db_test").Collection("widgets")
	for i := 0; i < 10; i++ {
		_, err := coll.InsertOne(ctx, bson.M{"i": i})
		require.NoError(t, err)
	}

	dir := t.TempDir()
	store, err := storage.Open(ctx, storage.Config{Path: filepath.Join(dir, "t.db")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	c := collector.New(collector.Config{
		IdleInterval:   100 * time.Millisecond,
		ActiveInterval: 50 * time.Millisecond,
	}, mc, storage.NewSamples(store), storage.NewSnapshots(store), storage.NewEvents(store), zerolog.Nop())

	// Run a single tick by running Run until we see at least one sample.
	runCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	go func() { _ = c.Run(runCtx) }()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		out, err := storage.NewSamples(store).Query(ctx, storage.Query{Scope: storage.ScopeCluster, MetricName: "storage_size_bytes"})
		require.NoError(t, err)
		if len(out) > 0 {
			cancel()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("collector produced no samples within timeout")
}

func TestTakeSnapshot(t *testing.T) {
	mc := startMongo(t)
	ctx := context.Background()
	dir := t.TempDir()
	store, err := storage.Open(ctx, storage.Config{Path: filepath.Join(dir, "t.db")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	c := collector.New(collector.Config{}, mc, storage.NewSamples(store), storage.NewSnapshots(store), storage.NewEvents(store), zerolog.Nop())
	id, err := c.TakeSnapshot(ctx, "baseline", "integration", "")
	require.NoError(t, err)
	require.NotEmpty(t, id)
	got, err := storage.NewSnapshots(store).Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "baseline", got.Label)
}
