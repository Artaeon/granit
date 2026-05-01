---
type: reference
tags: [meta, security]
created: 2026-04-01
---

# Where to find passwords

> This note **is not** a password store. It tells you where the real ones live.

## Primary

- **Bitwarden** — [vault.bitwarden.com](https://vault.bitwarden.com)
  - Master password: memorized
  - 2FA: hardware key (YubiKey)
  - Recovery codes: paper backup, see `[[Bootstrap]]`

## Service-specific

| Service | Where credentials live |
|---|---|
| Email | Bitwarden → "Personal email" |
| GitHub | Bitwarden + SSH key in `~/.ssh/` |
| Nextcloud | Bitwarden → "Cloud" |
| AWS | Bitwarden → "Work / cloud" (for `mealtime`) |

## API tokens & app secrets

- Granit web bearer token: `<vault>/.granit/everything-token` — auto-generated, regenerate anytime by deleting the file.
- OpenAI API key: Bitwarden → "API tokens"
- GitHub PAT: Bitwarden → "API tokens"

## On a new device

1. Install Bitwarden CLI: `bw login` then `bw unlock`
2. Pull SSH keys from secure notes
3. Run `granit open ~/Documents/Main`

> If you can't unlock Bitwarden: the recovery codes are in your "Bootstrap" workflow.
