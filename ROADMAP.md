# Granit Roadmap

Granit is under active development. The codebase is changing fast and the
"unreleased" section of [`CHANGELOG.md`](CHANGELOG.md) is the most accurate
historical record. This roadmap is a higher-level view: what was recently
shipped, what is in flight now, and what the next bands of work look like.

If something on this page interests you, see
[`CONTRIBUTING.md`](CONTRIBUTING.md) — Granit is open to contributors and
none of these items are reserved for the maintainer.

---

## Recently shipped

These have all landed on `main`. Roughly chronological, newest first.

### Editor + notes

- File history per note with one-click restore (UI + backend, version browser).
- Print preview overhaul: multi-page printing, real signature footer with
  signer / purpose / document ID, document IDs, dark-mode rendering fix,
  24h timestamp format.
- Selection toolbar with bold / italic / link / heading shortcuts.
- Slash command palette for blocks (`/h1`, `/quote`, `/code`, etc.).
- Markdown keyboard shortcuts: `Mod-B`, `Mod-I`, `Mod-K` for link,
  `Mod-Alt-1..6` for headings, `Mod-Shift-7` for bullet.
- Smart paste for URLs (auto-link with selected text as title).
- Reading time + sticky scroll position per note.
- Autolink-suggest extension (suggest existing notes when typing).
- Block-level wikilinks (`[[Note#Heading]]`) and inline embeds
  (`![[path]]`).
- Mermaid diagram rendering (lazy-loaded).
- Frontmatter helper UI.
- Selection → Ask AI (`Mod-Shift-A`) and Selection → Extract note
  (`Mod-Shift-X`) with editable title and folder picker.
- Snippets, tag autocomplete, footnotes.

### Tasks

- Kanban view with drag-reorder.
- Smart due-groups + visual urgency polish + at-a-glance stats chips.
- Bulk actions, keyboard shortcuts (`j`/`k`/`x`/`d`/`e`/`p`), inline edit.
- Quick-add bar with smart syntax.
- Recurring-task picker in task detail.
- Stripped property markers from displayed text.

### Calendar

- Drag-to-move and drag-to-resize events (works for both Granit-native
  events and writable ICS calendars).
- Real 24h time inputs throughout (no AM/PM).
- Multi-day ICS all-day events render only on start date.
- Collapsible all-day strip with "+N more" overflow.
- `WEEKLY+BYDAY` rrule expansion for recurring events.

### Life-management modules

- **Hub** — personal launch pad. Schema, storage, CRUD handlers, /hub
  page with add/edit/reorder, drag-reorder, favicons, last-visited
  tracking, browser-bookmark import.
- **Ventures** — first-class entity above projects/goals, dedicated
  detail page at `/ventures/[name]`.
- **Daily examen** — backend handler, module gate, /examen wizard.
- **Virtues tracker** — schema, weekly check entries, /virtues page,
  Weekly Review integration, habit-virtue linkage.
- **Shopping list** — backend, /shopping page, finance integration
  (run-rate from recurring "standards"), mobile + desktop polish.
- **Habits** — view modes (List / Today / Week), sort, weekly insight,
  add-from-UI, deduplication, leakage-into-tasks fix.
- **Prayer** — link intentions to projects/goals/ventures, dedicated
  /prayer page, dashboard widget.
- **Bible** — full embedded WEB Bible reader with search, bookmarks,
  random passage; daily scripture + AI devotional reflection.
- **Goals** — target-date urgency tinting, next-target hero card.
- **Deadlines** — page UI/UX polish.
- **Morning routine** — added prayer step.
- **Quick capture** — global FAB on every page.

### Dashboard

- Today-at-a-glance hero widget.
- Saved layout presets (focus / morning / shutdown).
- Ventures + Prayer + Quick-links widgets.
- Layout polish.

### AI

- Streaming SSE responses on `/chat`.
- Selection → Ask AI inline edits with diff preview.
- `plan-my-day` agent with dry-run + apply phases (proposes scheduled
  starts; user edits before commit).
- Anthropic Claude as a first-class provider alongside OpenAI + Ollama.
- Live Ollama model list in settings.
- Per-preset model override.

### Web infrastructure

