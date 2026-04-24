package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/pagombin/fragmention-poc/internal/config"
	"github.com/pagombin/fragmention-poc/internal/storage"
)

// These tests confirm route registration. Mongo-dependent endpoints return
// 404 or 500 when Mongo is nil; what we're validating is that (a) auth
// passes, (b) the chi router has the path mounted, (c) the request doesn't
// panic.

func newTestDepsWithServices(t *testing.T) Deps {
	t.Helper()
	dir := t.TempDir()
	s, err := storage.Open(context.Background(), storage.Config{Path: filepath.Join(dir, "t.db")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	cfg := config.Example()
	cfg.Auth.Enabled = false
	return Deps{
		Cfg:    cfg,
		Logger: zerolog.Nop(),
		Store:  s,
		Readyz: func(context.Context) error { return nil },
	}
}

func TestRouter_RunsListEmpty(t *testing.T) {
	r := NewRouter(newTestDepsWithServices(t))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestRouter_EventsListEmpty(t *testing.T) {
	r := NewRouter(newTestDepsWithServices(t))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestRouter_SnapshotsCompareMissingArgs(t *testing.T) {
	// Snapshots routes only mount when Collector is non-nil. Without Mongo,
	// Collector is nil; the route should 404.
	r := NewRouter(newTestDepsWithServices(t))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshots/compare", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
