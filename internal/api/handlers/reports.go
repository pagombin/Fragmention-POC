package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pagombin/fragmention-poc/internal/api/apiresp"
	"github.com/pagombin/fragmention-poc/internal/reports"
)

// ReportDeps aggregates the repositories the reports package needs. The
// runs handler set already owns Runs / Snaps / Samples / Events; reports
// reuse those via a parallel Deps to keep registration clean.
type ReportDeps struct {
	Generator *reports.Generator
}

// RegisterReports attaches /api/v1/runs/{id}/report(.csv) and
// /api/v1/snapshots/compare/report for offline-friendly exports.
func RegisterReports(r chi.Router, d ReportDeps) {
	r.Get("/api/v1/runs/{id}/report", reportGet(d, false))
	r.Get("/api/v1/runs/{id}/report.csv", reportGet(d, true))
	r.Get("/api/v1/snapshots/compare/report", compareReport(d, false))
	r.Get("/api/v1/snapshots/compare/report.csv", compareReport(d, true))
}

func reportGet(d ReportDeps, csv bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		rep, err := d.Generator.GenerateRun(r.Context(), id)
		if err != nil {
			apiresp.WriteError(w, http.StatusNotFound, "report_failed", err.Error(), nil)
			return
		}
		if csv {
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			safeName := strings.Map(func(r rune) rune {
				if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
					return r
				}
				return '_'
			}, strings.ToLower(rep.Run.Name))
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="report-%s.csv"`, safeName))
			if err := reports.WriteCSV(w, rep); err != nil {
				apiresp.WriteError(w, http.StatusInternalServerError, "csv_failed", err.Error(), nil)
			}
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, rep)
	}
}

func compareReport(d ReportDeps, csv bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := r.URL.Query().Get("a")
		b := r.URL.Query().Get("b")
		if a == "" || b == "" {
			apiresp.WriteError(w, http.StatusBadRequest, "invalid_query", "both a and b required", nil)
			return
		}
		rep, err := d.Generator.GenerateCompareSnapshots(r.Context(), a, b)
		if err != nil {
			apiresp.WriteError(w, http.StatusNotFound, "report_failed", err.Error(), nil)
			return
		}
		if csv {
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="compare.csv"`)
			if err := reports.WriteCSV(w, rep); err != nil {
				apiresp.WriteError(w, http.StatusInternalServerError, "csv_failed", err.Error(), nil)
			}
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, rep)
	}
}
