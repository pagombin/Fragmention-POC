# Architecture

## Overview

`mfpoc` is a single Go binary that drives a target MongoDB cluster through a
controlled fragmentation → reclaim cycle and records enterprise-grade
comparison data. It embeds a React SPA and serves both the API and the
dashboard from one process on a single DigitalOcean droplet.

```
             ┌──────────────────────────── mfpoc (single binary) ────────────────────────────┐
             │                                                                               │
             │   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌───────────────┐  │
             │   │ Loader   │  │ Deleter  │  │ Compact  │  │ Workload │  │   Collector   │  │
             │   │  svc     │  │   svc    │  │  orch.   │  │   gen    │  │ (time series) │  │
             │   └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘  └───────┬───────┘  │
             │        │             │             │             │                │          │
             │        └─────────────┴─────┬───────┴─────────────┴────────────────┘          │
             │                            ▼                                                  │
             │                   ┌──────────────────┐                                        │
             │                   │  Supervisor +    │                                        │
             │                   │  State store     │─────── SQLite (WAL, modernc)           │
             │                   │  (SQLite)        │                                        │
             │                   └────────┬─────────┘                                        │
             │                            ▼                                                  │
             │                ┌────────────────────────┐                                     │
             │                │ HTTP/WebSocket + SPA   │─ embedded via go:embed              │
             │                └───────────┬────────────┘                                     │
             └────────────────────────────┼──────────────────────────────────────────────────┘
                                          ▼
                            Target MongoDB (single-node or 3-node RS,
                                mongodb:// or mongodb+srv://)
```

## Services

| Service            | Responsibility                                                             |
|--------------------|----------------------------------------------------------------------------|
| Loader             | Generate documents from 6+ templates, insert at bounded throughput         |
| Deleter            | Apply fragmentation patterns (preview → confirm → execute)                 |
| Compact orchestrator | Rolling `compact` across RS members with safety gates                    |
| Workload generator | Drive concurrent traffic during reclaim to measure user-visible impact    |
| Collector          | Sample `collStats`/`dbStats`/`rs.status()` on an adaptive cadence          |
| Supervisor         | Lifecycle, graceful shutdown, orphan recovery                              |

All services communicate through typed Go channels and the shared state store.

## Interaction model

Two coexisting modes, both first-class:

- **Run mode** — structured experiments from the New Run Wizard (load → delete
  → compact phases scripted for clean comparison reports).
- **Console mode** — ad-hoc operational control. Any operation can be started
  at any time against any scope.

Both write to the same `operations` table; ad-hoc ops are recorded with a
`NULL run_id` but remain fully auditable.

## State store (SQLite)

Per § 5.8 of the spec, the schema comprises:

- `runs` — structured experiment metadata.
- `snapshots` — tagged point-in-time readings. `run_id` is nullable.
- `metrics_samples` — time-series readings, indexed on
  `(run_id, metric_name, timestamp)` and `(timestamp)`.
- `operations` — every execution (loader, deleter, compact, workload), with
  lifecycle state and target scope.
- `operation_progress` — per-collection progress so pause/resume/recover work.
- `delete_candidates` — persisted candidate-ID blobs for patterns that need a
  stable pause/resume set (e.g., `random_by_id`).
- `events` — audit + application event log.

WAL mode, `synchronous=NORMAL`, 20 MB cache. Writes are serialized by keeping
the `max_open_conns` small and routing single-writer workloads through a
mutex-protected path where the repositories demand it.

Migrations live under `internal/storage/migrations/` and are embedded via
`go:embed`. This is a **Phase-1 deviation** from the spec's suggested
top-level `migrations/` directory: `go:embed` cannot reach outside the
package's subtree and we prefer a single canonical source of truth over a
symlink.

## Transport and auth

- HTTP/1.1 by default; TLS via explicit cert+key, self-signed (generated on
  first run if enabled), or ACME / Let's Encrypt.
- Bearer token mandatory when bound to a non-loopback address without TLS —
  the server refuses to start otherwise unless
  `server.insecure_allow_http_on_public` is set.
- Auth failures are rate-limited per IP.
- Writes are per-IP rate-limited.

## Decisions

_Decisions that deviated from, went beyond, or resolved ambiguity in the
spec. Keep this list append-only so operators can audit later._

### D-001: Module path is `github.com/pagombin/fragmention-poc`

The spec suggests `mongodb-frag-poc` as a project name and `mfpoc` as the
binary name. The git remote is `pagombin/fragmention-poc`; we use that as
the module path. The binary remains `mfpoc` per spec.

### D-002: Migrations embedded inside `internal/storage/`

`go:embed` can only reference files inside the embedding package's subtree.
We kept the canonical migration files at `internal/storage/migrations/`
instead of the spec's suggested top-level `migrations/` directory to avoid
having two copies or a symlink. Operators still receive a single-binary
deployment where migrations are baked in.

### D-003: Self-signed cert on first run writes to `server.tls.data_dir`

Spec says "written to `./data/tls/` and reused on restarts." We make the
directory configurable (default `./data/tls`) so the droplet install can
point it at `/var/lib/mfpoc/tls` without source edits.

### D-004: `ineffassign`/`errcheck` elevated over spec baseline

The spec lists `govet`, `staticcheck`, `errcheck`, `gosec`, `revive`,
`unused`, `gocyclo`, `misspell`, `ineffassign`. We added nothing; `gosec`
excludes `G104` (audited via `errcheck`), `G304` (intentional file-path
variables for config), and `G404` (math/rand is fine for synthetic data).

### D-005: Write rate limit default is 60/min, auth failures 10/min

Spec default is "60 req/min per IP" for writes; we track failed auth
attempts separately with a lower threshold (10/min) to deter brute-force
without penalizing legitimate retry bursts.

### D-006: `mongo.uri` wins over parts; a warning is logged

When both `mongo.uri` and `mongo.host` are supplied, the URI wins. This is
the spec-mandated behavior; we surface a startup INFO log so operators see
the resolution.

### D-007: SRV URIs get a 30s connect timeout by default

Per spec §6. If the operator explicitly sets `mongo.connect_timeout`
below or above 10s we leave it alone; only the default is bumped.

## Roadmap

The 20 development phases from § 19 of the spec drive implementation order.
The current phase is marked in the project's commit history (look for the
latest `feat:` or `chore:` conventional-commit trailer).
