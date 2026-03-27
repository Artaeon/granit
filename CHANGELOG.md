# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

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
