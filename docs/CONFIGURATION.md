# Configuration

Canonical source: [`configs/config.example.yaml`](../configs/config.example.yaml).
Every YAML key is overridable through an `MFPOC_`-prefixed environment
variable; nested keys use underscores (`logging.level` →
`MFPOC_LOGGING_LEVEL`). Secret-bearing strings accept `${env:VAR}` and
`${file:/path}` resolution.

## Sections

### `server`

| Key | Default | Notes |
|---|---|---|
| `listen` | `0.0.0.0:8080` | Public binds without TLS are refused unless `insecure_allow_http_on_public=true`. |
| `max_request_bytes` | `1048576` | 1 MiB. Enforced via `MaxBytesReader`. |
| `read_header_timeout` | `10s` | |
| `read_timeout` | `30s` | |
| `write_timeout` | `60s` | |
| `idle_timeout` | `120s` | |
| `shutdown_timeout` | `30s` | Graceful drain on SIGINT/SIGTERM. |
| `rate_limit_per_minute` | `60` | Per-IP, write methods only. |
| `rate_limit_failed_auth_per_minute` | `10` | Per-IP backoff for bad tokens. |
| `insecure_allow_http_on_public` | `false` | Escape hatch for HTTP public binds. |
| `tls.enabled` | `false` | Pick one of `cert_file+key_file`, `self_signed`, or `acme.enabled`. |
| `tls.cert_file`, `tls.key_file` | — | PEM paths. |
| `tls.self_signed` | `false` | Generated on first run into `tls.data_dir`. |
| `tls.data_dir` | `./data/tls` | |
| `tls.acme.enabled` | `false` | ACME autocert; requires a DNS name. |
| `tls.acme.domain` | — | |

### `mongo`

| Key | Default | Notes |
|---|---|---|
| `uri` | — | `mongodb://` or `mongodb+srv://`. Wins over individual parts when both set. |
| `host`, `port`, `username`, `password`, `replica_set`, `auth_source`, `tls` | — | Composed into a URI if `uri` is absent. |
| `connect_timeout` | `10s` | Automatically promoted to `30s` when an SRV URI is detected. |
| `operation_timeout` | `30s` | |
| `max_pool_size` | `100` | |
| `app_name` | `mfpoc` | |

### `storage`

| Key | Default | Notes |
|---|---|---|
| `path` | `./data/mfpoc.db` | SQLite file. |
| `busy_timeout` | `5s` | |
| `metrics_retention_days` | `30` | Per-collection/database retention window. |
| `cluster_metrics_retention_days` | `365` | Cluster-scope samples kept longer. |
| `janitor_interval` | `24h` | (Janitor wiring is a Phase-20 backlog item; column defined for forward compatibility.) |

### `logging`

| Key | Default | Notes |
|---|---|---|
| `level` | `info` | `trace\|debug\|info\|warn\|error`. Changeable at runtime via `POST /api/v1/admin/log-level`. |
| `format` | `json` | `json\|console`. |

### `auth`

| Key | Default | Notes |
|---|---|---|
| `enabled` | `true` | Off in `config.dev.yaml`. |
| `bearer_token` | — | ≥32 bytes required when `enabled`. Supports `${env:...}` and `${file:...}`. |
| `basic_user`, `basic_pass` | — | Alternative to bearer; mutually exclusive with token-only flows. |

### `loader`

| Key | Default | Notes |
|---|---|---|
| `default_workers` | `0` | `0` → `2 × NumCPU()` at runtime. |
| `default_batch_size` | `1000` | |
| `default_write_concern` | `"1"` | `"1"`, `"majority"`. |
| `storage_headroom_percent` | `30.0` | Preflight refuses a load when `free < required × (1 + headroom/100)`. |

### `deleter`

| Key | Default | Notes |
|---|---|---|
| `default_batch_size` | `1000` | |
| `preview_ttl` | `2m` | Confirmation-token lifetime. |
| `max_ratio` | `0.95` | Enforced at preview time; presets ≥0.5 force typed confirmation in UI. |
| `inter_batch_jitter` | `25ms` | Between-delete pause to avoid saturation. |

### `collector`

| Key | Default | Notes |
|---|---|---|
| `idle_interval` | `10s` | Base poll cadence when no ops active. |
| `active_interval` | `2s` | Poll cadence while loader/deleter/compact are running. |
| `backoff_initial` / `backoff_max` | `1s` / `30s` | Exponential transient-error backoff. |

### `compact`

| Key | Default | Notes |
|---|---|---|
| `max_replication_lag` | `10s` | Warning threshold in preview; does not auto-abort. |
| `stepdown_wait_timeout` | `60s` | Budget for a new primary to be elected. |
| `validate_after_compact` | `false` | Opt-in; runs `validate` per compacted collection. |

### `workload`

| Key | Default | Notes |
|---|---|---|
| `default_target_ops_per_sec` | `100` | |
| `default_read_weight` / `default_write_weight` / `default_aggregate_weight` | `0.7 / 0.2 / 0.1` | |

### `poc`

| Key | Default | Notes |
|---|---|---|
| `database_prefix` | `poc_db_` | Used by forthcoming `mfpoc purge` to refuse non-POC DBs. |

## Secret references

```yaml
auth:
  bearer_token: "${env:MFPOC_AUTH_BEARER_TOKEN}"
mongo:
  uri: "${file:/var/run/secrets/mongo/uri}"
```

Resolution happens once at startup; changing the env var after boot has
no effect. Secret-bearing fields are redacted in startup logs.
