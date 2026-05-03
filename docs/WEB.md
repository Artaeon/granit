# Web companion (`granit web`)

Granit ships a self-hosted web frontend that wraps the same vault, task
store, and daily-note pipeline the TUI uses. It is a single binary —
the SvelteKit SPA is embedded — so deploying it means copying one file
to a server and running it next to your vault.

## Quick start

```bash
# 1. From the granit checkout, build everything (frontend + binary):
make build

# 2. Boot it against a vault:
./bin/granit web /path/to/your/vault

# 3. Open http://localhost:8787 — the first paint asks you to set a password.
```

On first launch you'll be prompted to **set a password**. After that:

- Web UI logs in with the password and stores a per-device session token
  in `localStorage`. Sessions live for 60 days of inactivity.
- A bootstrap **bearer token** is also printed at startup. It is the
  legacy/CLI auth path — useful for `curl` scripts and the Tauri desktop
  wrapper. The web UI's "Sign in with bearer token" link drops you into
  that path if you ever need it.

The bootstrap token is stored at `<vault>/.granit/everything-token`.
The password hash + active sessions live at `<vault>/.granit/web-auth.json`
(argon2id, 64 MiB / 1 iteration / 4 threads).

## Flags

```text
granit web [--addr :8787] [--dev] [--sync] [--sync-interval 1m] [vault-path]
```

- `--addr` — listen address. Default `:8787` (`PORT` env overrides).
- `--dev` — relaxes CORS so a Vite dev server on `:5173` can hit the API.
- `--sync` — turn on the git auto-sync loop. The server runs
  `git pull` + auto-commit/push on `--sync-interval` (min 10s).

The vault path defaults to `.` (current directory).

## What's in the web app

