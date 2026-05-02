# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added — Phase 19: Chat, daily-note context, settings, recurring + tracking

#### Chat surface
- `Chatter` interface added to `internal/agentruntime` so OpenAI and
  Ollama can serve multi-turn conversations natively (chat-completions
  endpoint for OpenAI, /api/chat for Ollama).
- `POST /chat` — single-shot helper. Caller supplies the full message
  history, server reads the AI config, calls the model, returns the
  next assistant message. Stateless on the server (history lives in
  the client). Optional `notePath` attaches the note's body as a
  system message so the LLM has vault context without copy-paste.
- `/chat` web page — multi-turn UI. Enter sends, Shift+Enter newlines.
  History persisted to localStorage; "save as note" writes the
  conversation to `Chats/{ts}.md`. Bouncing-dots indicator while the
  model thinks. Empty messages/errors drop the input back so the user
  can retry.

#### Devotional reflection
- New `devotional` preset in `agents.BuiltinPresets()` — takes a verse
  + citation, writes a structured 200-300 word reflection into
  `Devotionals/{date}-{slug}.md`. System prompt enforces structure
  (insight rooted in the text, concrete posture for today, closing
  question/prayer) so the model doesn't drift into platitudes.
- `/scripture` page gained an "AI reflection ✨" button next to the
  existing manual "Reflect on this" path. The agent panel opens
  pre-filled with the verse so the user just hits Run.
- `AgentRunPanel` accepts an optional `initialGoal` prop for the same
  pre-fill pattern.

#### Daily-note carryover + habits checklist
- `GET /daily/context` returns yesterday's open tasks (carryover) +
  today's habits checklist (derived from `daily_recurring_tasks` in
  config + the daily note's `[x]` lines). Cheap on the server; the
  web fetches it once on note load and on every WS event.
- `DailyContext.svelte` renders both as collapsible bands at the top
  of every daily note. Carryover row has a one-click "mark done";
  habits row shows tick-status chips with a hint to write
  `- [x] {habit}` in the daily.

#### Settings (config.json read/write)
- `GET /config` and `PATCH /config` expose a curated subset of the
  same `~/.config/granit/config.json` the TUI reads. Fields exposed:
  AI provider/model/keys (OpenAI + Anthropic + Ollama), daily +
  weekly folder, daily recurring tasks, theme, editor toggles, task
  filter mode, sync, pomodoro goal.
- API keys never echoed back — the GET response carries `*_key_set`
  booleans only. PATCH accepts a non-empty string to set, an
  explicit empty string to clear.
- Settings page gained AI / Daily notes / Editor & behavior /
  Recurring tasks sections. Set up the AI once on either surface
  and both pick it up.

#### Recurring tasks
- `internal/recurring` — shared store. Read/write
  `<vault>/.granit/recurring.json` (same file the TUI's overlay
  edits). `IsDue` rule mirrors the TUI's: daily / weekly (matching
  weekday) / monthly (matching day-of-month).
- `GET /recurring` and `PUT /recurring` for the web. Server fires
  due rules at every local-time midnight (goroutine wakes 30s past
  midnight) + on boot + as a side effect of every list/mutate
  request (defends against the goroutine missing a tick).
- Created tasks carry `tasks.OriginRecurring` on the sidecar — same
  provenance the TUI tags them with.
- `RecurringEditor.svelte` — full-list editor embedded in /settings.
  Per-row enable/text/frequency + conditional weekday/day-of-month
  picker. Save button writes the canonical list back via PUT.

#### Time tracking
- `internal/timetracker` — shared store at
  `<vault>/.granit/timetracker.json`. One Entry per completed
  session. Optional `task_id` field for cross-surface task rollup
  (TUI uses free-form text, web uses the stable sidecar ID).
- `GET /timetracker` returns recent entries + active timer + pre-
  rolled-up `minutesByTaskId` and `minutesToday`.
- `POST /timetracker/start` and `/stop` manage one server-wide
  active timer. Start auto-stops any prior timer so users don't
  abandon time. WS broadcasts `timer.started` / `timer.stopped`
  so other connected devices update their pill.
- Web active-timer store (`lib/stores/timer.ts`) hydrated on auth +
  WS frames. `RunningTimer.svelte` floats a green pill in the top-
  right while a timer runs; click to stop. `TaskCard` gained a
  play/stop button next to snooze + a "1h 23m total" minutes pill
  next to its action row.

### Added — Phase 18: server-side AI agents + Bible practicer

#### Shared agent runtime (`internal/agentruntime`)
- New TUI-free runtime package that wraps `internal/agents` so any caller
  (HTTP handlers, future scheduled jobs, future CLI subcommands) can run
  agents the same way the TUI does.
- LLM impls for OpenAI (`gpt-4o-mini` default) and Ollama. Provider
  switch keyed off the existing `config.Config` — same `ai_provider` /
  `openai_key` / `openai_model` / `ollama_url` / `ollama_model` fields
  the TUI reads. A working `granit tui` setup runs on the web with no
  extra config.
- Bridge wires vault/objects/tasks into agent tools (`read_note`,
  `list_notes`, `search_vault`, `query_objects`, `query_tasks`,
  `get_today`, `write_note`, `create_task`, `create_object`). Mirrors
  the TUI bridge but with no TUI dep so the server uses it directly.
- Auto-approve writes when the preset's `IncludeWrite=true` — preset
  metadata IS the user's prior consent, no per-step prompt.

#### `POST /agents/run` with WS event streaming
- Kicks off an agent run on the server; returns `{runId}`.
- For every step the agent emits, broadcasts `agent.event` over the
  existing WS hub with `{step, kind, text}` so the page that started
  the run (and any other connected device) can render the transcript
  live.
- On completion, persists a TUI-compatible `agent_run` note into
  `Agents/{utc-timestamp}-{preset}.md` (same frontmatter shape, so
  `/agents/runs` picks it up identically) and broadcasts
  `agent.complete` with status + final answer + path to the note.
- Stateless: no server-side run map. The WS stream is the live
  channel; the persisted note is the post-run record.
- Surfaces misconfiguration (missing API key, wrong provider) as 400
  to the kicker, not 500.

