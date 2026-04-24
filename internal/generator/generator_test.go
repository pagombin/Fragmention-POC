package generator

import (
	"math/rand"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
)

func TestDefaultTemplates_SixTemplates(t *testing.T) {
	tpls, weights := DefaultTemplates()
	require.Len(t, tpls, 6)
	require.Len(t, weights, 6)
	seen := map[string]bool{}
	for _, tpl := range tpls {
		seen[tpl.Name()] = true
	}
	for _, want := range []string{"user_profile", "event_log", "order", "telemetry", "document_blob", "iot_timeseries"} {
		require.True(t, seen[want], "missing template %s", want)
	}
}

func TestTemplates_GenerateRealisticDocuments(t *testing.T) {
	tpls, _ := DefaultTemplates()
	r := rand.New(rand.NewSource(42))
	fk := gofakeit.NewFaker(r, false)
	for _, tpl := range tpls {
		doc, size, err := tpl.Generate(r, fk, "run-xyz")
		require.NoError(t, err, tpl.Name())
		require.Greater(t, size, 0, tpl.Name())
		// Every doc must have _id, created_at, _load_run_id.
		keys := map[string]bool{}
		for _, e := range doc {
			keys[e.Key] = true
		}
		require.True(t, keys["_id"], "%s missing _id", tpl.Name())
		require.True(t, keys["created_at"], "%s missing created_at", tpl.Name())
		require.True(t, keys["_load_run_id"], "%s missing _load_run_id", tpl.Name())
	}
}

func TestRegistry_WeightedPickHitsEverything(t *testing.T) {
	tpls, weights := DefaultTemplates()
	reg, err := NewRegistry(tpls, weights)
	require.NoError(t, err)

	counts := map[string]int{}
	r := rand.New(rand.NewSource(7))
	const N = 20000
	for i := 0; i < N; i++ {
		counts[reg.Pick(r).Name()]++
	}
	// All six templates should appear at least once.
	require.Len(t, counts, 6)
	// event_log and telemetry share the highest weights - they should each
	// exceed document_blob (the smallest weight).
	require.Greater(t, counts["event_log"], counts["document_blob"])
	require.Greater(t, counts["telemetry"], counts["document_blob"])
}

func TestRegistry_RejectsBadWeights(t *testing.T) {
	tpls := []Template{&UserProfile{}}
	_, err := NewRegistry(tpls, []float64{0})
	require.Error(t, err)
	_, err = NewRegistry(tpls, []float64{1, 2})
	require.Error(t, err)
}

func TestDocumentBlob_SizeIsLarge(t *testing.T) {
	r := rand.New(rand.NewSource(99))
	fk := gofakeit.NewFaker(r, false)
	_, size, err := (DocumentBlob{}).Generate(r, fk, "run")
	require.NoError(t, err)
	require.GreaterOrEqual(t, size, 40*1024) // at least 40KB on-wire
}

func TestTelemetry_SizeIsSmall(t *testing.T) {
	r := rand.New(rand.NewSource(99))
	fk := gofakeit.NewFaker(r, false)
	_, size, err := (Telemetry{}).Generate(r, fk, "run")
	require.NoError(t, err)
	require.Less(t, size, 2048)
}
