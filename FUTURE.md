# Future work

Items noted during the POC build that are **not** in the spec but would
sharpen the platform. Kept here per spec § 5 ("If you think something is
missing, note it in FUTURE.md and keep building").

## Backlog

- **Retention janitor goroutine.** `storage.metrics_retention_days` and
  `cluster_metrics_retention_days` are honored by the schema but the
  nightly sweep+VACUUM hasn't been wired. See LIMITATIONS.md.
- **`mfpoc purge` UI + CLI.** Safe drop of POC-prefixed databases with
  typed confirmation. CLI stub present but errors out.
- **OpenAPI 3.0 export.** Spec § 15 asks for one. API.md covers the
  same ground by hand.
- **Update-mix phase for loader.** Spec § 5.1 notes this as optional.
  `$set`/`$push` traffic is currently available only via the Workload
  generator.
- **Per-operation write concern on loader.** The current
  implementation honors the client's default write concern and accepts
  a per-request WC value in the Params but does not thread it all the
  way through to per-InsertMany options.
- **PreviewResult caching with cross-session durability.** Today the
  preview token cache lives in process memory; a server restart
  invalidates outstanding tokens. This is arguably the right behavior
  (force re-preview after a restart) but should be documented.
- **Auto-snapshot on initial-sync auto-detect.** The collector detects
  `STARTUP2` per spec § 5.3; taking the auto pre-snapshot is wired but
  the dashboard banner for "Initial sync detected on member X" has not
  been surfaced yet beyond the events log.
- **Per-index sampling.** `collStats.indexSizes` is captured in the
  snapshot payload but not yet persisted as `ScopeIndex` samples; the
  collector code path is present in `internal/mongo/stats.IndexSizes`
  but not called from the tick loop.
- **WebSocket streams in the dashboard.** Hubs publish reliably, but
  the React app currently polls via TanStack Query (2s cadence for
  active operations). The hooks are structured so a WebSocket
  subscription can replace `refetchInterval` without reshaping
  consumers.
- **Adaptive rate limiting on loader.** Spec § 20 hints at slowing
  down when the cluster hits 90%+ cache pressure. `serverStatus`
  signals are captured but the feedback loop is not yet implemented.
- **Sharded cluster support.** Out of scope per spec § 18; leaving the
  `Topology.Sharded` flag so a future phase can opt in.
- **E2E test for compact rolling across a 3-node RS.** Current e2e
  covers standalone only because testcontainers doesn't cleanly
  bootstrap a 3-node set in under a minute.

## Deliberate omissions

- Mobile / responsive-first styling of the dashboard. The spec explicitly
  deprioritises this.
- OAuth / OIDC auth providers. Spec § 18 out of scope.
- Multi-tenant authorisation. One operator only.