#### `plan-my-day` built-in preset
- Reads today's date, open tasks, active project next-actions, and the
  current daily note (so existing manual plans aren't clobbered);
  writes a time-blocked `## Plan` section into today's daily.
- Hard rules in the system prompt: 25–60 min focus blocks, lunch
  12:30–13:15, ≤1 90-min deep-work block per morning, cite task IDs.
- One-click button on the daily note's quick-add bar opens the agent
  run panel. After completion, the editor's WS subscriber catches
  `note.changed` and reloads — the new `## Plan` appears in-place.

#### Web agent run UI
- New "▶ Run" button on every preset card on `/agents`.
- Slide-in `AgentRunPanel` shows the goal input, then streams each
  ReAct step (thought / tool_call / tool_result / final_answer) with
  tone-coded labels and step numbers.
- Final answer floats to the top in a green callout when the run
  completes; `view transcript →` link jumps to the persisted
  `agent_run` note.

#### Scripture / Bible practicer (`internal/scripture` + `/scripture`)
- Extracted scripture loader to a shared package — TUI now thin-wraps
  it. Single source of truth for the verse list and the
  `<vault>/.granit/scriptures.md` format.
- Endpoints: `GET /scripture` (full library), `/scripture/today`
  (deterministic verse-of-the-day, same on every device), `/scripture/random`
  (uniform random; `?seed=N` for testing). `POST /devotionals` creates
  a `Devotionals/{date}-{slug}.md` note pre-seeded with the verse text
  as a blockquote and a "## Reflection" section.
- New `/scripture` page with three modes:
  - **Read**: verse-of-the-day in big serif, "another verse" / "reflect
    on this" buttons. Reflect creates a devotional note and routes the
    user to it for editing.
  - **Memorize**: cloze-deletion drill. Hides ~25% of significant
    words (filler words skipped). Per-verse accuracy stored in
    localStorage; the picker weights weaker verses preferentially so
    practice converges on the gaps.
  - **Browse**: the full library with a text-search filter.
- Dashboard widget shows the verse of the day with a click-through.
  Enabled by default in new vaults.
- Nav entry with a book icon.

#### WebSocket protocol
- `wshub.Event` gained an optional `Data map[string]any` so events that
  need a richer payload than (path, id) can carry it. Agent streaming
  is the first user; other features can adopt the same field.
- Web `WsEvent` union extended with `agent.event` and `agent.complete`.

### Added — Phase 17: web companion (`granit web`)

A self-hosted web frontend over the same vault, task store, and daily-note
pipeline as the TUI. Single-binary deployment — the SvelteKit SPA is
embedded via `go:embed` and served from the same process. See `docs/WEB.md`.

#### Core
- New subcommand `granit web [--addr :8787] [--dev] [--sync] [vault-path]` —
  boots an HTTP/JSON + WebSocket API plus the embedded SPA. Auto-watches
  the vault and broadcasts file events to connected clients.
- New packages: `internal/granitmeta` (Read/Write helpers for the JSON
  sidecars: events, projects, goals), `internal/wshub` (small WebSocket
  fan-out hub), `internal/templates` (template defs extracted from TUI so
  both surfaces share one source of truth), `internal/serveapi` (HTTP
  router, handlers, embed).
- TUI's hardcoded agent presets moved into `agents.BuiltinPresets()` —
  `agents/preset.go` is now the single source of truth; the TUI's
  `builtinAgentPresets()` thinly wraps it.

#### Auth
- argon2id password login (m=64MiB, t=1, p=4). Hash + sessions stored at
  `<vault>/.granit/web-auth.json`. Sessions are 256-bit random tokens;
  only their sha256 is persisted.
- 60-day inactive session expiry. Per-device labels at login (Web / iOS /
  Android / macOS / Linux / Windows).
- Endpoints: `/auth/status` `/auth/setup` `/auth/login` `/auth/logout`
  `/auth/change-password` `/auth/revoke-all`. 250ms anti-bruteforce delay
  on wrong password.
- Legacy bearer token (`<vault>/.granit/everything-token`) still printed
  at boot for CLI scripts; both auth paths accepted by `requireToken`.

#### Surfaces
- **Dashboard**: customizable widgets (Now / Streaks / Today's Tasks /
  Scheduled / Goals / Projects / Inbox / Calendar Week / Recent Notes /
  Daily Note / Quick Capture / Habits / Pomodoro / Pinned). Drag-to-reorder.
- **Tasks**: 7 view modes (List / Kanban / Inbox / Triage / Quick Wins /
  Stale / Review). Triage state cycle, snooze with TUI presets,
  bulk-select + bulk actions, side-panel detail drawer (priority pills,
  full triage grid, recurrence picker, free-form notes textarea).
  Subtask indent display, recurrence chip, dependency badge.
- **Calendar**: 6 views (Day / 3-day / Week / Month / Year / Agenda).
  Click-and-drag on the time grid creates a task or event; resize
  bottom edge to change duration. Per-source ICS toggles synced with
  TUI's `disabled_calendars`. Full events.json CRUD. ICS dedup matches
  the TUI's `title|start|end` algorithm.
- **Notes**: CodeMirror editor with wikilink autocomplete, frontmatter
  editor, outline + backlinks, three view modes (edit / preview /
  split), folder breadcrumbs in header, frontmatter tag chips, per-note
  draft persistence in localStorage with conflict guard against server-
  newer.
- **Daily note**: inline quick-add band parses TUI shorthand
  (`!1 due:YYYY-MM-DD #tag`), task ↔ event toggle.
- **Projects**: full CRUD with goals + milestones (auto-progress), next-
  action chip, linked-tasks panel, status lifecycle pills, color picker,
  category, priority levels.
- **Agents**: presets gallery (built-in + vault overrides) and run
  history (any note with `type: agent_run` frontmatter). Per-preset stats.
- **Settings**: theme picker, security (password change, sign out
  everywhere), **devices** (active sessions list with revoke per-row,
  current-device pill), git sync status, vault info, keyboard shortcuts.
- **Habits / Goals / Tags / Stats / Templates / Morning / Objects**: full
  page implementations.

#### Offline + PWA
- Service worker with stale-while-revalidate for `GET /api/v1/*` and
  cache-first for hashed bundle assets. Edits queue when offline; the
  client retries on `online` event.
- Per-note drafts in localStorage survive tab close, reload, and brief
  power loss (writes 600 ms after last keystroke).
- PWA manifest + apple-touch icons. Installable on iOS / Android.
- Theme bootstrap inline in `index.html` to eliminate dark↔light flash.

#### Sync & devices
- Optional `git` auto-sync (`--sync`): periodic `git pull` + auto-commit/
  push. Status surfaced at `/api/v1/sync` and the Settings page.
- `/api/v1/devices` lists active sessions; `DELETE /api/v1/devices/{id}`
  revokes one. Stable per-device IDs are sha256-prefix hashes — raw
  tokens never leave the server.

### Added — Phase 16: more Deepnote-like AI surface

#### Inline AI diff preview (BEFORE / AFTER, accept/discard/retry)
- After `Alt+/` or a slash-menu AI action returns, instead of writing the result immediately, granit shows a centred overlay with the original selection and the proposed replacement
- **`y` or `Enter`** — accept and splice into the editor at the originally-captured range
- **`n` or `Esc`** — discard; selection stays untouched, brief "AI proposal discarded" status
- **`r`** — re-run the same action against the same range (regenerate without re-typing the slash-menu navigation)
- Original text re-extracted from the editor at the captured range so a cursor move during the AI roundtrip can't break the BEFORE preview
- Output capped at 8 wrapped lines per pane (with "(truncated)" gutter) so a giant rewrite doesn't push the hint footer offscreen
- Power-user opt-out: `AIAutoApplyEdits: true` in `config.json` skips the preview entirely; original behaviour restored

#### Agent transcripts persisted as `agent_run` typed objects
- New built-in type (#14): `agent_run` with properties `title`, `preset`, `model`, `goal`, `status` (ok / budget / error / cancelled), `started`, `steps`, `tags`. Folder: `Agents/`
- AgentRunner queues a persist request when a run finishes; host writes via the same `createTypedObjectFile` plumbing the Object Browser uses. Filename pattern encodes UTC timestamp + preset ID so multiple runs of the same preset on the same day stay distinct (`Agents/2026-04-30T1542-research-synthesizer.md`)
- Body is a structured markdown transcript: `## Answer` (final answer first so search hits land readers on the conclusion), then `## Transcript` with one `### Step N` per ReAct iteration containing Thought / Action / Observation. Long observations truncated to 1000 chars with a `(truncated)` marker
- New built-in saved view: **Recent Agent Runs** — `type:agent_run` sorted by `started` desc, limit 50. Pin as the dashboard primary view to see your AI usage history at a glance

#### Tests
- 11 new tests covering: diff-preview Open/Reset/empty-output-noop, View renders BEFORE/AFTER + accept hint, `extractEditorRange` (same-line, multi-line, clamping, reversed-range normalisation), transcript markdown serialisation (goal+answer rendered, long-output truncation, budget-status propagation), queuePersist builds the right note + filename + frontmatter

### Added — Phase 15: project notes ↔ folder bridge

#### Open repo folder + copy path from anywhere
- New cross-platform `repos.OpenFolder(path)` helper: `xdg-open` on Linux/BSD, `open` on macOS, `explorer` on Windows. Detached so the file manager outlives granit; missing-path / missing-handler failures surface as clear errors instead of silent no-ops
- **In the Repo Tracker:** `o` opens the focused repo's folder externally; `c` copies its absolute path to the system clipboard. Both queue a status-bar message via a new `ConsumePendingStatus` channel so feedback is consistent ("Opened /home/me/Projects/foo" or "Copied … to clipboard")
- **From any project note** (when the note has a `repo:` property): `Alt+\` opens the folder externally; `Alt+'` copies the path. Mnemonics: `\` = "shell" (think shell escape), `'` = "literal" (the path literal). No-op on regular notes; clear hint when the project note has no `repo:` set
- Tracker footer hint updated: `j/k nav · Enter import/open · g jump · o open folder · c copy path · r refresh · Esc close`

#### `RepoScanRoot` editable in Settings UI
- Settings (Ctrl+,) under Files now has a `Repo Scan Root` entry — string field, supports `~` expansion, persisted to `config.json`. No more "edit the JSON to discover the feature"
- Default value (`~/Projects`) shipped via the standard config-default chain so first-launch behaviour is sensible

#### Tests
- 4 new tests covering `OpenFolder` (empty path errors, non-existent path errors, platform binary selection); 3 new tracker action tests (`o` queues open status, `c` queues copy status, action on empty rows is safe)

### Added — Phase 14: local git repos as typed-project notes

#### `internal/repos/` package — git status as a library
- New `Status(path)` helper shells out to `git -C <path> status --porcelain=v2 --branch` and `git log -1 --format=%ct` (3s timeout, zero new dependencies)
- Returns `Status{IsRepo, Branch, Dirty, Ahead, Behind, LastCommit}` with ergonomic helpers `IsClean()` and `AgeSinceLastCommit()`
- Cheap path probe (`looksLikeRepo`) avoids forking `git` for plain directories during a folder scan — important when scanning a `~/Projects/` with hundreds of subdirs
- Why shell instead of go-git/libgit2? Zero new deps, status flags exactly match the user's mental model from the command line, and git's already on every dev machine
- 6 tests against real temp git repos: clean repo, modified file detection, untracked file detection, non-repo path returns ErrNotARepo, age helper

#### `project` typed-object gains a `repo:` property
- Optional path-string property on the built-in project type. Backwards compatible — existing project notes keep working unchanged
- Documented as "Local path to the project's git repo (e.g. /home/me/Projects/foo). Hub strip + Repo Tracker pull live status from here"

#### Project Hub strip surfaces live git status
- When a `type: project` note has `repo:` set, the hub strip above the editor adds an inline chip: `git: main · 3 dirty · ↑2 ↓0 · 2h`
- Status results cached at the package level for 30s so the strip stays cheap on rapid renders (no `git` fork per frame)
- Colour-coded: green `clean` when in-sync · yellow when dirty/ahead/behind · dim when stale (>30 days idle)

#### Repo Tracker — discover and import every local repo
- New feature tab (Command palette → "Repo Tracker") that scans a configured root for direct subdirectories containing a `.git` and lists each with live status badges
- Scan root configured via new `RepoScanRoot` field in `config.json` (defaults to `~/Projects` when unset). Tilde expansion handled automatically
- Already-imported repos are detected by walking the typed-objects index for project notes with `repo:` matching the absolute path; row gets a green ✓ marker. Enter on those jumps to the existing note instead of creating a duplicate
- Enter on a not-yet-imported row writes `Projects/<repo-name>.md` with `type: project`, `status: active`, `repo: <abs path>` pre-filled, then opens the new note in the editor — instant capture for the local "what am I building?" inventory
- Keys: `j/k` nav · `Enter` import (or jump) · `g` jump-only · `r` rescan + drop status cache · `Esc` close
- One-level-deep scan keeps it fast — matches the typical `Projects/foo/` layout; deeper hierarchies use the agent runtime instead

#### New built-in saved view: "Code Projects"
- `type:project` where `repo: exists` — surfaces every project with a local git repo as a smart collection. Pin it as your dashboard primary view to see your build pipeline at a glance

#### Tests
- 6 new RepoTracker tests: scan finds repos, Enter imports + builds correct frontmatter, already-imported row jumps instead of duplicating, empty-root hint, tilde expansion, absolute-path passthrough

### Added + Fixed — Phase 13: Saved View parity, Task Manager project filter, midnight-flake hardening

#### Saved View tab: `/` filter + `D` delete (full UX parity with Object Browser)
- The two big gaps in the saved-views experience — no in-result search, no way to delete an object you've decided isn't worth keeping — are closed
- **`/`** enters filter mode: characters narrow the displayed match list by case-insensitive title-substring; Enter commits, Esc clears. Composes with the view's where clause (where runs first, filter narrows the result). Header shows `(N/total)` so you see how much was filtered out
- **`D`** (also `Ctrl+D`, `Delete`) deletes the focused object: in-tab y/n confirmation prompt with full path shown ("Delete People/Alice.md ? (y/n) — irreversible…"); host removes the file via the same `deleteObjectFile` helper Object Browser uses, so behaviour is identical
- **`Esc`** behaviour clarified: clears active filter first, then returns to picker (less drastic than closing the tab)
- Empty-state distinguishes "view has nothing" from "filter excluded everything" — the fix is different and now the message says so

#### Task Manager: `=` filters to cursor task's project
- New shortcut: with the cursor on a task whose `Project` field is set, **`=`** narrows the list to tasks for that project. Press `=` again on any task in that project (or with the same project name) to clear the filter — toggle UX
- Mnemonic: `=` as in "show me what equals this project"
- Composes with the existing `9` By-Project view, the tag filter, the priority filter, and triage filters (all stack via `applyActiveFilters`)
- No-op when the cursor task has no project set, with a clear hint: "No project on this task — set one via frontmatter or @project:Name"
- Status bar confirms: `Filter: project = Apollo (= to clear)`
- Help bar gained `= this project`; `help.go` task-manager section gained a "PROJECT & TAG FILTERS" subsection covering `=`, `P`, and `#`

#### Cleanup: midnight-window flaky tests
- Three tests (`TestPomodoro_StartForCurrentBlock_SeedsQueueFromOverlappingBlock`, `TestPomodoro_FinishWorkSession_AppendsDoesNotCollapseRepeats`, `TestPomodoro_StartForCurrentBlock_AllowsRestartFromBreak` in `internal/tui`, plus `TestTodayTotalTime_OnlyTodaySessions` in `cmd/granit`) used `now ± Nminutes` to construct planner blocks or sessions. When the suite happened to run within ~30 minutes of midnight, the synthesised time strings straddled a date boundary and the assertions failed
- New `skipIfMidnightWindow` helper (in `planmyday_test.go`, mirroring the existing `skipIfLateEvening`) gates these tests; `cmd/granit` test got an inline guard. Tests now skip cleanly in the danger window instead of flapping

#### Tests
- 9 new tests: SavedViews `/` enters filter mode, filter narrows by title substring, Esc clears, `D` arms delete, y commits, n cancels, Enter commits the filter while preserving results; Task Manager `applyProjectFilter` keeps only matching project (case-insensitive), empty-filter no-op safety

### Fixed + Added — Phase 12: reliable tab cycling, in-view object creation, doc refresh

#### Tab cycling now works in every terminal
- The original `Ctrl+Tab` / `Ctrl+Shift+Tab` shortcuts were intercepted by gnome-terminal, alacritty, kitty, iTerm and many other terminals (each grabs Ctrl+Tab for THEIR tab switching, or sends an escape sequence bubbletea doesn't decode). Users couldn't switch granit tabs at all in these environments
- Added two new shortcut pairs that **always** reach the TUI:
  - `Ctrl+PageDown` / `Ctrl+PageUp` — browser convention, universally supported
  - `Alt+.` / `Alt+,` — one-handed, mnemonic (`<` and `>` are `,` and `.` shifted)
- Both pairs added to the passthrough chord list so feature tabs don't trap them
- `help.go` Editor Tabs section explicitly calls out the terminal-intercept issue and recommends the alternatives; sidebar's tab-switching mini-section updated to list all six bindings

#### Saved View tab: `n` quick-creates an object of the view's type
- Inside a loaded saved view (e.g. "Articles to Read"), pressing `n` opens an inline title prompt — the same flow as Object Browser's `n` but the type is auto-resolved from the view's `Type` field
- After Enter, the host writes the file using the type's `Folder` + `FilenamePattern` and pre-populated frontmatter, closes the saved-view tab, and opens the new note in the editor
- Esc cancels; empty title rejected; existing-file collision surfaces as a status warning
- No-op for views with `Type: ""` (no schema to instantiate from)
- Hint footer gained `n new` when the view has a type

#### Object Browser preview pane visible at narrower widths
- Threshold lowered from **110 → 95 cols** so users on the typical 13"-15" laptop terminal (~95-120 cols) actually see the preview pane
- Preview width also shrinks (40 → 32 cols) on tighter screens so the gallery doesn't get squeezed when both panes share the available space

#### Documentation refresh
- `README.md` Typed Objects section now lists all 13 built-in types, all the recently-added keys (`n` create, `D` delete, `Alt+N` quick-add, `Alt+V` saved views), the Project/Goal Hub strip, and the dashboard + daily-jot integrations
- `docs/FEATURES.md` Saved Views section now lists all 8 built-in views (added Active Goals, Overdue Goals); new "Project / Goal Hub strip" subsection; new "Object Browser actions" subsection covering `n` / `D` / preview-width threshold

#### Tests
- 4 new tests: passthrough-chord guard for the new tab-cycle shortcuts; SavedViews `n` quick-create commits with consumed-once semantics; SavedViews `n` is a no-op for type-less views; SavedViews create-prompt Esc cancellation

### Fixed — Phase 11: discoverable delete (tasks + typed objects)

#### Task Manager: Ctrl+D and Delete now delete a task
- The original delete shortcut was `!` (shift+1) — undiscoverable. Users couldn't find how to delete tasks at all
- Now bound to **`Ctrl+D`** (universal convention), **`Delete`** (the keyboard key), and `!` (legacy alias kept for muscle memory) — all three trigger the same y/n confirmation prompt + irreversible disk removal via `TaskStore.Delete`
- Footer keybind hint bar updated to surface "Ctrl+D delete" prominently — was hidden behind the obscure `!`
- `help.go` task-manager section explains all three bindings + the confirmation gate

#### Object Browser: 'D' deletes the focused object
- Same gap existed for typed objects — once captured, no way to remove them through the UI
- New `D` key (also `Ctrl+D`, also `Delete`) on a focused object row arms a y/n confirmation in the footer ("Delete People/Alice.md ? (y/n) — irreversible, removes the underlying note file")
- y commits → host removes the file from disk, refreshes vault scan, rebuilds the typed-objects index, clears active editor tab if it was open. n / Esc cancels
- Defence-in-depth: the file path comes from the typed-objects index but the helper still rejects paths containing `..` and outside the vault root
- Only fires from the grid pane — `D` on the type list is a no-op so users can't accidentally delete an entire type by hitting D in the wrong column
- Footer keybind hint updated: `j/k nav · Tab swap pane · Enter open · n new · D delete · / filter · Esc close`

#### Tests
- 4 new tests for the Object Browser delete flow: D arms confirmation, y commits with consumed-once semantics, n cancels, D on type-list pane is a no-op

### Added — Phase 10: goal type, project/goal hub strip, quick-add task

#### `goal` is a first-class typed object
- New built-in type joining the 12 existing ones — total now 13. Properties: `title`, `status` (active/completed/paused/archived — mirrors `GoalStatus` from the legacy GoalsMode for future migration), `target_date`, `priority` (low/medium/high), `why`, `started`, `tags`. Folder: `Goals/`
- Two new built-in saved views: **Active Goals** (sorted by target_date asc) and **Overdue Goals** (active + target_date present)
- Coexists with the legacy GoalsMode store — no migration of existing goal data, no breaking changes. Users can create new goals as typed-object notes via Object Browser → `n`, and they participate in saved views, agents, the dashboard panel, and the agents `query_objects` tool

#### Project / Goal Hub strip above the editor
- When the active note is a typed-project or typed-goal, granit prepends a 1-line summary strip above the editor: `🎯 Project: Apollo  ● active  · 7 tasks (3 done)  · Alt+N to add task`
- Strip pulls from already-cached state (`m.cachedTasks` + `objectsIndex` + the note's frontmatter) — zero extra I/O per render
- Counts logic differs by type: projects match by `Task.Project == obj.Title` (uses the Phase 9 enrichment); goals match by `Task.NotePath == obj.NotePath` (tasks written inside the goal note). A future enrichment could honour `goal:G…` references too
- Status badge with semantic colours (green for active, blue for completed/shipped, yellow for paused, dim for archived/abandoned/backlog)
- Hidden on regular notes, on feature tabs, and on the welcome screen — strip only renders when it has something to say

#### Quick-add task (Alt+N)
- On a typed-project or typed-goal note, `Alt+N` appends a `- [ ] ` line at end of file with the cursor placed right after the prefix, ready for the user to type. Inserts a blank line before it when the previous content isn't already empty so the new task isn't visually glued to prior text
- New `Editor.AppendTaskLine()` primitive with full undo support
- No-op on regular notes (silently does nothing — no "wrong-context" error, just doesn't fire)
- Status bar confirms `✚ task — type the title, Esc to cancel`

#### Cleanup
- Fixed two pre-existing flaky tests (`TestPlanMyDayParseAIResponseValid` / `TestPlanMyDayParseAIResponseMalformed`) that asserted on `23:xx` schedule slots and silently broke when the suite happened to run between 23:15 and midnight. Added `skipIfLateEvening` helper so they skip cleanly in the danger window instead of flapping

#### Tests
- 8 new tests: hub strip empty for non-typed and non-project/goal notes; renders for project with task counts; renders for goal with target date; empty-state hint; AppendTaskLine cursor placement and blank-line insertion; built-in starter set now includes `goal`

### Added — Phase 9: typed-objects integration with tasks, dashboard, daily jot

#### Tasks ↔ Projects via typed-object frontmatter
- Tasks already had a `Project` field on the struct (`internal/tasks/task.go:80`) but it was unwired in the TUI. Now `Model.currentTasks()` runs an enrichment pass that populates `Task.Project` from:
  1. The note's `type: project` typed-object — task's project = the project's title
  2. A `project: <ref>` frontmatter key on the note — free-form ref string
  3. (Existing) An explicit `@project:X` mention in the markdown body — never overwritten
- The Task Manager's existing **By-Project view** (key `9`) and project chip rendering now light up automatically — no UI changes needed
- Pure post-process pass: no I/O, no tasks-package refactor, no-op when objectsIndex isn't ready (early startup safety)

#### Dashboard typed-objects panel
- New row beneath "Today's Tasks | Recent Notes" with two columns:
  - **Left:** per-type counts sorted by frequency (top 6) with icon + label + number — single-glance "what does my vault contain"
  - **Right:** "🕐 Recently Captured" list (top 6 by mtime, with relative time), plus an inline section for the **primary saved view** (default: `articles-to-read`) showing top-5 results
- Hidden entirely when the vault has no typed objects, so brand-new users don't see an empty section confusingly
- Re-evaluates the index on each Open so frontmatter saved since the last refresh shows up immediately
- Dashboard footer hint mentions `Alt+O to browse · Alt+V for views` so the panel doubles as a discovery surface

#### Daily Jot today's-captures panel
- New "📦 Captured today (N)" inline section between the carry-over notice and the input line, listing typed objects modified today (type icon + title + type ID)
- "Today" = files whose mtime falls within the local-day window of `time.Now()` — mtime over parsed `created` frontmatter because most users don't fill that field, mtime is always present
- Capped at 6 visible rows; "+N more — Alt+O to browse" footer when there are more
- Hidden when zero, so the jot stays clean on slow days
- Capture velocity feedback loop: type a jot or capture an article via `n` in Object Browser, the jot tab refreshes the next time it's opened and you see the count tick up

#### Tests
- 9 new tests across enrichment, dashboard SetTypedObjects (counts + recent + primary view + nil-safe), DailyJot SetTypedObjects (today filter + nil-safe), task project resolution paths

### Added — Phase 8: Object Browser create-from-browser + show all types

#### `n` key creates a new object of the focused type
- Press `n` in the Object Browser to create a new note of the cursored type. A small inline title prompt opens in the footer (mnemonic placement: same row as the keybind hints, so the user knows what they're typing without an over-the-top modal)
- Enter writes the file using the type's `Folder` + `FilenamePattern` and pre-populated frontmatter (`type:`, `title:`, all required properties with their defaults — `{today}` and `{now}` substitutions applied) — then closes the browser and opens the new note in the editor for the user to fill in the body
- Esc cancels; empty title is silently rejected; existing-file collision surfaces as a status warning ("X.md already exists — choose a different title") rather than overwriting prior work
- Path & frontmatter generation extracted into `internal/objects/template.go` (`PathFor`, `BuildFrontmatter`, `SanitiseFilename`) — the agent runtime's `create_object` tool will migrate to share this in a future pass

#### Type list now shows EVERY registered type, not just populated ones
- The original UX hid empty types, leaving users wondering "where are the other 10 built-ins?" when they only had two notes typed. Now all 12 built-ins appear; empty types render dimmed (overlay0) so populated types still stand out for browsing
- Combined with `n`, this means a brand-new vault can immediately discover + create across the full type catalog without first hand-editing frontmatter

#### Help & feedback
- Footer keybind bar gained `n new`
- New regression tests: shows-all-types, prompt opens on n, prompt commits on Enter, Esc cancels, empty title rejected, path/frontmatter helpers (8 tests)

### Fixed — Phase 7: layout regressions in typed-objects views, Ctrl+Z is undo

#### Sidebar Types view + Object Browser no longer wrap rows
- Long type names + object titles previously wrapped to extra lines because lipgloss `Width()` defaults to wrapping when content exceeds the box. Fixed by hard-truncating every row with `TruncateDisplay` BEFORE rendering, then padding to a constant width
- Affects: Sidebar's Types mode (the cycle from Files mode), Object Browser type list, Object Browser grid, Object Browser preview-pane property labels, and the header strip when the untyped-types warning was wide
- Object Browser header now puts the warning on its own line instead of letting it push the rule onto a third row on narrow terminals
- Two new regression tests assert that **no rendered line exceeds pane width** for the typical 28-col sidebar / 80-col grid / 120-col full layout

#### Ctrl+Z is undo (universal convention)
- The editor now binds `Ctrl+Z` → undo and `Ctrl+Shift+Z` → redo, matching every other text editor on the planet. The non-standard `Ctrl+U` / `Ctrl+Y` aliases still work for muscle memory
- Focus / Zen mode moved from `Ctrl+Z` to `Alt+Z` (the `z = zen` mnemonic survives)
- Passthrough chord list updated: `Ctrl+Z` is now consumed by the editor (not pass-through to global), and `Alt+Z` joins the Alt-shortcut family for Focus Mode access from any feature tab
- `internal/tui/help.go` and `cmd/granit/manpage.go` documentation updated

### Added — Phase 6: inline AI editor + saved views

#### Inline AI selection edits (Alt+/ and `/` slash menu)
- The classic `/` slash menu now offers six selection-aware AI actions alongside the insert templates: **AI: Rewrite**, **Expand**, **Summarize**, **Improve**, **Shorten**, **Fix Grammar**
- `Alt+/` is a dedicated AI-only menu shortcut that **preserves the editor's selection** (typing `/` would have replaced it). Mnemonic: same key as the slash menu, plus Alt for AI mode
- Both modes route to the same dispatcher (`runAIEdit` in `internal/tui/ai_selection.go`) which uses the configured provider/model — Ollama, OpenAI, Anthropic, Nous, Nerve all work transparently
- Output is spliced back at the **originally-captured range**, not the cursor's current position — so a slow Ollama response doesn't write into the wrong spot if the user moved on
- New `Editor.ReplaceRange(sl, sc, el, ec, text)` primitive replaces multi-line ranges atomically with full undo support; clamped to content bounds so a stale dispatch can't crash on out-of-bounds writes
- Output cleaner strips common LLM preambles ("Sure! Here's the rewritten text:"), wrapping code fences, and surrounding quotes — small models otherwise frequently ignore the system prompt's "no preamble" instruction
- Per-action prompts in `aiEditPrompts` are tuned to reject explanation/preamble and preserve markdown formatting; 60s context timeout for cold-load tolerance
- Same `translateProviderError` pipeline as the agent runtime, so failures surface as actionable hints in the status bar

#### Saved views — Capacities-style smart collections
- New `internal/objects/view.go` data model: `View {ID, Name, Type, Where[], Sort, Limit}` with operators `eq`, `ne`, `contains`, `exists`, `missing`, `gt`, `lt`. AND-only for now (every real-world example was expressible as AND; OR can come later as a clause group)
- Best-effort numeric comparison: `gt`/`lt` parse both sides as floats and fail open on non-numeric values
- String comparisons are case-insensitive throughout — frontmatter values are hand-typed and we don't want users tripping over "Read" vs "read"
- Six built-in views ship with granit:
  - **Articles to Read** — `type:article` where `status != read AND status != archived`
  - **Recent Highlights** — `type:highlight` sorted by capture date desc
  - **Active Projects** — `type:project` where `status == active`
  - **Raw Ideas** — `type:idea` where `status == raw`
  - **Top-Rated Podcasts** — `type:podcast` where `rating > 3` desc
  - **Currently Reading** — `type:book` where `status == reading`
- Vault-local overrides at `.granit/views/<id>.json` REPLACE built-ins by ID (same full-override semantics as Type registry / Preset catalog — merge semantics on user-edited JSON make for surprising behaviour, full override is the simpler mental model)
- Filename basename must match the embedded `id` field (case-insensitive); per-file errors surface together so the UI can render them all at once
- New `SavedView` feature tab opens via `Alt+V` (or palette → "Saved Views") in catalog-picker mode; Enter loads the cursored view, `p` returns to picker, `r` re-evaluates against the latest index, Enter on an object opens the underlying note
- Result list renders title + up to 3 property columns pulled from the matched objects; em-dash for empty values
- Re-evaluates against a refreshed index when the vault changed since the last interaction (same lazy-refresh pattern as Object Browser)

#### Tests
- 27 new tests across the package: slash-menu mode/filter/dispatch, AI output cleaner (preamble/fence/quote stripping), `ReplaceRange` (single-line, multi-line, CRLF normalization, clamping, reversed ranges), SavedViews picker→view round trip, jump request, refresh
- View engine: 18 tests covering each operator, sort semantics, limit, promoted-field access (title/type), built-in validity, vault-local override + filename mismatch handling

### Added — Phase 5.5–5.7: Anthropic provider, AI provider self-test, live Ollama model list

#### Anthropic Claude as a first-class provider
- New `chatAnthropic` path in `aiconfig.go` speaks the Messages API directly (`x-api-key`, `anthropic-version: 2023-06-01`, top-level `system`)
- New config fields `AnthropicKey` + `AnthropicModel` (default `claude-haiku-4-5`); presets and agents pick this up automatically when `AIProvider == "anthropic"` or `"claude"`
- Settings panel exposes both fields with a curated model dropdown (`claude-haiku-4-5`, `claude-sonnet-4-6`, `claude-opus-4-7`)
- Provider key separated from `OpenAIKey` so a user can keep both configured side-by-side and flip between them with one keystroke

#### `>> Test AI Provider` action button
- One-shot button in Settings that fires a `ping` against the currently-configured provider/model and surfaces the result in `setupStatus`
- Uses the same `translateProviderError` translator as the agent runtime so the failure mode you see during a manual test matches what an agent would hit
- 30s context timeout — long enough for an Ollama cold-load on a chunky model, short enough that a misconfigured cloud key fails fast

#### Live Ollama model list
- The Ollama Model dropdown now queries `/api/tags` on the configured `OllamaURL` (800ms timeout) and renders the user's actually-installed models instead of a guessed list
- Falls back to a curated starter list when the daemon is down — and *always* includes the current selection so switching back to a known-good model is never blocked by a daemon being offline
- OpenAI dropdown widened to include `gpt-4.1`, `gpt-4.1-mini`, `gpt-4.1-nano`, `o1-mini`, `o3-mini`

### Added — Phase 5: 7 new built-in types, per-preset model, provider error hints, Object Browser preview pane

#### 7 new built-in types (Capacities parity)
- `article` 📰 — saved web articles with URL, author, status, publication
- `podcast` 🎙️ — episodes with show, host, duration, rating
- `video` 📺 — YouTube/Vimeo videos with channel, URL, watched date
- `quote` 💬 — pithy quotes with author + source attribution
- `place` 📍 — venues, cities, restaurants with kind + rating
- `recipe` 🍳 — cooking recipes with cuisine, course, prep/cook times
- `highlight` 🔖 — passages worth remembering (with link to source note)

Total built-in types now 12 (was 5). Same JSON schema as before so vault-local overrides at `.granit/types/<id>.json` work for these too.

#### Per-preset model override
- New `Model` field on `agents.Preset` — preset declares which model to use for its runs
- Provider is NEVER overridden — preset rides the user's configured provider (Ollama / OpenAI / Anthropic / Nous / Nerve) with this model name swapped in
- Pattern: fast cheap models (`qwen2.5:0.5b`) for routing tasks like Inbox Triager; bigger smarter models (`llama3.1:8b`, `gpt-4o-mini`) for Research Synthesizer
- Global Settings stays the source of truth for the default; presets only override when explicitly set

#### Better AI provider error messages
- New `translateProviderError` in agentbridge.go converts low-level network/API errors into actionable hints:
  - "connection refused" → "Ollama isn't running. Start it with `ollama serve`"
  - "model not found" → "Run `ollama pull <model>` and retry"
  - 401/403 → "API key for X is invalid. Open Settings (Ctrl+,) → AI"
  - "context deadline exceeded" → "Try a smaller model or simpler goal"
- Empty-Provider check in agent runner already in place from Phase 4
- 6 new tests across the translator covering each provider's error shape

#### Object Browser preview pane (≥110-col terminals)
- 3-pane layout activates on wide terminals: type list (left) + gallery (middle) + preview (right, 40 cols)
- Preview shows: object title, type name, EVERY declared property (with em-dash for empty values), note path
- Empty properties render as `—` so the user sees schema completeness at a glance
- Falls back to the 2-pane layout on narrow terminals — no UX regression for laptop screens
- 3 new tests for preview rendering at full / narrow / out-of-range cursor

#### Documentation
- `docs/OBJECTS.md` updated with all 12 built-in types
- `docs/AGENTS.md` documents `model` preset field + the where-clause exact-match limitation
- CHANGELOG (this section)

### Added — Phase 4: typed-mention picker, sidebar Types view, vault schemas, type-aware bookmarks

#### Sidebar Types view (`m` key)
- New SidebarMode enum with two modes: Files (existing) and Types (new)
- 'm' key cycles modes when sidebar focused
- Header chip shows current mode ("files" / "types") + object count
- Types view groups typed objects by Type with icon + count badge, sorted by registry order
- j/k auto-skip type headers so navigation feels continuous
- Search ('/') filters objects by title; type headers stay visible only when at least one object matches
- Active note marked with `●`
- Empty types hidden (no `Meeting (0)` dead rows)
- Pinned typed objects render in a `★ Pinned` section at the top, mirroring Files-mode PINNED behavior
- 9 new tests covering mode toggling, row rebuild, filter, navigation, header skipping

#### Per-type bookmarks (`b` in Types view)
- `b` on a typed-object row pins/unpins it
- Pinned objects appear with a yellow ★ marker AND in the dedicated PINNED section
- Reuses the existing pinned map / persistence at `.granit/sidebar-pinned.json` — works across both views
- Status message confirms the action ("Pinned Alice", "Unpinned Bob")

#### Type-aware mention picker (`Alt+@`)
- New `TypedMentionPicker` overlay — Capacities-style structured-mention insertion without editor surgery
- Open with `Alt+@` or via command palette ("Insert Typed Mention")
- Filter syntax: bare query (e.g. `alice`) matches across all typed-object titles; `typeID:query` (e.g. `person:alice` or `book:`) scopes to a single type
- Prefix matches sort before substring matches so the most likely target lands at the top
- Enter inserts `[[Title]]` at the editor cursor — compatible with the existing wikilink renderer + publish pipeline
- 9 new tests covering filter parsing, scope syntax, navigation clamps, insert behavior, prefix-match ordering

#### Custom vault type schemas (review nudge → real galleries)
- Wrote `~/Documents/Main/.granit/types/research.json` (44 notes) and `daily.json` (13 notes) so the user's existing `type:` frontmatter no longer surfaces as "unknown types" warnings
- Both schemas follow the documented Type JSON format — hand-editable, version-controllable, vault-local

#### Help overlay
- Sidebar section now documents both modes, the cycle key, and Types-mode navigation
- Typed Objects section adds `Alt+@` for the mention picker

#### Phase 4.4 (agent-driven Daily Hub widgets) — deferred
- Background agent execution needs scheduler + result cache + cost discussion. Earmarked as a follow-up phase rather than rushed alongside Phase 4.1-4.3.

### Added — Phase 3: vault-local agent presets + 2 more flagships

The agent runtime ships with three built-in presets now (Research Synthesizer, Project Manager, Inbox Triager) and supports user-defined agents via JSON files at `<vault>/.granit/agents/<id>.json` — no recompile required. Vault-local presets replace built-ins by ID.

#### New built-in presets
- **Project Manager** — read-only. Looks up project objects via `query_objects(type=project)`, reads them, queries related tasks for blockers, produces a structured status report
- **Inbox Triager** — read+write. Walks an inbox folder, proposes next-action tasks via `create_task` (capped at 5 per run, every proposed write shown in the live transcript so the user can `Esc`-cancel)

#### Custom agent definitions
- New `Preset` schema in `internal/agents/preset.go` — JSON-serializable, hand-editable
- `PresetCatalog` merges built-in + vault-local with full-override semantics
- Validation rejects malformed presets per-file with clear errors; remaining presets still load
- Filename basename must match embedded `id` (case-insensitive); mismatches surface in the picker's status line
- `BuildRegistryForPreset` filters tool factories by the preset's `tools` allow-list and `includeWrite` flag — preset opt-in for write access without rewriting the runner
- `MaxSteps` override per-preset (0 falls through to runtime default of 8)

#### Runner integration
- `agentRunner.Open()` now loads built-ins then overlays vault-local presets each time the overlay opens — fresh definitions show up without an app restart
- Write tools wired into the agent bridge (`agentRunner.writeNote`, `appentTaskLine`) — actually mutate the vault when the LLM calls them, with the in-memory vault cache updated so subsequent reads see the write
- Approve callback shipped at "auto-approve with transcript visibility" — every proposed write is rendered in the streamed transcript before execution, user `Esc`-cancels if needed

#### Quality fixes (review pass)
- `agentrunner.go` race fix: agent goroutine now captures `eventCh` / `doneCh` in local closure variables instead of via the receiver, so a rapid Esc-then-Alt+A flow can't make the old goroutine write to the new run's channels
- `agent.go` parser: defensive `strings.TrimSpace` on captured arg values to guard against `\s*$` regex edge cases that leak trailing whitespace into path arguments
- `objectbrowser_index.go` parser: handles tab-indented YAML continuation lines (some editors emit tabs in YAML); also skips keys with empty values whose next non-blank line is an indented continuation, so `tags:\n  - foo` no longer leaves a stray empty `tags` entry in the flat map
- `feature_tabs.go`: ObjectBrowser now refreshes its typed-objects index when the vault has changed since the last interaction (was previously stale until manual re-open)

#### Tests
- New `internal/agents/preset_test.go` — 9 tests covering Validate/built-in loading/vault-local override/round-trip/registry filtering
- New `internal/tui/agentrunner_test.go` — 5 tests covering event rendering, line truncation, defensive tag-copy in agentTaskBridge, parseFlatFrontmatter (including the regression for the empty-key-with-continuation bug)
- Total: ~60 agent-related tests across the two packages, all green

#### Documentation
- `docs/AGENTS.md` updated: new presets section, custom-agent JSON schema with field-by-field reference, override semantics
- CHANGELOG (this section)

### Added — Multi-step Agent Runtime (Deepnote-inspired)

Beyond the existing 19 single-shot bots, granit now supports multi-step agents: an LLM that picks tools from a registered catalog, observes their output, and iterates in a Thought / Action / Observation loop until it produces a final answer. Inspired by Deepnote's tool-calling AI; designed to work on any LLM including small local Ollama models (text-based ReAct protocol, no JSON-mode required).

#### Core (`internal/agents/`)
- New package, fully independent of the TUI — `Tool` interface, `Registry`, `ToolCall`, `ToolResult`, ReAct loop, `LLM` interface
- 6 read tools: `read_note`, `list_notes`, `search_vault`, `query_objects`, `query_tasks`, `get_today` — all bounded (path containment, output truncation, result-count limits)
- 3 write tools (off by default in v1): `write_note` (refuses overwrite without `overwrite=true`), `create_task` (assembles "- [ ] text 📅 date 🔼" line), `create_object` (assembles frontmatter + body for typed objects)
- `Options.MaxSteps` budget cap (default 8) prevents runaway loops
- `Options.Approve` callback required when write tools registered (fail-fast at construction, not silent)
- `Options.OnEvent` streams Goal / Thought / ToolCall / ToolResult / FinalAnswer / Error / BudgetHit events for live UI
- 45 tests across the package — registry contract, validation, parser variants, full happy-path runs, recovery from bad actions, budget cap, write approval gate, LLM error propagation, context cancellation

#### TUI integration
- `Alt+A` opens the agent runner overlay (also "Run Agent" in command palette)
- Phase 2.5 ships one flagship preset: **Research Synthesizer** — given a topic, finds related notes via `search_vault`, reads them with `read_note`, synthesises themes + open questions
- Streamed live transcript with per-step Thought / Tool call / Observation rendering, distinct colours for each event kind
- Cancellation via `Esc` during a run honours `ctx.Done()` on the next LLM call boundary
- Reuses the existing `AIConfig` (Ollama / OpenAI / Nous / Nerve), so nothing leaves the user's machine unless an external provider is configured

#### Wire format
- Plain-text Thought / Action / Args / Final Answer protocol — works on any model
- Parser is line-walking (Go's RE2 lacks lookaheads); handles case-insensitive headers (`Thought:` / `THOUGHT:` / `thought:`) and multi-line thoughts/answers
- Tool catalog rendered into the system prompt with arg schemas so the LLM picks correctly without hardcoded prompt templates

#### Safety
- KindRead vs KindWrite tool classification — registries without write tools cannot mutate disk
- Path containment on every read+write tool that takes a path (refuses `..` escapes and absolute paths)
- Approve callback gates every write before the tool runs; in interactive mode surfaces a confirmation prompt
- Full audit transcript returned from every Run — Step, Thought, ToolCall, ToolResult — for compliance / debugging

#### Documentation
- New `docs/AGENTS.md` — full reference: why agents vs bots, tool catalog, safety model, architecture, how to write goals
- README "Multi-step AI Agents" feature section + docs table entry
- CHANGELOG (this section)

### Added — Typed Objects: Capacities-style structured notes

A new layer on top of plain markdown that lets notes declare a `type:` in frontmatter (Person, Book, Project, Meeting, Idea — or any custom type) and treats them as structured objects with galleries, schemas, and (in Phase 2/3) typed mentions + queries. Inspired by Capacities; designed to stay file-first, git-friendly, and Obsidian-compatible.

#### Core
- New `internal/objects/` package — Type / Property / PropertyKind schema, JSON-serializable
- 8 property kinds: text, number, date, url, tag, checkbox, link, select
- Per-vault type definitions at `.granit/types/<id>.json` (full override of built-ins by ID, not deep merge)
- 5 built-in starter types: person, book, project, meeting, idea — opinionated 4-6 properties each, never a long-tail JSON-Schema
- Validation surface: per-Type `.Validate()`, per-Property `.Validate()`, registry guards `Set` against invalid input

#### Object Browser (`Alt+O`)
- New TUI feature tab — two-pane layout, type list left, gallery grid right
- Auto-derived columns from type properties (title + up to 3 property columns, prioritising required fields)
- In-grid filter (`/`) matches title OR any property value, case-insensitive
- Per-type object counts, with empty types hidden from the list
- "⚠ N notes reference unknown types" warning when a frontmatter `type:` doesn't match any registered schema
- Object selection (`Enter`) opens the underlying note in the editor pane and closes the browser tab
- Mobile-aware: collapses to title-only when pane width < 30 cols

#### Index pipeline
- `rebuildObjectsIndex` walks the vault on startup and on vault refresh, parses frontmatter into an `objects.Index` keyed by TypeID
- `parseFlatFrontmatter` is a shallow YAML parser (single-line `key: value` only) intentionally decoupled from any specific YAML library — the typed-object schema covers the common case, richer YAML belongs in the body
- Sub-millisecond rebuild on a 1000-note vault

#### Tests
- 28 tests across `internal/objects/` covering type/property validation, JSON round-trips, registry overrides, builder semantics, search filtering, nil-safety
- 10 ObjectBrowser TUI tests covering Tab/Enter/search/refresh flows + column-spec heuristics
- Built-in type lints: every default validates, has a unique ID, ships an icon, declares at least one required field

#### Documentation
- New `docs/OBJECTS.md` — full reference: quick start, frontmatter conventions, custom types, property kinds, keyboard, roadmap
- README "Typed Objects" feature section + docs table entry
- TUI help overlay (F5) entry under Knowledge Management
- CHANGELOG (this section)

### Added — `granit publish` static site generator

A new top-level subcommand that renders any folder of markdown notes into a self-hostable static website. Inspired by Obsidian Publish; designed for GitHub Pages but works on any static-file host (Cloudflare Pages, Netlify, S3, fleetdeck, plain VPS via rsync). Output is plain HTML + one CSS file + a small vanilla-JS search shim — no Node.js, no build step.

#### CLI
- `granit publish build <folder> [flags]` — render to `./dist` (or `--output`)
- `granit publish preview <folder> [flags]` — build + serve on `http://localhost:8080`
- `granit publish init <folder>` — write a starter `.granit/publish.json` template
- Flags: `--output`, `--title`, `--homepage`, `--site-url`, `--author`, `--auto-og`, `--og-image`, `--math`, `--mermaid`, `--hero`, `--cookie-banner`, `--no-branding`, `--no-search`, `--config`

#### Content rendering
- Goldmark markdown with GFM (tables, autolinks, strikethrough, task lists)
- Code highlighting via chroma (B&W "bw" style, no color)
- Wikilinks (`[[Note]]`, `[[Note|Display]]`, `[[Note#section]]`) resolved across the published note set
- Auto-rewrites relative image paths (`![alt](./diagram.png)`) so they resolve from the published note's URL
- Image asset copying: every non-markdown file in the source folder mirrors into the output preserving relative paths
- Per-note Contents outline (auto-extracted from H2-H4)
- Backlinks panel ("Linked from") on every note
- Prev/Next note navigation (filename-ordered for `00_*` / `01_*` flows)
- Reading-time chip for notes over 100 words (220 wpm)
- KaTeX math rendering (opt-in via `--math`, loaded only on pages containing `$math$`)
- Mermaid diagrams (opt-in via `--mermaid`, loaded only on pages containing fenced mermaid blocks)

#### SEO
- `<title>`, `<meta description>`, canonical link
- Open Graph: `og:title`, `og:description`, `og:type` (article for notes, website for index/tags/graph), `og:url`, `og:site_name`, `og:image` (with width/height/type)
- Twitter Card: `summary_large_image` when og:image present, `summary` otherwise
- JSON-LD Article schema on every note page (headline, dates, author, publisher, description, url)
- `<meta name="author">` from frontmatter or site-wide `--author`
- `<meta name="robots" content="noindex,nofollow">` for legal pages and any note with `noindex: true` frontmatter
- `sitemap.xml` (standards-conformant, absolute URLs when `--site-url` is set)
- `robots.txt` allow-everything + Sitemap directive
- RSS 2.0 feed (`feed.xml`) with `<link rel="alternate">` auto-discovery on every page
- `404.html` custom not-found page
- `.nojekyll` for GitHub Pages (files starting with `_` work)

#### OG images (per-note social-share previews)
- Frontmatter `image:` (relative path in source folder)
- Site-wide default via `--og-image` / `defaultOGImage`
- Auto-generation via `--auto-og`: 1200×630 PNG per note, B&W, embedded Go font (`golang.org/x/image/font/gofont`), word-wrapped title, ~40 KB each, deterministic across builds

#### Layout & theming
- Black-and-white minimal theme (4 KB CSS, light + dark via `prefers-color-scheme`)
- Mobile-responsive breakpoints at 640px and 380px (header stacks, prev/next stacks, table overflow scroll, bigger tap targets, no iOS zoom-on-focus)
- Force-directed wikilink graph as inline SVG (Fruchterman-Reingold, 200 iterations, deterministic seed, JS-free)
- Hero homepage layout (`--hero`): centered title block + intro + 3-column note card grid with hover lift
- Default list homepage: dense vertical note list sorted by date desc

#### Legal & compliance (German / EU)
- Auto-detected Impressum / Datenschutz pages: filename heuristics (`impressum.md`, `datenschutz.md`, `imprint.md`, `privacy.md`) OR frontmatter `legal: impressum` / `legal: datenschutz`
- Render to root-level URLs (`/impressum.html`, `/datenschutz.html`), not under `/notes/`
- Auto footer links on every page when detected
- Auto-marked `noindex,nofollow` (search engines don't index legal boilerplate)
- Excluded from search index, auto note list, and graph
- Cross-site wikilinks (`[[Impressum]]`) resolve to the root URLs automatically
- Optional cookie banner (`--cookie-banner`): bottom-fixed, single OK button, localStorage dismissal, `{datenschutzURL}` placeholder substitution in custom messages

#### Search
- Client-side fuzzy filter on the homepage
- ~30 lines of vanilla ES5, no framework
- `search-index.json` carries title + first 800 chars of body per note
- Excludes legal pages and `noindex: true` notes

#### Branding
- Subtle "Built with Granit" footer link (dotted underline, muted color, opens in new tab) on every page
- Suppress with `--no-branding` / `noBranding: true`

#### Documentation
- New [docs/PUBLISH.md](docs/PUBLISH.md) — full reference: quick start, CLI flags, configuration, frontmatter, wikilinks, tags, graph, SEO, mobile, legal pages, cookie banner, OG images, math, mermaid, RSS, image assets, GitHub Pages workflow, multi-host deploy recipes, troubleshooting, roadmap
- New "Static Site Publishing" section in README and "Publishing" entry in CLI reference
- New "granit publish — Obsidian-Publish-style folder-to-website" section in docs/FEATURES.md
- Manpage entries under Data Management for `publish build` / `publish preview` / `publish init`
- TUI help overlay (F5) "Publish — static site generator" section
- `granit publish help` covers every flag with examples and deploy recipes

### Added — Relaunch Phase 3: Profiles and the Daily Hub

#### Profiles
- New `internal/profiles` package — workspaces that bundle (enabled modules + dashboard layout + templates + default bot + keybind overrides). Switch contexts in one chord without menu walking.
- 4 launch profiles: **Classic** (the pre-relaunch behavior, all modules on, the migration default for existing vaults), **Daily Operator** (planning loop foregrounded — pomodoro, tasks, calendar, habits, plan-my-day), **Researcher** (knowledge work — graph, semantic search, AI tooling), **Builder** (kanban, projects, goals, standup).
- Layered loader: built-in → `~/.config/granit/profiles/` (user-global) → `<vault>/.granit/profiles/` (vault-local). Same ID later wins. Hand-edit the JSON to fork or override anything.
- Active profile pointer at `<vault>/.granit/active-profile` (one-line text, just the ID). New vaults default to Classic, no picker spam.
- `Shift+Alt+W` opens the profile switcher (4 horizontal cards, arrow-key + Enter, number keys for direct jump). Also reachable as `Switch Profile` in the command palette.

#### Daily Hub
- Replaces the existing dashboard overlay on `Alt+H` with a 12-column widget grid driven by the active profile's `DashboardSpec`. The Classic profile mirrors the current dashboard's content (today's tasks, overdue, scripture, business pulse) plus recent notes, so existing users see "the same dashboard, but bigger and now profile-switchable."
- 10 built-in widgets at v1: `today.jot` (front-door capture cell, focus lands here on open), `today.tasks` (due-today, single-key complete), `today.overdue` (with days-overdue badge), `today.calendar` (events + planner blocks), `triage.count` (inbox count, color-coded by backlog size), `goal.progress` (top goals with `████░░` bars), `habit.streak` (today's habits + streak counts), `recent.notes` (last N modified, single-key open), `dashboard.scripture` (verse of the day), `dashboard.businesspulse` (sparkline of business metrics).
- `Tab` / `Shift+Tab` cycles widget focus, `Alt+1..9` jumps to that cell directly, `Esc` closes. Focused cell gets a brighter border. Widgets too small for their `MinSize` render an inline stub instead of a clipped half-render.
- Lua-implementable widget surface: the `profiles.Widget` interface and `WidgetCtx` shape carry no Go-only types, so a future Lua bridge can register profiles + widgets verbatim.

#### Module registry batch operations
- New `modules.Registry.SetEnabledBatch` atomically applies a multi-module enable/disable map in dependency-safe order. Profile switches that flip a dozen modules now succeed in one call instead of failing on the dep wall.

### Added — Relaunch foundations: Module Registry and Unified Task Store

#### Module Registry (Phase 1)
- New `internal/modules` package — single source of truth for which features are enabled. Replaces the scattered `config.CorePluginEnabled` checks with a first-class registry that the command palette, keybind dispatcher, and Settings UI all consult.
- 9 built-in modules registered (pomodoro, flashcards, quiz, habits, task manager, blog publisher, research agent, AI templates, language learning) with the legacy `CorePlugins` keys preserved so existing user toggles transfer transparently.
- `alt+b` (Habit Tracker) and `ctrl+k` (Task Manager) now route through the registry; disabling those modules makes their key chords no-ops automatically.
- Bug fix folded in: `Quiz Mode` and the `ctrl+k` Task Manager bypass both ignored their feature gates before — they now respect the toggle, and `ctrl+k` no longer skips project/time-tracker enrichment.
- Module declarations are JSON-shaped (no Go-only types) so a future Lua plugin bridge can implement them verbatim.

#### Unified Task Store (Phase 2)
- New `internal/tasks` package — canonical task layer with stable ULID identity that survives markdown edits. `Tasks.md` and any daily/project note remain user-editable; the store glues sidecar metadata to lines via fingerprint reconciliation.
- `.granit/tasks-meta.json` sidecar tracks per-task triage state (`inbox` / `triaged` / `scheduled` / `done` / `dropped` / `snoozed`), scheduled time blocks, project/goal links, origin (jot vs manual vs recurring vs project import), and timestamps.
- 6-pass reconciliation algorithm: exact match → same-file drift → cross-file move → ambiguous-fp by line proximity → fuzzy text match (gated at ≥0.85 similarity, ≤10 lines) → mint new ULID. IDs survive done-toggle, line moves, indent changes, due-date edits, cross-file moves, and small wording edits.
- Tombstones (30-day TTL) so a `git pull` that re-introduces a deleted task line revives the original ID instead of minting a new one — triage state and schedule attachments come back with it.
- Crash-safe: every read returns a snapshot copy under RWMutex; every write does atomic temp+rename; goroutine-safe under `-race`.
- Existing vaults migrate transparently on first open — no user action required, no markdown changes.
- Readers migrated: TaskManager, Daily Review, Weekly Review, Project Dashboard, and every Model-internal `cachedTasks` site (10 call sites). Watcher events automatically trigger reconciliation.
- Writers migrated: Ideas Board (convert-to-task), Recurring Tasks (instance generation), Goals Mode (milestone-to-task with sidecar `goal_id`), Morning Routine (new tasks from routine), Task Manager (add).
- `internal/atomicio` package extracted so the new layer can use crash-safe writes without depending on `tui`.

### Added — Calendar, Reading View, and UI Polish

#### Calendar overhaul
- **Full-width weekly view** using the entire terminal width with column separators between days
- **Hour-by-hour navigation** — `↑/↓` moves between time slots, `←/→` changes days
- **Today's column highlighted** — subtle background tint on all cells with `●` header indicator
- **Current time line** — green dashed `╌╌╌` marker with exact `▸HH:MM` across the grid
- **Event blocks** — events render with background cards, type-colored (task=blue, break=green, focus=peach)
- **Task time-blocking** (`b`) — pick from tasks due this week and assign to a time slot; writes to planner file
- **Event creation wizard** (`a`) — visible step-by-step form (title → time → duration → location → recurrence → color → description)
- **Goals strip** — active goals shown as mini progress bar badges `████░░` in the week header
- **Weekly milestone creation** (`g`) — create a milestone linked to an existing goal with end-of-week due date
- **Daily focus from Plan My Day** — `## Focus` section in planner files; shows top goal in calendar day headers
- **Calendar panel auto-loads ICS events** on startup for cockpit and widescreen layouts
- **ICS per-file toggles** in Settings > Files — show/hide individual `.ics` calendars
- **ICS parsing improvements** — RRULE supports COUNT, UNTIL, INTERVAL; error reporting for malformed dates

#### Reading view (`Ctrl+E`)
- **Auto-enables reading style** when entering view mode (was using "default" style)
- **Reading column** — 100 char max width with left margin on wide terminals
- **Clean headings** — strips bold/italic markers, simple underlines
- **Frontmatter** — dim key:value pairs instead of bordered box
- **Horizontal rules** — simple line without diamond decoration
- **Progress bar** — thin mauve/dim bar at top of reading view

#### Status bar
- **Context-aware help bar** — different keybindings for FILES, EDIT, VIEW, LINKS, and VIM modes
- **Separator line** — visual distinction between editor and status bar
- **Two-tier indicators** — alert badges (colored) for urgent items, dim badges for informational
- **Compact cursor position** — `line:col` format for all VIM modes

#### Splash screens
- **Startup** — logo reveal, expanding rule, typewriter tagline, vault info, ready check
- **Exit** — logo fade, session stats, clean goodbye
- Faster animations (25/35 ticks instead of 40/50)

#### Tab bar
- **Distinct indicators** — pin `◆` and modified `●` (were both `*`)
- **Move highlight timeout** — resets after 500ms
- **Smart truncation** — accounts for indicator width

#### Themes
- Fixed 8 themes for color differentiation (solarized, rose-pine, nightfox, iceberg, cobalt2, poimandres, vitesse-dark, ayu-light)
- Surface hierarchy fixes (solarized had identical Surface0/1/2)
- Custom theme JSON fallback validation for missing color fields
- Separator standardization (dashed → solid in rose-pine, poimandres)

#### Sidebar / Explorer
- Fixed width calculations using `lipgloss.Width()` for emoji icon support
- Accent bar positioning no longer overflows selection highlight

#### Error handling
- **Canvas** — save/load errors shown as dismissible status messages
- **Blog publisher** — token masking, API error parsing, field validation before publish
- **Flashcards** — corrupted progress file warning with graceful fallback
- **Quiz** — true/false questions now generate both true and false answers

#### AI improvements
- **Small-model prompt optimization** for PlanMyDay, TaskTriage, AIScheduler
- **Skip retry on timeout** for small models (was doubling wait time)
- **90-second streaming timeout** for small models (was 5 minutes)
- **Live streaming preview** — AI output visible token-by-token during PlanMyDay and TaskTriage
- **Ollama server management** — start/stop from Settings with process tracking and zombie prevention

#### Infrastructure
- `loadActiveGoals()` shared helper for goal data access
- `loadPlannerBlocks()` now returns daily focus data alongside planner blocks
- `renderStreamPreview()` shared helper for streaming AI output display
- `stripInlineMarkers()` for heading text cleanup
- `sameDay()` helper for date comparison
- Settings page navigation (PgUp/PgDn, Home/End)
- Task manager height reservation fix for help bar visibility

### Added — AI Reliability & Quality Overhaul

#### AI infrastructure

- **Small-model auto-detection** — `AIConfig.IsSmallModel()` identifies models ≤3B parameters by suffix (`0.5b`, `1b`, `1.5b`, `2b`, `3b`) or family name (`tinyllama`, `phi3:mini`, `gemma:2b`, etc.). Every AI feature adapts prompt size, system prompts, temperature, and context limits when a small model is detected.
- **Temperature tuning** — small models get `temperature: 0.3` for focused, deterministic output; larger models get `0.7` for creativity.
- **`num_ctx` / `num_predict` tuning** — small models get 2048/512; larger models get 4096/1024. Sent as Ollama options on every request so small models don't silently truncate context.
- **`ChatShort` / `chatOllamaCtx`** — dedicated short-response API with `num_predict: 64` for ghostwriter, auto-tagger, and auto-link suggest. 4-8× faster on small models.
- **Retry on transient errors** — `Chat`, `ChatShort`, and streaming all retry once with a 500ms backoff on connection refused, timeout, EOF, reset. Permanent errors (bad API key, missing model) are not retried.
- **Real HTTP cancellation via `context.Context`** — `sendToAIStreamingCtx` returns both the channel and a `context.CancelFunc`. Pressing Esc in AI Chat, Plan My Day, or Task Triage aborts the actual HTTP request, freeing the local model CPU/GPU immediately.
- **Hard per-request deadlines** — ghostwriter 15s/30s, auto-tag/auto-link 45s/90s, bots 3min. A stuck Ollama can't lock the UI forever.
- **In-flight guards** — auto-tagger and auto-link suggest skip new requests while a previous one is running, preventing pile-up on rapid saves.
- **Token-budget fit checks** — `EstimateTokens` and `PromptFitsContext` let auto-features skip oversized prompts gracefully instead of silently overflowing the context window.
- **Empty-response fallback** — when a small model returns an empty/whitespace-only response, bots fall back to local analysis with a clear yellow warning.
- **Word-boundary truncation** — new `truncateAtBoundary` helper used across all content truncation. No more mid-word cuts that confuse small models.
- **Ghostwriter completion cache** — 32-entry FIFO cache keyed by context. Backspacing and retyping the same content is a zero-latency cache hit. Invalidated on model change.
- **Elapsed time display** — every AI loading screen (bots, aichat, planmyday, tasktriage, devotional, morningroutine, dailybriefing, composer, blogdraft, threadweaver, vaultrefactor, aitemplates, writingcoach) shows elapsed seconds with slow-model hints after 15s / 30s.
- **Streaming retry** — streaming paths also retry the initial connection once on transient failures.

#### Bots — from 11 to 19

New bots added:
- **TL;DR** — one-sentence summary capturing the single most important idea
- **Explain Simply** — rewrites for a curious 12-year-old using everyday analogies
- **Outline Generator** — hierarchical outline with markdown headings and bullets (local fallback extracts existing headings)
- **Key Terms** — glossary of 5-10 key terms with 1-sentence definitions grounded in the note's context
- **Counter-Argument** — 3-5 strong opposing viewpoints to sharpen thinking (devil's advocate)
- **Pros & Cons** — structured decision-analysis list with color-coded green pros and red cons
- **Expand** — flesh out a terse note with detail while preserving author's voice

#### Bot UX polish

- **6 semantic categories** — SUMMARIZE, WRITING, ANALYSIS, ORGANIZE, LEARNING, VAULT, rendered as bold section headers
- **Type-to-filter** — just start typing in the bot list to filter by name or description (case-insensitive); `Esc` clears the filter
- **Number-key quick-pick** — press `1`–`9` to run the first nine visible bots instantly
- **Remember last-used bot** — cursor automatically positions on the most recently used bot when the overlay opens
- **Wrap-around navigation** — `↑` at the top wraps to the last item; `↓` at the bottom wraps to first
- **Home/End navigation** in bot list
- **Results navigation** — `pgup`/`pgdn`/`ctrl+u`/`ctrl+d` for page scrolling, `g`/`home` top, `G`/`end` bottom, `r` to re-run the same bot (invaluable for small-model retries)
- **Copy to clipboard** (`c` or `y`) — copies raw AI response via xclip/xsel/wl-copy/pbcopy
- **Save to note** (`s`) — writes result to `<vault>/Bots/<note>-<bot>-<timestamp>.md` with full YAML frontmatter (source wikilink, bot name, provider, model, `ai-generated` tag)
- **Results header metadata** — shows model name + elapsed time (`qwen2.5:0.5b • 4.2s`)
- **Animated "comet" progress bar** during loading with category pill and yellow elapsed time
- **Per-bot system prompts** — each of 19 bots gets a role-specific system prompt with a compact small-model variant

#### Git overlay

- **Fixed: vault path** — `GitOverlay` now accepts the vault root via `Open(vaultRoot)` and passes it to every `git` invocation as `cmd.Dir`. Launching granit from anywhere and hitting the Git overlay now finds the repo correctly (was broken for vaults in subdirectories).
- **Unicode commit messages** — commit input now accepts emoji, accented characters, and all non-ASCII text (was ASCII-only due to byte-length check).

#### Editor

- **Unicode input** — the editor now correctly handles multi-byte UTF-8 characters for typing (emoji, accented letters, CJK). Rune-aware length check replaces the byte-length check that silently dropped non-ASCII input.

#### Prompt quality (all features)

- **Small-model prompt variants** for 20+ features: bots, devotional, morningroutine, dailybriefing, dailyreview, weeklyreview, goalsmode, habits, projectmode, composer, blogdraft, writingcoach, nlsearch, aiprojectplanner, tasktriage, planmyday, devotional, and more.
- **Per-bot system prompts** replace the generic "helpful assistant" prompt with role-specific instructions (tagger, summarizer, title generator, devil's advocate, etc.)
- **Deterministic map iteration** — Question Bot and MOC Generator sort vault paths before iterating so Go's random map order doesn't produce inconsistent results.
- **Multi-line YAML tag parsing** — `extractFrontmatterTags` now handles Obsidian's `tags:\n  - foo\n  - bar` format in addition to inline `[a, b]` and `a, b, c`.
- **Unicode tag support** — `atParseSuggestedTags` preserves accented letters and digits via `unicode.IsLetter`/`IsDigit` instead of stripping to ASCII.
- **Robust list-item parsing** — shared `stripListPrefix` regex handles `1.`, `1)`, `1:`, `- `, `* `, `• ` consistently in title/link suggester output parsing.
- **Tag deduplication** — auto-tagger de-dupes case-insensitively and handles both comma- and newline-separated output.
- **Sentence-boundary detection in ghostwriter** — completion cleanup only breaks at `. ` followed by ≥4-letter word then uppercase, preserving "Dr. Smith", "e.g.", "Mr.", "Ph.D", "etc.".
- **Whole-word matching in ghostwriter** — `findRelatedVaultNote` tokenizes titles on word boundaries and requires a minimum match score, eliminating spurious matches like "test" → "contest".
- **AI chat keyword filter** — preserves domain abbreviations (`ai`, `ml`, `ui`, `ux`, `os`, `db`, `go`, `js`, `ts`, `py`).
- **Ollama model matching** — tightened so configuring `phi` no longer silently matches `phi3.5:mini`; uses explicit `:` / `-` boundary check.

#### Tests

- **`aiconfig_test.go`** — 11 test functions covering `IsSmallModel`, `MaxPromptContext`, `OllamaOptions`, `OllamaOptionsShort`, `TruncateAtBoundary` (+ property test), `EstimateTokens`, `PromptFitsContext`, `IsTransientAIError`, `ModelOrDefault`, `OllamaEndpoint`.
- **`ai_helpers_test.go`** — 11 test functions covering `StripListPrefix`, `AtParseSuggestedTags` (with unicode), `ExtractFrontmatterTags` (all three YAML formats), `GhostCleanCompletion`, ghostwriter cache LRU/hit/miss/invalidation, bot filtering, per-bot system prompt coverage, category integrity, wrap-around navigation.
- **~100 new assertions** — caught 2 real bugs during development (quoted YAML inline-list trim order; "Dr. Smith" sentence-boundary false positive).
- **Race detector clean** — full TUI package passes under `go test -race`.

### Added — previous

- **Universal search shortcut** (`Ctrl+/`) — global keyboard shortcut opens Search Everything overlay instantly
- **Milestone due dates** (`!` key) — set due dates on individual milestones (1=1wk, 2=2wk, 3=1mo, 4=3mo); color-coded red/yellow when overdue or approaching
- **Milestone reordering** (`J`/`K` keys) — move milestones up and down within a goal
- **Goal progress sparkline** — expanded goals with 2+ reviews show ASCII sparkline chart (▁▂▃▄▅▆▇█) from review history
- **Recurring task auto-creation** — completing a recurring task (daily/weekly/monthly/3x-week) automatically creates the next instance with updated due date
- **Task-to-calendar sync** — tasks with due date markers now appear in calendar week grid (9AM badge) and agenda view alongside daily note tasks
- **Active goals in daily planner** — planner shows active goals with progress; included in clipboard copy and markdown export
- **Theme-aware goal colors** (`C` key) — assign one of 7 theme colors (blue/red/green/yellow/mauve/pink/teal) to goals; applied to status icon and progress bar
- **Export daily plan as markdown** (`Shift+S`) — saves plan to `Plans/plan-YYYY-MM-DD.md` with frontmatter
- **Onboarding tutorial update** — new page 7 covering the productivity suite (tasks, goals, planner, search)
- **Copy daily plan to clipboard** — press `c` in daily planner or command palette > "Copy Daily Plan"; formats schedule, tasks, habits, and active goals as shareable text
- **Standalone Goals module** — first-class goal manager (command palette > "Goals") with 4 views (Active/Category/Timeline/Completed), milestone CRUD, goal lifecycle (active/paused/completed/archived), categories, target dates, overdue detection, progress bars, notes, creation wizard, and help overlay; stored in `.granit/goals.json`
- **Goal Manager UX** — quick-pick date selection (1-7 keys for 1mo to 5yr), numbered category picker, human-readable timeframe badges ("3mo left"), color-coded progress bars, edit title (`e`), edit description (`E`), permanent delete (`D`), archive (`A`), inline milestone counts
- **`{{active_goals}}` template variable** — daily notes can include active goals with progress percentage
- **Task archiving** (`X` key) — move completed tasks older than 30 days from Tasks.md to `Archive/tasks-YYYY-MM.md`; undated completed tasks also archived
- **Task templates** (`T`/`t` keys) — save any task as a reusable template (`T`), create from template (`t` + number); persisted to `.granit/task-templates.json`
- **Natural language dates** — quick-add now parses `@next week`, `@next friday`, `@end of month`, `@in 3 days`, `@in 2 weeks`, `@next month`
- **Overdue status bar badge** — red "N overdue" badge in status bar alongside yellow "N due" badge; updates on save and refresh
- **Task manager help overlay** (`?` key) — full keybinding reference organized into 5 sections (Navigation, Actions, Power Features, Filters, Bulk); any key closes
- **10-deep undo stack** — `u` key now supports up to 10 consecutive undos; shows remaining count "Undone (3 more)"
- **29 new tests** — comprehensive test coverage for auto-priority, snooze, Eisenhower quadrants, inline syntax, natural language dates, undo, pinned tasks, time tracking
- **Eisenhower Matrix** — 7th task manager view (`7` key); 2×2 grid (DO / SCHEDULE / DELEGATE / ELIMINATE) based on priority and due date proximity
- **Batch reschedule** (`R` key) — walk through all overdue tasks one-by-one in Today view with quick date picks (tomorrow / +1 week / skip)
- **Task snooze** (`z` key) — hide task for 1 hour, 4 hours, or until tomorrow 9 am; `snooze:YYYY-MM-DDTHH:MM` syntax; snoozed tasks auto-reappear when time expires
- **Per-task time tracking display** — actual logged time shown as badge (green when under estimate, red when over); data from TimeTracker sessions
- **Task notes** (`n` key) — add or edit a freeform note on any task; persisted to `.granit/task-notes.json`; note icon shown in task row
- **Pinned tasks** (`W` key) — pin important tasks to the top of every view; persisted to `.granit/pinned-tasks.json`; pin icon shown in task row
- **Auto-priority** (`A` key) — heuristic scorer: +2 overdue, +1 today, +1 due ≤ 2 days, +1 blocks others, +1 in project, −1 no date; maps to priority 1–4
- **Quick-add inline syntax** — Ctrl+T quick capture now parses `@today`/`@tomorrow`/`@monday`, `!high`/`!low`/`!med`, `~30m`/`~2h` from task text
- **Smart daily note template** — 4 new template variables: `{{overdue_tasks}}`, `{{today_tasks}}`, `{{today_habits}}`, `{{today_schedule}}`
- **Undo last task action** (`u` key) — single-level undo for any task modification (toggle, date, priority, reschedule, dependency, etc.)
- **Task reschedule** (`r` key) — quick move to tomorrow, next Monday, +1 week, +1 month, or custom date
- **Advanced task sorting** (`s` key) — cycle between priority, due date, alphabetical, source note, and first tag
- **Subtask hierarchy** — indentation-based parent/child nesting with expand/collapse (`e` key) and tree rendering
- **Task dependencies** — `depends:text` or `depends:"multi word"` syntax; blocked tasks show lock icon and dimmed text
- **Bulk task operations** (`v` key) — visual select mode with batch toggle, date set, and priority change
- **Task time estimation** — `~30m` / `~2h` syntax with badge display and workload total in Today view; `E` key for quick presets
- **Focus session launcher** (`f` key) — start focus session pre-loaded with selected task from task manager
- **Tag and priority filters** — `#` cycles tags, `P` cycles priorities, `c` clears; filter badges in title bar; tab counts reflect active filters
- **Overdue task grouping** — Today view splits into OVERDUE (red header) and TODAY (green header) sections
- **Custom kanban columns** — configurable via `kanban_columns` and `kanban_column_tags` in settings; dynamic column count with cycling color palette
- **Daily review overlay** — guided 5-phase end-of-day review: celebrate, reschedule overdue, plan tomorrow, reflect, save to Reviews/
- **Overdue warnings on dashboard** — red warning section with count and task list when overdue items exist
- **Habit widget on dashboard** — shows today's habits with completion checkboxes and streak counts
- **Inbox count indicator** — sapphire badge in status bar showing unchecked Inbox.md items
- **Link suggestions** — "Suggested" tab in backlinks panel powered by TF-IDF similarity; Enter inserts `[[wikilink]]`
- **Reading list with progress** — "Reading" tab in bookmarks with status (to-read/reading/completed) and 1-5 star ratings
- **Multi-hour time blocks** — daily planner supports 30m to 3h block durations via `-`/`+` keys
- **Week view time grid** — calendar week view shows hourly rows x 7 day columns with events and planner blocks in cells
- **Project health dashboard** — traffic-light health indicator (On Track/At Risk/Behind), velocity tracking (milestones/week)
- **Goal burndown charts** — ASCII chart with ideal vs actual milestone pace and "On track" / "Behind by N" indicator
- **Nextcloud WebDAV sync** — bidirectional sync with conflict resolution, TUI overlay with test/push/pull/sync controls
- **Desktop app** — Wails-based desktop application with Svelte frontend
- **Weekly review overlay** — structured weekly reflection with metrics and planning
- **AI project planner** — automated project breakdown with milestones
- **Cross-project dashboard** — overview of all projects grouped by status with health indicators
- **Task recurrence parsing** — `daily`, `weekly`, `monthly`, `3x-week` via emoji or tag syntax
- **Task filtering by config** — `task_filter_mode`, `task_required_tags`, `task_exclude_folders`, `task_exclude_done`
- **Task-to-project matching** — auto-assign tasks to projects by folder path or tag

### Fixed

- Calendar agenda view losing state due to value receiver (agendaItems never persisted)
- Calendar task toggles not syncing to other components (missing refreshComponents call)
- Kanban task toggles and file watcher not calling refreshComponents
- Search filter persisting across task manager tab switches
- Search on kanban view applying to nil slice
- Calendar day filter showing completed tasks (now excluded like other views)
- Today and Upcoming tabs overlapping (today's tasks no longer in Upcoming)
- Tab counts not reflecting active tag/priority filters
- Calendar task data empty at app initialization
- SetPlannerBlocks not rebuilding agenda items when in agenda view
- Bare `#` in search matching any task with `#` in text
- Priority filter cycling through value 0 (unset)
- Nested subtask collapse not hiding grandchildren
- Kanban WIP tag routing placing cards in wrong columns
- Dependency matching too loose (substring → prefix match)
- Text truncation not accounting for variable-width prefixes
- Link completer and slash menu bounds checks for empty editors
- Select mode `q` key closing overlay instead of exiting select mode
- Week grid single-digit hour matching (9:00 vs 09)
- Burndown chart x-axis label alignment
- Daily review file write setting fileChanged on I/O error
- Reading list delete missing scroll adjustment
- Dependency blocked status computed per-render (now cached in rebuildFiltered)
- CI workflow excluding desktop package (needs npm-built frontend assets)

- **Vim text objects** — inner/around operators (`iw`, `aw`, `is`, `as`, `ip`, `ap`, `i"`, `a"`, `i)`, `a)`, `i}`, `a}`, `i]`, `a]`, `i>`, `a<`, `` i` ``, `` a` ``); works with `d`, `c`, `y` operators and visual mode
- **Vim marks** — `m` + a-z to set mark, `'` + a-z to jump to line, `` ` `` + a-z to jump to exact position, `''` to jump to previous position
- **Vim search highlighting** — `/pattern` and `?pattern` highlight ALL matches in yellow, current match in orange; `n`/`N` cycle with wrap-around; match count in status bar; Esc clears highlights
- **Landing page overhaul** — feature comparison table (Granit vs Obsidian), embedded screenshots and GIF, theme showcase with palette swatches, "By the Numbers" stats, keyboard-first design section, enhanced installation guide, scroll animations and mobile menu
- **Markdown definition lists** — `Term\n: Definition` syntax rendered with bold term and indented colored marker
- **Markdown soft/hard line breaks** — CommonMark-compliant: single newline = soft break (space), trailing two spaces = hard break (new line)
- **Markdown HTML inline tags** — `<kbd>`, `<sub>`, `<sup>`, `<mark>`, `<abbr title="...">` rendered with appropriate styling
- **Nested task lists** — indented checkboxes render with `└` connectors and proportional indentation
- **Improved callout types** — added `[!important]`, `[!attention]`, `[!failure]`/`[!fail]`, `[!missing]` with distinct colors; callout blocks now have top/bottom borders
- **Table alignment indicators** — header separator rows show `:═══` (left), `:═══:` (center), `═══:` (right) alignment markers
- **Settings search** — press `/` in settings to fuzzy-filter by name, description, or category; case-insensitive matching
- **Settings categories** — 6 organized groups (Appearance, Editor, AI, Files, Plugins, Advanced) with visual headers
- **Settings reset to default** — press `Del` on any setting to restore default; modified settings show `*` indicator
- **Theme color preview** — color palette swatches shown when browsing themes in settings
- **5 accessibility themes** — high-contrast-dark, high-contrast-light, deuteranopia, protanopia, tritanopia (40 themes total)
- **Full-text search index** — inverted index with TF-IDF scoring built during vault scan; O(1) search with phrase matching and relevance ranking; incremental updates on file save; thread-safe with RWMutex
- **CLI: `granit init`** — initialize new vaults with Welcome.md, templates folder, and default config
- **CLI: `granit search`** — search vault from command line with `--regex`, `--json`, `--case-sensitive`, `--no-color`
- **CLI: `granit export`** — export notes to HTML/text/JSON with `--all` or `--note`; HTML includes CSS styling and index page
- **CLI: `granit import`** — import from Obsidian, Logseq, or Notion with format conversion
- **CLI: `granit backup`** — timestamped zip backups with `--restore` and `--list`
- **CLI: `granit plugin`** — list/install/remove/enable/disable/info/create plugins from command line
- **Plugin management package** — shared `internal/plugins/` package for CLI and TUI plugin operations
- **Comprehensive test suites** — 9 new test files: AI scheduler (32 tests), daily planner (83 tests), language learning (56 tests), habit tracker (58 tests), project mode (52 tests), time tracker (48 tests), writing coach (49 tests), NL search (68 tests), knowledge gaps (53 tests)
- **Search index tests** — 27 tests covering index construction, multi-word search, regex, TF-IDF ranking, incremental updates, thread safety
- **Plugin management tests** — 21 tests covering install, remove, enable/disable, validation, scaffolding
- **Task manager rewrite** — 6 views (Today, Upcoming, All, Done, Calendar, Kanban board), 5 priority levels (highest/high/medium/low/none), date picker with keyboard shortcuts (t=today, m=tomorrow, w=next Monday), dedicated `Tasks.md` storage, source file badges showing which note each task comes from, cross-vault task scanning from all files
- **Blog publisher** — publish notes directly to Medium (draft/public/unlisted with frontmatter tag extraction) or GitHub (push Markdown to any repo and branch with SHA-based updates)
- **Breadcrumb navigation** — folder-path breadcrumb bar above the editor showing `vault > folder > subfolder > note`, with left-truncation for deep paths
- **User-defined templates** — drop `.md` files into your vault's `templates/` folder and they appear alongside built-in templates in the template picker
- **Status bar task counter** — yellow badge showing number of tasks due today or overdue
- **Ctrl+K shortcut** — opens the task manager from any context
- **Custom diagram engine** — 6 diagram types rendered in view mode: sequence (combos/flows), tree (decisions), movement (footwork grids), timeline, comparison tables, and figure (10 pre-drawn fighting technique illustrations with colored body parts and key points)
- **Global search & replace** — find and replace text across all vault files with live preview, replace one (Ctrl+R), replace in file (Ctrl+F), or replace all (Ctrl+A)
- **AI template generator** — 9 template types (meeting notes, project plan, technical doc, blog post, tutorial, comparison, book summary, training plan, custom) with AI generation via Ollama/OpenAI or local fallback, topic input, live preview, and one-click note creation
- **Enhanced research agent** — 3 new Claude Code modes: Vault Analyzer (structure/gap analysis), Note Enhancer (improve current note with links and depth), Daily Digest (weekly review from recent vault activity); Deep Dive now has 4 research profiles (general/academic/technical/creative) and 4 source filters (any/web/docs/papers) with research log tracking
- **Language learning** — vocabulary tracker with 9 languages, spaced repetition practice mode, grammar notes with templates, progress dashboard with streak tracking and level distribution charts; stores everything in `Languages/` folder as markdown
- **Habit and goal tracker** — daily habit checkboxes with 7-day streak visualization, goal setting with milestones and progress bars, stats dashboard with completion rates and charts; stores in `Habits/` folder as markdown
- **Core plugins system** — enable/disable 16 built-in modules (task manager, calendar, canvas, flashcards, quiz, pomodoro, git, blog publisher, AI templates, research agent, language learning, habit tracker, ghost writer, encryption, spell check) via Settings > Core Plugins
- **Focus sessions** — guided work sessions with configurable timer (25/45/60/90 min), goal setting, scratchpad for session notes, break timer, session review with stats; logs saved to `FocusSessions/` folder
- **Daily standup generator** — auto-generates standup notes from git commits, modified files, completed tasks, and recently created notes; editable 3-section format (yesterday/today/blockers); saves to `Standups/` folder
- **Note versioning timeline** — git history for current note with visual timeline, inline diff viewer (colored additions/deletions), and full snapshot view at any commit
- **Smart connections** — TF-IDF content similarity engine that finds semantically related notes across vault; shows similarity percentage, shared keywords, and note preview; option to insert wikilink
- **Writing statistics** — 3-tab dashboard: overview (total words, streak, longest note), activity (14-day word count chart), notes (top by length and recency); persists daily stats to `.granit/writing-stats.json`
- **Quick capture** — compact floating input for rapid thought capture; 4 destinations: Inbox.md, daily note, Tasks.md, or new note; accessible from command palette
- **Vault dashboard** — home screen showing greeting, today's tasks, recent notes, vault stats (notes/words/tags/folders), writing streak, 7-day activity chart, and quick-action shortcuts (n/t/c/s/d/f)
- **Enhanced calendar** — year view (12 mini months with activity dots), 14-day agenda lookahead with task priority colors, task count badges on month view, quick event add (a), week numbers, improved visual styling with weekend colors
- **Mind map view** — ASCII mind map from note headings and wikilinks; two modes: headings (note structure tree) and links (vault connection graph 2 levels deep); scrollable with h/j/k/l, toggle mode with m
- **Daily journal prompts** — 100+ curated reflection prompts across 8 categories (gratitude, reflection, growth, creativity, relationships, goals, mindfulness, challenge); daily deterministic prompt, shuffle, category filter, guided write mode saving to `Journal/` folder
- **Clipboard manager** — 50-entry clipboard history with search (/), pin (p), preview pane, paste (Enter); tracks text from editor copy/cut with timestamp and source note
- **Recurring tasks** — define daily/weekly/monthly repeating tasks that auto-create in Tasks.md; management overlay with add/edit/delete/toggle
- **Note preview popup** — floating preview of any note with scroll, basic markdown formatting, and open-to-navigate
- **Floating scratchpad** — persistent scratch area saved to `.granit/scratchpad.md`; survives across notes and sessions with auto-save
- **Project mode** — project management with 9 categories (development, social-media, personal, business, writing, research, health, finance, other); project dashboards showing notes, tasks, completion stats; color-coded status badges
- **Natural language vault search** — AI-powered search ("find notes about the meeting with Sarah") using Ollama/OpenAI with keyword-based local fallback; relevance explanations and snippets
- **AI writing coach** — clarity/structure/style/full analysis of current note with severity-coded feedback; soul note persona support (`.granit/soul-note.md`); local fallback checks for long sentences, passive voice, readability
- **Dataview query builder** — interactive overlay to query notes by frontmatter properties with filters, sort, table/list views; supports virtual fields (title, path, words, created, folder)
- **Time tracker** — per-note/task time tracking with start/stop timer, pomodoro counting, daily/weekly reports with bar charts; stored in `.granit/timetracker.json`
- **Breadcrumb click navigation** — select-mode for breadcrumb bar to jump to any parent folder segment
- **Daily planner** — time-blocked daily schedule overlay (6:00–22:00 in 30-min slots) with auto-import of tasks, calendar events, and habits; 3 panels (schedule grid, unscheduled tasks, habits); move/delete/toggle blocks; progress bar; day navigation with h/l; focus session launch from any block; saves to `Planner/YYYY-MM-DD.md`
- **AI smart scheduler** — AI-powered optimal schedule generation using Ollama/OpenAI with local greedy algorithm fallback; priority-based task ordering; configurable work hours, lunch break, focus block minimum, and break intervals; estimated time input per task; generates schedule and applies directly to daily planner
- **AUR packages** — PKGBUILD for Arch Linux installation (release and git variants)
- AI-powered features: ghost writer, AI chat, bots, and auto-tag suggestions
- Markdown editor with syntax highlighting and vim mode support
- Vault selector and splash screen for quick project switching
- Calendar view and graph view for visual note organization
- Git integration for version control, export functionality, and plugin system
- Slash command menu for fast in-editor actions
- 35 built-in themes and a command palette for quick navigation
- Pomodoro timer, flashcards, and quiz mode for productivity and learning
- Mermaid diagram rendering (flowcharts, sequence, pie, class, Gantt charts)
- Link assistant — find unlinked mentions and batch-create wikilinks
- Tab reordering with Alt+Shift+Left/Right
- Note encryption (AES-256-GCM) for secure GitHub sync
- Deep Dive research agent via Claude Code
- Vault refactor, daily briefing, quiz mode, learning dashboard
- Static site publisher, web clipper, image manager, theme editor

- **Heading folding** — collapse/expand sections by heading level or code fences; fold indicators (▶/▼) in gutter; vim keybindings (za toggle, zM fold all, zR unfold all) and Alt+F for non-vim; command palette entries; folded headings show hidden line count
- **Table editor improvements** — create new tables from command palette when cursor is not on an existing table (3-column, 2-row default); vertical scrolling for large tables with row indicator; insert mode vs replace mode
- **7 new themes** — matrix (green-on-black), cobalt2 (deep blue/gold), monokai-pro (warm dark), horizon (purple/teal), zenburn (low-contrast earthy), iceberg (cool blue-gray), amber (retro CRT)
- **Blog publisher token persistence** — Medium and GitHub tokens, repo, and branch saved to `~/.config/granit/config.json`; pre-filled on open so you never re-enter credentials
- **Blog publisher reliability** — 30-second HTTP timeout, retry with exponential backoff (3 attempts) on server errors and rate limits, updated GitHub API header
- **Enhanced research agent** — CLAUDE.md project context loaded into all research prompts; soul note persona shapes research writing tone; 10-minute process timeout; Esc cancels running research; WebFetch tool enabled for URL fetching
- **4 new layouts** — zen (centered distraction-free editor, 80-char max width, no chrome), taskboard (sidebar + editor + task summary with overdue/today/upcoming), research (sidebar + editor + recent notes/backlinks/links panel), dashboard (sidebar + editor + persistent outline + backlinks, 4-panel)
- **Reading layout wired** — previously defined but not rendered; now shows editor + backlinks with no sidebar
- **Interactive onboarding** — 10-step tutorial walkthrough on first launch; covers navigation, editing, vim mode, task manager, AI features, command palette, customization; reopenable from command palette ("Show Tutorial"); auto-dismissed after completion
- **Vault backup system** — create timestamped zip archives in `.granit/backups/`; restore, delete, auto-prune (max 10); auto-backup modes (none/on_save/daily); management overlay from command palette
- **Enhanced web clipper** — reader mode (extracts `<article>`/`<main>` content), URL input with cursor, `<pre><code>` to fenced code blocks, `<table>` to markdown tables, `<img>` to `![alt](src)`, `<ol>` numbered lists, `<del>` strikethrough, relative URL resolution, save format toggle (full/simplified), custom tag editor
- **GitHub Pages landing page** — professional landing page at `docs/index.html` with Catppuccin Mocha theme, feature grid, install guide, doc links
- **Comprehensive test suite** — config tests (27 tests), vault index/parser tests (46 tests), TUI folding/clipboard/similarity tests (26 tests); ~200 total test cases across all packages
- **Enterprise documentation** — 8 professional docs: Feature Guide, Installation, AI Guide, Keybindings, Architecture, Configuration, Plugins, Themes (4,600+ lines)
- **Auto-release CI** — GitHub Actions workflow auto-creates releases on push to main with date-based tags and cross-compiled binaries (linux/darwin, amd64/arm64)
- **Split pane note picker** — fuzzy search picker for selecting a second note in split view; `p` to re-open picker, Esc context-aware, scrollable filtered list
- **Editor tests** — comprehensive tests for insert, delete, selection, undo/redo, multi-cursor editing
- **Vim mode tests** — tests for normal, insert, visual mode, motions, and commands
- **SVG terminal screenshots** — 6 feature mockup screenshots (task manager, AI bots, vim, themes, calendar, command palette)
- **Contributing guide** — CONTRIBUTING.md with development setup, code conventions, PR workflow
- **Issue and PR templates** — bug report, feature request, and pull request templates
- **Vim macro recording** — `q` + register (a-z) to record, `q` to stop, `@` + register to replay, `@@` for last macro; status bar shows recording indicator; command palette entries for start/stop/play
- **Persistent note tabs** — Ctrl+1-9 tab switching by position; session persistence to `.granit/tabs.json`; close others/right, pin/unpin, reopen closed tab from command palette; scroll indicators for overflow; drag-reorder highlight
- **Improved markdown renderer** — `~~strikethrough~~` and `==highlight==` inline support; `$math$` and `$$block math$$` rendering; box-drawing table borders with header styling, alternating rows, and column alignment; nested blockquotes with depth-colored borders; footnote references and definitions; Unicode checkbox symbols (☐/☑); styled horizontal rule variants
- **Demo vault** — 18 interconnected markdown files across 7 folders showcasing wikilinks, tasks, code blocks, diagrams, frontmatter, templates, and project management
- **VHS recording scripts** — 6 tape files for creating real terminal GIF recordings (demo, vim, tasks, AI, themes, split pane)
- **Vault stress tests** — 5000-note vault, 50k-line notes, 10k-char lines, 20-level nesting, 500+ wikilinks, circular links, malformed frontmatter, empty vault, special char filenames
- **App smoke tests** — model initialization, overlay priority, focus transitions, resize propagation, splash dismissal
- **Editor edge case tests** — insert at every position, 100 rapid undo/redo cycles, 10k-line paste, empty file operations, 50 simultaneous multi-cursors
- **Renderer tests** — 80 tests covering all markdown elements, callouts, embeds, edge cases, and performance
- **Tab bar tests** — 60 tests covering operations, navigation, pins, rendering, and edge cases
- **Macro recording tests** — 12 tests covering start/stop, replay, multiple registers, recursive prevention
- **Bidirectional planner ↔ task sync** — toggling tasks in the daily planner now updates Tasks.md; source file/line tracking on PlannerTask and timeBlock; consumed-once `GetCompletedTasks()` pattern
- **Calendar ↔ planner integration** — calendar agenda/week/month views show planner blocks with time ranges; quick-add events from calendar flow to planner files; task toggle in calendar syncs back to source notes
- **Change notification system** — centralized `refreshComponents()` method re-scans vault, rebuilds index, updates sidebar/autocomplete/calendar/status bar after any component modifies files; active task manager auto-refreshes via `needsRefresh` flag
- **AI scheduler full sync** — completed tasks filtered from scheduler input; AI-scheduled times persisted to Tasks.md with `⏰ HH:MM-HH:MM` markers; task manager shows scheduled times in teal badge; planner auto-saves after AI schedule applied
- **Sync integration tests** — 15 tests verifying data flow between planner, calendar, task manager, and AI scheduler
- **Word wrap toggle** — soft wrap long lines at viewport width with `↪` continuation indicators; word-boundary splitting; toggle from settings or command palette
- **Scroll position memory** — cursor line/col/scroll position saved per note and persisted to `.granit/viewport.json` across sessions (LRU 100 entries)
- **Search history** — Up/Down arrow recalls last 20 queries in content search and find/replace; persisted to `.granit/search-history.json`
- **Bracket matching** — highlight matching `()` `[]` `{}` pairs with nesting support; skips brackets inside strings and code fences
- **Regex search** — Alt+R toggles regex mode in content search, find/replace, and global replace; capture group support (`$1`, `$2`); invalid patterns show inline error
- **Shift+Arrow text selection** — Shift+Left/Right/Up/Down/Home/End for traditional text selection; copy/cut selected text; typing replaces selection; Esc clears
- **Smart paste** — paste URL over selected text creates `[text](url)` markdown link; reverse case also supported (paste text over URL)
- **Reading progress bar** — `████░░░░ 42%` visual indicator in status bar during view mode; vertical scroll position indicator on right edge
- **Shell piping CLI** — `granit list --json/--paths/--tags`, `granit search --regex --json`, `granit capture --stdin --daily --to`, `granit query 'tag:X AND folder:Y' --json`, `granit scan --json`
- **Security hardening** — config file permissions `0600` (owner-only) for API keys; plugin script path traversal prevention; file write error handling; task line validation before sync toggle; corrupted tabs.json cleanup; vim count cap at 10000; unclosed delimiter rendering fix; multi-cursor bounds `clampCursor` helper; Task struct JSON tags

### Changed

- Task manager, daily planner, calendar, and AI scheduler now share a unified sync layer — changes in any component propagate to all others

- Task manager now stores tasks in a dedicated `Tasks.md` file instead of the active note
- Task manager scans all vault files for tasks (Obsidian emoji format)
- Command palette entry added for blog publisher

### Fixed

- Task add not persisting when Today view filtered out new tasks without due dates
- Task toggle/date/priority changes not saving (consumed-once pattern eliminated)
- Task manager now uses direct file I/O instead of signaling through app.go
- View mode cutting off top of screen on long notes (height calculation didn't account for status bar + borders)
- Scratchpad scroll drift (every update moved content down due to value-receiver scroll state)
