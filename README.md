# mfpoc — MongoDB Fragmentation Reclamation POC

Single-binary platform to empirically compare MongoDB `compact` against full
initial-sync resyncs across single-node and 3-node replica-set topologies.
Loads massive synthetic datasets, induces controlled fragmentation, measures
`collStats`/`dbStats` as a time series, runs rolling compacts, and captures
before/after snapshots around externally-triggered initial syncs.

## Status

Under active development — see [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)
for the current phase and design decisions.

**Checkpoint #1 (backend complete, Phases 1–8 merged):** loader, deleter,
compact orchestrator, workload generator, metrics collector, REST API,
and live WebSocket streams are all in place and exercised by unit +
Docker-gated integration tests.

## Try the backend in one command

`scripts/checkpoint1-smoke.sh` spins up a disposable `mongo:7.0`
container, builds the binary, launches the server, and curls through
every Phase 1–8 endpoint so you can see the backend working end-to-end.

Prereqs: `go`, `docker` daemon running, `curl`, `jq`, `make`,
optionally `golangci-lint`.

```bash
git clone https://github.com/pagombin/Fragmention-POC.git
cd Fragmention-POC
git checkout claude/review-mongodb-spec-uEmyz

./scripts/checkpoint1-smoke.sh          # runs the full walkthrough
./scripts/checkpoint1-smoke.sh clean    # tear down the mongo container
```

The script prints a labeled trace of every request/response so you can
see topology detection, a 200KB loader run, a preview→confirm→execute
delete, a snapshot, a compact preview, and the Prometheus `mfpoc_*`
metrics.

## Quick start (manual)

```bash
make build                                  # bin/mfpoc
./bin/mfpoc version
./bin/mfpoc config-validate --config configs/config.dev.yaml

# Start a local MongoDB however you like, then:
MFPOC_MONGO_URI="mongodb://127.0.0.1:27017" \
  ./bin/mfpoc server --config configs/config.dev.yaml

# In another terminal:
curl -s http://127.0.0.1:8080/api/v1/cluster/topology | jq .
curl -s http://127.0.0.1:8080/api/v1/cluster/databases | jq .
```

With `auth.enabled: true` the config.dev profile leaves auth off; all
other profiles require `Authorization: Bearer <token>`.

## Testing

```bash
make test              # unit tests (no Docker required)
make test-integration  # starts real mongo:7.0 containers; needs Docker
make lint              # golangci-lint run ./...
```

`make test-integration` runs loader + deleter + collector + topology
suites against a throwaway container. It is gated by the `integration`
build tag so `make test` stays fast.

## Make targets

| Target              | Description                                            |
|---------------------|--------------------------------------------------------|
| `make build`        | Build the binary for the host architecture            |
| `make build-linux-amd64` | Static Linux/amd64 build for the droplet         |
| `make test`         | Unit tests (race, no build tags)                      |
| `make test-integration` | Integration tests (require Docker)                |
| `make lint`         | `golangci-lint run ./...`                             |
| `make fmt`          | `go fmt ./...`                                        |
| `make deploy-droplet` | Cross-compile + scp + remote install (env `DROPLET_IP`, `DROPLET_USER`) |

## Configuration

See [`configs/config.example.yaml`](configs/config.example.yaml) and
[`docs/CONFIGURATION.md`](docs/CONFIGURATION.md) (populated in Phase 19).

## Documentation

| Document | Purpose |
|---|---|
| [ARCHITECTURE.md](docs/ARCHITECTURE.md) | Component diagram, dataflow, design decisions |
| [API.md](docs/API.md) | HTTP endpoint reference |
| [CONFIGURATION.md](docs/CONFIGURATION.md) | Every config option |
| [OPERATIONS.md](docs/OPERATIONS.md) | Runbook |
| [METHODOLOGY.md](docs/METHODOLOGY.md) | POC methodology and interpretation |
| [LIMITATIONS.md](docs/LIMITATIONS.md) | Known limits |
| [GLOSSARY.md](docs/GLOSSARY.md) | Terms used in UI and reports |
| [TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) | Common errors |
| [FUTURE.md](FUTURE.md) | Backlog & deliberate omissions |

## License

See [LICENSE](LICENSE).
