# API reference

All endpoints live under `/api/v1/` and return the uniform envelope:

```json
{ "data": <payload>|null, "error": null|{ "code","message","details":{} } }
```

Unauthenticated access is limited to `/health`, `/ready`,
`/api/v1/health`, `/api/v1/ready`, and `/metrics`. Every other route
requires `Authorization: Bearer <token>` when `auth.enabled: true`.

Every state-changing endpoint returns `X-Request-ID` (echoing the request
header if present, otherwise minting a UUID).

Bodies that exceed `server.max_request_bytes` (default 1 MiB) are
rejected with 413.

## Health / version / admin

| Method | Path | Notes |
|---|---|---|
| GET | `/health`, `/api/v1/health` | Liveness; 200 always |
| GET | `/ready`, `/api/v1/ready` | Pings the state store + Mongo; 503 when degraded |
| GET | `/api/v1/version` | Build identity JSON |
| POST | `/api/v1/admin/log-level` | `{"level":"debug"}` — runtime log level |

## Cluster introspection

| Method | Path | Notes |
|---|---|---|
| GET | `/api/v1/cluster/topology` | Topology + server info + redacted URI |
| GET | `/api/v1/cluster/preflight` | Storage rollup for load readiness |
| GET | `/api/v1/cluster/databases` | Array of DatabaseSummary |
| GET | `/api/v1/cluster/databases/{db}/collections` | Array of CollectionSummary |
| GET | `/api/v1/cluster/databases/{db}/collections/{coll}/stats` | Full `collStats` for one collection |

## Loader

| Method | Path | Body |
|---|---|---|
| POST | `/api/v1/loader/start` | `{ spec: { entries: [...] }, params: { workers, batch_size, docs_per_second?, force_start? }, run_id? }` |
| POST | `/api/v1/loader/{id}/pause` | — |
| POST | `/api/v1/loader/{id}/resume` | — |
| POST | `/api/v1/loader/{id}/stop` | — |
| PATCH | `/api/v1/loader/{id}/params` | Partial Params |
| GET | `/api/v1/loader/{id}` | Status + progress |
| GET | `/api/v1/loader/active` | Array of active loaders |

Conflicts (scope overlap with another active loader) return 409 with
`error.code = "start_failed"` and the conflicting id(s).

## Deleter

Preview → confirmation-token → start. Tokens are 32-byte hex, single-use,
with a 2-minute TTL (configurable).

| Method | Path | Body |
|---|---|---|
| POST | `/api/v1/deleter/preview` | `{ spec: {...}, params: { batch_size, max_ratio } }` |
| POST | `/api/v1/deleter/start` | `{ confirmation_token, run_id? }` |
| POST | `/api/v1/deleter/{id}/pause` | — |
| POST | `/api/v1/deleter/{id}/resume` | — |
| POST | `/api/v1/deleter/{id}/stop` | — |
| GET | `/api/v1/deleter/{id}` | Status + progress |
| GET | `/api/v1/deleter/active` | Array of active deleters |

Preview response carries `requires_typed_confirmation` (true when
ratio > 0.5 or > 5 collections) and `max_ratio_breached`. The UI forces
typed `DELETE` confirmation in the first case and blocks execution in the
second.

## Compact

| Method | Path | Body |
|---|---|---|
| POST | `/api/v1/compact/preview` | `{ scope: {kind, ...}, params: {...} }` |
| POST | `/api/v1/compact/start` | `{ scope, params, run_id? }` |
| POST | `/api/v1/compact/{id}/cancel` | — |
| GET | `/api/v1/compact/{id}` | Status + `current_step` (e.g. `node-2/poc_db_1.coll_a`) |
| GET | `/api/v1/compact/active` | Array |

Scope kinds: `cluster`, `databases`, `collections`.

## Workload

| Method | Path | Body |
|---|---|---|
| POST | `/api/v1/workload/start` | `{ spec: { targets, read_weight, write_weight, aggregate_weight }, params: { target_ops_per_sec, workers } }` |
| POST | `/api/v1/workload/{id}/stop` | — |
| PATCH | `/api/v1/workload/{id}/rate` | Live rate + workers adjust |
| GET | `/api/v1/workload/{id}` | Status + ops_done + errors |
| GET | `/api/v1/workload/active` | Array |

## Snapshots

| Method | Path | Body |
|---|---|---|
| POST | `/api/v1/snapshots/` | `{ label, note?, run_id? }` |
| GET | `/api/v1/snapshots/?label=&limit=` | Array |
| GET | `/api/v1/snapshots/{id}` | Full snapshot |
| GET | `/api/v1/snapshots/compare?a=&b=` | Per-database + per-collection diff |

## Runs / events / reports

| Method | Path | Notes |
|---|---|---|
| POST | `/api/v1/runs/` | Create a run |
| GET | `/api/v1/runs/` | List runs |
| GET | `/api/v1/runs/{id}` | Run detail |
| POST | `/api/v1/runs/{id}/cancel` | — |
| GET | `/api/v1/runs/{id}/snapshots` | Run-scoped snapshots |
| GET | `/api/v1/runs/{id}/metrics` | Time-series query |
| GET | `/api/v1/runs/{id}/report` | Structured JSON report |
| GET | `/api/v1/runs/{id}/report.csv` | CSV per-collection reclaim |
| GET | `/api/v1/snapshots/compare/report?a=&b=` | Pair-based report (JSON) |
| GET | `/api/v1/snapshots/compare/report.csv` | …same as CSV |
| GET | `/api/v1/events/?category=&run_id=&limit=` | Event / audit log |

## Streams

| Method | Path | Notes |
|---|---|---|
| GET | `/api/v1/stream/metrics` | WebSocket; frames: `{type,timestamp,payload}`. `type=collector_tick` after each sampling pass. |
| GET | `/api/v1/stream/operations` | WebSocket; frames for every service lifecycle transition. |

Both streams send a 30s ping so proxies don't idle the socket closed. Slow
clients are dropped silently rather than blocking the hub.

## Prometheus

`GET /metrics` exposes the app's dedicated registry:

- `mfpoc_info{version,commit,go_version}`
- `mfpoc_mongo_server_info{version,storage_engine,compressor,is_srv}`
- `mfpoc_collector_ticks_total{outcome}`
- `mfpoc_collector_tick_duration_seconds`
- `mfpoc_collector_fragmentation_ratio{scope,scope_id}`
- `mfpoc_collector_storage_bytes{scope,scope_id}`
- `mfpoc_collector_free_storage_bytes{scope,scope_id}`
- `mfpoc_loader_{docs_inserted_total,bytes_inserted_total,insert_duration_seconds,errors_total,active_workers}`
- `mfpoc_deleter_{docs_deleted_total,batch_duration_seconds,errors_total}`
- `mfpoc_compact_{started_total,completed_total,duration_seconds,reclaim_bytes_total,stepdowns_total}`
- `mfpoc_workload_{op_duration_seconds,ops_total,errors_total}`

Spec § 13's OpenAPI export is on the backlog.
