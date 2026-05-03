# Deploying granit to a multi-tenant Docker server

This is the safe-deploy guide for a server already running other
Docker SaaS apps. It assumes:

- You're root / have sudo on `178.63.9.183`
- Docker + Docker Compose are installed (granit needs both)
- You have a reverse proxy already terminating HTTPS for your other
  services — Traefik, nginx-proxy, Caddy, or similar
- DNS for `granit.raphaellugmayr.at` will point to that server

The plan touches only:
- One new directory (your choice — `/opt/granit` is suggested)
- One new Docker network attachment (your existing reverse-proxy network)
- One new container, one new volume

**Nothing else on your server is modified.** No global config, no
firewall, no other containers, no shared volumes.

---

## Step 1 — Read-only inventory

Run these on the server. They only read, never modify. Paste the
output back to me so I can tailor the compose file to your setup.

```bash
# What reverse proxy is running, and on which Docker network?
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}' | head -30

# Which Docker networks exist? Your reverse proxy is usually on one
# of these (often called "traefik", "proxy", "web", or similar).
docker network ls

# Which ports are bound on the host? Want to be sure 8787 is free
# (we won't bind it publicly anyway, but useful sanity check).
ss -tlnp 2>/dev/null | grep LISTEN | head -20

# Disk + memory headroom — granit web is light (~30MB image, <50MB
# RSS at rest), but verify nothing's already squeezed.
df -h /
free -h

# Docker version (compose v2 is required — compose v1 won't read
# the file format we're using).
docker version --format '{{.Server.Version}}'
docker compose version 2>/dev/null || docker-compose version
```

Three numbers I specifically need from the output:
1. **Reverse proxy** — which container is fronting your other apps?
2. **Reverse proxy network name** — which `docker network ls` entry
   does that container join?
3. **DNS / TLS** — is your reverse proxy doing automatic Let's Encrypt
   (Traefik default), or do you provision certs manually?

---

## Step 2 — Pull the granit source onto the server

```bash
# Pick a directory. /opt/granit is the convention; anything works.
sudo mkdir -p /opt/granit
sudo chown $USER:$USER /opt/granit
cd /opt/granit

# Clone the repo. Replace the URL if your mirror is elsewhere —
# the Dockerfile + everything else is in this tree.
git clone https://github.com/Artaeon/granit.git .
```

If the repo isn't pushed to a public remote yet, alternative is to
build the image locally (on this dev machine) and load it on the
server. Tell me which you prefer.

---

## Step 3 — Vault directory

The vault is where granit stores everything. Three options:

| Option | When |
|---|---|
| **A. Bind a host directory.** Easiest. `mkdir /srv/granit-vault && cd /srv/granit-vault && git init` | You don't already have a vault on this server |
| **B. Clone an existing vault repo.** `git clone <vault-remote> /srv/granit-vault` | You have a vault repo (GitHub, Gitea, etc) | 
| **C. Bind your existing vault on the host.** `chown -R 100:101 /path/to/your/vault` | The vault is already on this server |

Whichever you pick, the directory must be **writable by uid 100, gid
101** (the granit user inside the alpine container):

```bash
sudo chown -R 100:101 /srv/granit-vault
```

If you skip this, the container will fail health checks because it
can't write to the vault. The chown only affects the vault dir —
nothing else on the server.

---

## Step 4 — Tailored compose file (I'll send this once you reply with the inventory)

The repo ships `docker-compose.example.yml` as a starting point but
the production version needs your reverse-proxy labels (Traefik) or
`expose:` line (nginx-proxy), the right network attachment, and your
domain wired in. Once you reply with the inventory output I'll send a
finished compose with:

- The exact image build flags
- A unique container name (`granit-web`) so it doesn't collide with
  anything else
- Your reverse-proxy network attachment + labels for `granit.raphaellugmayr.at`
- A non-public port binding (only the reverse proxy reaches granit;
  port 8787 is **not** opened on the host)
- A bind mount to `/srv/granit-vault` (or wherever you chose)
- A healthcheck against `/api/v1/health`
- Restart policy `unless-stopped`

You'll review and run `docker compose up -d` in `/opt/granit`.

---

## Step 5 — DNS

Point `granit.raphaellugmayr.at` at `178.63.9.183`. A simple A record
is enough:

```
granit  A  178.63.9.183  3600
```

Do this **before** the first `docker compose up -d` if your reverse
proxy uses Let's Encrypt — the cert challenge happens on first request.

---

## Step 6 — First launch

```bash
cd /opt/granit
docker compose up -d
docker compose logs -f granit
```

The first launch prints a bootstrap bearer token. Note it down — it's
also stored at `/srv/granit-vault/.granit/everything-token` for the
CLI / Tauri desktop. Then open `https://granit.raphaellugmayr.at` and
**set a password** — that becomes your normal login.

---

## Rollback

If anything breaks:

```bash
cd /opt/granit
docker compose down  # stops only the granit container
# Optional: delete the deploy dir entirely — vault data lives at
# /srv/granit-vault, which is untouched
```

Nothing else on the server is affected by `docker compose down` —
granit is a single container in its own compose project.

---

## What I won't touch on your server

- No edits to existing containers
- No edits to your reverse-proxy config (you add the labels yourself
  via the compose file we draft together)
- No package installs (Docker is already there)
- No firewall changes
- No shared-volume changes
- No DNS changes (you do the A-record yourself)

If anything in the inventory looks weird, we stop and discuss before
running anything that writes to disk.
