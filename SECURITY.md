# Security Policy

This document describes Granit's threat model, the protections it relies on,
and how to report vulnerabilities. It applies to the Go binary and the
embedded SvelteKit web app served by `granit web`.

---

## Threat model

### What Granit is

Granit is a **single-tenant, self-hosted** knowledge manager. The author and
the user are the same person. One running instance serves one human, against
one vault directory of plain Markdown files on the host filesystem.

The expected deployment shapes are:

- A laptop or workstation: bound to `localhost` or behind a firewall, used
  by the human sitting in front of it.
- A home server / NAS / VPS: reachable over LAN or VPN, optionally fronted
  by a reverse proxy that adds TLS and an extra access layer.

### What Granit is NOT

Granit is not a multi-tenant SaaS. It does not isolate users from each
other, partition vaults across accounts, or attempt to defend one user
from another sharing the same instance. A successful login is treated as
"the operator is here" and grants full access to the vault.

If you need a shared collaborative product for a team, Granit is not it.

### Attack surface

In the deployments above, the realistic adversaries are:

1. **A network attacker** on the same LAN, the public internet (if exposed),
   or the path to a reverse proxy. Mitigations: bearer-token auth on every
   API route, `argon2id` for password verification, no plaintext credentials
   on the wire, body-size cap, atomic file writes.
2. **A malicious or curious local user** on the same machine. Mitigations:
   sensitive state under `.granit/` is written `0600`, password hash uses
   per-record salt, session tokens are stored only as `sha256` digests on
   disk.
3. **Hostile content inside the vault** — a Markdown note crafted to
   trip a parser, a `.ics` subscription file with malicious entries, an
   embedded image with a path-traversal name. Mitigations: every vault
   path is validated to be inside the root, traversal components and
   absolute paths are rejected, the atomicio writer refuses to follow
   symlinks at the destination.
4. **Buggy or malicious clients** sending oversized or malformed requests.
   Mitigations: 4 MiB request body cap, JSON decoders, server-side schema
   validation, `Recoverer` middleware so a panicking handler can't bring
   down the process.

Out of scope:

- Physical access to an unlocked machine.
- Compromise of the host OS or another local process running as the same
  user.
- Vulnerabilities in third-party Go or npm dependencies — please report
  those upstream and let us know so we can pin / update.
- Denial-of-service via a dedicated attacker willing to saturate the
  process. Granit is not architected as a public-internet service.

---

## Authentication

`granit web` requires a bearer token on every API route under
`/api/v1/...` except the auth-bootstrap endpoints (`/auth/status`,
`/auth/setup`, `/auth/login`).

There are two valid token shapes:

1. **Session tokens** issued by `POST /auth/login`. 256 bits of entropy
   from `crypto/rand`, hex-encoded, handed to the client once. The server
   stores only `sha256(token)` on disk — a stolen `web-auth.json` alone
   cannot impersonate a real session.
2. **A legacy bootstrap token** generated on first launch and printed to
   stderr. CLI scripts and the desktop wrapper use this. The legacy
   token is compared with `crypto/subtle.ConstantTimeCompare` so an
   attacker cannot byte-grind the value out via timing.

Password storage and verification:

- **Algorithm:** `argon2id` (`golang.org/x/crypto/argon2`).
- **Parameters:** `m=64 MiB`, `t=1`, `p=4`, 16-byte random salt, 32-byte
  derived key. Encoded as `argon2id$v=N$m=N,t=N,p=N$<salt>$<hash>`.
- **Comparison:** `crypto/subtle.ConstantTimeCompare` after recomputing the
  derived key with the stored parameters.
- **Brute-force throttle:** every wrong password sleeps 250 ms before
  responding. Single-user threat model — explicit lockout / IP banning
  is not warranted.

Session lifecycle:

- Fresh login creates a new session. Multiple concurrent sessions are
  supported (web, mobile PWA, etc.) with optional per-device labels.
- Sessions expire after **60 days of inactivity**. The expiry sweeper
  runs on every authentication and on every `IsValidToken` call.
- `POST /auth/logout` revokes the calling session. `POST /auth/revoke-all`
  revokes every session, including the caller's — used as the
  "log out everywhere" safety button.
- A password change wipes every existing session as a deliberate side
  effect, kicking old devices off.

Relevant code:

- `internal/serveapi/auth.go` — bearer token middleware
- `internal/serveapi/auth_password.go` — argon2id, session store, sweeper
- `internal/serveapi/handlers_auth.go` — public + authed auth endpoints