| Surface       | Notes                                                                                                                                            |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| Dashboard     | Customizable widgets (time, streaks, today's tasks, scheduled, goals, projects, inbox, calendar week, recent notes, quick capture).              |
| Tasks         | Seven views: list, kanban, **inbox / triage / quick wins / stale / review**. Triage state cycle, snooze with presets, bulk actions, side-panel detail. |
| Calendar      | Six views: day / 3-day / week / month / year / agenda. Click+drag to create. Resize event chips. Per-source ICS toggles synced with TUI's `disabled_calendars`. |
| Notes         | CodeMirror-based editor with wikilink autocomplete, frontmatter editor, outline + backlinks, markdown preview, three view modes. Per-note draft persistence. |
| Daily note    | Inline quick-add for tasks/events at the top of every daily note. Shorthand parsed (`!1 due:YYYY-MM-DD #tag`).                                   |
| Projects      | Full CRUD with goals + milestones (nested progress), next-action chip, linked tasks, status lifecycle, color picker.                              |
| Habits        | Streak overview + completion toggles.                                                                                                            |
| Goals         | Read view of `.granit/goals.json`.                                                                                                               |
| Agents        | Two-tab page. Presets gallery (built-in + vault-local) with one-click **Run** that streams the ReAct transcript live via WS. Run history (any note with `type: agent_run` frontmatter). Per-preset stats. |
| Scripture     | Verse-of-the-day, "another one" random pick, **Memorize** mode (cloze drill with per-verse accuracy tracking), Browse the full library. AI-generated reflection via the `devotional` preset. Reads `.granit/scriptures.md` — same file the TUI uses. |
| Chat          | Multi-turn conversation with the configured AI provider. History in localStorage; "save as note" writes the conversation to `Chats/`. Optional vault-context attachment (named note's body fed as a system message). |
| Settings      | Theme, security (password change / "log out everywhere"), **devices** (active sessions, revoke per-row), git sync status, vault info.            |
| Morning       | Mirrors the TUI's morning-startup wizard.                                                                                                        |
| Templates     | Browse built-in + vault templates, create note from one.                                                                                         |

## Sharing data with the TUI

The web frontend reads and writes the same on-disk artifacts as the TUI:

- Notes — markdown files in the vault.
- Tasks — `<vault>/.granit/tasks-meta.json` plus the markdown checkboxes.
- Pinned — `<vault>/.granit/sidebar-pinned.json`.
- Events — `<vault>/.granit/events.json`.
- Projects — `<vault>/.granit/projects.json`.
- Goals — `<vault>/.granit/goals.json`.
- Habits — `<vault>/.granit/habits-times.json`.
- Calendar source toggles — `disabled_calendars` in `~/.config/granit/config.json`
  or `<vault>/.granit.json`.
- Agent presets — `<vault>/.granit/agents/<id>.json`.

Edits made in the web propagate to the TUI immediately on its next read,
and the WebSocket fanout pushes vault file events to connected web
clients live.

## API

Everything the UI does goes through `/api/v1/*`. A few highlights:

| Endpoint                              | Method     | Purpose                                            |
| ------------------------------------- | ---------- | -------------------------------------------------- |
| `/auth/status` `/auth/login` `/auth/setup` `/auth/logout` `/auth/change-password` `/auth/revoke-all` | various    | Password auth + session management.                |
| `/notes` `/notes/{path}`              | GET/POST/PUT | List, read, write notes.                          |
| `/tasks` `/tasks/{id}`                | GET/POST/PATCH | List, create, update tasks (incl. triage, snooze, recurrence, notes). |
| `/calendar`                           | GET        | Unified feed: daily notes, due tasks, scheduled tasks, events, ICS. |
| `/calendar/sources`                   | GET/PATCH  | List `.ics` files; toggle disabled.                |
| `/events` `/events/{id}`              | GET/POST/PATCH/DELETE | `events.json` CRUD.                    |
| `/projects` `/projects/{name}`        | GET/POST/PATCH/DELETE | Full project lifecycle.                |
| `/agents/presets` `/agents/runs`      | GET        | Agent catalog and run history.                     |
| `/agents/run`                         | POST       | Kick off an agent run server-side. Returns runId; events stream via WS frames `agent.event` / `agent.complete`. |
| `/chat`                               | POST       | Multi-turn chat with the configured LLM. Optional `notePath` attaches the note's body as system context. |
| `/scripture` `/scripture/today` `/scripture/random` | GET | Verse library, daily verse, random pick.            |
| `/devotionals`                        | POST       | Create a devotional note pre-seeded with a verse.   |
| `/daily/context`                      | GET        | Carryover (yesterday's open tasks) + today's habits checklist for the daily-note band. |
| `/config`                             | GET/PATCH  | Curated config.json read/write — AI provider, keys, daily folder + recurring, theme, editor toggles. |
| `/recurring`                          | GET/PUT    | Daily/weekly/monthly auto-creating task rules. Server fires due rules at midnight. |
| `/timetracker` `/timetracker/start` `/timetracker/stop` | GET/POST | Clock-in/out + session history. WS frames `timer.started` / `timer.stopped` for live cross-device pill updates. |
| `/devices` `/devices/{id}`            | GET/DELETE | Active session list, revoke per-device.            |
| `/dashboard`                          | GET/PUT    | Per-vault widget config.                            |
| `/sync`                               | GET/POST   | Git auto-sync status / manual trigger.             |

All authed endpoints accept either:

1. The bootstrap bearer token (`<vault>/.granit/everything-token`), or
2. A session token from a successful `POST /auth/login`.

## AI: server-side agents

`granit web` reads the same `~/.config/granit/config.json` the TUI uses,
so the AI provider, key, and model settings are shared across both
surfaces. Setting up the TUI's AI once works on the web automatically.

Supported providers (via `internal/agentruntime`):

- **OpenAI** — set `ai_provider: "openai"`, `openai_key`, `openai_model`
  (defaults to `gpt-4o-mini`).
- **Ollama / local** — set `ai_provider: "ollama"`, `ollama_url`
  (defaults to `http://localhost:11434`), `ollama_model`. Free, runs
  on your hardware, slower.

Built-in presets (visible on `/agents`):

| Preset | What it does |
|---|---|
| `research-synthesizer` | Topic → finds related notes, synthesises patterns + open questions. Read-only. |
| `project-manager`      | Project name → status, blockers, next-actions report. Read-only. |
| `inbox-triager`        | Reviews recent captures, proposes one next-action task per item. Writes tasks. |
| `plan-my-day`          | Reads today's calendar + open tasks + project next-actions, writes a `## Plan` time-block schedule into today's daily note. Writes notes. |
| `devotional`           | Verse + citation → 200-300 word reflection in `Devotionals/{date}-{slug}.md`. Triggered from `/scripture`. Writes notes. |

The daily note has a one-click **"Plan my day"** button that fires
`plan-my-day` and inserts the result. Other presets get their Run button
on the `/agents` page; the transcript streams live in a side panel.

Vault-local overrides at `<vault>/.granit/agents/<id>.json` shadow built-
ins by ID. Custom prompts work the same in both surfaces.

## Offline + PWA

The web app installs as a PWA. The service worker uses
**stale-while-revalidate** for `GET /api/v1/*`, so a brief network outage
doesn't blank the UI — you keep the last-known data and edits queue for
when the connection returns. Note edits also persist to localStorage as
**drafts**, surviving tab close, reload, and full power loss.

## Deployment — server, Docker, FleetDeck

`granit web` is a single Go binary with the SvelteKit SPA embedded via
`go:embed`, so deploying it is "copy one file to a server, run it next
to a vault." Three supported paths in increasing automation:

### 1. Bare binary on a Linux box

```bash
# On the server:
GOOS=linux GOARCH=amd64 go install github.com/artaeon/granit/cmd/granit@latest
git clone git@github.com:you/your-vault.git /srv/granit-vault
granit web --addr 0.0.0.0:8787 --sync --sync-interval 60s /srv/granit-vault
```

`--sync` runs `git pull --rebase --autostash` + auto-commit + push on
every interval (min 10 s). A TUI commit pushed locally lands on the
server within ~one tick; web-side writes commit + push back. The vault
stays a normal git repo — you can `cd` in and run git commands by hand.

### 2. Docker / docker-compose

The repo ships a multi-stage `Dockerfile` (Node → Go → Alpine) and a
`docker-compose.example.yml` template:

```bash
cp docker-compose.example.yml docker-compose.yml
# edit volumes: line for your vault path, optionally uncomment the
# Traefik labels block for auto-HTTPS.
docker compose up -d
```

The container runs as a non-root user; chown the host vault directory
(`sudo chown -R 100:101 /srv/granit-vault`) so the Alpine `granit`
user can write. Mount `~/.ssh` read-only into `/home/granit/.ssh` if
you want git auto-sync over SSH.

### 3. FleetDeck

[FleetDeck](https://github.com/Artaeon/fleetdeck) auto-detects Go via
`go.mod` and assigns the `server` profile (single binary, exposes a
port, has a health endpoint at `/api/v1/health`). One command takes
this repo from local to a Traefik-fronted production deployment with
GitHub Actions CI/CD wired up:

```bash
fleetdeck deploy . --server root@your-server.ip --domain granit.example.com --profile server
```

The provided `Dockerfile` is what FleetDeck will pick up; if you let
it generate its own, regenerate after every granit upgrade so the Node
+ Go versions stay aligned with this repo's pinned versions (Node 22,
Go 1.25 currently).

### Things to think about before going public

- **Bootstrap token + password**: first launch prints the bootstrap
  bearer token and asks the web UI to set a password. The token lives
  at `<vault>/.granit/everything-token` — the same file the TUI uses;
  treat it like a password.
- **Vault on the server vs. the desktop**: with `--sync`, the server
  is just another peer in the git replication. Your TUI on a laptop
  pushes; the server pulls within ~one interval; the web UI sees the
  changes via the file watcher → WS broadcast. Same the other way.
- **HTTPS**: the binary speaks plain HTTP on 8787. Front it with
  Traefik / nginx / Caddy / fleetdeck for TLS + a real domain.
