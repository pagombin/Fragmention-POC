package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pagombin/fragmention-poc/internal/api/apiresp"
	"github.com/pagombin/fragmention-poc/internal/collector"
	"github.com/pagombin/fragmention-poc/internal/storage"
)

// SnapshotsDeps aggregates the dependencies snapshot handlers need.
type SnapshotsDeps struct {
	Collector *collector.Collector
	Snaps     *storage.Snapshots
}

// RegisterSnapshots attaches /api/v1/snapshots/* routes.
func RegisterSnapshots(r chi.Router, d SnapshotsDeps) {
	r.Route("/api/v1/snapshots", func(sr chi.Router) {
		sr.Post("/", snapshotCreate(d))
		sr.Get("/", snapshotList(d))
		sr.Get("/{id}", snapshotGet(d))
		sr.Get("/compare", snapshotCompare(d))
	})
}

type snapshotCreateReq struct {
	Label string `json:"label"`
	Note  string `json:"note"`
	RunID string `json:"run_id,omitempty"`
}

func snapshotCreate(d SnapshotsDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body snapshotCreateReq
		if err := apiresp.DecodeJSON(r, &body); err != nil {
			apiresp.WriteError(w, http.StatusBadRequest, "invalid_body", err.Error(), nil)
			return
		}
		if body.Label == "" {
			apiresp.WriteError(w, http.StatusBadRequest, "invalid_body", "label required", nil)
			return
		}
		ctx, cancel := ctxWithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		id, err := d.Collector.TakeSnapshot(ctx, body.Label, body.Note, body.RunID)
		if err != nil {
			apiresp.WriteError(w, http.StatusBadGateway, "snapshot_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusCreated, map[string]any{"id": id})
	}
}

func snapshotList(d SnapshotsDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		label := r.URL.Query().Get("label")
		limitStr := r.URL.Query().Get("limit")
		limit := 50
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
		}
		list, err := d.Snaps.List(r.Context(), label, limit)
		if err != nil {
			apiresp.WriteError(w, http.StatusInternalServerError, "list_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, list)
	}
}

func snapshotGet(d SnapshotsDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		s, err := d.Snaps.Get(r.Context(), id)
		if err != nil {
			apiresp.WriteError(w, http.StatusInternalServerError, "lookup_failed", err.Error(), nil)
			return
		}
		if s == nil {
			apiresp.WriteError(w, http.StatusNotFound, "not_found", "snapshot not found", nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, s)
	}
}

// snapshotCompare returns a structural diff of two snapshots so the Initial
// Sync Companion view can render reclaim-per-collection without a run.
func snapshotCompare(d SnapshotsDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		aID := r.URL.Query().Get("a")
		bID := r.URL.Query().Get("b")
		if aID == "" || bID == "" {
			apiresp.WriteError(w, http.StatusBadRequest, "invalid_query", "both a and b required", nil)
			return
		}
		a, err := d.Snaps.Get(r.Context(), aID)
		if err != nil || a == nil {
			apiresp.WriteError(w, http.StatusNotFound, "not_found", "snapshot a not found", nil)
			return
		}
		b, err := d.Snaps.Get(r.Context(), bID)
		if err != nil || b == nil {
			apiresp.WriteError(w, http.StatusNotFound, "not_found", "snapshot b not found", nil)
			return
		}
		diff := diffSnapshots(a.RawStats, b.RawStats)
		apiresp.WriteJSON(w, http.StatusOK, map[string]any{
			"a": map[string]any{"id": a.ID, "label": a.Label, "taken_at": a.TakenAt},
			"b": map[string]any{"id": b.ID, "label": b.Label, "taken_at": b.TakenAt},
			"diff": diff,
		})
	}
}

// diffSnapshots computes per-collection and per-database deltas between two
// snapshot payloads produced by Collector.TakeSnapshot. The shape is stable
// so the UI can render a clean reclaim table.
func diffSnapshots(a, b json.RawMessage) map[string]any {
	var A, B struct {
		Databases   []map[string]any                     `json:"databases"`
		Collections map[string][]map[string]any          `json:"collections"`
	}
	_ = json.Unmarshal(a, &A)
	_ = json.Unmarshal(b, &B)

	// Database deltas keyed by name.
	dbMapA := mapByName(A.Databases)
	dbMapB := mapByName(B.Databases)
	var dbDeltas []map[string]any
	for name, a := range dbMapA {
		bv, ok := dbMapB[name]
		if !ok {
			continue
		}
		dbDeltas = append(dbDeltas, map[string]any{
			"name":            name,
			"storage_delta":   asInt64(bv["storage_size"]) - asInt64(a["storage_size"]),
			"data_delta":      asInt64(bv["data_size"]) - asInt64(a["data_size"]),
			"index_delta":     asInt64(bv["index_size"]) - asInt64(a["index_size"]),
			"storage_before":  asInt64(a["storage_size"]),
			"storage_after":   asInt64(bv["storage_size"]),
		})
	}

	// Collection deltas: flatten into (db, coll).
	var collDeltas []map[string]any
	for db, colls := range A.Collections {
		aByName := collMapByName(colls)
		bByName := collMapByName(B.Collections[db])
		for name, a := range aByName {
			bv, ok := bByName[name]
			if !ok {
				continue
			}
			collDeltas = append(collDeltas, map[string]any{
				"database":             db,
				"collection":           name,
				"storage_delta":        asInt64(bv["storage_size"]) - asInt64(a["storage_size"]),
				"free_storage_delta":   asInt64(bv["free_storage_size"]) - asInt64(a["free_storage_size"]),
				"count_delta":          asInt64(bv["count"]) - asInt64(a["count"]),
				"storage_before":       asInt64(a["storage_size"]),
				"storage_after":        asInt64(bv["storage_size"]),
				"fragmentation_before": asFloat64(a["fragmentation_ratio"]),
				"fragmentation_after":  asFloat64(bv["fragmentation_ratio"]),
			})
		}
	}
	return map[string]any{
		"databases":   dbDeltas,
		"collections": collDeltas,
	}
}

func mapByName(list []map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any, len(list))
	for _, m := range list {
		if n, ok := m["name"].(string); ok {
			out[n] = m
		}
	}
	return out
}

func collMapByName(list []map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any, len(list))
	for _, m := range list {
		if n, ok := m["name"].(string); ok {
			out[n] = m
		}
	}
	return out
}

func asInt64(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	}
	return 0
}

func asFloat64(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	}
	return 0
}
