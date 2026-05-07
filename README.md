<p align="center">
  <pre align="center">
   ██████╗ ██████╗  █████╗ ███╗   ██╗██╗████████╗
  ██╔════╝ ██╔══██╗██╔══██╗████╗  ██║██║╚══██╔══╝
  ██║  ███╗██████╔╝███████║██╔██╗ ██║██║   ██║
  ██║   ██║██╔══██╗██╔══██║██║╚██╗██║██║   ██║
  ╚██████╔╝██║  ██║██║  ██║██║ ╚████║██║   ██║
   ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝╚═╝   ╚═╝
  </pre>
</p>

<p align="center">
  <strong>A self-hosted, single-tenant knowledge manager built on plain Markdown.</strong><br>
  <em>Notes, tasks, calendar, daily routines, finance, habits, and more — one binary, your filesystem, no telemetry.</em>
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue?style=flat-square" alt="License"></a>
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/Frontend-SvelteKit%205-FF3E00?style=flat-square&logo=svelte&logoColor=white" alt="SvelteKit">
  <img src="https://img.shields.io/badge/Platform-Linux%20%7C%20macOS-lightgrey?style=flat-square" alt="Platform">
</p>

<p align="center">
  <a href="#what-is-granit">What is Granit</a> &bull;
  <a href="#install">Install</a> &bull;
  <a href="#quick-start">Quick start</a> &bull;
  <a href="#features">Features</a> &bull;
  <a href="#architecture">Architecture</a> &bull;
  <a href="#documentation">Docs</a> &bull;
  <a href="#contributing">Contributing</a>
</p>

---

## What is Granit?

Granit is a knowledge management system you run on your own machine, against your own files.

- **Vault-first.** A vault is a directory of plain `.md` files. No proprietary database, no opaque storage. Open the same vault in Obsidian, Logseq, vim, VS Code — Granit only adds a `.granit/` sidecar folder for state that doesn't belong inside the markdown.
- **Single-tenant.** Granit assumes one person per running instance. There is no shared multi-tenant cloud, no team invites, no per-seat pricing. Run it on your laptop, on a NAS, on a VPS behind your own auth.
- **Self-hosted.** Ships as a single Go binary with the SvelteKit web app embedded via `go:embed`. `granit web -v <vault>` boots an HTTP + WebSocket server, sets a password on first launch, and serves the SPA from the same port.
- **Web + TUI.** A browser frontend for daily use on desktop and mobile, and a Bubble Tea TUI for terminal workflows. Both read and write the same files.
- **No telemetry. No analytics. No outbound calls.** Granit makes network requests only for features the user explicitly turns on (AI providers, git remotes, ICS subscription URLs).
- **Open source under MIT.** Read it, fork it, audit it, ship it. No CLA.

If you want a productivity stack that survives the next pricing change, the next acquihire, the next SaaS shutdown — your data lives in plain text in a folder you own, and the software that reads it is a few hundred kilobytes of Go and Svelte you can build yourself.

---

## Install

The recommended deployment is a single Go binary running `granit web` against a vault directory.

### From source

```bash
git clone https://github.com/artaeon/granit.git
cd granit
make build                       # builds the SvelteKit SPA + the Go binary
./bin/granit web ~/Vault         # serve on http://localhost:8787
```

`make build` does two things: it runs `pnpm build` inside `web/` to produce the static SPA, then `go build` embeds that SPA into the binary via `go:embed`. The result is a self-contained executable — no Node, no static-asset hosting at runtime.

### Go install (TUI only, no embedded web)

```bash
go install github.com/artaeon/granit/cmd/granit@latest
granit ~/Vault                   # opens the terminal UI
```

Note: `go install` skips the SPA build step. You'll have a working TUI but `granit web` will serve an empty SPA. Use `make build` if you want the web frontend.

### Docker

```bash
docker compose -f docker-compose.example.yml up -d
# vault bind-mounted from the host, exposed on :8787
```

See [`docker-compose.example.yml`](docker-compose.example.yml) for the reference deployment, including the optional `--sync` flag for git auto-pull/commit/push.

### Arch Linux (AUR)

A [`PKGBUILD`](PKGBUILD) is included in the repo. Build the AUR package against an upstream tarball or your local clone.

### Cross-compiled binaries

The included [`.goreleaser.yml`](.goreleaser.yml) produces `linux/amd64`, `linux/arm64`, `darwin/amd64`, and `darwin/arm64` archives.

### Prerequisites

