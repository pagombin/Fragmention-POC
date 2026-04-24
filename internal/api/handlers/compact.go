package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pagombin/fragmention-poc/internal/api/apiresp"
	"github.com/pagombin/fragmention-poc/internal/compact"
	"github.com/pagombin/fragmention-poc/internal/storage"
)

// CompactDeps aggregates compact handler deps.
type CompactDeps struct {
	Service *compact.Service
	Ops     *storage.Operations
}

// RegisterCompact attaches /api/v1/compact/* routes.
func RegisterCompact(r chi.Router, d CompactDeps) {
	r.Route("/api/v1/compact", func(cr chi.Router) {
		cr.Post("/preview", compactPreview(d))
		cr.Post("/start", compactStart(d))
		cr.Post("/{id}/cancel", compactCancel(d))
		cr.Get("/{id}", compactGet(d))
		cr.Get("/active", compactList(d))
	})
}

type compactBodyReq struct {
	Scope  compact.Scope  `json:"scope"`
	Params compact.Params `json:"params"`
	RunID  string         `json:"run_id,omitempty"`
}

func compactPreview(d CompactDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body compactBodyReq
		if err := apiresp.DecodeJSON(r, &body); err != nil {
			apiresp.WriteError(w, http.StatusBadRequest, "invalid_body", err.Error(), nil)
			return
		}
		ctx, cancel := ctxWithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		res, err := d.Service.Preview(ctx, body.Scope, body.Params)
		if err != nil {
			apiresp.WriteError(w, http.StatusBadRequest, "preview_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, res)
	}
}

func compactStart(d CompactDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body compactBodyReq
		if err := apiresp.DecodeJSON(r, &body); err != nil {
			apiresp.WriteError(w, http.StatusBadRequest, "invalid_body", err.Error(), nil)
			return
		}
		ctx, cancel := ctxWithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		o, err := d.Service.Start(ctx, body.Scope, body.Params, body.RunID)
		if err != nil {
			apiresp.WriteError(w, http.StatusConflict, "start_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusAccepted, map[string]any{
			"operation_id": o.ID(),
			"state":        o.State(),
		})
	}
}

func compactCancel(d CompactDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		o := d.Service.Get(id)
		if o == nil {
			apiresp.WriteError(w, http.StatusNotFound, "not_found", "compact not active", nil)
			return
		}
		if err := o.Cancel(r.Context()); err != nil {
			apiresp.WriteError(w, http.StatusConflict, "cancel_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, map[string]any{"state": o.State()})
	}
}

func compactGet(d CompactDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if o := d.Service.Get(id); o != nil {
			apiresp.WriteJSON(w, http.StatusOK, map[string]any{
				"operation_id": o.ID(),
				"state":        o.State(),
				"current_step": o.CurrentStep(),
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

func compactList(d CompactDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		active := d.Service.ListActive()
		out := make([]map[string]any, 0, len(active))
		for _, o := range active {
			out = append(out, map[string]any{
				"operation_id": o.ID(),
				"state":        o.State(),
				"current_step": o.CurrentStep(),
			})
		}
		apiresp.WriteJSON(w, http.StatusOK, out)
	}
}
