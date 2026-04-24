# How to test this thing

There are 4 paths from "fresh clone" to "I can see it work." Pick the
shortest one that matches what you have installed.

---

## What this thing actually is

A single Go binary that:

1. Connects to a MongoDB cluster you point it at.
2. Loads synthetic documents, deletes some on purpose to create
   fragmentation, runs `compact`, and measures the before/after.
3. Ships a web dashboard (built into the binary) so you do all of
   that with mouse clicks instead of curl.

That's it. The rest of the commits are plumbing, tests, docs, and the
droplet install script.

---

## Path 1 — "I just want to see it work" (5 minutes, needs Docker)

One command puts up a 3-node MongoDB replica set **and** the app
together, wired up:

```bash
git checkout claude/review-mongodb-spec-uEmyz
make docker-up
```

Wait ~60s for the images to pull and the replica set to initiate.

Then open <http://localhost:8080> in a browser. The top bar will go
green ("connected"). Click around:

- **Cluster Overview** (default page) — you should see `replica_set`
  kind, 3 members, one of them PRIMARY.
- **Data Browser** — empty at first. Come back here after you load.
- **Operations Console → Load** — the DB/collection picker won't have
  anything in it yet because we haven't loaded. Skip to step below.
- **New Run Wizard** (sidebar) — click through to create a run, just
  to see the form.

To actually get some data in there:

1. Go to **Operations Console → Load**.
2. The target selector is empty. Type in a database name? No — this UI
   expects collections to already exist so you can select them.
3. Easiest way to seed collections: use the REST API once.

```bash
curl -s -X POST http://localhost:8080/api/v1/loader/start \
  -H 'content-type: application/json' \
  -d '{
    "spec": { "entries": [
      { "database": "poc_db_demo", "collection": "a", "bytes_target": 5000000 },
      { "database": "poc_db_demo", "collection": "b", "bytes_target": 5000000 }
    ]},
    "params": { "workers": 4, "batch_size": 200, "force_start": true }
  }' | jq
```

Now reload the Data Browser — `poc_db_demo` is there with 2
collections. Check the boxes, jump to **Operations Console → Delete**,
pick `random_by_id` with ratio `0.3`, click Preview, confirm, watch
fragmentation rise. Then **Compact**, preview, start, watch it fall.

When done:

```bash
make docker-down   # wipes the mongo volumes
```

---

## Path 2 — "Give me a terminal tour without the UI" (3 minutes)

If you want to see the backend work end-to-end but don't want to click:

```bash
./scripts/checkpoint1-smoke.sh
```

It pulls a disposable `mongo:7.0` container, builds the binary, starts
the server, and curls through every interesting endpoint with
pretty-printed output. Cleanup:

```bash
./scripts/checkpoint1-smoke.sh clean
```

Prereqs: `go`, `docker`, `curl`, `jq`, `make`. Single-node (not RS).

---

## Path 3 — "I have my own MongoDB somewhere" (2 minutes)

Point the binary at whatever Mongo you have:

```bash
make build

MFPOC_MONGO_URI="mongodb://your-host:27017" \
MFPOC_AUTH_ENABLED=false \
  ./bin/mfpoc server --config configs/config.dev.yaml
```

Open <http://localhost:8080>. Same UI.

For an SRV URI (Atlas/Aiven):

```bash
MFPOC_MONGO_URI="mongodb+srv://user:pass@cluster.example.com/?authSource=admin" \
MFPOC_AUTH_ENABLED=false \
  ./bin/mfpoc server --config configs/config.dev.yaml
```

---

## Path 4 — "I have a DigitalOcean droplet" (10 minutes, checkpoint #3)

```bash
export DROPLET_IP=your.droplet.ip
export DROPLET_USER=root
make deploy-droplet
```

This cross-compiles, scps, runs `install.sh` remotely. The output
prints your auto-generated bearer token and a URL like
`https://<ip>:8443`. Self-signed cert — accept the browser warning,
paste the token into the top-bar input, and you're in.

Edit `/etc/mfpoc/mfpoc.env` on the droplet to set `MFPOC_MONGO_URI` to
your real cluster, then `sudo systemctl restart mfpoc`.

Details: [`deploy/droplet/README.md`](deploy/droplet/README.md).

---

## What works vs. what's a stub

Working end-to-end (backend + UI):

- Cluster overview, Data Browser with live stats and multi-select.
- Load / Delete (with preview-confirm) / Compact / Workload tabs, all
  with pause/resume/stop/cancel buttons that actually work.
- Snapshots (take, list, compare two → diff).
- Runs (create, list, detail with fragmentation-over-time chart).
- Compare Runs (A/B overlay chart).
- Initial Sync Companion (take pre-snapshot → do your sync → take
  post-snapshot → inline reclaim diff).
- Event Log with filters.
- Settings: token rotation, runtime log level, theme.
- Prometheus `/metrics` with `mfpoc_*` metrics.
- JSON + CSV reports at `/api/v1/runs/{id}/report[.csv]` and
  `/api/v1/snapshots/compare/report[.csv]`.

Stubs / partial (tracked in [`FUTURE.md`](FUTURE.md)):

- `mfpoc purge` CLI subcommand errors out — use the REST API to drop
  `poc_db_*` databases for now.
- Retention janitor goroutine (auto-purging old samples + VACUUM) is
  not wired; the config keys exist.
- WebSocket streams are live on the server, but the SPA polls every 2s
  via TanStack Query instead of subscribing. No visible difference.
- OpenAPI 3.0 export not generated; `docs/API.md` is the reference.
- Loader update-mix phase not implemented; workload generator covers
  update traffic instead.

---

## If something goes wrong

| Symptom | Try |
|---|---|
| `make docker-up` hangs at "waiting for mongo" | Give it another minute on first run; images are pulling. |
| Top bar shows `disconnected` | Mongo isn't reachable. Check `docker logs mfpoc-mongo1` and `docker logs mfpoc-app`. |
| Dashboard is blank / 404 | The SPA bundle wasn't rebuilt. `make frontend-embed && go build ./cmd/mfpoc`. |
| 401 Unauthorized on every call | Auth is on and you haven't pasted the token. Top-bar input. |
| Self-signed browser warning | Expected on droplet deploy without DNS. Accept the exception. |

More in [`docs/TROUBLESHOOTING.md`](docs/TROUBLESHOOTING.md).

---

## One-line summary of what you'll see if it's working

- **Topology** detected (standalone or replica_set).
- A **Data Browser** row per collection with color-coded fragmentation.
- A **Live Ops Panel** on the right that shows every running loader /
  deleter / compact / workload with working buttons.
- **Fragmentation % goes up** after a delete, **goes down** after a
  compact.
