# Droplet deployment

End-to-end walkthrough for running mfpoc on a DigitalOcean droplet.

## TL;DR

```bash
export DROPLET_IP=203.0.113.10
export DROPLET_USER=root
make deploy-droplet           # works for both first install AND upgrade
```

The output ends with the dashboard URL and your bearer token. Open the
URL, paste the token into the login screen, you're in.

## Prereqs

- Ubuntu 22.04 / 24.04 droplet, 2 vCPU / 4 GB or larger.
- SSH access as a sudo user (root works fine).
- A MongoDB cluster reachable from the droplet:
  - Same VPC: `mongodb://...` URI.
  - Atlas / DigitalOcean Managed / Aiven: `mongodb+srv://...` URI; verify
    `nslookup -type=SRV _mongodb._tcp.<host>` resolves from the droplet.

## First-time install

```bash
export DROPLET_IP=203.0.113.10
export DROPLET_USER=root
make deploy-droplet
```

That cross-compiles a static `mfpoc-linux-amd64`, scps it along with
`install.sh`, `mfpoc.service`, and `config.droplet.yaml`, then runs
`install.sh` remotely.

The installer:

1. Creates the non-root `mfpoc` system user and `/var/lib/mfpoc/`.
2. Places the binary at `/usr/local/bin/mfpoc`.
3. Drops `config.droplet.yaml` at `/etc/mfpoc/config.yaml` (preserved on
   re-runs).
4. **Auto-generates a 32-byte hex bearer token** at
   `/etc/mfpoc/mfpoc.env` (mode 0640, root:mfpoc). **Preserved on re-runs.**
5. Installs the hardened `mfpoc.service` systemd unit (NoNewPrivileges,
   ProtectSystem=strict, ProtectHome, PrivateTmp, restricted capability
   set, ReadWritePaths=/var/lib/mfpoc).
6. Opens UFW for the configured port and SSH.
7. Starts the service and prints the connection URL + token.

Detected droplet IP comes from DigitalOcean instance metadata first,
then `ifconfig.me`, then the primary network interface, so the printed
URL is always your actual public IP.

## Upgrades / redeploys

Same command:

```bash
make deploy-droplet     # or: make redeploy-droplet (alias)
```

The installer detects the existing binary, **preserves**:

- `/etc/mfpoc/config.yaml`
- `/etc/mfpoc/mfpoc.env` (so your token does NOT change)
- `/var/lib/mfpoc/mfpoc.db` (your runs, snapshots, ops, events, samples)

… and only:

- swaps the binary
- refreshes the systemd unit
- restarts the service

The "UPGRADED" banner at the end shows the prior + new version. Token is
reprinted regardless so you can grab it again if the previous deploy's
output scrolled away.

## Configuring the Mongo URI

After the first install, the env file has a placeholder URI. Set yours:

```bash
ssh root@$DROPLET_IP
sudo $EDITOR /etc/mfpoc/mfpoc.env       # replace MFPOC_MONGO_URI=...
sudo systemctl restart mfpoc
```

Or non-interactively:

```bash
ssh root@$DROPLET_IP "sudo sed -i 's|^MFPOC_MONGO_URI=.*|MFPOC_MONGO_URI=mongodb+srv://USER:PASS@HOST/?authSource=admin\&retryWrites=true|' /etc/mfpoc/mfpoc.env && sudo systemctl restart mfpoc"
```

`retryWrites=true` is recommended — it stacks with the app's 20-attempt
exponential-backoff for managed cluster timeouts.

## Token: how it's stored & what happens on redeploy

| Question | Answer |
|---|---|
| Where is it generated? | On first install, by `openssl rand -hex 32` inside `install.sh`. |
| Where is it stored? | `/etc/mfpoc/mfpoc.env` (mode 0640, owner `root:mfpoc`). |
| Is it logged? | No. Redacted from server startup logs. Printed once by `install.sh` to operator stdout. |
| Does it change on redeploy? | **No.** install.sh detects the existing env file and preserves it. |
| How do I retrieve it later? | `make show-droplet-token DROPLET_IP=... DROPLET_USER=root` |
| How do I rotate it? | Edit `/etc/mfpoc/mfpoc.env`, replace the value, `systemctl restart mfpoc`. |
| Where does the dashboard store it? | Browser `localStorage` under key `mfpoc-auth`. Cleared by emptying the top-bar input. |

## Browser flow

When you open the dashboard:

1. The SPA probes `/api/v1/version` to see if auth is required.
2. If auth is enabled and no token is set, you get a **first-class auth
   screen** (not a tiny top-bar input you'd never find). Paste the token
   from `install.sh` output.
3. The token persists in browser localStorage. Subsequent visits skip the
   auth screen.

Self-signed cert warning is expected — accept the browser exception or
install the cert into your local trust store. ACME/Let's Encrypt is also
supported once you have a DNS name; see `config.droplet.yaml` for the
acme block.

## Operations

| Task | Command |
|---|---|
| Status | `systemctl status mfpoc` |
| Logs (follow) | `make tail-droplet-logs DROPLET_IP=... DROPLET_USER=...` or `journalctl -u mfpoc -f` on the droplet |
| Restart | `systemctl restart mfpoc` |
| Stop | `systemctl stop mfpoc` |
| Print token | `make show-droplet-token DROPLET_IP=... DROPLET_USER=...` |
| Upgrade / redeploy | `make deploy-droplet` (token + state preserved) |

## Backup / restore

Everything (runs, snapshots, ops, events, samples) is in
`/var/lib/mfpoc/mfpoc.db`. Stop the service, copy, restart:

```bash
ssh root@$DROPLET_IP "sudo systemctl stop mfpoc && \
  sudo cp /var/lib/mfpoc/mfpoc.db /backup/mfpoc-\$(date +%F).db && \
  sudo systemctl start mfpoc"
```

## Uninstall

```bash
ssh root@$DROPLET_IP <<'EOF'
sudo systemctl stop mfpoc && sudo systemctl disable mfpoc
sudo rm /etc/systemd/system/mfpoc.service
sudo rm -rf /etc/mfpoc /var/lib/mfpoc /usr/local/bin/mfpoc
sudo userdel mfpoc
sudo ufw delete allow 8443/tcp || true
EOF
```

## Troubleshooting

- **Service crash-loops with "auth.bearer_token must be at least 32 bytes"**
  — env file wasn't generated. Re-run `make deploy-droplet`.
- **`/ready` returns 503** — Mongo is unreachable. Check
  `MFPOC_MONGO_URI`, then `nslookup -type=SRV` if SRV, `nc -vz host 27017`
  otherwise.
- **Browser shows "missing bearer token" after redeploy** — token did
  *not* change; the browser localStorage is empty. Run
  `make show-droplet-token` and paste it into the auth screen.
- **Dashboard shows "disconnected"** — open the auth screen via the
  top-bar input, or check `journalctl -u mfpoc -n 100` for connection
  errors.
- **Want a fresh token** — `sudo sed -i "s|^MFPOC_AUTH_BEARER_TOKEN=.*|MFPOC_AUTH_BEARER_TOKEN=$(openssl rand -hex 32)|" /etc/mfpoc/mfpoc.env && sudo systemctl restart mfpoc && sudo grep MFPOC_AUTH_BEARER_TOKEN /etc/mfpoc/mfpoc.env`
