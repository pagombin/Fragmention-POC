// Package handlers contains the HTTP handlers for the REST surface. The
// package is organised by resource (cluster, runs, loader, deleter, ...);
// each file exposes a Register function that the router calls to attach
// its routes.
package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"

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

		cr.Delete("/databases/{db}", func(w http.ResponseWriter, req *http.Request) {
			dbName := chi.URLParam(req, "db")
			if isSystemDB(dbName) {
				apiresp.WriteError(w, http.StatusForbidden, "system_db",
					"refusing to drop system database "+dbName, nil)
				return
			}
			ctx, cancel := ctxWithTimeout(req.Context(), 30*time.Second)
			defer cancel()
			if err := d.Client.Raw().Database(dbName).Drop(ctx); err != nil {
				apiresp.WriteError(w, http.StatusBadGateway, "drop_failed", err.Error(), nil)
				return
			}
			apiresp.WriteJSON(w, http.StatusOK, map[string]any{"dropped": dbName})
		})

		cr.Delete("/databases/{db}/collections/{coll}", func(w http.ResponseWriter, req *http.Request) {
			dbName := chi.URLParam(req, "db")
			coll := chi.URLParam(req, "coll")
			if isSystemDB(dbName) {
				apiresp.WriteError(w, http.StatusForbidden, "system_db",
					"refusing to drop collection in system database "+dbName, nil)
				return
			}
			ctx, cancel := ctxWithTimeout(req.Context(), 30*time.Second)
			defer cancel()
			if err := d.Client.Raw().Database(dbName).Collection(coll).Drop(ctx); err != nil {
				apiresp.WriteError(w, http.StatusBadGateway, "drop_failed", err.Error(), nil)
				return
			}
			apiresp.WriteJSON(w, http.StatusOK, map[string]any{"dropped": dbName + "." + coll})
		})
	})
}

// isSystemDB protects admin/config/local from destructive endpoints.
// Spec § 21.5 / § 5.4 require these never be touched by user-driven flows.
func isSystemDB(name string) bool {
	switch name {
	case "admin", "config", "local":
		return true
	}
	return false
}

// RegisterClusterDiag attaches the /api/v1/cluster/diag endpoint that
// returns the raw replSetGetStatus + hello payloads so the operator can see
// exactly what the cluster returned. Useful when the topology view shows
// an empty members list and we need to know if it's a permission problem,
// a parsing problem, or a managed-cluster restriction. The endpoint is
// read-only and only runs commands from a fixed allow-list.
func RegisterClusterDiag(r chi.Router, d ClusterDeps) {
	r.Get("/api/v1/cluster/diag", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := ctxWithTimeout(req.Context(), 10*time.Second)
		defer cancel()

		out := map[string]any{}

		hello, err := d.Client.RawCommand(ctx, "admin", bson.D{{Key: "hello", Value: 1}})
		if err != nil {
			out["hello_error"] = err.Error()
		} else {
			out["hello"] = hello
		}

		status, err := d.Client.RawCommand(ctx, "admin", bson.D{{Key: "replSetGetStatus", Value: 1}})
		if err != nil {
			out["replSetGetStatus_error"] = err.Error()
		} else {
			out["replSetGetStatus"] = status
		}

		apiresp.WriteJSON(w, http.StatusOK, out)
	})
}
