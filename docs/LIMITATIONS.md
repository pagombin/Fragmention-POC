# Known limitations

## MongoDB behaviors we surface but can't change

- **`compact` reclaim is bounded by relocatable blocks.** Reclaim less
  than `freeStorageSize` is expected, not a bug. Reports always show
  `storage_before → storage_after` so you see the real answer, not an
  optimistic projection.
- **`freeStorageSize` is an estimate.** Treat ±5% swings across ticks as
  noise; the collector captures the raw number so you can decide.
- **Filesystem lag.** `fsUsedSize`/`fsTotalSize` may not reflect recent
  block reclaim until the OS commits page decommits.
- **Initial sync throughput** depends on sync-source load and network
  paths; the POC cannot control either.
- **Single-node topologies** do not support initial sync. The Initial
  Sync Companion view refuses to start on a standalone.
- **MongoDB < 5.0** is refused at connect time. `compact` semantics are
  too different.

## Out of scope for this POC (spec § 18)

- Sharded clusters. The topology detector flags `mongos` targets and
  the compact orchestrator refuses to run.
- Initial-sync orchestration on `mongod` hosts. The operator drives the
  actual sync externally; the app captures pre/post snapshots only.
- Multi-tenancy of the POC app itself. One operator, one workspace.
- External auth providers (OAuth, OIDC). Static bearer token only.
- Alerting / paging integrations. Events are persisted; operators
  consume them via `/api/v1/events` or the Event Log view.
- Mobile UI. The dashboard is desktop-targeted.

## Known limitations of the current implementation

- **Retention janitor not yet wired.** The config keys are honored by
  the schema (see `storage.metrics_retention_days` /
  `cluster_metrics_retention_days`) but the nightly janitor goroutine
  is a Phase-20 backlog item. Manual SQLite `VACUUM` works.
- **OpenAPI export** (spec § 15) is not yet generated. The reference
  lives in [`API.md`](API.md).
- **`mfpoc purge`** exposes an error-returning CLI stub; the actual
  purge UI is planned for a future phase.
- **Storage-size estimation during load preflight** conservatively
  compares against the raw target bytes. Compression typically shrinks
  on-disk size by 2–3×, so the check errs on the safe side; use
  `force_start: true` when you know better.
- **Update-mix phase for loader** (spec § 5.1 optional) is not yet
  implemented. `$set`/`$push` update traffic is currently only
  available via the Workload generator.
- **Secondary-targeted compact** is not directly supported by the v2
  driver's `RunCommand` for write-like operations; the rolling
  orchestrator runs compacts against the current primary and uses
  stepdowns to rotate. The visible effect per member is identical for
  non-sharded replica sets.

See also [FUTURE.md](../FUTURE.md) for items that the spec does not
require but would sharpen the POC.
