package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pagombin/fragmention-poc/internal/api/apiresp"
	"github.com/pagombin/fragmention-poc/internal/storage"
	"github.com/pagombin/fragmention-poc/internal/workload"
)

// WorkloadDeps aggregates workload handler deps.
type WorkloadDeps struct {
	Service *workload.Service
	Ops     *storage.Operations
}

// RegisterWorkload attaches /api/v1/workload/* routes.
func RegisterWorkload(r chi.Router, d WorkloadDeps) {
	r.Route("/api/v1/workload", func(wr chi.Router) {
		wr.Post("/start", workloadStart(d))
		wr.Post("/{id}/stop", workloadStop(d))
		wr.Patch("/{id}/rate", workloadRate(d))
		wr.Get("/{id}", workloadGet(d))
		wr.Get("/active", workloadList(d))
	})
}

type workloadStartReq struct {
	Spec   workload.Spec   `json:"spec"`
	Params workload.Params `json:"params"`
	RunID  string          `json:"run_id,omitempty"`
}

func workloadStart(d WorkloadDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body workloadStartReq
		if err := apiresp.DecodeJSON(r, &body); err != nil {
			apiresp.WriteError(w, http.StatusBadRequest, "invalid_body", err.Error(), nil)
			return
		}
		wl, err := d.Service.Start(r.Context(), body.Spec, body.Params, body.RunID)
		if err != nil {
			apiresp.WriteError(w, http.StatusConflict, "start_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusAccepted, map[string]any{
			"operation_id": wl.ID(),
			"state":        wl.State(),
		})
	}
}

func workloadStop(d WorkloadDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		wl := d.Service.Get(id)
		if wl == nil {
			apiresp.WriteError(w, http.StatusNotFound, "not_found", "workload not active", nil)
			return
		}
		if err := wl.Stop(r.Context()); err != nil {
			apiresp.WriteError(w, http.StatusConflict, "stop_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, map[string]any{"state": wl.State()})
	}
}

func workloadRate(d WorkloadDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		wl := d.Service.Get(id)
		if wl == nil {
			apiresp.WriteError(w, http.StatusNotFound, "not_found", "workload not active", nil)
			return
		}
		var p workload.Params
		if err := apiresp.DecodeJSON(r, &p); err != nil {
			apiresp.WriteError(w, http.StatusBadRequest, "invalid_body", err.Error(), nil)
			return
		}
		if err := wl.SetParams(p); err != nil {
			apiresp.WriteError(w, http.StatusBadRequest, "invalid_params", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, wl.Params())
	}
}

func workloadGet(d WorkloadDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if wl := d.Service.Get(id); wl != nil {
			opsDone, errs := wl.SnapshotStats()
			apiresp.WriteJSON(w, http.StatusOK, map[string]any{
				"operation_id": wl.ID(),
				"state":        wl.State(),
				"params":       wl.Params(),
				"ops_done":     opsDone,
				"errors":       errs,
			})
			return
		}
		op, err := d.Ops.Get(r.Context(), id)
		if err != nil {
			apiresp.WriteError(w, http.StatusInternalServerError, "lookup_failed", err.Error(), nil)
			return
		}
		if op == nil {
			apiresp.WriteError(w, http.StatusNotFound, "not_found", "operation not found", nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, op)
	}
}

func workloadList(d WorkloadDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		active := d.Service.ListActive()
		out := make([]map[string]any, 0, len(active))
		for _, wl := range active {
			opsDone, errs := wl.SnapshotStats()
			out = append(out, map[string]any{
				"operation_id": wl.ID(),
				"state":        wl.State(),
				"params":       wl.Params(),
				"ops_done":     opsDone,
				"errors":       errs,
			})
		}
		apiresp.WriteJSON(w, http.StatusOK, out)
	}
}
