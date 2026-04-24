//go:build integration

package integration

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/pagombin/fragmention-poc/internal/deleter"
	"github.com/pagombin/fragmention-poc/internal/storage"
)

func TestDeleter_RandomByID_EndToEnd(t *testing.T) {
	mc := startMongo(t)
	ctx := context.Background()

	// Seed 1000 docs.
	db := "poc_db_del"
	coll := "widgets"
	dbh := mc.Raw().Database(db).Collection(coll)
	batch := make([]any, 1000)
	for i := 0; i < 1000; i++ {
		batch[i] = bson.M{"i": i}
	}
	_, err := dbh.InsertMany(ctx, batch)
	require.NoError(t, err)

	dir := t.TempDir()
	store, err := storage.Open(ctx, storage.Config{Path: filepath.Join(dir, "t.db")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	svc, err := deleter.NewService(deleter.Deps{
		Logger: zerolog.Nop(),
		Mongo:  mc,
		Ops:    storage.NewOperations(store),
		Events: storage.NewEvents(store),
		Store:  store,
	}, deleter.Params{BatchSize: 100, MaxRatio: 0.95}, 2*time.Minute)
	require.NoError(t, err)

	spec := deleter.TargetSpec{Entries: []deleter.Target{
		{Database: db, Collection: coll, Pattern: deleter.PatternConfig{
			Kind: deleter.PatternRandomByID, Ratio: 0.3, Seed: 42,
		}},
	}}
	preview, err := svc.Preview(ctx, spec, deleter.Params{BatchSize: 100, MaxRatio: 0.95})
	require.NoError(t, err)
	require.Greater(t, preview.TotalMatches, int64(200))
	require.Less(t, preview.TotalMatches, int64(400))
	require.NotEmpty(t, preview.Token)

	d, err := svc.Start(ctx, preview.Token, "")
	require.NoError(t, err)

	// Wait for completion.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if st := d.State(); st == storage.StateCompleted || st == storage.StateFailed {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, storage.StateCompleted, d.State())

	// Count remaining - should be ~700.
	count, err := dbh.CountDocuments(ctx, bson.M{})
	require.NoError(t, err)
	require.InDelta(t, 700, count, 50)
}

func TestDeleter_PreviewTokenExpiresOnUse(t *testing.T) {
	mc := startMongo(t)
	ctx := context.Background()
	// Empty collection is fine; we just want the token flow.
	dir := t.TempDir()
	store, err := storage.Open(ctx, storage.Config{Path: filepath.Join(dir, "t.db")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	// Seed so the preview path has something to count.
	dbh := mc.Raw().Database("poc_db_tok").Collection("c")
	_, err = dbh.InsertMany(ctx, []any{bson.M{"i": 1}, bson.M{"i": 2}, bson.M{"i": 3}})
	require.NoError(t, err)

	svc, err := deleter.NewService(deleter.Deps{
		Logger: zerolog.Nop(), Mongo: mc, Ops: storage.NewOperations(store),
		Events: storage.NewEvents(store), Store: store,
	}, deleter.Params{BatchSize: 50, MaxRatio: 0.95}, time.Minute)
	require.NoError(t, err)

	spec := deleter.TargetSpec{Entries: []deleter.Target{
		{Database: "poc_db_tok", Collection: "c", Pattern: deleter.PatternConfig{
			Kind: deleter.PatternRandomByID, Ratio: 0.5, Seed: 1,
		}},
	}}
	preview, err := svc.Preview(ctx, spec, deleter.Params{BatchSize: 50, MaxRatio: 0.95})
	require.NoError(t, err)

	_, err = svc.Start(ctx, preview.Token, "")
	require.NoError(t, err)

	// Second use of the same token must fail.
	_, err = svc.Start(ctx, preview.Token, "")
	require.Error(t, err)
}
