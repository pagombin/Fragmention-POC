package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/pagombin/fragmention-poc/internal/config"
	"github.com/pagombin/fragmention-poc/internal/storage"
)

func newTestDeps(t *testing.T, cfg *config.Config) Deps {
	t.Helper()
	dir := t.TempDir()
	s, err := storage.Open(context.Background(), storage.Config{Path: filepath.Join(dir, "t.db")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return Deps{
		Cfg:    cfg,
		Logger: zerolog.Nop(),
		Store:  s,
		Readyz: func(context.Context) error { return nil },
	}
}

func TestRouter_HealthPublic(t *testing.T) {
	cfg := config.Example()
	cfg.Auth.Enabled = true
	cfg.Auth.BearerToken = "abcdefghijklmnopqrstuvwxyz0123456789"
	r := NewRouter(newTestDeps(t, cfg))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var env Envelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Nil(t, env.Error)
}

func TestRouter_AuthedVersionRoute(t *testing.T) {
	cfg := config.Example()
	cfg.Auth.Enabled = true
	cfg.Auth.BearerToken = "abcdefghijklmnopqrstuvwxyz0123456789"
	r := NewRouter(newTestDeps(t, cfg))

	// No token - expect 401.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	// With token.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.Auth.BearerToken)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestRouter_AdminLogLevel(t *testing.T) {
	cfg := config.Example()
	cfg.Auth.Enabled = false
	r := NewRouter(newTestDeps(t, cfg))

	body, _ := json.Marshal(map[string]string{"level": "debug"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/log-level", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Malformed - expect 400.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/log-level", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRouter_SecurityHeaders(t *testing.T) {
	cfg := config.Example()
	cfg.Auth.Enabled = false
	r := NewRouter(newTestDeps(t, cfg))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	require.NotEmpty(t, rec.Header().Get("Content-Security-Policy"))
}
