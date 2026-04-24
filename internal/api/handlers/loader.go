package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pagombin/fragmention-poc/internal/api/apiresp"
	"github.com/pagombin/fragmention-poc/internal/loader"
	"github.com/pagombin/fragmention-poc/internal/storage"
)

// LoaderDeps carries the dependencies the loader handlers need.
type LoaderDeps struct {
	Service *loader.Service
	Ops     *storage.Operations
}

// RegisterLoader attaches /api/v1/loader/* routes.
func RegisterLoader(r chi.Router, d LoaderDeps) {
	r.Route("/api/v1/loader", func(lr chi.Router) {
		lr.Post("/start", loaderStart(d))
		lr.Post("/{id}/pause", loaderPause(d))
		lr.Post("/{id}/resume", loaderResume(d))
		lr.Post("/{id}/stop", loaderStop(d))
		lr.Patch("/{id}/params", loaderPatch(d))
		lr.Get("/{id}", loaderGet(d))
		lr.Get("/active", loaderList(d))
	})
}

type loaderStartReq struct {
	RunID  string            `json:"run_id,omitempty"`
	Spec   loader.TargetSpec `json:"spec"`
	Params loader.Params     `json:"params"`
}

func loaderStart(d LoaderDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body loaderStartReq
		if err := apiresp.DecodeJSON(r, &body); err != nil {
			apiresp.WriteError(w, http.StatusBadRequest, "invalid_body", err.Error(), nil)
			return
		}
		ctx, cancel := ctxWithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		l, err := d.Service.Start(ctx, body.Spec, body.Params, body.RunID)
		if err != nil {
			apiresp.WriteError(w, http.StatusConflict, "start_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusAccepted, map[string]any{
			"operation_id": l.ID(),
			"state":        l.State(),
		})
	}
}

func loaderPause(d LoaderDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		l := d.Service.Get(id)
		if l == nil {
			apiresp.WriteError(w, http.StatusNotFound, "not_found", "loader not active", nil)
			return
		}
		if err := l.Pause(r.Context()); err != nil {
			apiresp.WriteError(w, http.StatusConflict, "pause_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, map[string]any{"state": l.State()})
	}
}

func loaderResume(d LoaderDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		l := d.Service.Get(id)
		if l == nil {
			apiresp.WriteError(w, http.StatusNotFound, "not_found", "loader not active", nil)
			return
		}
		if err := l.Resume(r.Context()); err != nil {
			apiresp.WriteError(w, http.StatusConflict, "resume_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, map[string]any{"state": l.State()})
	}
}

func loaderStop(d LoaderDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		l := d.Service.Get(id)
		if l == nil {
			apiresp.WriteError(w, http.StatusNotFound, "not_found", "loader not active", nil)
			return
		}
		if err := l.Stop(r.Context()); err != nil {
			apiresp.WriteError(w, http.StatusConflict, "stop_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, map[string]any{"state": l.State()})
	}
}

func loaderPatch(d LoaderDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		l := d.Service.Get(id)
		if l == nil {
			apiresp.WriteError(w, http.StatusNotFound, "not_found", "loader not active", nil)
			return
		}
		var p loader.Params
		if err := apiresp.DecodeJSON(r, &p); err != nil {
			apiresp.WriteError(w, http.StatusBadRequest, "invalid_body", err.Error(), nil)
			return
		}
		if err := l.SetParams(p); err != nil {
			apiresp.WriteError(w, http.StatusBadRequest, "invalid_params", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, l.Params())
	}
}

func loaderGet(d LoaderDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if l := d.Service.Get(id); l != nil {
			progress, _ := d.Ops.ListProgress(r.Context(), id)
			apiresp.WriteJSON(w, http.StatusOK, map[string]any{
				"operation_id": l.ID(),
				"state":        l.State(),
				"params":       l.Params(),
				"stats":        l.SnapshotStats(),
				"progress":     progress,
			})
			return
		}
		// Fall back to persisted record.
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

func loaderList(d LoaderDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		active := d.Service.ListActive()
		out := make([]map[string]any, 0, len(active))
		for _, l := range active {
			out = append(out, map[string]any{
				"operation_id": l.ID(),
				"state":        l.State(),
				"params":       l.Params(),
				"stats":        l.SnapshotStats(),
			})
		}
		apiresp.WriteJSON(w, http.StatusOK, out)
	}
}
