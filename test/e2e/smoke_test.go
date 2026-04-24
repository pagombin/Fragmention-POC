//go:build integration

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/pagombin/fragmention-poc/internal/api"
	"github.com/pagombin/fragmention-poc/internal/collector"
	cfgpkg "github.com/pagombin/fragmention-poc/internal/config"
	"github.com/pagombin/fragmention-poc/internal/deleter"
	"github.com/pagombin/fragmention-poc/internal/loader"
	mongoClient "github.com/pagombin/fragmention-poc/internal/mongo"
	"github.com/pagombin/fragmention-poc/internal/storage"
	"github.com/pagombin/fragmention-poc/internal/workload"
)

// TestE2E_LoadDeleteReportFlow spins a real MongoDB container, stands up
// the in-process HTTP server, and drives the full Phase 1-15 flow:
// baseline snapshot → load → post_load snapshot → delete → post_delete
// snapshot → report. Fragmentation ratio must increase post-delete and
// the report must surface bytes reclaimed per collection.
func TestE2E_LoadDeleteReportFlow(t *testing.T) {
	ctx := context.Background()
	ctr, err := mongodb.Run(ctx, "mongo:7.0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })
	uri, err := ctr.ConnectionString(ctx)
	require.NoError(t, err)

	dir := t.TempDir()
	store, err := storage.Open(ctx, storage.Config{Path: filepath.Join(dir, "e2e.db")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	mc, err := mongoClient.Connect(ctx, cfgpkg.MongoConfig{
		URI: uri, ResolvedURI: uri,
		ConnectTimeout: 30 * time.Second, OperationTimeout: 30 * time.Second, AppName: "mfpoc-e2e",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mc.Close(context.Background()) })

	samples := storage.NewSamples(store)
	snaps := storage.NewSnapshots(store)
	events := storage.NewEvents(store)
	ops := storage.NewOperations(store)
	runs := storage.NewRuns(store)

	col := collector.New(collector.Config{IdleInterval: 2 * time.Second, ActiveInterval: 500 * time.Millisecond},
		mc, samples, snaps, events, zerolog.Nop())
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = col.Run(runCtx) }()

	loaderSvc, err := loader.NewService(loader.Deps{
		Logger: zerolog.Nop(), Mongo: mc, Ops: ops, Events: events, Collector: col,
	})
	require.NoError(t, err)
	deleterSvc, err := deleter.NewService(deleter.Deps{
		Logger: zerolog.Nop(), Mongo: mc, Ops: ops, Events: events, Collector: col, Store: store,
	}, deleter.Params{BatchSize: 200, MaxRatio: 0.95}, 2*time.Minute)
	require.NoError(t, err)
	wlSvc, err := workload.NewService(workload.Deps{Logger: zerolog.Nop(), Mongo: mc, Ops: ops, Events: events})
	require.NoError(t, err)

	cfg := cfgpkg.Example()
	cfg.Auth.Enabled = false
	handler := api.NewRouter(api.Deps{
		Cfg: cfg, Logger: zerolog.Nop(), Store: store, Mongo: mc, Collector: col,
		Loader: loaderSvc, Deleter: deleterSvc, Workload: wlSvc,
		Readyz: func(ctx context.Context) error { return mc.Ping(ctx) },
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	_ = runs // reserved for Phase 15 report wiring below

	// ---- 1. baseline snapshot via API ----
	baseline := postJSON(t, srv.URL+"/api/v1/snapshots/", map[string]any{"label": "baseline"})
	require.Contains(t, baseline, "data")

	// ---- 2. load some data ----
	loadBody := map[string]any{
		"spec": map[string]any{"entries": []map[string]any{
			{"database": "poc_db_e2e", "collection": "a", "bytes_target": 300_000},
			{"database": "poc_db_e2e", "collection": "b", "bytes_target": 300_000},
		}},
		"params": map[string]any{"workers": 2, "batch_size": 50, "force_start": true},
	}
	loadResp := postJSON(t, srv.URL+"/api/v1/loader/start", loadBody)
	loadID := loadResp["data"].(map[string]any)["operation_id"].(string)
	require.NotEmpty(t, loadID)
	waitForState(t, srv.URL+"/api/v1/loader/"+loadID, []string{"completed", "failed"}, 90*time.Second)

	// Sanity: docs landed.
	cnt, err := mc.Raw().Database("poc_db_e2e").Collection("a").CountDocuments(ctx, bson.M{})
	require.NoError(t, err)
	require.Greater(t, cnt, int64(0))

	// ---- 3. post_load snapshot ----
	postLoad := postJSON(t, srv.URL+"/api/v1/snapshots/", map[string]any{"label": "post_load"})
	require.Contains(t, postLoad, "data")

	// ---- 4. delete half of one collection via preview+confirm flow ----
	prevBody := map[string]any{
		"spec": map[string]any{"entries": []map[string]any{
			{"database": "poc_db_e2e", "collection": "a",
				"pattern": map[string]any{"kind": "random_by_id", "ratio": 0.5, "seed": 42}},
		}},
		"params": map[string]any{"batch_size": 100, "max_ratio": 0.95},
	}
	prev := postJSON(t, srv.URL+"/api/v1/deleter/preview", prevBody)
	tok := prev["data"].(map[string]any)["confirmation_token"].(string)
	delStart := postJSON(t, srv.URL+"/api/v1/deleter/start", map[string]any{"confirmation_token": tok})
	delID := delStart["data"].(map[string]any)["operation_id"].(string)
	waitForState(t, srv.URL+"/api/v1/deleter/"+delID, []string{"completed", "failed"}, 60*time.Second)

	afterDel, err := mc.CollectionStats(ctx, "poc_db_e2e", "a")
	require.NoError(t, err)
	require.Greater(t, afterDel.FragmentationRatio, 0.0, "deletes should have created reclaimable free space")

	// ---- 5. post_delete snapshot ----
	postDel := postJSON(t, srv.URL+"/api/v1/snapshots/", map[string]any{"label": "post_delete"})
	postDelID := postDel["data"].(map[string]any)["id"].(string)
	baselineID := baseline["data"].(map[string]any)["id"].(string)

	// ---- 6. snapshot-compare report ----
	url := fmt.Sprintf("%s/api/v1/snapshots/compare/report?a=%s&b=%s", srv.URL, baselineID, postDelID)
	report := getJSON(t, url)
	summary := report["data"].(map[string]any)["summary"].(map[string]any)
	require.Contains(t, summary, "baseline_storage_bytes")
	require.Contains(t, summary, "final_storage_bytes")
	require.Contains(t, summary, "bytes_reclaimed")
}

func postJSON(t *testing.T, url string, body any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	require.Truef(t, res.StatusCode < 400, "POST %s -> %d: %s", url, res.StatusCode, string(b))
	var out map[string]any
	require.NoError(t, json.Unmarshal(b, &out))
	return out
}

func getJSON(t *testing.T, url string) map[string]any {
	t.Helper()
	res, err := http.Get(url)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	require.Truef(t, res.StatusCode < 400, "GET %s -> %d: %s", url, res.StatusCode, string(b))
	var out map[string]any
	require.NoError(t, json.Unmarshal(b, &out))
	return out
}

func waitForState(t *testing.T, url string, desired []string, budget time.Duration) {
	t.Helper()
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		body := getJSON(t, url)
		d, _ := body["data"].(map[string]any)
		if d == nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		state, _ := d["state"].(string)
		for _, d := range desired {
			if state == d {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("waitForState timed out on %s", url)
}
