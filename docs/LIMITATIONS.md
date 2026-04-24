# Known limitations

This document tracks limitations of `mfpoc` and of the underlying MongoDB
behaviors the POC exercises. It is updated per-phase as items are confirmed.

## MongoDB behaviors

- `compact` does not always reclaim 100% of fragmented space; some blocks
  cannot be relocated. See METHODOLOGY.md for interpretation guidance.
- `freeStorageSize` from `collStats` is an estimate, not a guarantee.
- Filesystem-level space may lag behind `collStats` due to OS caching.
- Initial-sync outcomes depend heavily on sync-source load and network.
- Single-node topology does not support initial sync; the Initial Sync
  Companion view is disabled on single-node deployments.

## Out of scope (by spec)

- Sharded cluster support.
- Initial-sync orchestration — handled externally by the operator.
- Multi-tenancy of the POC app itself.
- External auth providers (OAuth, OIDC).
- Alerting / paging integrations.
- Mobile UI.