| | |
|---|---|
| Go | 1.24+ (the Dockerfile uses 1.25) |
| Node + pnpm | only for `make build` (web SPA) |
| Git | optional, for `granit sync` and `--sync` |
| Pandoc / aspell / xclip / wl-clipboard | optional, used by specific features |
| Ollama or an OpenAI key | optional, for AI features |

---

## Quick start

```bash
# 1. Build (or use a prebuilt binary).
make build

# 2. Serve a vault.
./bin/granit web -v ~/Documents/Vault

# 3. Open http://localhost:8787 in your browser.
# First launch asks you to set a password. Subsequent launches log in
# with that password and issue a per-device session.

# 4. Or use the TUI against the same vault.
./bin/granit ~/Documents/Vault
```

The vault directory is just markdown. To start fresh:

```bash
./bin/granit init ~/NewVault
./bin/granit web -v ~/NewVault
```

Both surfaces operate on the same files. Edit a note in the TUI, and the web SPA receives a WebSocket fan-out and re-renders. Edit it in the browser, and the next TUI scan picks up the change. External edits (vim, another editor, `git pull`) are detected by an fsnotify watcher and broadcast the same way.

---

## Features

Grouped by surface area. Every entry below is backed by code in this repository — features that don't exist or no longer ship are not listed.

### Notes

- **CodeMirror 6 editor** with markdown highlighting, lazy-loaded Mermaid diagram rendering, footnotes, and rendered preview mode.
- **Slash commands** (`/`) — heading shortcuts, snippets, and selection-aware AI actions (rewrite, expand, summarize, improve, shorten, fix grammar) inline in the editor.
- **Wikilinks and embeds** — `[[Note]]`, block-level `[[Note#Heading]]`, and inline `![[path]]` note embeds.
- **Smart paste** — pasting a URL on a selection wraps it as a markdown link.
- **Markdown keyboard shortcuts** — bold/italic/link/heading shortcuts, floating selection toolbar.
- **Tag autocomplete and frontmatter helper UI** — pill-style tag editor and structured frontmatter form.
- **Reading time + sticky scroll position** per note.
- **Autolink suggestions** — surface candidate `[[wikilinks]]` based on note content.
- **Ask AI on selection** — selection → AI prompt with SSE streaming.
- **Extract to note** — pull a heading or selection into a new file with title/folder/tags picker.
- **Quick switcher** — `Mod-P` / `Ctrl-P` fuzzy file switch across the vault.
- **File history** — per-note version snapshots stored under `.granit/history/`, browse and restore from the editor sidebar.
- **PDF export** with a trust-stamp signature footer (English and German), optional corporate header/footer templates, multi-page rendering for long notes.

### Tasks

Tasks live as `- [ ]` lines inside your markdown files. Granit indexes them and gives you views.

- **List, kanban, triage, and review** views with smart due-groups (Today / Tomorrow / This week / Later / Overdue).
- **Bulk select + bulk actions** — select multiple tasks with `x`, batch reschedule, bulk complete.
- **Keyboard-first** — `j`/`k` to move, `x` to toggle, `e` to edit, drag-and-drop to reorder, quick-add bar with smart syntax (`#tag !p1 ~30m @friday`).
- **Recurring tasks** with a picker in the task detail panel.
- **Snooze** with TUI-aligned presets.
- **Property markers** (`#tag`, `@date`, `!priority`, `~estimate`) are stripped from displayed text but preserved in the source.
- **Per-project linking** — tasks under a `type: project` note inherit the project on the task.

### Calendar

- **Day, 3-day, week, month views.**
- **Drag-to-create** — click+drag on a time slot to create a task or event.
- **Drag-to-move and drag-to-resize** for both events and ICS-backed entries.
- **24-hour time inputs** throughout (no AM/PM picker).
- **ICS sync** — local writable calendars under `<vault>/calendars/`, read-only remote subscriptions, RRULE expansion (including `WEEKLY+BYDAY`).
- **All-day strip** — collapsible, multi-day events render only on their start date with a `+N more` overflow.

### Daily routines

- **Morning routine** wizard — scripture, prayer, top-of-day goal, task selection, habit toggles, free-form thoughts. Saves a structured block to today's daily note.
- **Daily examen** — evening companion that writes an `## Examen` block.
- **Daily jots** — quick timestamped bullets.
- **Carryover** — today's note inherits unfinished items from yesterday's.

### Habits and virtues

- **Habits** with view modes (List / Today / Week), sort, streaks, and per-day toggle on past dates.
- **Virtues tracker** — character-formation entity with weekly self-evaluation checks.
- **Habit ↔ virtue linkage** — habits can roll up into a virtue's progress.
- **Measurements** — numeric series (weight, mood, hours-deep-work, etc.) with timestamped entries.

### Goals, deadlines, ventures, projects

