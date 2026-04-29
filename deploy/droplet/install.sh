#!/usr/bin/env bash
#
# install.sh — idempotent installer for mfpoc on a DigitalOcean droplet
# running Ubuntu 22.04 or 24.04.
#
# Behaviour:
#
# - First install:   creates user + dirs, installs binary + config + env
#                    (auto-generates bearer token), enables systemd unit,
#                    opens UFW, prints connection details + token.
#
# - Re-install:      detects existing /etc/mfpoc/mfpoc.env and PRESERVES the
#                    token, config, and SQLite state. Just swaps the binary,
#                    refreshes the systemd unit, and restarts the service.
#                    Always reprints the existing token + URL on completion.
#
# Run with sudo. Run as many times as you like - it's idempotent.
#
# Expected layout when scp'd from `make deploy-droplet`:
#   /tmp/mfpoc-linux-amd64
#   /tmp/install.sh            (this file)
#   /tmp/mfpoc.service
#   /tmp/config.droplet.yaml
#
# Env overrides:
#   MFPOC_PORT=8443              (default: 8443)
#   MFPOC_BIND=0.0.0.0           (default: 0.0.0.0)
#   MFPOC_BEARER_TOKEN=...       (auto-generated when unset, FIRST install only)
#   MFPOC_MONGO_URI=...          (placeholder when unset, FIRST install only)

set -euo pipefail

PORT="${MFPOC_PORT:-8443}"
BIND="${MFPOC_BIND:-0.0.0.0}"

cyan()   { printf '\033[36m%s\033[0m\n' "$*"; }
green()  { printf '\033[32m%s\033[0m\n' "$*"; }
yellow() { printf '\033[33m%s\033[0m\n' "$*"; }
red()    { printf '\033[31m%s\033[0m\n' "$*"; }
log()    { cyan "[mfpoc-install] $*"; }
warn()   { yellow "[mfpoc-install] $*" >&2; }
fail()   { red "[mfpoc-install] $*" >&2; exit 1; }

[[ "$(id -u)" -eq 0 ]] || fail "must run as root (sudo bash install.sh)"

for f in /tmp/mfpoc-linux-amd64 /tmp/mfpoc.service /tmp/config.droplet.yaml; do
  [[ -f "$f" ]] || fail "missing expected artifact: $f"
done

# --- detect first-install vs upgrade ---------------------------------------
MODE="install"
PRIOR_VERSION=""
if [[ -x /usr/local/bin/mfpoc ]]; then
  MODE="upgrade"
  PRIOR_VERSION="$(/usr/local/bin/mfpoc version 2>/dev/null | head -1 || true)"
fi
log "mode: ${MODE}${PRIOR_VERSION:+ (prior: $PRIOR_VERSION)}"

# --- resolve external IP ---------------------------------------------------
# Prefer DigitalOcean metadata (intra-VPC, fast). Fall back to ifconfig.me,
# then the first non-loopback interface IPv4. Used only for the post-install
# message - NOT for binding.
resolve_ip() {
  local ip
  ip="$(curl -fsS --max-time 2 http://169.254.169.254/metadata/v1/interfaces/public/0/ipv4/address 2>/dev/null || true)"
  if [[ -z "$ip" ]]; then
    ip="$(curl -fsS --max-time 3 https://ifconfig.me 2>/dev/null || true)"
  fi
  if [[ -z "$ip" ]]; then
    ip="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
  fi
  echo "${ip:-<this-droplet-ip>}"
}
DROPLET_IP="$(resolve_ip)"

# --- 1. user + dirs -------------------------------------------------------
if ! id mfpoc >/dev/null 2>&1; then
  log "creating mfpoc system user"
  useradd --system --no-create-home --shell /usr/sbin/nologin mfpoc
fi
install -d -o mfpoc -g mfpoc -m 0750 /var/lib/mfpoc /var/lib/mfpoc/tls
install -d -o root  -g mfpoc -m 0750 /etc/mfpoc

# --- 2. binary (always swap) ----------------------------------------------
log "installing /usr/local/bin/mfpoc"
install -o root -g root -m 0755 /tmp/mfpoc-linux-amd64 /usr/local/bin/mfpoc
NEW_VERSION="$(/usr/local/bin/mfpoc version 2>/dev/null | head -1 || true)"

# --- 3. config (preserve on upgrade) --------------------------------------
if [[ ! -f /etc/mfpoc/config.yaml ]]; then
  log "installing /etc/mfpoc/config.yaml (first install)"
  install -o root -g mfpoc -m 0640 /tmp/config.droplet.yaml /etc/mfpoc/config.yaml
