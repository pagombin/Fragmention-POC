# Operations runbook

## Local dev

```bash
make build                                  # builds frontend + binary
./bin/mfpoc config-validate --config configs/config.dev.yaml
./bin/mfpoc server --config configs/config.dev.yaml
```

Open <http://localhost:8080>. `config.dev.yaml` disables auth.

## docker-compose dev stack

```bash
make docker-up      # 3-node RS + app on :8080
make docker-down    # wipe volumes and stop
```

Browse <http://localhost:8080>. Bearer token is off in the compose config.

## Droplet

See [`deploy/droplet/README.md`](../deploy/droplet/README.md) for the
step-by-step walkthrough. Short form:

```bash
export DROPLET_IP=203.0.113.10
export DROPLET_USER=root
make deploy-droplet
```

## Common tasks

### Run a full load → delete → compact cycle (Run mode)

1. `/wizard` → fill wizard → Create run. Note the `run_id`.
2. `/ops?tab=load` → select collections → Start load (pass `run_id` via
   the wizard flow if you want the progress to correlate).
3. Wait for loader to reach `completed`.
4. `/ops?tab=delete` → pick pattern → Preview → Confirm.
5. `/ops?tab=compact` → Preview plan → Start compact.
6. `/runs/{id}` → review charts and the report.

### Run the same operations ad-hoc (Console mode)

1. `/data` → multi-select collections → "Load into selected" /
   "Delete from selected" / "Compact selected".
2. Each Ops Console tab prefills the target set and exposes the same
   controls.

### External initial-sync workflow

1. `/initial-sync` → Take pre-sync snapshot.
2. Perform `initial sync` on the target member externally.
3. Take post-sync snapshot.
4. The diff renders inline; optionally export CSV via
   `/api/v1/snapshots/compare/report.csv?a=&b=`.

### Rotate the bearer token (droplet)

```bash
sudo sed -i "s|^MFPOC_AUTH_BEARER_TOKEN=.*|MFPOC_AUTH_BEARER_TOKEN=$(openssl rand -hex 32)|" /etc/mfpoc/mfpoc.env
sudo systemctl restart mfpoc
sudo grep MFPOC_AUTH_BEARER_TOKEN /etc/mfpoc/mfpoc.env
```

Update the token in the top-bar input on the dashboard.

### Change log level at runtime

```bash
curl -s -X POST "$BASE/api/v1/admin/log-level" \
  -H "authorization: Bearer $TOKEN" \
  -H "content-type: application/json" \
  -d '{"level":"debug"}' | jq
```

Or use Settings → "Log level (runtime)".

### Backup / restore the state store

Everything (runs, snapshots, operations, events, samples) is in the
SQLite file at `storage.path` (`/var/lib/mfpoc/mfpoc.db` on the droplet).

```bash
sudo systemctl stop mfpoc
sudo cp /var/lib/mfpoc/mfpoc.db /backup/mfpoc-$(date +%F).db
sudo systemctl start mfpoc
```

Restore: replace the file while the service is stopped.

### Upgrade

Workstation:

```bash
git pull
make deploy-droplet    # cross-compiles and re-runs install.sh
```

install.sh detects an existing `/etc/mfpoc/{config.yaml,mfpoc.env}` and
leaves them alone; only the binary swaps.

### Uninstall

```bash
sudo systemctl stop mfpoc && sudo systemctl disable mfpoc
sudo rm /etc/systemd/system/mfpoc.service
sudo rm -rf /etc/mfpoc /var/lib/mfpoc /usr/local/bin/mfpoc
sudo userdel mfpoc
sudo ufw delete allow 8443/tcp
```

## Interpreting the dashboard

| Indicator | Meaning |
|---|---|
| Top-bar `connected` (green) | Most recent `/api/v1/cluster/topology` fetch succeeded. |
| Top-bar `disconnected` (red) | Auth failed or Mongo is unreachable. Check the token and `MFPOC_MONGO_URI`. |
| Data Browser row color | Fragmentation band: `<10%` green, `10-30%` yellow, `>30%` red. |
| Live Ops Panel (right) | Every in-flight operation across all services; pause/resume/stop/cancel inline. |
| Event toast (bottom-right) | Transient notifications; click to dismiss. |

## Startup behaviors worth knowing

- **Orphan recovery** (spec § 21.2) — on every startup, any operation row
  in `running`/`paused`/`stopping` is transitioned to `interrupted`. The
  UI surfaces those in future work; for now use the REST API to clear.
- **MongoDB version gate** (§ 21.1) — connection refuses against
  Mongo < 5.0 with a clear error.
- **Public HTTP bind** (§ 10) — startup refuses unless
  `server.insecure_allow_http_on_public=true` or TLS is configured.
- **Degraded mode** (ARCHITECTURE D-009) — if Mongo is unreachable at
  boot, the server keeps running so `/health`, `/version`, and `/metrics`
  answer; `/ready` returns 503 until the connection succeeds.
