# POC methodology

This document is the interpretation guide for the POC: how to design a
valid run, what the numbers mean, and why `compact` vs. initial-sync
comparisons are not apples-to-apples without careful setup.

## Designing a valid run

1. **Start from a known baseline.** Use the New Run Wizard or
   `POST /api/v1/snapshots/` with `label: "baseline"` on an empty (or
   newly-purged) cluster. Every report computes reclaim against this
   reference point.
2. **Load with realistic variety.** The default 6-template mix spans
   ~300 B telemetry docs through ~200 KB blobs. Weights favor small
   high-volume docs so collection counts look real, but blobs still
   land so WiredTiger large-page behavior is exercised.
3. **Pre-fragment deliberately.** Compact only reclaims blocks the
   storage engine has already marked free. A load-then-delete run
   creates a predictable hole pattern; a load-then-update run (which
   this POC does not exercise directly yet — see FUTURE.md) would
   produce different page-level fragmentation.
4. **Pick a pattern that matches your goal.**

   | Pattern | Use when |
   |---|---|
   | `random_by_id` | You want uniform fragmentation the compactor should reclaim cleanly. |
   | `modulo` | You want maximal fragmentation — every Nth doc deleted produces small, evenly-spaced holes that defy compaction best. |
   | `range_by_field` | Realistic "old rows expired" cleanup; tends to produce contiguous holes near one end of the B-tree. |
   | `ttl_simulated` | Same as range, specialised on `created_at`. |
   | `prefix_by_id` | Mimics partition purge on UUID _ids. |
5. **Snapshot at every boundary.** The built-in auto-snapshots
   (`post_delete`, `pre_compact`, `post_compact`) are sufficient for
   straight-line runs. The External Initial Sync flow uses
   `pre_initial_sync` / `post_initial_sync`.

## Compact vs. initial sync

Both strategies reclaim space, but they optimise different things.

|  | compact | initial sync |
|---|---|---|
| Target | single collection | whole member |
| Reclaim ceiling | ≤ `freeStorageSize` (some blocks are unrelocatable) | ≈ `storageSize - dataSize` of every collection |
| Availability cost | collection locked briefly per compact | the syncing member is out of rotation |
| Time | seconds to minutes per collection | minutes to hours per TB |
| Replication impact | none (single-member compact, serialised by the orchestrator) | pressures oplog + sync-source network |

Rules of thumb:

- **Prefer compact** when fragmentation is <30% and concentrated in a
  handful of hot collections.
- **Prefer initial sync** when fragmentation is high cluster-wide, or
  when you need the floor-of-`dataSize` storage cost and can afford the
  sync window.

## Why the same seed can produce different absolute results

- **Snappy vs. zstd.** Compressed on-disk size changes materially with
  the codec. The app surfaces `compressor` per collection; compare like
  with like.
- **ObjectID vs. UUID _ids.** ObjectIDs concentrate writes on the B-tree
  right edge, producing different block layouts than UUIDs' uniform
  spread. The `document_blob` template uses UUIDs by default; the rest
  use ObjectIDs. This is a deliberate asymmetry that mirrors real
  workloads.
- **Index vs. data fragmentation.** Compact reclaims both, but the
  ratios differ. The collector samples per-collection `totalIndexSize`
  so reports can separate the two.

## Interpreting fragmentation ratio

`fragmentation_ratio = freeStorageSize / storageSize` at the scope being
measured. Reports also expose the raw numerator and denominator so you
can reason about absolute reclaim rather than just percentages. The
cluster rollup is `(Σ storage - Σ data) / Σ storage` from `dbStats`; the
collector logs a discrepancy when the cluster-level and
sum-of-collection-level numbers disagree.

## Why compact sometimes reclaims less than the preview suggested

WiredTiger's compact only relocates blocks it can prove are safe to
move. Reclaim less than `freeStorageSize` is normal. Report consumers
should always compare `storage_before` to `storage_after`, not the
projected reclaim from a pre-compact snapshot.

## Known measurement caveats (see also LIMITATIONS.md)

- `freeStorageSize` is an estimate. Treat ±5% swings as noise.
- `fsUsedSize` / `fsTotalSize` lag behind collection-level numbers
  because the OS may not have relinquished pages yet.
- Single-node topologies produce clean but unrealistic results; always
  validate with the 3-node replica set for customer-facing claims.

## Reproducibility

Set a `Params.seed` on the loader to produce (nearly) identical
documents run-to-run. Timestamps and ObjectIDs are inherently
non-deterministic, so two runs with the same seed will not byte-match,
but their distributional properties will be nearly identical.
