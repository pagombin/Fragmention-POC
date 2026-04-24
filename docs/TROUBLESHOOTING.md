# Troubleshooting

## Server won't start

| Symptom | Cause | Fix |
|---|---|---|
| `config invalid: auth.bearer_token must be at least 32 bytes` | Token unset or too short. | `export MFPOC_AUTH_BEARER_TOKEN=$(openssl rand -hex 32)` or disable auth in dev. |
| `server bound to a public address without TLS` | Public bind without TLS or the explicit override. | Enable `server.tls` or set `server.insecure_allow_http_on_public: true` (dev only). |
| `mongo: server version X.Y is below minimum 5.0` | Target cluster is too old. | Upgrade the cluster; the app is version-gated per spec § 21.1. |
| `mongo connect failed; continuing in degraded mode` | Bad URI / network / credentials. | Run `./mfpoc config-validate` to see the redacted URI, then `mongosh "$MFPOC_MONGO_URI"` from the same host. |

## Dashboard shows "disconnected"

- Top-bar token input empty or wrong — paste the token from `mfpoc.env`.
- `/ready` returns 503 — see server logs: `journalctl -u mfpoc -n 100`.
- SRV DNS not resolving from the droplet — `nslookup -type=SRV _mongodb._tcp.<host>`.

## Loader immediately fails with "preflight unsafe"

Free disk is less than `target × (1 + headroom)`. Options: set
`force_start: true` in the request, free space, or lower the load
target. Headroom defaults to 30%.

## Delete preview shows `max_ratio_breached`

Ratio or server-side match count exceeds `deleter.max_ratio`. Lower the
ratio in the form or, for a TTL/range pattern, move the cutoff forward.

## Compact immediately returns "no healthy secondary available"

Rolling compact requires at least one `SECONDARY` that isn't unhealthy.
Run `rs.status()` on the target cluster, fix the broken member, retry.

## "target scope overlaps with active loader/deleter" 409

Another operation is already running against a collection in your scope
(spec § 21.3). Either wait for it to finish (the Live Ops Panel shows
all active ops) or pick a different scope.

## Self-signed cert warning in the browser

Expected on a droplet without DNS. Accept the browser exception or
install the self-signed cert into your local trust store. To switch to
ACME: point a DNS name at the droplet, edit
`/etc/mfpoc/config.yaml`'s `server.tls.acme` block, restart.

## WebSocket streams stop updating

The hub drops frames for clients that can't keep up (spec § 5.6). If the
stream feels stale, reload the page — a new subscription starts cleanly
and the fan-out catches up from the next tick.

## Tests

- `make test` fails with `missing go.sum entry` — run `go mod tidy`.
- `make test-integration` times out — Docker isn't running or the
  container pull is slow. Confirm with `docker info`.
- `make test-e2e` fails with "rootless Docker not found" — testcontainers
  couldn't find a daemon. Set `DOCKER_HOST` or enable rootless Docker.

## Where to look

| Question | Check |
|---|---|
| What did the app do at T? | `journalctl -u mfpoc --since '-1h'` or `GET /api/v1/events` |
| What metrics did it capture? | `GET /api/v1/runs/{id}/metrics?...` or `/metrics` (Prometheus) |
| Was an operation resumed after restart? | `GET /api/v1/loader/{id}` → check `state = interrupted` |
| Is the SQLite file growing unbounded? | `du -sh /var/lib/mfpoc` — retention is enforced by the janitor (24h cadence) |
