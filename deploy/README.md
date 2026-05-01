# Deploying `granit web`

Run granit's web UI on a server so any device can reach your vault from one URL.

## What you need

- A Linux box (any distro). VPS, home server, NAS — whatever you control.
- A domain pointed at it (e.g. `granit.example.com`). Optional for LAN-only.
- A git remote your vault is pushed to (GitHub, Gitea, self-hosted bare repo).
- SSH access to that remote from the server.

## Steps

### 1. Build granit

On a build machine (or directly on the server):

```bash
git clone https://github.com/artaeon/granit.git
cd granit
make build                            # produces ./bin/granit (with web embedded)
sudo cp bin/granit /usr/local/bin/
```

### 2. Create the service user + clone your vault

```bash
sudo useradd -r -s /bin/bash -m -d /var/lib/granit granit
sudo -u granit -i

# inside the granit user's shell:
ssh-keygen -t ed25519 -C "granit@server"
cat ~/.ssh/id_ed25519.pub                # add this to your git host as a deploy key

git clone git@github.com:yourname/granit-vault.git vault
exit
```

### 3. systemd service

Copy the unit:

```bash
sudo cp deploy/granit-web.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now granit-web
sudo systemctl status granit-web
```

It now binds `127.0.0.1:8787` and runs `git pull/commit/push` every 60s.

### 4. Reverse proxy + TLS (Caddy recommended)

```bash
sudo cp deploy/Caddyfile /etc/caddy/Caddyfile
# edit to put your real domain in place of granit.example.com
sudo systemctl reload caddy
```

Caddy will provision a Let's Encrypt cert automatically.

### 5. Get your bearer token

```bash
sudo cat /var/lib/granit/vault/.granit/everything-token
```

Visit `https://granit.example.com`, paste the token, sign in.

### 6. Add to home screen (PWA)

Open the URL in Safari / Chrome on your phone → share → **Add to Home Screen**. The app installs as a standalone, dark-themed icon.

## Running locally + on the server simultaneously

The vault is a git repo. Both your laptop's TUI session and the server's `granit web --sync` operate on the same data:

- **Local TUI edit** → `granit sync` (or autosync) commits & pushes → server pulls within `--sync-interval` (default 60s) → web reflects via WebSocket.
- **Web edit** → server commits & pushes → next time you `granit sync` locally, you pull the change.

There's only one canonical source of truth: the git history.

## Endpoints

- `GET /api/v1/sync` — last pull/push timestamps + counters
- `POST /api/v1/sync` — trigger an immediate sync run

## Updating the binary

```bash
cd granit && git pull
make build
sudo cp bin/granit /usr/local/bin/
sudo systemctl restart granit-web
```

## Notes

- The bearer token at `<vault>/.granit/everything-token` is auto-generated on first start. Delete it to rotate; it'll regenerate on next start.
- **Do not** put granit web on a public IP without the reverse-proxy + TLS. The token is the only auth — `https://` is non-negotiable.
- If your home network has dynamic IP, use [Tailscale](https://tailscale.com) — `granit web` on `localhost:8787` of any Tailnet machine becomes private-network-reachable from your phone.