---

## Network

### Default binding

- `granit web` defaults to `:8787`, which means **all interfaces**. This
  is the right default for the LAN / NAS deployment shape. For a
  laptop-only setup, pass `--addr 127.0.0.1:8787` so the port never
  leaves the host.
- `granit serve` (the read-only HTML preview of a vault) defaults to
  `localhost` and is intended for local browsing only.
- The Docker image's `CMD` binds `0.0.0.0:8787` so the container's
  exposed port works without extra flags. Lock down access at the
  Docker network / firewall layer.

### TLS

Granit does not terminate TLS itself. Run it behind a reverse proxy
(nginx, Caddy, Traefik, etc.) that performs:

- TLS termination with a real certificate (Let's Encrypt or your CA).
- Optional extra auth layers (Tailscale ACLs, Cloudflare Access, Basic
  Auth, mTLS) if you expose the service publicly.
- Logging, request limits, and rate limits beyond Granit's own caps.

The reference `docker-compose.example.yml` does not include TLS — pair
it with a proxy of your choice.

### CORS

CORS is closed by default. The `--dev` flag opens a permissive CORS
policy for `http://localhost:5173` and `http://127.0.0.1:5173` so the
Vite dev server can talk to the API. **Never run a production instance
with `--dev`.**

### Outbound traffic

By design, Granit does not phone home. The only outbound HTTP requests
are to user-configured endpoints, all triggered by an explicit user
action:

- **AI providers** — OpenAI (`api.openai.com`), Anthropic
  (`api.anthropic.com`), or a self-hosted Ollama endpoint. Only used
  when the user runs an agent, asks a chat question, or invokes an
  AI feature on a selection. See `internal/agentruntime/llm.go`.
- **Git remotes** — `granit web --sync` shells out to `git pull` /
  `git push` against the remote configured in the vault's
  `.git/config`.
- **ICS calendar subscriptions** — `.ics` files placed in
  `<vault>/calendars/` are read from disk; subscription URLs are not
  fetched by the server.

Anything else (telemetry, error reporting, update checks, package
analytics, fonts loaded from a CDN) is absent.

---

## Data at rest

### Vault contents

- Markdown files are written through `internal/atomicio.WriteNote`
  (`0644` for new files, mode preserved for existing files — `chmod 600
  secrets.md` survives an edit).
- The atomic writer:
  - opens the temp file with `O_EXCL | O_NOFOLLOW` so a stale or
    malicious symlink at the temp path cannot redirect the write,
  - uses a PID + nanosecond + atomic-counter suffix on the temp name
    to keep concurrent writers from colliding,
  - refuses to write through a symlink at the destination, and
  - renames over the destination atomically (POSIX same-filesystem).
- The `.granit-trash/` directory is the safety net for deletions —
  files are moved there rather than removed in place, so an
  accidental rm-via-API leaves a recoverable copy.

### Sidecar state

Anything Granit needs to remember outside the markdown lives under
`<vault>/.granit/`:

- `web-auth.json` — argon2id password hash + session records (token
  hashes only). Written `0600`.
- `everything-token` — the legacy bootstrap bearer token. Written
  `0600`.
- `tasks.json`, `events.json`, `goals.json`, `projects.json`,
  `ventures.json`, `hub.json`, `deadlines.json`, `prayer/`, `finance/`,
  `measurements/`, `virtues.json`, `shopping.json`, `vision.json`,
  `recurring.json`, `timetracker.json`, `modules.json`, `pinned.json`,
  `print-config.json`, `dashboard.json`, `scriptures.md`, etc. — all
  written through `atomicio.WriteState` (`0600` for new files,
  preserved for existing files).
- `history/` — per-note version snapshots used by the file-history
  panel. Same `0600` policy.

`.granit/` is created `0700`. Other vault files keep whatever permission
the user assigns; Granit will not silently downgrade them.

### Encryption

Granit does not encrypt the vault. Use full-disk encryption (FileVault,
LUKS, BitLocker) on the host, or filesystem-level encryption
(`fscrypt`, `gocryptfs`) on the vault directory if you need
data-at-rest protection beyond filesystem permissions.

---

## Vault path safety

Every API call that takes a path inside the vault validates it the same
way:

1. Reject empty paths.
2. Reject any path that contains a `..` segment (regardless of whether
   the segments would cancel out).
3. Reject any absolute path.
4. `filepath.Clean(filepath.Join(vault.Root, rel))` to normalise.
5. Confirm the cleaned absolute path equals the vault root or has
   `vault.Root + os.PathSeparator` as a prefix. Any deviation returns
   400 with `path escapes vault`.

The pattern lives in handlers like `handleGetFile` (binary passthrough),
`handleRenameNote` (move/rename), `handlePutNote` (write), and the
typed-object handlers. New handlers that touch the filesystem should
copy the same shape — see `internal/serveapi/handlers_files.go` for a
short reference implementation.

The atomicio writer enforces the second line of defence at the syscall
level: even a path that slipped past validation cannot be redirected
through a symlink because the temp open uses `O_NOFOLLOW` and the
destination Lstat rejects symlinks outright.

---

## Body size + parser limits

- Every request body is capped at **4 MiB** by the `maxBodyBytes`
  middleware. Notes can be large, but a multi-megabyte JSON payload is
  almost always a buggy client or an attempt to exhaust server memory.
- `ReadHeaderTimeout` is set to 10 seconds so a slowloris attempt
  doesn't tie up a goroutine.
- The chi router's `Recoverer` catches panics and returns 500 instead
  of crashing the process.

---

## Update channel

- Releases are published on **GitHub Releases** under the
  `artaeon/granit` repo. Tagged builds carry version, commit SHA, and
  build date in the binary (visible via `granit version`).
- There is **no auto-update** mechanism. The binary does not reach out
  to check for new versions. To upgrade, pull a new release archive (or
  rebuild from source) and restart the process.
- The Arch Linux AUR (`granit` and `granit-git`) and the Docker image
  (`ghcr.io/artaeon/granit`) follow the same upstream releases; they
  are not separate branches.

---

## Reporting a vulnerability

Please report security issues privately rather than opening a public
GitHub issue.

**Email:** `raphael.lugmayr@stoicera.com`

Include:

- A description of the issue and the affected code path / endpoint.
- Reproduction steps (a `curl` invocation or minimal vault is ideal).
- The version (`granit version`) and the deployment shape (binary,
  Docker, AUR).
- Your assessment of the impact, if you have one.

Expectations:

- **Acknowledgement** within 72 hours.
- **Initial assessment** within 7 days.
- **Coordinated disclosure**: a fix and an advisory go out together,
  typically within 30 days for high-severity issues. Reporter gets
  credit unless they opt out.

If the issue affects a third-party dependency, please also notify the
upstream project — Granit will pin / update on its own track once a
fix is available.

---

## Recent security-relevant work

Selected hardening visible in the changelog and the code under review:

- **argon2id password auth** with per-vault salt and constant-time
  comparison. Hash-only persistence of session tokens.
- **Atomic writes everywhere** via `internal/atomicio`. `O_EXCL +
  O_NOFOLLOW` on the temp file, symlink rejection at the destination,
  mode preservation across overwrites.
- **Vault containment** on every path-bearing handler — traversal
  rejection + cleaned-prefix check + `O_NOFOLLOW` at the syscall
  layer.
- **Request body cap** at 4 MiB.
- **CSRF posture:** the API uses bearer tokens out of `Authorization`
  headers (not cookies), so the classic browser-CSRF attack does not
  apply to the JSON endpoints.
- **Trash-on-delete** so an API-level delete leaves a recoverable
  copy under `.granit-trash/`.
- **Per-note version history** — restoring an older revision goes
  through the same auth + atomic-write paths as a normal save.
- **Session sweeper** so abandoned tokens age out after 60 days
  without an explicit logout.
- **Setup-once posture:** `/auth/setup` 409s after a password is set,
  forcing the change-password flow (which requires the current
  password) instead of letting an attacker silently rotate the
  credential.

---

## Defense-in-depth checklist for operators

If you are running `granit web` outside of a single laptop, consider:

- Bind to `127.0.0.1` and reach the service via SSH tunnel, Tailscale,
  or a reverse proxy on the same host.
- Terminate TLS at the proxy and force HTTPS for all browser sessions.
- Set a strong vault password and rotate it if the workstation is ever
  shared, lost, or compromised. Use `POST /auth/revoke-all` after
  rotation.
- Enable full-disk encryption on the host. The vault is plain text on
  disk by design.
- Back up the vault directory (the source of truth) on a schedule that
  matches your tolerance for loss. Git remotes via `--sync` are a
  convenient option but not a backup substitute on their own.
- Keep the binary updated. Subscribe to GitHub Releases for
  notifications.
