# Droplet deployment

End-to-end walkthrough for running mfpoc on a DigitalOcean droplet.

## Prereqs

- Ubuntu 22.04 or 24.04 droplet (2 vCPU / 4 GB is plenty for the app; size
  separately for the target MongoDB load you want to stage).
- SSH access as a user in `sudo`.
- A MongoDB cluster reachable from the droplet. Either a standard
  `mongodb://` URI in the same VPC, or an SRV URI (Atlas/Aiven) — make sure
  `nslookup -type=SRV _mongodb._tcp.<host>` resolves from the droplet.

## Deploy

From a workstation with this repository cloned, Go toolchain, and an SSH
key that can reach the droplet:

```bash
export DROPLET_IP=203.0.113.10
export DROPLET_USER=root          # or a sudo user
make deploy-droplet
```

That cross-compiles a static `mfpoc-linux-amd64`, scps it along with
`install.sh`, `mfpoc.service`, and `config.droplet.yaml`, then runs
`install.sh` remotely. On completion it prints the generated bearer token
and the URL to the dashboard.

## First-deploy checklist

1. Note the printed bearer token (it's only echoed once). It also lives in
   `/etc/mfpoc/mfpoc.env` readable only by `root` and the `mfpoc` group.
2. Edit `/etc/mfpoc/mfpoc.env` and set `MFPOC_MONGO_URI` to your real
   cluster URI, then `systemctl restart mfpoc`.
3. Open `https://<droplet-ip>:8443` — the cert is self-signed on first run
   so you'll need to accept the browser warning. The dashboard prompts for
   the bearer token at the top.

## What install.sh does

- Creates the non-root `mfpoc` system user and `/var/lib/mfpoc/`.
- Places the binary at `/usr/local/bin/mfpoc`.
- Drops config at `/etc/mfpoc/config.yaml` and env secrets at
  `/etc/mfpoc/mfpoc.env` (mode 0640, root:mfpoc).
- Installs `mfpoc.service` with systemd hardening
  (NoNewPrivileges, ProtectSystem=strict, ProtectHome, PrivateTmp,
  ReadWritePaths=/var/lib/mfpoc, capabilities dropped).
- Opens the configured TCP port (default 8443) and SSH on UFW.
- Starts and enables the service.

Re-running install.sh after a new binary build is safe — it preserves the
existing config and env file, swaps the binary, and restarts the service.

## Rotate the bearer token

```bash
sudo sed -i "s|^MFPOC_AUTH_BEARER_TOKEN=.*|MFPOC_AUTH_BEARER_TOKEN=$(openssl rand -hex 32)|" /etc/mfpoc/mfpoc.env
sudo systemctl restart mfpoc
sudo cat /etc/mfpoc/mfpoc.env | grep MFPOC_AUTH_BEARER_TOKEN
```

## Operations

| Task | Command |
|---|---|
| Status | `systemctl status mfpoc` |
| Logs (follow) | `journalctl -u mfpoc -f` |
| Restart | `systemctl restart mfpoc` |
| Stop | `systemctl stop mfpoc` |
| Config reload | Edit `/etc/mfpoc/config.yaml`, then restart |

## Backup / restore

The state store lives at `/var/lib/mfpoc/mfpoc.db`. Stop the service, copy
the file, bring the service back:

```bash
sudo systemctl stop mfpoc
sudo cp /var/lib/mfpoc/mfpoc.db /backup/mfpoc-$(date +%F).db
sudo systemctl start mfpoc
```

## Upgrade

Re-run `make deploy-droplet` from your workstation. The installer detects
the existing config and env file and only swaps the binary.

## Uninstall

```bash
sudo systemctl stop mfpoc
sudo systemctl disable mfpoc
sudo rm /etc/systemd/system/mfpoc.service
sudo rm -rf /etc/mfpoc /var/lib/mfpoc /usr/local/bin/mfpoc
sudo userdel mfpoc
sudo ufw delete allow 8443/tcp
```

## Troubleshooting

- **Service crash-loops with "config invalid: auth.bearer_token must be at
  least 32 bytes"** — the env file wasn't generated. Re-run
  `install.sh` or `export MFPOC_BEARER_TOKEN=$(openssl rand -hex 32)` and
  rerun.
- **`/ready` returns 503** — Mongo is unreachable. Check
  `MFPOC_MONGO_URI` in `/etc/mfpoc/mfpoc.env`, then
  `nslookup -type=SRV` if using SRV, `telnet <host> 27017` otherwise.
- **Browser warns about self-signed cert** — expected. Add a trust
  exception or switch to ACME once you have a DNS name.
