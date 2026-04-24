# Glossary

Terms as the UI and reports use them. When a term is overloaded in the
wider MongoDB ecosystem, the definition here reflects how mfpoc
measures and displays it.

**Baseline snapshot** — the earliest snapshot associated with a run, or
the one labelled `baseline`, used as the reference point for reports.

**Capped collection** — fixed-size collection that overwrites oldest docs.
`compact` is a no-op on capped collections; the collector tags them
separately so reports don't conflate them with regular collections.

**Cluster fragmentation** —
`(Σ storageSize − Σ dataSize) / Σ storageSize` across `dbStats`. The
collector reconciles this against the sum of per-collection ratios.

**Collection type** — one of `regular`, `capped`, `timeseries`,
`clustered`, `view`, `system`. Only `regular` collections participate
in loader/deleter/compact.

**Compact** — MongoDB command that rewrites a collection and its indexes
in place, returning free blocks to the filesystem. Partial reclaim is
normal (see LIMITATIONS).

**Confirmation token** — 32-byte hex string returned by a delete
preview. Single-use, 2-minute TTL. Required to execute the delete.

**Degraded mode** — app startup state when Mongo is unreachable;
`/health`, `/version`, and `/metrics` still answer but `/ready` is 503.

**Fragmentation ratio** — `freeStorageSize / storageSize` at the scope
being measured. Reports expose the raw numerator and denominator too so
operators can reason in absolute bytes.

**Free storage size** — bytes inside a collection's WiredTiger storage
envelope that are marked reusable but not yet released to the
filesystem.

**Initial sync** — replica-set operation that rebuilds a member's data
files from scratch from another member.

**Live Ops Panel** — right-hand sidebar showing every in-flight
operation across loader/deleter/compact/workload with inline lifecycle
buttons.

**Operation** — one row in the `operations` table. Every load, delete,
compact, or workload invocation produces exactly one operation,
regardless of whether it was launched ad-hoc (Console mode) or as part
of a run (Run mode).

**Orphan operation** — an operation row whose process died mid-flight.
At startup, such rows are transitioned to `interrupted` and surfaced in
the UI for resume/abort/retry.

**Preview** — a dry-run of a destructive action that returns exact
impact (counts, collections) and a confirmation token.

**Run** — a structured experiment (name, config, notes) that groups
related operations and snapshots. Optional; ad-hoc Console mode
operations work without one.

**Scope** — one of `cluster`, `database`, `collection`, `member`,
`index` as stored in `metrics_samples.scope`.

**Snapshot** — point-in-time capture of cluster storage stats, labelled
for humans. `baseline`, `post_load`, `post_delete`, `pre_compact`,
`post_compact`, `pre_initial_sync`, `post_initial_sync` are recognised;
any other label is also supported for ad-hoc flows.

**Stepdown** — voluntary demotion of the primary so another member can
be elected. Used by the rolling compact orchestrator between members.

**Storage engine** — WiredTiger for all supported versions.

**Storage size** — compressed on-disk bytes WiredTiger has reserved for
a collection, including free blocks awaiting reclaim.

**Target spec** — the `{ entries: [{database, collection, ...}] }`
payload consumed by loader and deleter APIs. The dashboard builds it
from the shared selection store.

**Time-series collection** — MongoDB's native time-series storage
model. Compact semantics differ from regular collections; tagged
separately in reports.

**Topology** — `standalone`, `replica_set`, or `unknown` (e.g. a
`mongos` target, which the app flags and refuses to drive).

**WiredTiger compressor** — block-level codec (`snappy`, `zlib`,
`zstd`, `none`). Exposed per collection; changes the interpretation of
storage_size materially.
