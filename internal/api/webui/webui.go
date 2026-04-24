// Package webui serves the embedded single-page application. The dist/
// directory is embedded at build time; when that directory is empty (e.g.
// during `go test` without a prior `make frontend-build`) a small
// placeholder page is served instead so the server still starts.
package webui

import (
	"bytes"
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler returns an http.Handler that serves the SPA. Requests that don't
// match a real asset fall through to index.html so client-side routing works.
// API paths (prefixed /api, /metrics, /health, /ready) are NOT served here
// and must be registered on the router before the SPA catch-all mount.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return placeholder()
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return placeholder()
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqPath := strings.TrimPrefix(r.URL.Path, "/")
		if reqPath == "" {
			reqPath = "index.html"
		}
		// Does the file exist in the FS?
		if f, err := sub.Open(reqPath); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		} else if !errors.Is(err, fs.ErrNotExist) {
			// Real IO error; let http.FileServer surface it.
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA fallback: serve index.html for any other path so deep links work.
		if idx, err := fs.ReadFile(sub, "index.html"); err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write(idx)
			return
		}
		http.NotFound(w, r)
	})
}

// placeholder returns a tiny stub served when the SPA bundle is not yet
// built. It tells the operator how to build the frontend without failing
// the server's startup.
func placeholder() http.Handler {
	msg := []byte(`<!doctype html><html><head><meta charset="utf-8"><title>mfpoc</title>` +
		`<style>body{font:14px system-ui;margin:2rem;max-width:720px}code{background:#eee;padding:2px 4px;border-radius:3px}</style>` +
		`</head><body><h1>mfpoc</h1>` +
		`<p>The SPA bundle has not been built. Run <code>make frontend-build</code> then rebuild the binary with <code>make build</code>.</p>` +
		`<p>The HTTP API is still available at <code>/api/v1/…</code> and <code>/metrics</code>.</p>` +
		`</body></html>`)
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(msg)
	})
}

// EnsureIndex returns whether the dist bundle contains an index.html. Used
// by the version endpoint to surface whether the UI is embedded.
func EnsureIndex() bool {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return false
	}
	b, err := fs.ReadFile(sub, "index.html")
	return err == nil && len(b) > 0 && !bytes.Contains(b, []byte("__placeholder__"))
}

// JoinPath is a tiny helper kept here so callers don't import path directly.
func JoinPath(a, b string) string { return path.Join(a, b) }
