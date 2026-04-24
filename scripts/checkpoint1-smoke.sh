#!/usr/bin/env bash
# Checkpoint #1 smoke walkthrough.
#
# Pulls up a disposable MongoDB container, builds the mfpoc binary, starts
# the server in the background, and curls through the Phase 1-8 endpoints
# so a reviewer can see the backend working end-to-end.
#
# Prereqs on the host: go >= 1.22, docker daemon running, curl, jq.
#
# Usage:
#   ./scripts/checkpoint1-smoke.sh        # run the full walkthrough
#   ./scripts/checkpoint1-smoke.sh clean  # stop and remove the mongo container
#
set -euo pipefail

PORT="${PORT:-8080}"
MONGO_PORT="${MONGO_PORT:-27017}"
CONTAINER_NAME="${CONTAINER_NAME:-mfpoc-smoke-mongo}"
DATA_DIR="${DATA_DIR:-./data}"
CONFIG_FILE="${CONFIG_FILE:-configs/config.dev.yaml}"
BIN="${BIN:-./bin/mfpoc}"
BASE="http://127.0.0.1:${PORT}"

red()   { printf '\033[31m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
blue()  { printf '\033[34m%s\033[0m\n' "$*"; }
step()  { printf '\n\033[1;36m== %s ==\033[0m\n' "$*"; }

cleanup() {
  if [[ -n "${SERVER_PID:-}" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

if [[ "${1:-}" == "clean" ]]; then
  docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
  rm -rf "$DATA_DIR"
  green "cleaned: container $CONTAINER_NAME and $DATA_DIR"
  exit 0
fi

step "1. preflight: tools"
for t in go docker curl jq; do
  command -v "$t" >/dev/null 2>&1 || { red "missing required tool: $t"; exit 1; }
done
green "ok (go=$(go env GOVERSION), docker=$(docker --version | head -1 | awk '{print $3}' | tr -d ','))"

step "2. build + unit tests + lint"
make build
go test -race -count=1 ./...
if command -v golangci-lint >/dev/null 2>&1; then
  golangci-lint run ./...
else
  blue "golangci-lint not installed, skipping"
fi

step "3. start disposable MongoDB 7.0 container"
if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}\$"; then
  docker rm -f "$CONTAINER_NAME" >/dev/null
fi
docker run -d --rm --name "$CONTAINER_NAME" \
  -p "${MONGO_PORT}:27017" \
  mongo:7.0 >/dev/null
for i in {1..30}; do
  if docker exec "$CONTAINER_NAME" mongosh --quiet --eval 'db.runCommand({ping:1}).ok' >/dev/null 2>&1; then
    green "mongo is up"
    break
  fi
  sleep 1
  [[ $i -eq 30 ]] && { red "mongo did not become ready"; exit 1; }
done

step "4. launch mfpoc server in the background"
rm -f /tmp/mfpoc-smoke.log
MFPOC_MONGO_URI="mongodb://127.0.0.1:${MONGO_PORT}" \
  MFPOC_SERVER_LISTEN="127.0.0.1:${PORT}" \
  MFPOC_AUTH_ENABLED=false \
  "$BIN" server --config "$CONFIG_FILE" \
  >/tmp/mfpoc-smoke.log 2>&1 &
SERVER_PID=$!
for i in {1..30}; do
  if curl -sf "${BASE}/health" >/dev/null 2>&1; then
    green "server is up (pid=$SERVER_PID)"
    break
  fi
  sleep 0.5
  [[ $i -eq 30 ]] && { red "server did not start; tail:"; tail -40 /tmp/mfpoc-smoke.log; exit 1; }
done

step "5. GET /health and /api/v1/version"
curl -s "${BASE}/health" | jq .
curl -s "${BASE}/api/v1/version" | jq .

step "6. GET /api/v1/cluster/topology"
curl -s "${BASE}/api/v1/cluster/topology" | jq '{kind: .data.topology.kind, version: .data.server_info.version, is_srv: .data.is_srv}'

step "7. GET /api/v1/cluster/databases"
curl -s "${BASE}/api/v1/cluster/databases" | jq '.data | length'

step "8. POST /api/v1/loader/start (small 200KB load)"
LOADER_ID=$(curl -s -X POST "${BASE}/api/v1/loader/start" \
  -H "content-type: application/json" \
  -d '{
    "spec": { "entries": [
      { "database": "poc_db_smoke", "collection": "coll_a", "bytes_target": 200000 }
    ]},
    "params": { "workers": 2, "batch_size": 50, "force_start": true }
  }' | jq -r '.data.operation_id')
echo "loader operation_id: $LOADER_ID"

step "9. wait for loader to finish, then GET /loader/{id}"
for i in {1..60}; do
  state=$(curl -s "${BASE}/api/v1/loader/${LOADER_ID}" | jq -r '.data.state // .data.State // empty')
  if [[ "$state" == "completed" || "$state" == "failed" || "$state" == "stopped" ]]; then
    break
  fi
  sleep 0.5
done
curl -s "${BASE}/api/v1/loader/${LOADER_ID}" | jq '.data | {state, stats, progress}'

step "10. POST /api/v1/deleter/preview (random_by_id, ratio 0.3)"
PREVIEW=$(curl -s -X POST "${BASE}/api/v1/deleter/preview" \
  -H "content-type: application/json" \
  -d '{
    "spec": { "entries": [
      { "database": "poc_db_smoke", "collection": "coll_a",
        "pattern": { "kind": "random_by_id", "ratio": 0.3, "seed": 42 }}
    ]},
    "params": { "batch_size": 100, "max_ratio": 0.95 }
  }')
echo "$PREVIEW" | jq '.data | {total_matches, per_collection, confirmation_token: (.confirmation_token | .[0:16] + "…")}'
TOKEN=$(echo "$PREVIEW" | jq -r '.data.confirmation_token')

step "11. POST /api/v1/deleter/start with the confirmation token"
DEL_ID=$(curl -s -X POST "${BASE}/api/v1/deleter/start" \
  -H "content-type: application/json" \
  -d "{\"confirmation_token\":\"${TOKEN}\"}" | jq -r '.data.operation_id')
echo "deleter operation_id: $DEL_ID"
for i in {1..60}; do
  state=$(curl -s "${BASE}/api/v1/deleter/${DEL_ID}" | jq -r '.data.state // empty')
  [[ "$state" == "completed" || "$state" == "failed" || "$state" == "stopped" ]] && break
  sleep 0.5
done
curl -s "${BASE}/api/v1/deleter/${DEL_ID}" | jq '.data | {state, stats}'

step "12. POST /api/v1/snapshots (baseline snapshot)"
curl -s -X POST "${BASE}/api/v1/snapshots/" \
  -H "content-type: application/json" \
  -d '{"label":"post_smoke","note":"checkpoint1 walkthrough"}' | jq '.data'

step "13. GET /api/v1/snapshots"
curl -s "${BASE}/api/v1/snapshots/" | jq '[.data[] | {id, label, taken_at}]'

step "14. POST /api/v1/compact/preview (cluster scope)"
curl -s -X POST "${BASE}/api/v1/compact/preview" \
  -H "content-type: application/json" \
  -d '{
    "scope": { "kind": "cluster" },
    "params": { "max_replication_lag_ns": 10000000000, "stepdown_wait_timeout_ns": 60000000000 }
  }' | jq '.data | {mode, total_collections, execution_order: [.execution_order[] | {member, role, collections: (.collections | length)}]}'

step "15. GET /api/v1/runs and /api/v1/events"
curl -s "${BASE}/api/v1/runs/" | jq '.data | length as $n | "\($n) runs"'
curl -s "${BASE}/api/v1/events/?limit=5" | jq '[.data[] | {category, message}]'

step "16. GET /metrics (Prometheus) - show mfpoc_* lines only"
curl -s "${BASE}/metrics" | grep -E '^mfpoc_' | head -20

step "done"
green "server log at /tmp/mfpoc-smoke.log"
green "to clean up: ./scripts/checkpoint1-smoke.sh clean"
