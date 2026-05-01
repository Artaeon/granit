---
type: bootstrap
tags: [meta, setup]
created: 2026-04-01
---

# Bootstrap — Setting up a new device

> The single page you open from `granit web` on a fresh laptop, and 60 seconds later you have everything.

## Cloud sync

- **Vault git remote:** `git@github.com:yourname/granit-vault.git`
- **Nextcloud:** `https://cloud.example.com` — your account email
- **First steps on a new machine:**
  1. Install granit (see Install widget on the dashboard)
  2. `git clone <vault-remote> ~/Documents/Main`
  3. `granit open ~/Documents/Main`
  4. `granit web ~/Documents/Main` (optional — to host the web UI)

## Accounts

- **Password manager:** [Bitwarden](https://vault.bitwarden.com)
- **Email:** `you@example.com`
- **GitHub:** [github.com/yourname](https://github.com)

> Don't store actual passwords here — store *where to find them*. Real secrets stay in Bitwarden / 1Password / a hardware key.

## Recovery

- Bitwarden recovery codes: paper printout, top-left desk drawer.
- Nextcloud 2FA backup codes: see `[[Recovery]]` (encrypted note).
- SSH key: `~/.ssh/id_ed25519` — backed up via your password manager's secure notes.

## Daily routine

See `[[Daily Notes]]` for the daily-note workflow.
Habits live in the daily note's `## Habits` section — see `[[Habits]]`.

## Useful links

- [[Reading List]]
- [[Projects/]]
- [[Architecture]]
