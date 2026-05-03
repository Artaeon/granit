# Deploying granit web

Three paths, in increasing automation. Pick whichever matches your
existing infrastructure.

This is a deploy-day checklist — the architecture / "what is this"
content lives in [`WEB.md`](WEB.md).

---

## Pre-flight checklist

Before any path:

- [ ] Your vault is a git repo with a remote (`git remote -v` shows
      something). Granit's `--sync` flag uses standard git, no Forge-
      specific protocol.
- [ ] You have an SSH key on the target server that can push to the
      vault remote (so the `--sync` loop's `git push` works).
- [ ] DNS for the chosen domain points at the server's IP.
- [ ] You're OK with the bootstrap bearer token + password being the
      only auth — there's no SSO / OIDC integration today.

---

## Path 1 — bare binary

Simplest. No Docker, no FleetDeck, just Go + systemd.

```bash
# On the server (one-time):
GOOS=linux GOARCH=amd64 go install github.com/artaeon/granit/cmd/granit@latest
git clone git@github.com:you/your-vault.git /srv/granit-vault

# As a systemd service (write this to /etc/systemd/system/granit.service):
[Unit]
Description=Granit web
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=granit
ExecStart=/usr/local/bin/granit web --addr 0.0.0.0:8787 --sync --sync-interval 60s /srv/granit-vault
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Front with nginx / Caddy / Traefik for HTTPS. The first launch prints
the bootstrap token + asks the web UI to set a password.

---

## Path 2 — Docker compose

Use the bundled `docker-compose.example.yml`:

```bash
cp docker-compose.example.yml docker-compose.yml
# Edit:
#   - volumes: line → your host vault path (e.g. /srv/granit-vault)
#   - optionally uncomment the Traefik labels for auto-HTTPS
#   - optionally mount ~/.ssh:/home/granit/.ssh:ro for git push

# Vault must be writable by the container's non-root user (uid 100,
# gid 101 in the Alpine adduser pool):
sudo chown -R 100:101 /srv/granit-vault

docker compose up -d
docker compose logs -f granit  # watch first launch + bootstrap token
```

**Healthcheck** is wired against `GET /api/v1/health` (public, no
auth). Compose marks the service unhealthy if three consecutive 30-s
probes fail.

---

## Path 3 — FleetDeck (one command)

[FleetDeck](https://github.com/Artaeon/fleetdeck) auto-detects the
Go binary + this repo's Dockerfile, generates a Traefik-fronted
docker-compose, and wires up GitHub Actions CI/CD.

### Run detection first (no side-effects)

```bash
fleetdeck detect /home/rrl/Projects/granit
```

Expected output:
- **Type**: go
- **Docker**: yes  (because the repo ships its own Dockerfile)
- **Confidence**: 90%
- **Recommended profile**: `bare`

The detector will say "Framework: Echo, Port: 8080" — that's a false
positive on Echo (an indirect transitive dep), and the port heuristic
doesn't read the Dockerfile EXPOSE line. The actual binary is chi-
based on 8787; the Dockerfile EXPOSEs 8787 correctly so FleetDeck
will pick that up at deploy time.

### Deploy

```bash
fleetdeck deploy /home/rrl/Projects/granit \
  --server root@your-server.ip \
  --domain granit.example.com \
  --profile bare \
  --watch 5m \
  --watch-rollback
```

Flags worth knowing about:

| Flag | What it does |
|---|---|
| `--profile bare` | Pin the profile so detection drift doesn't change behavior between runs. |
| `--watch 5m` | After a successful deploy, probe `https://<domain>` for 5 min. Three consecutive failures trip rollback. |
| `--watch-rollback` | Auto-restore the pre-deploy snapshot on watch failure. Pairs with `--watch`. |
| `--insecure` | Skip SSH host-key verification — only for first-launch on a brand-new server. |
| `--migrate "<cmd>"` | Run a command in the container after deploy. Granit doesn't have migrations today; leave unset. |

### Post-deploy

1. Open `https://granit.example.com` and set the password.
2. Go to **Settings → AI provider** and paste your OpenAI / Anthropic /
   Ollama endpoint.
3. Optionally toggle modules off in **Settings → Modules** (e.g.
   disable `finance` or `chat` if you don't use them).
4. The first git auto-sync tick fires after one interval; check
   `docker compose logs -f granit` to confirm `level=INFO msg="git
   auto-sync running"`.

---

## What's NOT supported in production today

Be honest about the boundaries before you put this on the public
internet:

- **Single user only.** The auth model is one password + one bootstrap
  bearer token per vault. Adding a second user means giving them the
  same credentials.
- **No rate limit** on most endpoints. The login flow has a 250 ms
  per-failure delay; everything else is wide open. Front with a
  reverse proxy that does rate-limiting if the box is exposed.
- **No backup automation.** The vault is a git repo; pushing to GitHub
  / a self-hosted forge is your backup. The `.granit/` state files
  ride along with the vault, so a `git push` covers everything.
- **No audit log.** Edits are git commits with the same author the
  syncer pushes under. If you want per-user attribution, that'd need
  user-level auth first.

---

## Verification commands

After the first successful deploy, run these from a separate machine
to confirm everything's wired right:

```bash
# Health (public, no auth)
curl -s https://granit.example.com/api/v1/health

# Auth status (public, returns whether a password is set)
curl -s https://granit.example.com/api/v1/auth/status

# WS upgrade reachable (returns 400 'expected websocket')
curl -s -i https://granit.example.com/api/v1/ws

# Removed-endpoint sanity (should 404 with JSON, not the SPA HTML)
curl -s https://granit.example.com/api/v1/totallyfake
# Expect: {"error":"endpoint not found: /api/v1/totallyfake"}
```
