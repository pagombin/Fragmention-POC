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
- MongoDB v2 driver wrapper (`internal/mongo`): connection, version gate
  (≥5.0), topology detection for standalone / replica set / (flagged)
  sharded, `collStats`/`dbStats` helpers, collection-type classification
  (regular / capped / timeseries / clustered / view / system).
- Prometheus registry (`internal/metrics`) with build and server identity
  gauges plus collector counters and latest-value gauges per scope.
- Samples and snapshots repositories under `internal/storage` with
  transactional batch writes and range queries.
- Supervisor (`internal/supervisor`) coordinating long-lived services via
  `errgroup` with clean cancellation propagation.
- Collector (`internal/collector`) with adaptive polling (idle/active),
  exponential backoff on transient errors, snapshot tagging, and event-log
  integration.
- Cluster introspection endpoints:
  `GET /api/v1/cluster/{topology,preflight,databases,databases/{db}/collections,
  databases/{db}/collections/{coll}/stats}` plus Prometheus at `/metrics`.
- Integration test suite (Docker-gated) against `mongo:7.0`.
