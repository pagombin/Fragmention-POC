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
- Operations repository (`internal/storage/operations.go`) with the full
  spec § 5.8 lifecycle state machine, atomic progress upsert, and
  startup orphan recovery (§ 21.2).
- Six-template synthetic generator (`internal/generator`) with a weighted
  registry, per-template `IndexSpecs` and `IDKind`, and seeded determinism.
- Loader service (`internal/loader`) implementing spec § 5.1: worker-pool
  insertion with InsertMany+ordered:false, live parameter adjustment,
  pause/resume/stop lifecycle, rate limiting via `x/time/rate`, pre-flight
  storage check with configurable headroom, resume-from-persisted-progress,
  Prometheus metrics (`loader_docs_inserted_total`,
  `loader_bytes_inserted_total`, `loader_insert_duration_seconds`,
  `loader_errors_total`, `loader_active_workers`), event-log integration,
  and a Service manager that enforces scope-overlap conflict detection
  (§ 21.3).
- Startup orphan recovery automatically transitions interrupted operations
  to `interrupted` so the UI can offer resume/abort/retry (§ 21.2).
- Deleter service (`internal/deleter`) implementing spec § 5.2:
  - Five patterns: `random_by_id`, `range_by_field`, `modulo`,
    `ttl_simulated`, `prefix_by_id`; each expressed via a `Pattern`
    interface with `BuildFilter`, `EstimateMatchCount`, and
    `CaptureCandidates` for pause/resume stability.
  - Mandatory preview → confirmation-token → execute flow. Tokens are
    32-byte hex, single-use, TTL-bounded.
  - Persisted candidate sets in `delete_candidates` (gob-encoded) so
    sampling patterns resume from the identical set after a restart.
  - Per-collection and aggregate progress tracked in
    `operation_progress`; completion auto-tags a `post_delete` snapshot
    when a collector is wired in.
  - Scope-overlap conflict detection shared with the loader (§ 21.3).
  - Prometheus metrics: `deleter_docs_deleted_total`,
    `deleter_batch_duration_seconds`, `deleter_errors_total`.
- Compact orchestrator (`internal/compact`) implementing spec § 5.4:
  - Detects topology and chooses single-node or rolling mode.
  - Rolling: secondaries first, then `replSetStepDown` + wait-for-new-
    primary + compact former primary.
  - Preview surfaces execution order, warnings (excess lag, missing
    secondary), and estimated duration.
  - Scope levels: cluster / databases / collections. System databases
    (admin/config/local) are never touched; only `regular` collections
    are compacted (capped/timeseries/view/system are excluded).
  - Optional post-compact `validate` when configured.
  - Auto-snapshots `pre_compact` and `post_compact` via the collector.
  - Prometheus metrics: `compact_started_total`, `compact_completed_total`
    (labelled by outcome), `compact_duration_seconds`,
    `compact_reclaim_bytes_total`, `compact_stepdowns_total`.
- Workload generator (`internal/workload`) implementing spec § 5.5:
  - Weighted read/write/aggregate mix, configurable per run.
  - Live-adjustable rate via `x/time/rate` token bucket; SetParams
    rebuilds the limiter atomically.
  - Prometheus histograms (`workload_op_duration_seconds`), counters
    (`workload_ops_total`, `workload_errors_total`) with bounded
    cardinality labels.
  - Service manager for Start/Stop/Get/ListActive.
- Full REST API surface under `/api/v1/`:
  - `loader/` — start, pause, resume, stop, adjust (PATCH params),
    get, list active.
  - `deleter/` — preview → start (token), pause, resume, stop, get,
    list active.
  - `compact/` — preview (planned execution order), start, cancel,
    get (with current step), list active.
  - `workload/` — start, stop, live rate adjust (PATCH), get, list.
  - `snapshots/` — create, list, get, compare (a vs b diff).
  - `runs/` — create, list, get, cancel, run-scoped snapshots,
    run-scoped metrics query.
  - `events/` — list with category + run_id filters.
- Runs repository (`internal/storage/runs.go`) with lifecycle status
  transitions (created → running → completed/cancelled/failed).
