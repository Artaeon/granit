# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

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
- **Daily planner** — time-blocked daily schedule overlay (6:00–22:00 in 30-min slots) with auto-import of tasks, calendar events, and habits; 3 panels (schedule grid, unscheduled tasks, habits); move/delete/toggle blocks; progress bar; day navigation with h/l; focus session launch from any block; saves to `Planner/YYYY-MM-DD.md`
- **AI smart scheduler** — AI-powered optimal schedule generation using Ollama/OpenAI with local greedy algorithm fallback; priority-based task ordering; configurable work hours, lunch break, focus block minimum, and break intervals; estimated time input per task; generates schedule and applies directly to daily planner
- **AUR packages** — PKGBUILD for Arch Linux installation (release and git variants)
- AI-powered features: ghost writer, AI chat, bots, and auto-tag suggestions
- Markdown editor with syntax highlighting and vim mode support
- Vault selector and splash screen for quick project switching
- Calendar view and graph view for visual note organization
- Git integration for version control, export functionality, and plugin system
- Slash command menu for fast in-editor actions
- 28 built-in themes and a command palette for quick navigation
- Pomodoro timer, flashcards, and quiz mode for productivity and learning
- Mermaid diagram rendering (flowcharts, sequence, pie, class, Gantt charts)
- Link assistant — find unlinked mentions and batch-create wikilinks
- Tab reordering with Alt+Shift+Left/Right
- Note encryption (AES-256-GCM) for secure GitHub sync
- Deep Dive research agent via Claude Code
- Vault refactor, daily briefing, quiz mode, learning dashboard
- Static site publisher, web clipper, image manager, theme editor

### Changed

- Task manager now stores tasks in a dedicated `Tasks.md` file instead of the active note
- Task manager scans all vault files for tasks (Obsidian emoji format)
- Command palette entry added for blog publisher

### Fixed

- Task add not persisting when Today view filtered out new tasks without due dates
- Task toggle/date/priority changes not saving (consumed-once pattern eliminated)
- Task manager now uses direct file I/O instead of signaling through app.go
