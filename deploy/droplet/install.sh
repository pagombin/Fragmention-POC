#!/usr/bin/env bash
#
# install.sh — idempotent installer for mfpoc on a DigitalOcean droplet
# running Ubuntu 22.04 or 24.04.
#
# Places the binary, config, and systemd unit. Creates the mfpoc user and
# opens UFW for the configured app port. Safe to re-run after an upgrade.
#
# Expected layout when scp'd from `make deploy-droplet`:
#   /tmp/mfpoc-linux-amd64
#   /tmp/install.sh            (this file)
#   /tmp/mfpoc.service
#   /tmp/config.droplet.yaml
#
# Env overrides:
#   MFPOC_PORT=8443         (default: 8443)
#   MFPOC_BIND=0.0.0.0      (default: 0.0.0.0)
#   MFPOC_BEARER_TOKEN=...  (auto-generated when unset)
#   MFPOC_MONGO_URI=...     (stays as placeholder when unset)

set -euo pipefail

PORT="${MFPOC_PORT:-8443}"
BIND="${MFPOC_BIND:-0.0.0.0}"

log() { printf '\033[36m[mfpoc-install]\033[0m %s\n' "$*"; }
warn() { printf '\033[33m[mfpoc-install]\033[0m %s\n' "$*" >&2; }
fail() { printf '\033[31m[mfpoc-install]\033[0m %s\n' "$*" >&2; exit 1; }

[[ "$(id -u)" -eq 0 ]] || fail "must run as root (sudo bash install.sh)"

for f in /tmp/mfpoc-linux-amd64 /tmp/mfpoc.service /tmp/config.droplet.yaml; do
  [[ -f "$f" ]] || fail "missing expected artifact: $f"
done

# 1. user + dirs
if ! id mfpoc >/dev/null 2>&1; then
  log "creating mfpoc system user"
  useradd --system --no-create-home --shell /usr/sbin/nologin mfpoc
fi

install -d -o mfpoc -g mfpoc -m 0750 /var/lib/mfpoc /var/lib/mfpoc/tls
install -d -o root -g mfpoc -m 0750 /etc/mfpoc

# 2. binary
log "installing /usr/local/bin/mfpoc"
install -o root -g root -m 0755 /tmp/mfpoc-linux-amd64 /usr/local/bin/mfpoc

# 3. config - only overwrite a fresh install
if [[ ! -f /etc/mfpoc/config.yaml ]]; then
  log "installing /etc/mfpoc/config.yaml"
  install -o root -g mfpoc -m 0640 /tmp/config.droplet.yaml /etc/mfpoc/config.yaml
else
  warn "/etc/mfpoc/config.yaml exists; leaving it untouched"
fi

# 4. environment file (secrets) - only create if absent
if [[ ! -f /etc/mfpoc/mfpoc.env ]]; then
  TOKEN="${MFPOC_BEARER_TOKEN:-$(openssl rand -hex 32)}"
  MONGO_URI="${MFPOC_MONGO_URI:-mongodb://REPLACE_ME:27017/?replicaSet=rs0}"
  log "creating /etc/mfpoc/mfpoc.env (0600)"
  cat > /etc/mfpoc/mfpoc.env <<EOF
# Managed by install.sh; edit carefully.
MFPOC_AUTH_BEARER_TOKEN=${TOKEN}
MFPOC_MONGO_URI=${MONGO_URI}
MFPOC_SERVER_LISTEN=${BIND}:${PORT}
EOF
  chown root:mfpoc /etc/mfpoc/mfpoc.env
  chmod 0640 /etc/mfpoc/mfpoc.env
else
  warn "/etc/mfpoc/mfpoc.env exists; not regenerating token"
fi

# 5. systemd unit
log "installing /etc/systemd/system/mfpoc.service"
install -o root -g root -m 0644 /tmp/mfpoc.service /etc/systemd/system/mfpoc.service
systemctl daemon-reload
systemctl enable mfpoc.service

# 6. firewall
if command -v ufw >/dev/null 2>&1; then
  log "configuring UFW ($PORT/tcp + SSH)"
  ufw allow OpenSSH >/dev/null 2>&1 || true
  ufw allow "$PORT/tcp" >/dev/null 2>&1 || true
  ufw --force enable >/dev/null 2>&1 || true
fi

# 7. start / restart
if systemctl is-active --quiet mfpoc.service; then
  log "restarting mfpoc.service"
  systemctl restart mfpoc.service
else
  log "starting mfpoc.service"
  systemctl start mfpoc.service
fi

sleep 2
systemctl is-active --quiet mfpoc.service && log "mfpoc.service is active" || fail "mfpoc.service did not start; run journalctl -u mfpoc -n 100"

TOKEN_LINE=$(grep -E '^MFPOC_AUTH_BEARER_TOKEN=' /etc/mfpoc/mfpoc.env || true)
cat <<EOF

===================================================================
mfpoc installed.

  Config:      /etc/mfpoc/config.yaml
  Env:         /etc/mfpoc/mfpoc.env (contains your bearer token)
  State:       /var/lib/mfpoc/
  Binary:      /usr/local/bin/mfpoc
  Service:     systemctl {status,restart,stop} mfpoc
  Logs:        journalctl -u mfpoc -f
  Dashboard:   https://<this-droplet-ip>:${PORT}  (self-signed; accept browser warning)

Bearer token (save this!):
  ${TOKEN_LINE}
===================================================================
EOF
