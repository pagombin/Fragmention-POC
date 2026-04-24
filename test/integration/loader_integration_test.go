//go:build integration

package integration

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/pagombin/fragmention-poc/internal/generator"
	"github.com/pagombin/fragmention-poc/internal/loader"
	"github.com/pagombin/fragmention-poc/internal/storage"
)

func TestLoader_SmallEndToEnd(t *testing.T) {
	mc := startMongo(t)
	ctx := context.Background()

	dir := t.TempDir()
	store, err := storage.Open(ctx, storage.Config{Path: filepath.Join(dir, "t.db")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	tpls, weights := generator.DefaultTemplates()
	reg, err := generator.NewRegistry(tpls, weights)
	require.NoError(t, err)

	svc, err := loader.NewService(loader.Deps{
		Logger:   zerolog.Nop(),
		Mongo:    mc,
		Ops:      storage.NewOperations(store),
		Events:   storage.NewEvents(store),
		Registry: reg,
	})
	require.NoError(t, err)

	l, err := svc.Start(ctx, loader.TargetSpec{Entries: []loader.Target{
		{Database: "poc_db_1", Collection: "coll_a", BytesTarget: 200 * 1024}, // 200KB target
		{Database: "poc_db_1", Collection: "coll_b", BytesTarget: 100 * 1024},
	}}, loader.Params{
		Workers:        2,
		BatchSize:      50,
		StorageHeadroom: 30,
		ForceStart:     true, // bypass preflight on ephemeral containers
	}, "")
	require.NoError(t, err)

	// Wait for the loader to finish (small target, should be quick).
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if st := l.State(); st == storage.StateCompleted || st == storage.StateFailed {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	require.Equal(t, storage.StateCompleted, l.State())

	// Verify progress rows reflect bytes written.
	rows, err := storage.NewOperations(store).ListProgress(ctx, l.ID())
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(rows), 2)
	for _, r := range rows {
		require.Greater(t, r.BytesProcessed, int64(0), "%s had 0 bytes", r.CollectionKey)
		require.Greater(t, r.CompletedCount, int64(0))
	}

	// Verify collection stats show at least some documents landed.
	stats, err := mc.CollectionStats(ctx, "poc_db_1", "coll_a")
	require.NoError(t, err)
	require.Greater(t, stats.Count, int64(0))
}

func TestLoader_PauseResume(t *testing.T) {
	mc := startMongo(t)
	ctx := context.Background()

	dir := t.TempDir()
	store, err := storage.Open(ctx, storage.Config{Path: filepath.Join(dir, "t.db")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	svc, err := loader.NewService(loader.Deps{
		Logger: zerolog.Nop(), Mongo: mc, Ops: storage.NewOperations(store), Events: storage.NewEvents(store),
	})
	require.NoError(t, err)
	l, err := svc.Start(ctx, loader.TargetSpec{Entries: []loader.Target{
		{Database: "poc_db_pause", Collection: "c1", BytesTarget: 500 * 1024},
	}}, loader.Params{Workers: 2, BatchSize: 20, ForceStart: true}, "")
	require.NoError(t, err)

	// Let the loader run briefly, then pause.
	time.Sleep(500 * time.Millisecond)
	require.NoError(t, l.Pause(ctx))
	require.Equal(t, storage.StatePaused, l.State())
	beforeDocs := l.SnapshotStats().DocsInserted

	time.Sleep(500 * time.Millisecond)
	afterPauseDocs := l.SnapshotStats().DocsInserted
	// While paused, no forward progress (allow a handful from in-flight batches).
	require.LessOrEqual(t, afterPauseDocs-beforeDocs, int64(200))

	require.NoError(t, l.Resume(ctx))
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if st := l.State(); st == storage.StateCompleted || st == storage.StateFailed {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	require.Equal(t, storage.StateCompleted, l.State())
}
