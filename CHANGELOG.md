# Changelog

All notable changes to this project are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Project scaffold, Go module, directory layout per spec § 14.
- Configuration loader (viper + YAML) with `${env:...}` and `${file:...}`
  secret references; SRV-aware MongoDB URI handling and redacted logging.
- zerolog-based JSON logger with runtime level adjustment.
- SQLite state store (modernc.org/sqlite, WAL) with goose migrations
  covering runs / snapshots / metrics_samples / operations /
  operation_progress / delete_candidates / events.
- HTTP middleware: request IDs, request-scoped logging, security headers,
  `MaxBytesReader`, bearer auth with per-IP failed-attempt rate limiting,
  per-IP write rate limiting.
- HTTP server with self-signed TLS generation, public-bind HTTP refusal,
  graceful shutdown.
- Health / ready / version / log-level admin endpoints.
- cobra CLI: `server`, `version`, `config-validate`, `purge` (stub).
- Makefile, golangci-lint config, editorconfig, config examples, initial
  documentation skeleton.
