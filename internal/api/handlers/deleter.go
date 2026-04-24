package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pagombin/fragmention-poc/internal/api/apiresp"
	"github.com/pagombin/fragmention-poc/internal/deleter"
	"github.com/pagombin/fragmention-poc/internal/storage"
)

// DeleterDeps aggregates deleter handler deps.
type DeleterDeps struct {
	Service *deleter.Service
	Ops     *storage.Operations
}

// RegisterDeleter attaches /api/v1/deleter/* routes.
func RegisterDeleter(r chi.Router, d DeleterDeps) {
	r.Route("/api/v1/deleter", func(dr chi.Router) {
		dr.Post("/preview", deleterPreview(d))
		dr.Post("/start", deleterStart(d))
		dr.Post("/{id}/pause", deleterPause(d))
		dr.Post("/{id}/resume", deleterResume(d))
		dr.Post("/{id}/stop", deleterStop(d))
		dr.Get("/{id}", deleterGet(d))
		dr.Get("/active", deleterList(d))
	})
}

type deleterPreviewReq struct {
	Spec   deleter.TargetSpec `json:"spec"`
	Params deleter.Params     `json:"params"`
}

func deleterPreview(d DeleterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body deleterPreviewReq
		if err := apiresp.DecodeJSON(r, &body); err != nil {
			apiresp.WriteError(w, http.StatusBadRequest, "invalid_body", err.Error(), nil)
			return
		}
		ctx, cancel := ctxWithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		res, err := d.Service.Preview(ctx, body.Spec, body.Params)
		if err != nil {
			apiresp.WriteError(w, http.StatusBadRequest, "preview_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, res)
	}
}

type deleterStartReq struct {
	ConfirmationToken string `json:"confirmation_token"`
	RunID             string `json:"run_id,omitempty"`
}

func deleterStart(d DeleterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body deleterStartReq
		if err := apiresp.DecodeJSON(r, &body); err != nil {
			apiresp.WriteError(w, http.StatusBadRequest, "invalid_body", err.Error(), nil)
			return
		}
		if body.ConfirmationToken == "" {
			apiresp.WriteError(w, http.StatusBadRequest, "missing_token", "confirmation_token required", nil)
			return
		}
		ctx, cancel := ctxWithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		del, err := d.Service.Start(ctx, body.ConfirmationToken, body.RunID)
		if err != nil {
			apiresp.WriteError(w, http.StatusConflict, "start_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusAccepted, map[string]any{
			"operation_id": del.ID(),
			"state":        del.State(),
		})
	}
}

func deleterPause(d DeleterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		del := d.Service.Get(id)
		if del == nil {
			apiresp.WriteError(w, http.StatusNotFound, "not_found", "deleter not active", nil)
			return
		}
		if err := del.Pause(r.Context()); err != nil {
			apiresp.WriteError(w, http.StatusConflict, "pause_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, map[string]any{"state": del.State()})
	}
}

func deleterResume(d DeleterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		del := d.Service.Get(id)
		if del == nil {
			apiresp.WriteError(w, http.StatusNotFound, "not_found", "deleter not active", nil)
			return
		}
		if err := del.Resume(r.Context()); err != nil {
			apiresp.WriteError(w, http.StatusConflict, "resume_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, map[string]any{"state": del.State()})
	}
}

func deleterStop(d DeleterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		del := d.Service.Get(id)
		if del == nil {
			apiresp.WriteError(w, http.StatusNotFound, "not_found", "deleter not active", nil)
			return
		}
		if err := del.Stop(r.Context()); err != nil {
			apiresp.WriteError(w, http.StatusConflict, "stop_failed", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, map[string]any{"state": del.State()})
	}
}

func deleterGet(d DeleterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if del := d.Service.Get(id); del != nil {
			progress, _ := d.Ops.ListProgress(r.Context(), id)
			apiresp.WriteJSON(w, http.StatusOK, map[string]any{
				"operation_id": del.ID(),
				"state":        del.State(),
				"params":       del.Params(),
				"stats":        del.SnapshotStats(),
				"progress":     progress,
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

func deleterList(d DeleterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		active := d.Service.ListActive()
		out := make([]map[string]any, 0, len(active))
		for _, del := range active {
			out = append(out, map[string]any{
				"operation_id": del.ID(),
				"state":        del.State(),
				"params":       del.Params(),
				"stats":        del.SnapshotStats(),
			})
		}
		apiresp.WriteJSON(w, http.StatusOK, out)
	}
}
