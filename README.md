# mfpoc — MongoDB Fragmentation Reclamation POC

Single-binary platform to empirically compare MongoDB `compact` against full
initial-sync resyncs across single-node and 3-node replica-set topologies.
Loads massive synthetic datasets, induces controlled fragmentation, measures
`collStats`/`dbStats` as a time series, runs rolling compacts, and captures
before/after snapshots around externally-triggered initial syncs.

## Status

Under active development — see [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)
for the current phase and design decisions.

## Quick start (local)

```bash
make build
./bin/mfpoc version
./bin/mfpoc config-validate --config configs/config.dev.yaml
./bin/mfpoc server --config configs/config.dev.yaml
```

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

## License

See [LICENSE](LICENSE).