- **Goals** with milestones, status lifecycle (active / paused / completed / archived), and review log.
- **Deadlines** — top-level "this matters by date X" markers, separate from tasks and goals.
- **Ventures** — umbrella entity above goals and projects (e.g. a side business, a research thread). Optional description, mission, color.
- **Projects** with goal/milestone linkage, next-action chip, status lifecycle, and full CRUD.

### Knowledge tools

- **Typed objects** — notes can declare `type: person | book | project | goal | meeting | idea | article | podcast | video | quote | place | recipe | highlight | <custom>` in frontmatter, and Granit exposes them as galleries with type-aware indexing.
- **Object browser** — filterable gallery per type with preview pane.
- **Tags page** — every tag in the vault with note counts.
- **Backlinks and outgoing links** — per-note panel.
- **Bible reader + bookmarks** — embedded WEB (World English Bible, public domain) for daily verse, random passage, and bookmarked-verse persistence.
- **Scripture devotional flow** — verse of the day, "another one", one-shot devotional-note creator.

### Personal life

- **Prayer intentions** with a status lifecycle (praying → answered → archived) and optional links to projects/goals/ventures.
- **People** — lightweight CRM (last contact, upcoming birthdays, "ping" log).
- **Hub** — a single login link manager for the URLs you check daily; importer for browser bookmarks; drag-to-reorder cards.

### Productivity

- **Shopping list** with `standard` recurring-need flag, and a `/finance` rollup of planned vs. bought spend.
- **Finance** — net worth (accounts), recurring drag (subscriptions), income streams (active + planned), money goals. Deliberately scope-limited: this is a life-management tracker, not accounting software.
- **Time tracker** — clock in/out per task or project, persisted in `.granit/timetracker.json`.
- **Templates** — note templates with date variables.
- **Snippets** — slash-command snippet picker in the editor.
- **Saved dashboard layouts** — focus / morning / shutdown widget presets you can switch between.

### Vision and review

- **Vision** — life mission + values + season focus, single record per vault, anchored above goals on the dashboard.
- **Review** — daily and weekly review pages with goal/task/habit recap.
- **Stats** — basic vault metrics.

### AI

- **Ask AI on selection** — slash-menu and `Alt+/` actions: rewrite, expand, summarize, improve, shorten, fix grammar.
- **Streaming chat** — `/chat` page backed by an SSE endpoint.
- **Plan-my-day** — agent that proposes a time-blocked plan; dry-run preview, then user-confirmed apply that writes `scheduledStart` back to matched tasks.
- **Multi-step agent runner** — ReAct-style loop with a registered tool catalog (read_note, list_notes, search_vault, query_objects, query_tasks, get_today, write_note, create_task, create_object). Read/write split is enforced at the registry level — an agent without write tools registered cannot mutate disk.
- **Providers** — Ollama (local, default), OpenAI, with a graceful offline fallback.

### Privacy and integrity

- **Per-vault password auth** — argon2id, per-device sessions, "sign out everywhere" from the devices page. Bootstrap bearer token printed once for CLI scripts.
- **Atomic file writes** — every save goes through `internal/atomicio` (write to temp file in the same dir, fsync, rename) so a crash mid-write never produces a half-written note.
- **`.granit/*.json` sidecars** — state that doesn't belong inside the markdown (task ordering, dashboard layout, finance rollups, hub items) lives in JSON sidecars next to the vault, not in a hidden global database.
- **WebSocket fan-out** — file changes detected by fsnotify, broadcast on `/api/v1/ws` to connected clients so two browsers stay in sync without polling.
- **No telemetry. No analytics. No update pings.**
- **Plugin sandbox** — Lua plugins (`internal/plugins/`, `internal/tui/lua.go`) run with a 10-second execution timeout and path-escape detection. See [`docs/PLUGINS.md`](docs/PLUGINS.md).

### PWA

The web frontend ships a service worker (`web/src/service-worker.ts`) and manifest (`web/static/manifest.webmanifest`) so it installs as a PWA on desktop and mobile, with stale-while-revalidate caching and per-note draft persistence in `localStorage`.

---

## Screenshots

Asciicasts and screenshots of the web frontend live in `assets/`. The web SPA looks roughly like a minimal Notion / Capacities — left sidebar with module nav, page-level header, and a content surface that swaps per route. The TUI is built on Bubble Tea with overlays for each major surface (calendar, tasks, settings).

> Screenshots will be re-shot from the current web frontend; the GIFs in `assets/` predate the web rewrite.

---

## Architecture