else
  log "preserving existing /etc/mfpoc/config.yaml"
fi

# --- 4. environment file (preserve token on upgrade) ----------------------
if [[ ! -f /etc/mfpoc/mfpoc.env ]]; then
  TOKEN="${MFPOC_BEARER_TOKEN:-$(openssl rand -hex 32)}"
  MONGO_URI="${MFPOC_MONGO_URI:-mongodb://REPLACE_ME:27017/?replicaSet=rs0}"
  log "creating /etc/mfpoc/mfpoc.env (first install)"
  cat > /etc/mfpoc/mfpoc.env <<EOF
# Managed by install.sh; edit carefully. Re-run install.sh to upgrade the
# binary without changing this file.
MFPOC_AUTH_BEARER_TOKEN=${TOKEN}
MFPOC_MONGO_URI=${MONGO_URI}
MFPOC_SERVER_LISTEN=${BIND}:${PORT}
EOF
  chown root:mfpoc /etc/mfpoc/mfpoc.env
  chmod 0640 /etc/mfpoc/mfpoc.env
else
  log "preserving existing /etc/mfpoc/mfpoc.env (token, MONGO_URI, listen unchanged)"
fi

# --- 5. systemd unit (always refresh) -------------------------------------
log "installing /etc/systemd/system/mfpoc.service"
install -o root -g root -m 0644 /tmp/mfpoc.service /etc/systemd/system/mfpoc.service
systemctl daemon-reload
systemctl enable mfpoc.service >/dev/null 2>&1 || true

# --- 6. firewall ----------------------------------------------------------
if command -v ufw >/dev/null 2>&1; then
  log "configuring UFW (allow $PORT/tcp + SSH)"
  ufw allow OpenSSH    >/dev/null 2>&1 || true
  ufw allow "$PORT/tcp" >/dev/null 2>&1 || true
  ufw --force enable    >/dev/null 2>&1 || true
fi

# --- 7. start / restart ---------------------------------------------------
if systemctl is-active --quiet mfpoc.service; then
  log "restarting mfpoc.service"
  systemctl restart mfpoc.service
else
  log "starting mfpoc.service"
  systemctl start mfpoc.service
fi

# Wait briefly for the service to settle so the readiness probe is meaningful.
for _ in 1 2 3 4 5; do
  systemctl is-active --quiet mfpoc.service && break
  sleep 1
done
systemctl is-active --quiet mfpoc.service \
  || fail "mfpoc.service did not start; run: journalctl -u mfpoc -n 100"

# --- 8. fetch the live token + listen for the summary ---------------------
TOKEN="$(grep -E '^MFPOC_AUTH_BEARER_TOKEN=' /etc/mfpoc/mfpoc.env | cut -d= -f2-)"
LISTEN="$(grep -E '^MFPOC_SERVER_LISTEN='   /etc/mfpoc/mfpoc.env | cut -d= -f2-)"
LISTEN_PORT="${LISTEN##*:}"
[[ "$LISTEN_PORT" =~ ^[0-9]+$ ]] || LISTEN_PORT="$PORT"

URL="https://${DROPLET_IP}:${LISTEN_PORT}"

# --- 9. summary -----------------------------------------------------------
green ""
green "==================================================================="
case "$MODE" in
  install) green "  mfpoc INSTALLED" ;;
  upgrade) green "  mfpoc UPGRADED  (was: ${PRIOR_VERSION:-unknown})" ;;
esac
green "==================================================================="
echo
echo "  Version:   ${NEW_VERSION:-unknown}"
echo "  Service:   $(systemctl is-active mfpoc.service)"
echo
echo "  Config:    /etc/mfpoc/config.yaml"
echo "  Env:       /etc/mfpoc/mfpoc.env  (mode 0640, root:mfpoc)"
echo "  State:     /var/lib/mfpoc/"
echo "  Logs:      journalctl -u mfpoc -f"
echo
green "  ►  Dashboard:  ${URL}"
green "  ►  Bearer token (paste into the dashboard):"
echo
echo "       ${TOKEN}"
echo
echo "  Verify from the droplet:"
echo "    curl -sk -H 'Authorization: Bearer \$TOKEN' ${URL}/api/v1/version | head -c 200"
echo
if [[ "$MODE" == "install" ]]; then
  yellow "  Next steps:"
  yellow "    1. Edit /etc/mfpoc/mfpoc.env and set MFPOC_MONGO_URI to your real cluster"
  yellow "    2. systemctl restart mfpoc"
  yellow "    3. Open ${URL} in a browser and paste the token above when prompted"
else
  echo "  (Token, config, and state are unchanged from the previous install.)"
fi
green "==================================================================="
