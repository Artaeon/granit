# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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
