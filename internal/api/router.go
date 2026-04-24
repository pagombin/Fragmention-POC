package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"

	"github.com/pagombin/fragmention-poc/internal/api/apiresp"
	"github.com/pagombin/fragmention-poc/internal/api/handlers"
	"github.com/pagombin/fragmention-poc/internal/api/middleware"
	"github.com/pagombin/fragmention-poc/internal/collector"
	"github.com/pagombin/fragmention-poc/internal/compact"
	"github.com/pagombin/fragmention-poc/internal/config"
	"github.com/pagombin/fragmention-poc/internal/deleter"
	"github.com/pagombin/fragmention-poc/internal/loader"
	"github.com/pagombin/fragmention-poc/internal/logging"
	"github.com/pagombin/fragmention-poc/internal/metrics"
	mongoClient "github.com/pagombin/fragmention-poc/internal/mongo"
	"github.com/pagombin/fragmention-poc/internal/storage"
	"github.com/pagombin/fragmention-poc/internal/version"
	"github.com/pagombin/fragmention-poc/internal/workload"
)

// Deps aggregates the runtime dependencies needed by the HTTP handlers.
type Deps struct {
	Cfg       *config.Config
	Logger    zerolog.Logger
	Store     *storage.Store
	Readyz    func(context.Context) error
	Mongo     *mongoClient.Client
	Collector *collector.Collector
	Loader    *loader.Service
	Deleter   *deleter.Service
	Compact   *compact.Service
	Workload  *workload.Service
}

// NewRouter constructs the Phase-1 API surface: uniform envelope, auth,
// rate limiting, request IDs, security headers, and health/ready endpoints.
// Later phases register additional routes onto the returned chi.Router.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	tlsActive := d.Cfg != nil && d.Cfg.Server.TLS.Enabled
	maxBytes := int64(1 << 20)
	rateLimit := 60
	authFailLimit := 10
	var bearer string
	if d.Cfg != nil {
		if d.Cfg.Server.MaxRequestBytes > 0 {
			maxBytes = d.Cfg.Server.MaxRequestBytes
		}
		if d.Cfg.Server.RateLimitPerMinute > 0 {
			rateLimit = d.Cfg.Server.RateLimitPerMinute
		}
		if d.Cfg.Server.RateLimitFailedAuthPerMin > 0 {
			authFailLimit = d.Cfg.Server.RateLimitFailedAuthPerMin
		}
		if d.Cfg.Auth.Enabled {
			bearer = d.Cfg.Auth.BearerToken
		}
	}

	authLimiter := middleware.NewAuthRateLimiter(authFailLimit, time.Minute)
	writeLimiter := middleware.NewRateLimiter(rateLimit, time.Minute)

	// Public, unauthenticated prefixes.
	publicPrefixes := []string{"/health", "/ready", "/metrics", "/api/v1/health", "/api/v1/ready"}

	r.Use(chimw.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.WithLogger(d.Logger))
	r.Use(middleware.SecurityHeaders(tlsActive))
	r.Use(middleware.MaxBodyBytes(maxBytes))
	r.Use(writeLimiter.Middleware(map[string]bool{
		http.MethodPost:   true,
		http.MethodPut:    true,
		http.MethodPatch:  true,
		http.MethodDelete: true,
	}))
	r.Use(middleware.BearerAuth(bearer, publicPrefixes, authLimiter))

	registerHealth(r, d)
	registerAdmin(r, d)
	registerMetrics(r)
	if d.Mongo != nil {
		handlers.RegisterCluster(r, handlers.ClusterDeps{Client: d.Mongo, Collector: d.Collector})
	}
	if d.Store != nil {
		ops := storage.NewOperations(d.Store)
		snaps := storage.NewSnapshots(d.Store)
		samples := storage.NewSamples(d.Store)
		events := storage.NewEvents(d.Store)
		runsRepo := storage.NewRuns(d.Store)

		if d.Loader != nil {
			handlers.RegisterLoader(r, handlers.LoaderDeps{Service: d.Loader, Ops: ops})
		}
		if d.Deleter != nil {
			handlers.RegisterDeleter(r, handlers.DeleterDeps{Service: d.Deleter, Ops: ops})
		}
		if d.Compact != nil {
			handlers.RegisterCompact(r, handlers.CompactDeps{Service: d.Compact, Ops: ops})
		}
		if d.Workload != nil {
			handlers.RegisterWorkload(r, handlers.WorkloadDeps{Service: d.Workload, Ops: ops})
		}
		if d.Collector != nil {
			handlers.RegisterSnapshots(r, handlers.SnapshotsDeps{Collector: d.Collector, Snaps: snaps})
		}
		handlers.RegisterRuns(r, handlers.RunsDeps{
			Runs: runsRepo, Snaps: snaps, Samples: samples, Events: events,
		})
	}
	return r
}

func registerMetrics(r chi.Router) {
	r.Handle("/metrics", promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{}))
}

func registerHealth(r chi.Router, d Deps) {
	r.Get("/health", healthHandler)
	r.Get("/api/v1/health", healthHandler)
	r.Get("/ready", readyHandler(d))
	r.Get("/api/v1/ready", readyHandler(d))
	r.Get("/api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		apiresp.WriteJSON(w, http.StatusOK, version.Get())
	})
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	apiresp.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func readyHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Readyz != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			if err := d.Readyz(ctx); err != nil {
				apiresp.WriteError(w, http.StatusServiceUnavailable, "not_ready", err.Error(), nil)
				return
			}
		}
		apiresp.WriteJSON(w, http.StatusOK, map[string]any{"status": "ready"})
	}
}

func registerAdmin(r chi.Router, _ Deps) {
	// POST /api/v1/admin/log-level { "level": "debug" }
	r.Post("/api/v1/admin/log-level", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Level string `json:"level"`
		}
		if err := apiresp.DecodeJSON(r, &body); err != nil {
			apiresp.WriteError(w, http.StatusBadRequest, "invalid_body", err.Error(), nil)
			return
		}
		prev, err := logging.SetLevel(body.Level)
		if err != nil {
			apiresp.WriteError(w, http.StatusBadRequest, "invalid_level", err.Error(), nil)
			return
		}
		apiresp.WriteJSON(w, http.StatusOK, map[string]any{
			"previous": prev,
			"current":  logging.Level(),
		})
	})
}