- Quick switcher (`Mod-P`).
- Service-worker update banner with visibility-aware refresh.
- Per-vault print defaults persisted at `.granit/print-config.json`.
- Page-header consistency pass (every route uses `PageHeader`).
- Auto-save: pause while autocomplete picker is open; preserve cursor
  across external value-dispatch.

### Auth + sync

- argon2id password login with per-device session labels.
- 60-day inactive session expiry.
- "Log out everywhere" endpoint.
- Optional `--sync` flag for git auto-pull/commit/push on a tick.

---

## In progress

Active branches and items the maintainer is currently working on.

- **Dashboard / Bible / notes parity** across surfaces. Different
  pages still use slightly different conventions for empty states,
  loading states, and shortcuts; the goal is one consistent shape.
- **Deadlines cross-linking** improvements — link deadlines to the
  goal / project / venture / note that owns them, surface them in
  more places.
- **Mobile UX** — the layout works on phones but several interactions
  (drag-to-move, kanban reorder, multi-select) need touch-tuned
  alternatives.
- **Tauri desktop wrapper** — packaging the existing web app as a
  native desktop binary so users get a real app icon and offline-first
  shell. Looking for contributors — open an issue to propose.

---

## Next up

Items the maintainer plans to tackle next, in rough priority order.
None of them are reserved — pick one and open a PR.

### Editor + notes

- Inline math / LaTeX rendering (KaTeX or similar).
- Footnote-style aside blocks.
- Outline / table-of-contents panel for long notes.
- Better diff rendering in the file-history view.

### Tasks + calendar

- Natural-language quick-add ("call dad next tuesday at 5pm").
- Recurring tasks with skip-this-occurrence.
- Calendar overlays (different colours per source, toggle visibility).
- iCal export for Granit-native events so external calendars can
  subscribe.

### AI

- Token-by-token streaming for the inline AI selection edit (the
  one-shot version exists, the streaming version doesn't).
- Local LLM provider that doesn't need Ollama (`llama.cpp` direct).
- Embeddings-based semantic search alongside the current full-text
  index.

### Life management

- Habits: streak repair after a missed day, finer-grained heatmap
  (intensity, not just done/not-done).
- Finance: import from CSV / OFX, account-balance reconciliation,
  goal projection charts.
- People: birthday + anniversary reminders surfaced on the dashboard.
- Reading: book log with note-attached highlights, "currently reading"
  shelf.

### Operations

- Backup tool (`granit backup` — bundle vault + sidecars + tarball
  with timestamp).
- Health endpoint that surfaces missing-files / parse-errors counts.
- Structured access log behind a flag for self-hosted deployments.
- ARM64 Docker images alongside amd64.

---

## Looking for contributors

The items below are valuable but not on the maintainer's near-term path.
If any of them resonate, open an issue to propose a direction and a PR
to implement. None are reserved.

- Plugin / extension API for the web app (the TUI has a Lua plugin
  system; the web side does not yet).
- Public API documentation for third-party integrations.
- Theme system for the web app — currently only the TUI has full
  user-themable colours.
- Snap / Flatpak packaging.
- Homebrew tap.
- Nix flake.
- FreeBSD and OpenBSD testing + packaging.
- Windows support (terminal compatibility for the TUI; the web app
  already runs on Windows via Docker / WSL).
- Multi-vault support in `granit web` — today one process serves one
  vault. The TUI already supports a vault picker.
- Offline-first conflict resolution for the PWA when several devices
  edit the same note while disconnected.
- Comments / annotations on notes (without breaking the
  plain-Markdown invariant).

---

## Anti-roadmap

Things Granit is **not** going to build, so contributors don't waste
their time:

- A multi-tenant SaaS or a hosted "Granit Cloud". Granit's value
  proposition is that you own the deployment.
- Real-time collaborative editing with shared cursors. The audience
  is a single user, not a team.
- A proprietary file format or database. Plain Markdown stays the
  source of truth.
- A built-in identity provider, OAuth, or SSO. The single-user
  password posture is intentional; if you need more, terminate auth
  in your reverse proxy.
- Telemetry, analytics, error reporting back to a server, or
  auto-update. The binary makes outbound calls only when the user
  does.

---

If you want to know exactly what changed in a specific release, the
canonical record is [`CHANGELOG.md`](CHANGELOG.md). This page is the
strategy document; the changelog is the receipts.
