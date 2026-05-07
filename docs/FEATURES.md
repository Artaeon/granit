# Granit — Features

A working tour of what Granit ships today, grouped by area, with file
pointers into the codebase. The maintainer record of *what changed when*
lives in [`CHANGELOG.md`](../CHANGELOG.md); this document answers the
question "what does Granit do right now?".

Granit has two surfaces — a SvelteKit web app served by `granit web`,
and a Bubble Tea terminal UI. Most life-management features are
web-first; the TUI focuses on note-taking and vault navigation. Both
surfaces operate on the same vault.

---

## Table of contents

- [Notes + editor](#notes--editor)
- [Tasks](#tasks)
- [Calendar + events](#calendar--events)
- [Daily routine](#daily-routine)
- [Goals + projects + ventures](#goals--projects--ventures)
- [Habits + virtues](#habits--virtues)
- [Finance + shopping](#finance--shopping)
- [Hub](#hub)
- [Prayer + scripture](#prayer--scripture)
- [Examen + review](#examen--review)
- [Vision](#vision)
- [Measurements](#measurements)
- [People](#people)
- [Deadlines](#deadlines)
- [Typed objects](#typed-objects)
- [Tags + search](#tags--search)
- [AI](#ai)
- [Print + export + publish](#print--export--publish)
- [Sync + history](#sync--history)
- [PWA + mobile](#pwa--mobile)
- [Settings + modules](#settings--modules)
- [Terminal UI](#terminal-ui)
- [CLI subcommands](#cli-subcommands)

---

## Notes + editor

CodeMirror 6 editor with a markdown grammar plus Granit-specific
extensions. Live preview mode toggles into a rendered Markdown view.
Auto-save persists every keystroke to localStorage and writes to the
vault on idle.

| Capability | Where |
| --- | --- |
| CodeMirror editor with markdown syntax + Tailwind theme | `web/src/lib/editor/Editor.svelte`, `theme.ts` |
| Bold / italic / link / inline code shortcuts | `web/src/lib/editor/markdown-shortcuts.ts` |
| Heading / list / quote shortcuts (`Mod-Alt-1..6`, `Mod-Shift-7..9`) | `web/src/lib/editor/heading-shortcuts.ts` |
| Checkbox shortcuts (`Mod-Enter`, `Mod-Shift-Enter`) | `web/src/lib/editor/checkbox-shortcuts.ts` |
| Wikilinks `[[Note]]` and block links `[[Note#Heading]]` | `web/src/lib/editor/wikilinks.ts` |
| Inline note embeds `![[path]]` | `web/src/lib/notes/MarkdownRenderer.svelte` |
| Tag autocomplete `#tag` | `web/src/lib/editor/tags.ts` |
| Autolink-suggest (suggest existing notes mid-typing) | `web/src/lib/editor/autolink.ts` |
| Smart paste for URLs (auto-link with selected text) | `web/src/lib/editor/Editor.svelte` |
| Slash command palette for blocks (`/h1`, `/quote`, `/code`, ...) | `web/src/lib/editor/block-completions.ts` |
| Snippet expansion (`/date`, `/today`, ...) | `web/src/lib/editor/snippets.ts` |
| Selection toolbar (bold / italic / link / heading) | `web/src/lib/editor/SelectionToolbar.svelte` |
| Selection → Ask AI (`Mod-Shift-A`) | `web/src/lib/editor/ask-ai.ts` |
| Selection → Extract to new note (`Mod-Shift-X`) | `web/src/lib/editor/extract-note.ts` |
| Footnotes | rendered in `web/src/lib/notes/MarkdownRenderer.svelte` |
| Mermaid diagrams (lazy-loaded) | rendered in `web/src/lib/notes/MarkdownRenderer.svelte` |
| Reading time + sticky scroll position per note | `web/src/routes/notes/[...path]/+page.svelte` |
| Frontmatter helper UI | `web/src/lib/notes/FrontmatterEditor.svelte` |
| Per-note version history with restore | `web/src/lib/notes/HistoryPanel.svelte`, `internal/history/` |
| Auto-save (paused while autocomplete open) | `web/src/routes/notes/[...path]/+page.svelte` |
| Vault path validation on every read/write | `internal/serveapi/handlers_notes.go` |
| Atomic file writes with symlink rejection | `internal/atomicio/atomicio.go` |

Notes index + parsing on the backend:

| Capability | Where |
| --- | --- |
| Vault scanning + lazy content loading | `internal/vault/vault.go` |
| Markdown / frontmatter / wikilink parser | `internal/vault/parser.go` |
| Backlink + forward-link index | `internal/vault/index.go` |
| Full-text search index | `internal/vault/searchindex.go` |
| File watcher + WebSocket fan-out | `internal/serveapi/watch.go`, `internal/wshub/` |

---

## Tasks

Single source of truth in `internal/tasks` shared between the web app
and the TUI. Tasks live as `- [ ] ...` lines in markdown notes; sidecar
state (priority, scheduled start, snooze, project, tags) lives in
`.granit/tasks.json`.

| Capability | Where |
| --- | --- |
| List view with smart due-groups + urgency tinting | `web/src/routes/tasks/+page.svelte` |
| Kanban view with drag-reorder | `web/src/lib/tasks/Kanban.svelte` |
| Quick-add bar with smart syntax | `web/src/routes/tasks/+page.svelte`, `web/src/lib/components/QuickAddTask.svelte` |
| Bulk actions + inline edit + multi-select | `web/src/routes/tasks/+page.svelte` |
| Keyboard navigation (`j`/`k`/`x`/`d`/`e`/`p`) | `web/src/routes/tasks/+page.svelte` |
| Task detail drawer | `web/src/lib/tasks/TaskDetail.svelte` |
| Snooze picker | `web/src/lib/tasks/SnoozePicker.svelte` |
| Task context menu | `web/src/lib/tasks/TaskContextMenu.svelte` |
| At-a-glance stats chips above the list | `web/src/routes/tasks/+page.svelte` |
| Recurring tasks shared store (TUI + web) | `internal/recurring/`, `internal/serveapi/handlers_recurring.go` |
| Recurring picker in task detail | `web/src/lib/components/RecurringEditor.svelte` |
| Time-tracking integration | `internal/timetracker/`, `web/src/lib/components/RunningTimer.svelte` |
| HTTP CRUD | `internal/serveapi/handlers_tasks.go` |

---

## Calendar + events

Calendar handles both Granit-native events (stored in
`.granit/events.json`) and `.ics` calendar files in `<vault>/calendars/`.
Subscriptions to remote `.ics` URLs are read-only; locally-created
calendars are writable.

| Capability | Where |
| --- | --- |
| Day / 3-day / week / month / year / agenda views | `web/src/routes/calendar/+page.svelte`, `web/src/lib/calendar/` |
| Drag-to-move events | `web/src/lib/calendar/` |
| Drag-to-resize events | `web/src/lib/calendar/` |
| 24-hour time inputs (no AM/PM) | `web/src/lib/calendar/EventDetail.svelte` |
| Touch-swipe navigation on mobile | `web/src/routes/calendar/+page.svelte` |
| Google-style key bindings (`t`/`j`/`k`/`d`/`w`/`m`/`y`/`a`/`?`) | `web/src/routes/calendar/+page.svelte` |
| Multi-day all-day events render only on start date | `web/src/lib/calendar/` |
| Collapsible all-day strip with "+N more" overflow | `web/src/lib/calendar/` |
| `WEEKLY+BYDAY` rrule expansion | `internal/serveapi/ics.go` |
| EXDATE handling for cancelled occurrences | `internal/serveapi/ics.go` |
| Local writable ICS calendars under `<vault>/calendars/` | `internal/serveapi/handlers_calsources.go`, `handlers_ics_events.go` |
| ICS writer (create + patch + delete events) | `internal/icswriter/writer.go` |
| Per-source colour coding | `web/src/lib/calendar/` |
| Calendar API + sources management | `internal/serveapi/handlers_calendar.go` |

---

## Daily routine

Granit's "daily" surface is a single markdown file per date under the
configured daily folder. Today's daily is created lazily on first
visit with the user's template applied.

| Capability | Where |
| --- | --- |
| Auto-create today's daily on first visit | `internal/daily/` |
| Yesterday's open-tasks carryover band | `internal/serveapi/handlers_daily_context.go`, `web/src/lib/dashboard/DailyContext.svelte` |
| Habits checklist on the daily | `internal/serveapi/handlers_daily_context.go`, `web/src/lib/dashboard/DailyContext.svelte` |
| Morning routine wizard with prayer step | `web/src/routes/morning/+page.svelte`, `internal/serveapi/handlers_morning.go` |
| Daily examen wizard (evening companion) | `web/src/routes/examen/+page.svelte`, `internal/serveapi/handlers_examen.go` |
| Today-at-a-glance hero widget | `web/src/lib/dashboard/`, `web/src/routes/+page.svelte` |
| Saved layout presets (focus / morning / shutdown) | `web/src/lib/dashboard/`, `internal/serveapi/handlers_dashboard.go` |
| Dashboard widget catalogue | `web/src/lib/dashboard/` |

---

## Goals + projects + ventures

Three-layer planning hierarchy:

- **Ventures** — umbrella entity (a business, a creative track, a
  multi-year arc). One step above projects and goals.
- **Goals** — outcome-shaped, with target dates and milestones.
- **Projects** — work containers, optionally tied to a venture and a
  goal.

| Capability | Where |
| --- | --- |
| Ventures list + detail | `web/src/routes/ventures/+page.svelte`, `[name]/+page.svelte` |
| Ventures storage | `internal/ventures/`, `internal/serveapi/handlers_ventures.go` |
| Goals list with target-date urgency tinting | `web/src/routes/goals/+page.svelte`, `web/src/lib/goals/` |
| Next-target hero card | `web/src/routes/goals/+page.svelte` |
| Goal milestones + review log | `internal/goals/`, `internal/serveapi/handlers_goals.go` |
| Projects list + detail | `web/src/routes/projects/+page.svelte`, `web/src/lib/projects/` |
| Repo-backed projects (git status surfaced) | `internal/repos/` |

---

## Habits + virtues

Habits and virtues are separate trackers that can optionally link.

| Capability | Where |
| --- | --- |
| Habits page with List / Today / Week views | `web/src/routes/habits/+page.svelte` |
| Habit heatmap with click-on-past-day toggle | `web/src/routes/habits/+page.svelte` |
| Habits storage + per-date toggle | `internal/habits/`, `internal/serveapi/handlers_habits.go` |
| Habit ↔ virtue linkage | `internal/habits/`, `internal/virtues/` |
| Habit deduplication + leakage-into-tasks fix | `internal/habits/` |
| Virtues page with weekly checks | `web/src/routes/virtues/+page.svelte`, `web/src/lib/virtues/` |
| Virtue check entries dedicated POST | `internal/serveapi/handlers_virtues.go` |
| Virtue weekly review integration | `web/src/routes/review/+page.svelte` |

---

## Finance + shopping

Personal finance: net worth (accounts), recurring drag (subscriptions),
income streams, money goals. Shopping list integrates with finance for
recurring "standards" run-rate.

| Capability | Where |
| --- | --- |
| Finance overview composite endpoint | `internal/serveapi/handlers_finance.go` |
| Accounts (net worth) | `internal/finance/`, `internal/serveapi/handlers_finance.go` |
| Subscriptions (recurring drag) | `internal/finance/` |
| Income streams | `internal/finance/` |
| Money goals | `internal/finance/` |
| Finance page | `web/src/routes/finance/+page.svelte` |
| Shopping list with `standard` flag for recurring needs | `internal/shopping/`, `internal/serveapi/handlers_shopping.go` |
| Shopping totals (planned vs bought this month) | `internal/serveapi/handlers_shopping.go` |
| Shopping page (mobile + desktop layout) | `web/src/routes/shopping/+page.svelte` |

---

## Hub

Personal launch pad — one place to go for the URLs, dashboards, and
tools you use every day. Backed by `.granit/hub.json`.

| Capability | Where |
| --- | --- |
| Hub page with search + group-by-category | `web/src/routes/hub/+page.svelte` |
| Add / edit modal | `web/src/routes/hub/+page.svelte` |
| Drag-to-reorder | `web/src/routes/hub/+page.svelte` |
| Favicons (auto-fetched per item) + whole-card click | `web/src/routes/hub/+page.svelte` |
| Recently-visited tracking + last-visited tinting | `internal/hub/`, `internal/serveapi/handlers_hub.go` |
| Browser-bookmark import | `web/src/routes/hub/+page.svelte` |
| Quick-links dashboard widget (top 5 favorites) | `web/src/lib/dashboard/` |
| Hub storage | `internal/hub/` |

---

## Prayer + scripture

| Capability | Where |
| --- | --- |
| Prayer intentions list with status lifecycle (praying / answered / archived) | `internal/prayer/`, `internal/serveapi/handlers_prayer.go` |
| Prayer page | `web/src/routes/prayer/+page.svelte` |
| Prayer ↔ goal / project / venture linkage | `web/src/routes/prayer/+page.svelte`, `internal/prayer/` |
| Prayer dashboard widget | `web/src/lib/dashboard/` |
| Scripture loader + verse-of-the-day (deterministic) | `internal/scripture/`, `internal/serveapi/handlers_scripture.go` |
| Scripture random-pick (with optional `?seed`) | `internal/serveapi/handlers_scripture.go` |
| Scripture memorize mode (cloze deletion) | `web/src/routes/scripture/+page.svelte` |
| Devotional creation (verse-seeded note) | `internal/serveapi/handlers_scripture.go` |
| AI devotional reflection preset | `internal/agents/preset.go` (devotional) |
| Embedded full WEB Bible reader | `internal/scripture/bible/`, `internal/serveapi/handlers_bible.go` |
| Bible search | `internal/serveapi/handlers_bible.go` |
| Bible bookmarks | `internal/biblebookmarks/`, `internal/serveapi/handlers_bible_bookmarks.go` |

---

## Examen + review

| Capability | Where |
| --- | --- |
| Daily examen wizard | `web/src/routes/examen/+page.svelte` |
| Examen handler writes a `## Examen` block to the daily | `internal/serveapi/handlers_examen.go`, `internal/examen/` |
| Weekly review page | `web/src/routes/review/+page.svelte` |
| Weekly review virtues integration | `web/src/routes/review/+page.svelte` |

---

## Vision

Single record per vault — life mission + values + season focus.
Re-read each morning by the dashboard to anchor focus.

| Capability | Where |
| --- | --- |
| Vision page | `web/src/routes/vision/+page.svelte` |
| Vision storage | `internal/vision/`, `internal/serveapi/handlers_vision.go` |
| Vision context strip (rendered on planning pages) | `web/src/lib/components/VisionContextStrip.svelte` |

---

## Measurements

Numeric tracking, companion to habits. One Series = one metric
definition; one Entry = one logged value.

| Capability | Where |
| --- | --- |
| Series CRUD | `internal/measurements/`, `internal/serveapi/handlers_measurements.go` |
| Entry CRUD | `internal/measurements/`, `internal/serveapi/handlers_measurements.go` |
| Measurements page (charts + log) | `web/src/routes/measurements/+page.svelte` |

---

## People

Lightweight relationship tracker. Birthdays + last-contact + stale
counts denormalised into the list response so the dashboard widgets
don't N+1.

| Capability | Where |
| --- | --- |
| People list with upcoming-birthdays + stale-count | `internal/people/`, `internal/serveapi/handlers_people.go` |
| People page | `web/src/routes/people/+page.svelte` |
| Ping endpoint (record an interaction) | `internal/serveapi/handlers_people.go` |

---

## Deadlines

Top-level "this matters by date X" markers. Distinct from goals —
deadlines are external commitments, goals are internal.

| Capability | Where |
| --- | --- |
| Deadline CRUD | `internal/deadlines/`, `internal/serveapi/handlers_deadlines.go` |
| Deadlines page | `web/src/routes/deadlines/+page.svelte` |
| Calendar overlay | `internal/serveapi/handlers_calendar.go` |

---

## Typed objects

Capacities-style structured notes. A typed object is a markdown note
with a `type:` field in its frontmatter; the type's schema defines
which other frontmatter fields are expected.

| Capability | Where |
| --- | --- |
| Typed-object schema + index | `internal/objects/` |
| Per-vault custom type schemas | `internal/objects/` |
| Object Browser page | `web/src/routes/objects/+page.svelte` |
| Type list + per-type object list | `internal/serveapi/handlers_types.go` |
| Tasks ↔ Projects via typed-object frontmatter | `internal/tasks/`, `internal/objects/` |

Detail and authoring docs: [`docs/OBJECTS.md`](OBJECTS.md).

---

## Tags + search

| Capability | Where |
| --- | --- |
| Tag browser | `web/src/routes/tags/+page.svelte`, `internal/serveapi/handlers_tags.go` |
| Full-text vault search | `internal/vault/searchindex.go`, `internal/serveapi/handlers_search.go` |
| Search-everything overlay | `web/src/lib/components/CommandPalette.svelte` |
| Pinned items | `internal/serveapi/handlers_pinned.go` |

---

## AI

Three providers behind one runtime. Each AI feature calls the runtime
the same way regardless of provider.

| Capability | Where |
| --- | --- |
| Provider abstraction (Ollama / OpenAI / Anthropic) | `internal/agentruntime/llm.go` |
| Multi-step agent runtime (ReAct loop, tool use) | `internal/agents/` |
| Agent presets (devotional, plan-my-day, ...) | `internal/agents/preset.go` |
| Agent run handler with WebSocket streaming | `internal/serveapi/handlers_agentrun.go` |
| `/agents` page with run panel + history | `web/src/routes/agents/+page.svelte`, `web/src/lib/agents/` |
| `/chat` multi-turn page with SSE streaming | `web/src/routes/chat/+page.svelte`, `internal/serveapi/handlers_chat.go` |
| Selection → Ask AI inline edit | `web/src/lib/editor/ask-ai.ts` |
| `plan-my-day` agent with dry-run + apply | `internal/serveapi/handlers_plan_day_schedule.go` |
| Curated OpenAI model picker | `internal/serveapi/handlers_config.go` |
| Live Ollama model list | `internal/serveapi/handlers_config.go` |
| Per-preset model override | `internal/agents/preset.go` |

Detail: [`docs/AGENTS.md`](AGENTS.md), [`docs/AI-GUIDE.md`](AI-GUIDE.md).

---

## Print + export + publish

Browser-rendered print preview is the primary export shape — one
button on every note prints to PDF via the OS print dialog.

| Capability | Where |
| --- | --- |
| Multi-page print preview with proper page breaks | `web/src/lib/notes/PrintPreview.svelte` |
| Per-vault print defaults (header / footer / mode) | `internal/serveapi/handlers_print.go` |
| Document signature footer (signer / purpose / document ID) | `web/src/lib/notes/PrintPreview.svelte` |
| 24h timestamp format | `web/src/lib/notes/PrintPreview.svelte` |
| Dark-mode print rendering fix | `web/src/lib/notes/PrintPreview.svelte` |
| Static-site generator (`granit publish`) | `cmd/granit/publish.go`, `internal/publish/` |

Detail: [`docs/PUBLISH.md`](PUBLISH.md).

---

## Sync + history

| Capability | Where |
| --- | --- |
| Optional `--sync` flag for git auto-pull/commit/push | `cmd/granit/web.go`, `internal/serveapi/sync.go` |
| Per-note version history backend | `internal/history/` |
| Per-note version history UI with one-click restore | `web/src/lib/notes/HistoryPanel.svelte` |
| WebSocket fan-out for live multi-device sync | `internal/wshub/`, `web/src/lib/ws.ts` |
| File watcher to broadcast external edits | `internal/serveapi/watch.go` |

---

## PWA + mobile

| Capability | Where |
| --- | --- |
| Service worker + offline-first cache | `web/src/service-worker.ts` |
| Install prompt (Add to Home Screen) | `web/src/lib/components/InstallPrompt.svelte` |
| Offline banner | `web/src/lib/components/OfflineBanner.svelte` |
| Service-worker update banner with visibility refresh | `web/src/routes/+layout.svelte` |
| Bottom navigation on mobile | `web/src/lib/components/BottomNav.svelte` |
| Mobile bar (action buttons) | `web/src/lib/components/MobileBar.svelte` |
| Quick-capture FAB on every page | `web/src/lib/components/QuickCaptureFab.svelte` |
| Touch-swipe navigation in calendar | `web/src/routes/calendar/+page.svelte` |
| Mobile-tuned shopping layout | `web/src/routes/shopping/+page.svelte` |

---

## Settings + modules

| Capability | Where |
| --- | --- |
| Curated settings page mapping config.json fields | `web/src/routes/settings/+page.svelte`, `internal/serveapi/handlers_config.go` |
| AI provider / model / keys (write-once for keys) | `internal/serveapi/handlers_config.go` |
| Daily folder + template + recurring tasks | `internal/serveapi/handlers_recurring.go` |
| Module registry (toggle features on/off) | `internal/modules/`, `internal/serveapi/handlers_modules.go` |
| Sabbath overlay (temporally hide work modules) | `web/src/lib/stores/sabbath.ts` |
| Theme toggle (light / dark / system) | `web/src/lib/stores/theme.ts` |
| Devices / sessions list + revoke | `internal/serveapi/handlers_devices.go` |
| Password change | `internal/serveapi/handlers_auth.go` |
| Log-out everywhere | `internal/serveapi/handlers_auth.go` |
| Quick switcher (`Mod-P`) | `web/src/lib/components/CommandPalette.svelte` |

---

## Terminal UI

The TUI is the original Granit interface and remains fully supported
for note-taking, vault navigation, and AI features. Run with `granit
<vault>` (no subcommand). Many of the life-management modules
(habits, finance, prayer, virtues, hub, ventures, deadlines) live in
the web app only.

| Capability | Where |
| --- | --- |
| Bubble Tea Model/Update/View loop | `internal/tui/app.go` |
| Multi-cursor editor with vim mode | `internal/tui/editor.go`, `vim.go` |
| Sidebar file tree + fuzzy search | `internal/tui/sidebar.go`, `filetree.go` |
| Backlinks + outgoing links panel | `internal/tui/backlinks.go` |
| Live wikilink hover preview | `internal/tui/backlinkpreview.go` |
| Command palette (Ctrl+X) | `internal/tui/command.go` |
| 38 built-in themes | `internal/tui/themes.go` |
| Note graph visualization | `internal/tui/graph.go` |
| Tag browser, outline, bookmarks | `internal/tui/tags.go`, `outline.go`, `bookmarks.go` |
| Calendar + tasks + kanban overlays | `internal/tui/calendar.go`, `taskmanager.go`, `kanban.go` |
| Pomodoro / focus session / time tracker | `internal/tui/pomodoro.go`, `focussession.go`, `timetracker.go` |
| Lua plugin system | `internal/tui/lua.go`, `plugins.go` |
| Git overlay + auto-sync | `internal/tui/git.go`, `autosync.go` |
| AI bots (Ollama / OpenAI / local fallback) | `internal/tui/bots.go`, `aichat.go`, `aistreaming.go` |
| Inline AI selection edits | `internal/tui/composer.go`, `ghostwriter.go` |

Detail: [`docs/KEYBINDINGS.md`](KEYBINDINGS.md), [`docs/THEMES.md`](THEMES.md).

---

## CLI subcommands

`granit` is the binary; it dispatches to subcommands based on the first
argument.

| Subcommand | Purpose | Source |
| --- | --- | --- |
| `granit <vault>` | Open the TUI on a vault. | `cmd/granit/main.go` |
| `granit web [--addr] [--dev] [--sync] [vault]` | Boot the HTTP API + embedded SPA. | `cmd/granit/web.go` |
| `granit serve [--port] [vault]` | Read-only HTML preview of the vault. | `cmd/granit/serve.go` |
| `granit scan [vault]` | Walk the vault and print stats. | `cmd/granit/scan.go` |
| `granit daily [vault]` | Create / open today's daily note. | `cmd/granit/daily.go` |
| `granit publish ...` | Static-site generator. | `cmd/granit/publish.go` |
| `granit version` | Print version + commit + build date. | `cmd/granit/main.go` |
| `granit help` | Print top-level help. | `cmd/granit/main.go` |
| `granit man` | Print the roff man page (pipe to `man -l -`). | `cmd/granit/manpage.go` |
| `granit completion {bash\|zsh\|fish}` | Print shell completion. | `cmd/granit/completion.go` |
| `granit config` | Print effective config. | `cmd/granit/main.go` |
