package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/pagombin/fragmention-poc/internal/api/apiresp"
	"github.com/pagombin/fragmention-poc/internal/storage"
)

// RunsDeps aggregates run handler deps.
type RunsDeps struct {
	Runs     *storage.Runs
	Snaps    *storage.Snapshots
	Samples  *storage.Samples
	Events   *storage.Events
}

// RegisterRuns attaches /api/v1/runs/* routes.
func RegisterRuns(r chi.Router, d RunsDeps) {
	r.Route("/api/v1/runs", func(rr chi.Router) {
		rr.Post("/", runsCreate(d))
		rr.Get("/", runsList(d))
		rr.Get("/{id}", runsGet(d))
		rr.Post("/{id}/cancel", runsCancel(d))
		rr.Get("/{id}/snapshots", runsSnapshots(d))
		rr.Post("/{id}/snapshots", runsTagSnapshot(d))
		rr.Get("/{id}/metrics", runsMetrics(d))
	})
	r.Route("/api/v1/events", func(er chi.Router) {
		er.Get("/", eventsList(d))
	})
}

type runsCreateReq struct {
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config"`
	Notes  string          `json:"notes,omitempty"`
}

func runsCreate(d RunsDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body runsCreateReq
		if err := apiresp.DecodeJSON(r, &body); err != nil {
			apiresp.WriteError(w, http.StatusBadRequest, "invalid_body", err.Error(), nil)
			return
		}
		if body.Name == "" {
			body.Name = "run-" + time.Now().UTC().Format("20060102T150405")
		}
		run := storage.Run{
			ID: uuid.NewString(), Name: body.Name, Config: body.Config,
			Status: "created", Notes: body.Notes,
		}
		if err := d.Runs.Create(r.Context(), run); err != nil {
			apiresp.WriteError(w, http.StatusInternalServerError, "create_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusCreated, run)
	}
}

func runsList(d RunsDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 100
		if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
			limit = v
		}
		list, err := d.Runs.List(r.Context(), limit)
		if err != nil {
			apiresp.WriteError(w, http.StatusInternalServerError, "list_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, list)
	}
}

func runsGet(d RunsDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		run, err := d.Runs.Get(r.Context(), id)
		if err != nil {
			apiresp.WriteError(w, http.StatusInternalServerError, "lookup_failed", err.Error(), nil)
			return
		}
		if run == nil {
			apiresp.WriteError(w, http.StatusNotFound, "not_found", "run not found", nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, run)
	}
}

func runsCancel(d RunsDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := d.Runs.UpdateStatus(r.Context(), id, "cancelled"); err != nil {
			apiresp.WriteError(w, http.StatusNotFound, "cancel_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, map[string]any{"status": "cancelled"})
	}
}

func runsSnapshots(d RunsDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		list, err := d.Snaps.List(r.Context(), "", 1000)
		if err != nil {
			apiresp.WriteError(w, http.StatusInternalServerError, "list_failed", err.Error(), nil)
			return
		}
		var filtered []storage.Snapshot
		for _, s := range list {
			if s.RunID != nil && *s.RunID == id {
				filtered = append(filtered, s)
			}
		}
		apiresp.WriteJSON(w, http.StatusOK, filtered)
	}
}

// runsTagSnapshot returns a 501 for now; the Phase 7 scaffold exposes the
// route so frontend wiring can proceed. Actually taking snapshots is done
// by the collector - the snapshots handler above already covers the
// collector-driven flow. Surface a 307 redirect so the UI can call the
// non-run snapshot endpoint directly if it prefers.
func runsTagSnapshot(d RunsDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiresp.WriteError(w, http.StatusNotImplemented, "not_implemented",
			"tag a snapshot via POST /api/v1/snapshots with run_id in the body", nil)
	}
}

func runsMetrics(d RunsDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		q := r.URL.Query()
		limit := 10000
		if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 {
			limit = v
		}
		query := storage.Query{
			RunID:      id,
			Scope:      storage.MetricScope(q.Get("scope")),
			ScopeID:    q.Get("scope_id"),
			MetricName: q.Get("metric"),
			Limit:      limit,
		}
		if v := q.Get("since"); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				query.Since = t
			}
		}
		if v := q.Get("until"); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				query.Until = t
			}
		}
		out, err := d.Samples.Query(r.Context(), query)
		if err != nil {
			apiresp.WriteError(w, http.StatusInternalServerError, "query_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, out)
	}
}

func eventsList(d RunsDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit := 200
		if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 {
			limit = v
		}
		list, err := d.Events.List(r.Context(), q.Get("category"), q.Get("run_id"), limit)
		if err != nil {
			apiresp.WriteError(w, http.StatusInternalServerError, "list_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, list)
	}
}
