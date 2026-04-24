# Glossary

_Populated incrementally._

- **Fragmentation ratio** — `freeStorageSize / storageSize` at the scope
  being measured (collection, database, or cluster).
- **Storage size** — compressed, on-disk bytes WiredTiger has reserved for
  the collection, including free blocks not yet reclaimed.
- **Free storage size** — bytes inside a collection's storage envelope that
  WiredTiger has marked reusable but not yet relinquished to the filesystem.
- **Logical size** (aka `size`) — sum of BSON-encoded documents, before
  compression.
- **Compact** — MongoDB command that rewrites a collection and its indexes
  in place, returning free blocks to the filesystem.
- **Initial sync** — replica-set operation that rebuilds a member's data
  files from scratch by copying from another member.
- **Rolling operation** — performed one member at a time to preserve
  availability.
- **Stepdown** — voluntary demotion of the primary so another member can be
  elected.
- **WiredTiger** — MongoDB's default storage engine.
- **Snappy / zstd** — compression codecs WiredTiger supports.
- **Oplog** — `local.oplog.rs`, the replication log that bounds how far a
  secondary can lag.
