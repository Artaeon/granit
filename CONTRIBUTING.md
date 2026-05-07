# Contributing to Granit

Welcome, and thank you for considering a contribution to **Granit** — a
self-hosted, single-tenant knowledge manager built on plain Markdown.
The codebase has two halves: a Go backend that owns the vault and the
HTTP/WebSocket API, and a SvelteKit web app that ships embedded inside
the Go binary. There is also a Bubble Tea terminal UI that operates on
the same vault. Contributions to any of those are welcome — bug fixes,
docs, new features, themes, plugins, performance work.

This guide will help you get oriented quickly.

---

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [How to Contribute](#how-to-contribute)
- [Code Style](#code-style)
- [Pull Request Process](#pull-request-process)
- [Architecture Quick Reference](#architecture-quick-reference)
- [Testing](#testing)
- [License](#license)

---

## Code of Conduct

This project is maintained by a small team and a growing community of
contributors. We ask that everyone:

- **Be respectful.** Treat others the way you would want to be treated.
  Disagreements are fine; personal attacks are not.
- **Be constructive.** When reviewing code or discussing ideas, focus
  on the work, not the person. Offer suggestions, not just criticism.
- **Be inclusive.** We welcome contributors of all experience levels and
  backgrounds. If someone is new, help them learn rather than dismissing
  their effort.
- **Assume good intent.** Most misunderstandings are exactly that —
  misunderstandings. Ask for clarification before assuming the worst.

Unacceptable behavior (harassment, trolling, spam) will result in
removal from the project. If you experience or witness such behavior,
please reach out to the maintainers directly.

---

## Getting Started

1. **Fork** the repository on GitHub:
   [github.com/artaeon/granit](https://github.com/artaeon/granit)

2. **Clone** your fork:

   ```bash
   git clone https://github.com/<your-username>/granit.git
   cd granit
   ```

3. **Build** the project. There are two paths depending on what you
   want to work on.

   For the **TUI only** (no web):

   ```bash
   go build ./cmd/granit/
   ./granit ~/your-vault           # opens the terminal UI
   ```

   For the **web app + Go backend** (the recommended development
   shape — both surfaces share the same vault):

   ```bash
   make web-setup                  # one-time: pnpm install in web/
   make build                      # builds the SPA + Go binary
   ./bin/granit web ~/your-vault   # serves on http://localhost:8787
   ```

4. **Verify** that everything passes:

   ```bash
   go vet ./...
   go test ./...
   ```

If your Go toolchain lives at a custom path, substitute that for `go`
in the commands above.

---

## Development Setup

### Prerequisites

| Requirement     | Version  | Notes                                             |
| --------------- | -------- | ------------------------------------------------- |
| Go              | 1.25+    | `go.mod` requires `1.25.0`; the Dockerfile pins   |
|                 |          | `golang:1.25-alpine`. Older 1.23 is no longer     |
|                 |          | supported.                                        |
| Node            | 22 LTS   | For building the web app. Used by `pnpm`.         |
| pnpm            | 9+       | Package manager for `web/`. `corepack enable`     |
|                 |          | gives you the right version.                      |
| Git             | recent   | For clone, sync, and the file-history feature.    |

The web app is **only** required if you want to build the embedded
SPA. `go build ./cmd/granit/` works without Node — the resulting
binary will serve an empty SPA when `granit web` is invoked, but the
TUI is fully functional.

### Project structure

```
cmd/granit/
  main.go                Entry point + subcommand dispatch.
  web.go                 `granit web`: HTTP API + embedded SPA.
  serve.go               `granit serve`: read-only HTML preview.
  publish.go             `granit publish`: static-site generator.
  ...                    (one file per subcommand)

internal/
  agentruntime/          TUI-free LLM runtime (Ollama / OpenAI / Anthropic).
  agents/                Agent presets + ReAct loop.
  atomicio/              Crash-safe file writes (O_EXCL, O_NOFOLLOW, ...).
  biblebookmarks/        Bible verse bookmarks.
  config/                JSON config: global ~/.config/granit/, per-vault .granit.json.
  daily/                 Daily-note utilities (template, EnsureDaily).
  deadlines/             Top-level "this matters by date X" markers.
  examen/                Daily examen records.
  finance/               Accounts, subscriptions, income, money goals.
  goals/                 Goals + milestones.
  granitmeta/            Read/Write helpers for JSON sidecars.
  habits/                Habit tracker + heatmap state.
  history/               Per-note version history.
  hub/                   Personal launch-pad (links + tools).
  icswriter/             Outbound-side .ics writing.
  measurements/          Numeric tracking series + entries.
  modules/               Module registry (feature toggles).
  objects/               Typed-object schema + index.
  people/                Lightweight relationship tracker.
  plugins/               Lua plugin system (TUI-side, currently).
  prayer/                Prayer intentions list.
  profiles/              Multi-profile support.
  publish/               Static-site generator internals.
  recurring/             Recurring tasks shared store.
  repos/                 Local git repos as typed-project notes.
  scripture/             Scripture loader + embedded WEB Bible.
  serveapi/              HTTP/WebSocket API + embedded SPA + auth.
  shopping/              Shopping list (single source for /finance run-rate).
  snippets/              Editor snippet definitions.
  tasks/                 Unified task store (TUI + web share it).
  templates/             Note template definitions.
  timetracker/           Clock-in/out + session history.
  tui/                   Bubble Tea TUI (40+ overlays).
  vault/                 Vault scanning, parsing, indexing.
  ventures/              Ventures (umbrella above projects/goals).
  virtues/               Character-formation tracker.
  vision/                Mission + values + season focus.
  wshub/                 WebSocket fan-out hub.

web/
  src/
    routes/              SvelteKit routes (one folder per top-level page).
      tasks/             /tasks
      calendar/          /calendar
      notes/             /notes + /notes/[...path]
      hub/               /hub
      finance/           /finance
      goals/             /goals
      habits/            /habits
      ...                (every page surfaced in the sidebar lives here)
    lib/
      api.ts             Typed fetch client over /api/v1/*.
      ws.ts              WebSocket client + event union.
      stores/            Svelte stores (auth, modules, theme, timer, ...).
      components/        Shared shell components (PageHeader, FAB, ...).
      editor/            CodeMirror 6 editor + extensions.
      tasks/             Task-page components.
      calendar/          Calendar components.
      notes/             Note components (history, print, ...).
      ...
    app.css              Tailwind 4 entry.
    service-worker.ts    PWA service worker.
  package.json           pnpm workspace entry.
  svelte.config.js       SvelteKit + adapter-static (dist → internal/serveapi/dist).

docs/
  ARCHITECTURE.md        Module layout + control flow.
  AGENTS.md              Agent runtime + presets.
  AI-GUIDE.md            AI features tour.
  CONFIGURATION.md       config.json reference.
  DEPLOY.md              Deploy recipes.
  DEPLOY-PRODUCTION.md   Production hardening.
  FEATURES.md            Feature inventory.
  INSTALLATION.md        Install instructions.
  KEYBINDINGS.md         Keyboard shortcuts.
  OBJECTS.md             Typed-object reference.
  PLUGINS.md             Plugin authoring.
  PUBLISH.md             granit publish reference.
  THEMES.md              Theme system.
  WEB.md                 granit web reference.

example-vault/           Demo vault with sample notes.
demo-vault/              Smaller demo vault used by tests.
deploy/                  Deployment artifacts (systemd unit, etc.).
desktop/                 Tauri desktop shell scaffolding.
aur/                     AUR package definition.
tapes/                   VHS tape files for terminal demo recordings.
vhs/                     More VHS tapes.
Makefile                 Build, install, test, web-build, web-dev.
PKGBUILD                 Arch Linux PKGBUILD.
Dockerfile               Multi-stage build (web + Go + alpine runtime).
.goreleaser.yml          Release build matrix.
docker-compose.example.yml  Reference Docker Compose deployment.
go.mod / go.sum          Go modules.
CHANGELOG.md             Release changelog.
ROADMAP.md               Forward-looking plan.
SECURITY.md              Security policy + threat model.
LICENSE                  MIT.
README.md                Project overview + quick start.
```

### Makefile targets

```bash
make build            # build SPA + Go binary into bin/granit
make web-setup        # pnpm install in web/ (one-time)
make web-build        # build SPA only (writes to internal/serveapi/dist)
make web-dev          # run Vite dev server (web/) — pair with `granit web --dev`
make web-serve VAULT=~/Vault  # boots `granit web --dev` AND vite dev together
make run ARGS=~/Vault # run the TUI directly via go run
make test             # go test ./...
make install          # build and copy bin/granit to ~/go/bin/
make build-all        # cross-compile linux+darwin+windows
make clean            # remove build artifacts
```

### Live web development loop

When iterating on the SvelteKit app, the fastest loop is:

```bash
make web-serve VAULT=~/your-vault
```

This boots the Go API on `:8787` with permissive dev CORS, then runs
the Vite dev server on `:5173`. Open `http://localhost:5173` for hot
module reload against the local API. On save, `make build` picks up
the latest SPA into the Go binary.

---

## How to Contribute

### Bug Reports

Found a bug? [Open an issue](https://github.com/artaeon/granit/issues/new?template=bug_report.md) with:

- **Steps to reproduce** — Numbered steps someone else can follow to
  trigger the issue.
- **Expected behavior** — What you expected to happen.
- **Actual behavior** — What actually happened (include error messages
  or stack traces if applicable).
- **Environment** — OS, browser (for web bugs) or terminal emulator
  (for TUI bugs), Granit version (`granit version`), and Go version
  (`go version`).
- **Screenshots / recordings** — If the issue is visual, a screenshot
  or short clip helps enormously.

### Feature Requests

Have an idea? [Open a feature request](https://github.com/artaeon/granit/issues/new?template=feature_request.md).
Focus on the **use case** rather than the feature itself. Explain:

- What problem are you trying to solve?
- How do you currently work around it (if at all)?
- Why would this benefit other Granit users?

A well-explained use case helps maintainers evaluate and prioritize the
request, even if the final implementation looks different from what
you originally proposed.

### Code Contributions

1. **Fork** the repository and create a feature branch from `main`:

   ```bash
   git checkout -b feat-my-feature
   ```

2. **Make your changes** in focused, logical commits.
3. **Test** your changes (see [Testing](#testing)).
4. **Push** to your fork:

   ```bash
   git push origin feat-my-feature
   ```

5. **Open a pull request** against `main`.

### Documentation Improvements

Documentation fixes — typos, clarifications, better examples — are
always welcome. No issue required; just open a PR.

### Theme Contributions

The TUI ships 38 built-in themes defined in `internal/tui/themes.go` as
`Theme` structs in the `builtinThemes` map. Each theme has 16 colour
roles (Primary / Secondary / Accent / Warning / Success / Error / Info
/ Text / Subtext / Dim / Surface0/1/2 / Base / Mantle / Crust). To add
one, drop another entry into `builtinThemes` and follow the existing
shape — it'll appear in the settings panel automatically.

The web app uses the user's system / browser dark-mode preference plus
a small set of CSS variables in `web/src/app.css`. A user-themable web
palette is on the roadmap but not yet implemented.

### Plugin Contributions

The TUI supports external plugins. Plugins live in:

- `~/.config/granit/plugins/<name>/` (global)
- `<vault>/.granit/plugins/<name>/` (per-vault)

Each plugin has a `plugin.json` manifest defining its name,
description, version, commands, and hooks (`on_save`, `on_open`,
`on_create`, `on_delete`). Scripts receive context via environment
variables (`GRANIT_NOTE_PATH`, `GRANIT_NOTE_NAME`, `GRANIT_VAULT_PATH`)
and note content via stdin. See `docs/PLUGINS.md` and the existing
plugins under `internal/plugins/` for examples.

The web app does not yet have a plugin system — see ROADMAP.md.

---

## Code Style

### Follow Existing Patterns

Granit has a consistent internal style. Before writing new code, read a
handful of existing files in the area you're working on to absorb the
patterns. The HTTP handlers in `internal/serveapi/handlers_*.go` and
the page routes under `web/src/routes/*` are good entry points.

### Go

- **Vault path validation** is paranoid by convention. Every handler
  that touches a vault path must reject `..` segments, absolute paths,
  and any cleaned-and-joined path that escapes the vault root. Copy
  the shape from `internal/serveapi/handlers_files.go` /
  `handleGetFile`.
- **Atomic writes**: never `os.WriteFile` a vault file directly. Use
  `internal/atomicio.WriteNote` for markdown (`0644`) and
  `atomicio.WriteState` for sidecars under `.granit/` (`0600`).
- **Comments belong on `why`, not `what`.** Short doc comments above
  exported symbols are required by `go vet`; longer block comments
  inside non-trivial functions are encouraged when there's a subtle
  reason for the choice.
- **Errors**: wrap with `fmt.Errorf("%s: %w", op, err)` so the chain
  survives. Don't print errors directly inside packages — return them
  and let `cmd/granit/` log.
- **Tests**: every handler in `serveapi` should have a sibling
  `handlers_<name>_test.go`. Reuse `setupTestServer` (or the closest
  equivalent in the package).

### TypeScript / Svelte

- **Svelte 5 runes** (`$state`, `$derived`, `$effect`, `$props`) are
  the new standard. Don't reach for `writable()` stores when a
  `$state` will do.
- **API calls** go through `lib/api.ts`. Add a typed wrapper there
  rather than calling `fetch` from a component.
- **WebSocket events** are typed in `lib/ws.ts` as a discriminated
  union. Add new event names there before subscribing.
- **Tailwind 4** for styling. The shared shell components
  (`PageHeader`, `Drawer`, `Toaster`, ...) are in
  `lib/components/` — reuse them so the layout stays uniform.
- **No hardcoded colours.** Colour tokens come from CSS variables
  defined in `app.css` so dark / light modes work uniformly.

### Dependencies

Keep dependency growth slow.

- The Go side currently depends on a handful of well-known modules:
  `chi/v5` (router), `fsnotify`, `bubbletea`, `lipgloss`, `chroma/v2`,
  `gopher-lua`, `golang.org/x/crypto` (argon2id), and the Wails desktop
  shell.
- The web side has CodeMirror 6, Marked, and Mermaid for note
  rendering, plus SvelteKit / Svelte 5 / Vite / Tailwind 4 for the
  framework. Adding anything new should be discussed in an issue
  first.

### Commits

Granit follows a **conventional-commits-style** prefix in the subject
line. Look at `git log` for the recent style:

```
feat(scope): short imperative summary
fix(scope): short imperative summary
fix+feat(scope): both
```

Common scopes: `tasks`, `calendar`, `notes`, `editor`, `hub`,
`history`, `print`, `signature`, `dashboard`, `extract-dialog`,
`event-detail`, `calendar-create`. Use whatever scope makes the diff
easy to find six months later.

- **Make small focused commits** rather than bundling unrelated work.
  Each commit should be a self-contained, coherent unit.
- **Subject line under 72 characters**, imperative mood ("Add X", not
  "Added X"), no trailing period.
- **Body** (if needed) explains the *why*, not the *what*. The diff
  shows the what.
- **Don't include AI-generation footer trailers** in commits.

### Before Submitting

Always run these and ensure they pass:

```bash
go vet ./...
go test ./...
go build ./cmd/granit/
```

If you touched the web app:

```bash
cd web
pnpm check
pnpm build
```

---

## Pull Request Process

1. **Fork** the repository and create a feature branch from `main`.
2. Make your changes in focused, logical commits.
3. Confirm that `go build ./...`, `go vet ./...`, and `go test ./...`
   all pass. If you touched the web app, also run `pnpm check` and
   `pnpm build` from `web/`.
4. Open a pull request against `main` with:
   - **A clear title and description** — explain what the change does
     and why.
   - **A link to the related issue**, if one exists.
   - **Screenshots or short clips** for any UI changes. Visual
     changes are much easier to review with a before/after
     comparison.
   - **A note about new dependencies or configuration options**, if
     any were introduced.
5. Ensure CI passes (build + vet + test).
6. Be responsive to review feedback. Small, focused PRs are easier to
   review and merge.

---

## Architecture Quick Reference

### Vault as source of truth

The vault directory holds plain `.md` files. Everything else — task
state, calendar events, goals, projects, hub items, finance, prayer,
virtues, sessions, the search index — lives in JSON sidecars under
`<vault>/.granit/`. There is no proprietary database. Open the same
vault in Obsidian, Logseq, vim, or VS Code and the markdown still
makes sense.

### Single-tenant single-process

`granit web` serves one vault per process. Auth is one password →
many session tokens. There is no multi-tenant isolation; a successful
login is "the operator is here" and grants full vault access. See
SECURITY.md for the full threat model.

### Two surfaces, one backend

The web app and the TUI share `internal/vault`, `internal/tasks`,
`internal/recurring`, `internal/agentruntime`, `internal/scripture`,
and friends. A change to a shared package shows up in both surfaces
on next launch. The TUI's `internal/tui/` package is intentionally
TUI-specific; the web app talks to the same underlying state through
HTTP handlers in `internal/serveapi/`.

### Auth flow

- First launch: legacy bearer token printed to stderr. Web UI loads,
  notices `hasPassword: false`, prompts for setup, calls `/auth/setup`.
- Subsequent launches: web UI calls `/auth/status`, sees `hasPassword:
  true`, renders login form, calls `/auth/login`.
- Sessions are 256-bit tokens; only their `sha256` is stored on disk.
- Bearer middleware accepts EITHER the legacy token OR a session
  token, so existing CLI scripts keep working after the user sets a
  password. See `internal/serveapi/auth.go`.

### WebSocket fan-out

`internal/wshub` is a small fan-out hub. Every connected client
subscribes to vault events (`note.changed`, `task.changed`,
`event.changed`, `agent.event`, `agent.complete`, `timer.started`,
`timer.stopped`). The HTTP handlers broadcast on every mutation, the
file watcher broadcasts on every external change. The web client
patches its in-memory state from those events so multiple devices
stay in sync without polling.

### Modules registry

Optional features (habits, finance, prayer, hub, virtues, ...) are
gated by `internal/modules`. The user toggles them in `/settings`,
the toggle is persisted to `.granit/modules.json`, and disabled
modules drop out of the sidebar / dashboard / nav. Core surfaces
(notes, tasks, calendar, settings) are always-on.

### AI provider abstraction

`internal/agentruntime` wraps the three supported providers (Ollama,
OpenAI, Anthropic) behind a `Chatter` interface. The active provider
is set via the `ai_provider` config key. AI features call the runtime
through the same shape regardless of which provider is active. The
TUI used to embed its own provider client; that path is now thin
wrapper around the same runtime.

---

## Testing

### Running Tests

```bash
go test ./...                    # run all tests
go test ./internal/serveapi/...  # run tests for a specific package
go test -run TestX ./...         # run a single test by name
go vet ./...                     # static analysis
```

### Where Test Files Go

Test files live alongside the code they cover, following Go
convention:

- `internal/vault/vault_test.go`
- `internal/serveapi/handlers_tasks_transform_test.go`
- `internal/serveapi/handlers_calendar_test.go`
- `internal/serveapi/morning_test.go`
- `internal/atomicio/atomicio_test.go`

### Testing Conventions

- New logic in `internal/vault`, `internal/tasks`, `internal/serveapi`,
  and the storage packages (`internal/finance`, `internal/habits`,
  `internal/measurements`, etc.) must come with tests.
- HTTP handlers should be tested through the chi router with a
  per-test temp vault. See `handlers_dashboard_test.go` and
  `handlers_calendar_test.go` for the established pattern.
- Test function names should be descriptive: `TestParseWikiLink`,
  `TestICSExpandWeekly`, `TestAuthSessionExpiry`, etc.
- For TUI overlay code, at minimum ensure that:
  - `go build ./...` succeeds with no errors.
  - `go vet ./...` reports no issues.
  - Manual smoke-testing covers the happy path of your feature.

### Web testing

End-to-end tests for the web app are not yet established. The
maintainer's test loop is:

1. `pnpm check` for type-checking.
2. `make web-serve` for live development against a real vault.
3. Manual smoke testing across the affected pages.

A Playwright / Vitest harness is on the roadmap; contributions are
welcome.

---

## License

Granit is licensed under the [MIT License](LICENSE). By submitting a
contribution (code, documentation, themes, plugins, or otherwise), you
agree that your contribution will be licensed under the same MIT
License that covers the project.

---

Thank you for helping make Granit better.
