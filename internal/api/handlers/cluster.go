// Package handlers contains the HTTP handlers for the REST surface. The
// package is organised by resource (cluster, runs, loader, deleter, ...);
// each file exposes a Register function that the router calls to attach
// its routes.
package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pagombin/fragmention-poc/internal/api/apiresp"
	"github.com/pagombin/fragmention-poc/internal/collector"
	mongoClient "github.com/pagombin/fragmention-poc/internal/mongo"
)

// ClusterDeps aggregates the dependencies cluster handlers need.
type ClusterDeps struct {
	Client    *mongoClient.Client
	Collector *collector.Collector
}

// RegisterCluster attaches /api/v1/cluster/* routes to r.
func RegisterCluster(r chi.Router, d ClusterDeps) {
	r.Route("/api/v1/cluster", func(cr chi.Router) {
		cr.Get("/topology", func(w http.ResponseWriter, req *http.Request) {
			ctx, cancel := ctxWithTimeout(req.Context(), 5*time.Second)
			defer cancel()
			top, err := d.Client.DetectTopology(ctx)
			if err != nil {
				apiresp.WriteError(w, http.StatusBadGateway, "topology_error", err.Error(), nil)
				return
			}
			apiresp.WriteJSON(w, http.StatusOK, map[string]any{
				"topology":    top,
				"server_info": d.Client.ServerInfo(),
				"redacted_uri": d.Client.RedactedURI(),
				"is_srv":      d.Client.IsSRV(),
			})
		})

		cr.Get("/preflight", func(w http.ResponseWriter, req *http.Request) {
			ctx, cancel := ctxWithTimeout(req.Context(), 10*time.Second)
			defer cancel()
			dbs, err := d.Client.ListDatabases(ctx)
			if err != nil {
				apiresp.WriteError(w, http.StatusBadGateway, "preflight_error", err.Error(), nil)
				return
			}
			var storageTotal, dataTotal, fsUsed, fsTotal int64
			for _, db := range dbs {
				storageTotal += db.StorageSize
				dataTotal += db.DataSize
				if db.FsTotalSize > fsTotal {
					fsTotal = db.FsTotalSize
					fsUsed = db.FsUsedSize
				}
			}
			frag := 0.0
			if storageTotal > 0 {
				frag = float64(storageTotal-dataTotal) / float64(storageTotal)
			}
			apiresp.WriteJSON(w, http.StatusOK, map[string]any{
				"storage_size_bytes":  storageTotal,
				"data_size_bytes":     dataTotal,
				"fragmentation_ratio": frag,
				"fs_used_bytes":       fsUsed,
				"fs_total_bytes":      fsTotal,
				"databases":           dbs,
			})
		})

		cr.Get("/databases", func(w http.ResponseWriter, req *http.Request) {
			ctx, cancel := ctxWithTimeout(req.Context(), 10*time.Second)
			defer cancel()
			dbs, err := d.Client.ListDatabases(ctx)
			if err != nil {
				apiresp.WriteError(w, http.StatusBadGateway, "list_error", err.Error(), nil)
				return
			}
			apiresp.WriteJSON(w, http.StatusOK, dbs)
		})

		cr.Get("/databases/{db}/collections", func(w http.ResponseWriter, req *http.Request) {
			dbName := chi.URLParam(req, "db")
			if dbName == "" {
				apiresp.WriteError(w, http.StatusBadRequest, "invalid_db", "database name required", nil)
				return
			}
			ctx, cancel := ctxWithTimeout(req.Context(), 15*time.Second)
			defer cancel()
			colls, err := d.Client.ListCollections(ctx, dbName)
			if err != nil {
				apiresp.WriteError(w, http.StatusBadGateway, "list_error", err.Error(), nil)
				return
			}
			apiresp.WriteJSON(w, http.StatusOK, colls)
		})

		cr.Get("/databases/{db}/collections/{coll}/stats", func(w http.ResponseWriter, req *http.Request) {
			dbName := chi.URLParam(req, "db")
			coll := chi.URLParam(req, "coll")
			if dbName == "" || coll == "" {
				apiresp.WriteError(w, http.StatusBadRequest, "invalid_target", "database and collection required", nil)
				return
			}
			ctx, cancel := ctxWithTimeout(req.Context(), 10*time.Second)
			defer cancel()
			stats, err := d.Client.CollectionStats(ctx, dbName, coll)
			if err != nil {
				apiresp.WriteError(w, http.StatusBadGateway, "stats_error", err.Error(), nil)
				return
			}
			apiresp.WriteJSON(w, http.StatusOK, stats)
		})
	})
}