```
                +---------------------------+
                |  granit (single binary)   |
                +-------------+-------------+
                              |
            +-----------------+-----------------+
            |                                   |
   +--------v--------+               +----------v----------+
   |   chi router    |               |     Bubble Tea TUI  |
   |  /api/v1/...    |               |   (internal/tui)    |
   |  WebSocket /ws  |               +----------+----------+
   |  embed SPA      |                          |
   +--------+--------+                          |
            |                                   |
            +-----------+   +-------------------+
                        |   |
                        v   v
                +---------------------+
                |    vault directory  |
                |   *.md (your notes) |
                |   .granit/*.json    |
                +---------------------+
```

- **Backend** — Go, [chi](https://github.com/go-chi/chi) router, atomic file writes, WebSocket fan-out via `internal/wshub`, fsnotify watcher, embedded SPA via `go:embed`. Module registry under `internal/modules/` lets a vault disable feature surfaces (sidebar nav + dashboard widgets + route guards) per-deployment.
- **Frontend** — SvelteKit 5 (runes), Tailwind CSS 4, CodeMirror 6, Marked, Mermaid. Static-adapter build, embedded into the Go binary; SPA fallback handled server-side so client-side routing works on hard refresh.
- **TUI** — Bubble Tea + Lip Gloss, Chroma syntax highlighting, Lua scripting bridge for plugins.
- **Storage** — plain markdown files in the vault directory, plus `.granit/*.json` sidecars for state (tasks ordering, finance, hub, vision, recurring rules, dashboard layout, etc.). Every write goes through `internal/atomicio` for crash safety.
- **Tasks** — parsed live from `- [ ]` lines in markdown, indexed at scan time, kept in sync via `tasks.Reload` on every relevant fs event.
- **Auth** — `internal/serveapi/auth*.go`: argon2id password, per-device sessions, bootstrap bearer token for scripts.
- **AI** — `internal/agentruntime` and `internal/agents`. Tool catalog registered separately from the runtime, so a read-only agent literally cannot write.

For a deeper tour, see [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).

---

## Documentation

| Doc | Topic |
|---|---|
| [docs/INSTALLATION.md](docs/INSTALLATION.md) | Build from source, system-wide install, cross-compile, optional deps |
| [docs/CONFIGURATION.md](docs/CONFIGURATION.md) | Global + per-vault config, every option with its default |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Codebase tour, package map, design decisions |
| [docs/FEATURES.md](docs/FEATURES.md) | Long-form feature reference with examples |
| [docs/WEB.md](docs/WEB.md) | Web frontend deployment, PWA, devices, sessions |
| [docs/AI-GUIDE.md](docs/AI-GUIDE.md) | Provider setup, model recommendations, troubleshooting |
| [docs/AGENTS.md](docs/AGENTS.md) | Multi-step agent runner — tool catalog, read/write split, write gating |
| [docs/OBJECTS.md](docs/OBJECTS.md) | Typed objects — declaring `type:`, galleries, vault overrides |
| [docs/PUBLISH.md](docs/PUBLISH.md) | `granit publish` — vault folder → static site |
| [docs/PLUGINS.md](docs/PLUGINS.md) | Lua plugin API, sandbox model |
| [docs/THEMES.md](docs/THEMES.md) | TUI theme catalogue + custom themes |
| [docs/KEYBINDINGS.md](docs/KEYBINDINGS.md) | Full keybinding reference |
| [docs/DEPLOY.md](docs/DEPLOY.md), [docs/DEPLOY-PRODUCTION.md](docs/DEPLOY-PRODUCTION.md) | Reverse-proxy, HTTPS, systemd |

---

## Contributing

Contributions are welcome — bug fixes, new features, docs, and themes. Before opening a PR, please read [`CONTRIBUTING.md`](CONTRIBUTING.md). The short version:

- One focused change per PR.
- Add tests for new behaviour. `go test ./...` must pass.
- Match existing patterns. Each feature has a self-contained Go package under `internal/<feature>/` and (where it has a UI) a self-contained Svelte route under `web/src/routes/<feature>/`.
- No unnecessary dependencies. Granit ships as a single binary — keep it that way.
- Don't break the vault format. Sidecars under `.granit/` are versioned in their JSON; markdown stays canonical.

---

## Security

If you find a vulnerability, please don't open a public issue. See [`SECURITY.md`](SECURITY.md) for the disclosure process and scope. Granit's threat model assumes the operator owns the host; the security boundaries that matter most are **path containment** (the vault path), **auth** (per-device sessions), and **AI write gating** (tool registry).

---

## License

MIT. See [`LICENSE`](LICENSE).

---

<p align="center">
  <sub>Plain markdown, your filesystem, your rules.</sub>
</p>
