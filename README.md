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
  <strong>Terminal-native knowledge manager with AI, calendar, goals, and tasks -- fully Obsidian compatible</strong>
</p>

<p align="center">
  <a href="#installation"><img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go Version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue?style=for-the-badge" alt="License"></a>
  <img src="https://img.shields.io/badge/Platform-Linux%20%7C%20macOS-lightgrey?style=for-the-badge" alt="Platform">
  <img src="https://img.shields.io/badge/Themes-38-purple?style=for-the-badge" alt="Themes">
  <img src="https://img.shields.io/badge/AI%20Bots-19-green?style=for-the-badge" alt="AI Bots">
  <img src="https://img.shields.io/badge/Tests-1971-orange?style=for-the-badge" alt="Tests">
</p>

<p align="center">
  <a href="#features">Features</a> &bull;
  <a href="#screenshots">Screenshots</a> &bull;
  <a href="#installation">Installation</a> &bull;
  <a href="#quick-start">Quick Start</a> &bull;
  <a href="#keyboard-shortcuts">Shortcuts</a> &bull;
  <a href="#cli-reference">CLI</a> &bull;
  <a href="#ai-setup">AI Setup</a> &bull;
  <a href="#configuration">Config</a> &bull;
  <a href="#contributing">Contributing</a> &bull;
  <a href="docs/">Docs</a>
</p>

---

Granit is a **free, open-source** knowledge management system built entirely in Go. A single 32 MB binary that replaces Obsidian, Notion, and Todoist. Your notes are plain Markdown files with `[[wikilinks]]` and YAML frontmatter -- **fully compatible** with Obsidian, Logseq, and any Markdown editor.

**No Electron. No browser. No subscriptions. No telemetry.**

```
granit                          # open your vault
granit note "ship the release"  # quick capture from shell
granit sync                     # git pull + commit + push
granit today                    # print today's tasks and habits
```

---

## Why Granit

| | |
|---|---|
| **Full Vim mode** | Modal editing with text objects, macros, dot repeat, marks, and ex commands |
| **19 AI bots** | Auto-tagger, summarizer, flashcards, writing coach, plan my day -- works with local models |
| **Calendar with time-blocking** | Month, week, 3-day, day, agenda, and year views. ICS sync, event wizard, goal badges |
| **7 task views** | Today, Upcoming, All, Done, Calendar, Kanban, Eisenhower Matrix |
| **Goal manager** | Milestones with due dates, reviews, progress sparklines, AI coaching |
| **38 themes** | Catppuccin, Tokyo Night, Gruvbox, Nord, Dracula, and 33 more -- plus a live theme editor |
| **Obsidian compatible** | `[[wikilinks]]`, backlinks, graph, canvas, dataview queries |
| **Zero dependencies** | Single static binary, works offline, syncs with git or Nextcloud |

---

## Screenshots

<p align="center">
  <img src="assets/editor.gif" alt="Granit Editor" width="800">
  <br><em>Markdown editing with syntax highlighting, wikilink autocomplete, and multi-cursor</em>
</p>

<p align="center">
  <img src="assets/calendar.gif" alt="Calendar" width="800">
  <br><em>Full-width weekly grid with time-blocking, event wizard, and goal integration</em>
</p>

<p align="center">
  <img src="assets/ai-features.gif" alt="AI Features" width="800">
  <br><em>19 AI bots running locally via Ollama -- auto-tag, summarize, flashcards, and more</em>
</p>

<p align="center">
  <img src="assets/themes.gif" alt="Themes" width="800">
  <br><em>38 built-in themes including 5 accessibility themes and a live theme editor</em>
</p>

---

## Features

### Editor

- Syntax-highlighted Markdown with Chroma (200+ languages)
- Full Vim modal editing -- Normal, Insert, Visual, Command modes with macros, marks, and dot repeat
- Multi-cursor editing (`Ctrl+D`), heading folding, split pane view, visual table editor
- Ghost Writer -- inline AI writing suggestions (Tab to accept) with 32-entry completion cache
- **Reading mode** (`Ctrl+E`) -- distraction-free rendered view with reading-width column, clean headings, progress bar
- Find and replace (in-file and global), 18 built-in snippets, `[[` autocomplete
- Mermaid diagram rendering, footnotes, callout blocks, auto-close brackets, smart indentation

### Calendar

- 6 views: month, week, 3-day, 1-day, agenda, year (`Ctrl+L`)
- **Full-width weekly grid** -- hourly time slots with column separators, today highlighted
- **Current time marker** -- green `>HH:MM` line across the grid
- **Task time-blocking** (`b`) -- assign tasks to time slots, writes to planner file
- **Event creation wizard** (`a`) -- title, time, duration, location, recurrence, color, description
- **Goals integration** -- active goals as progress badges in the week header
- **Weekly milestones** (`g`) -- create a milestone linked to an existing goal
- **Daily focus** -- Plan My Day's top goal shown in day headers
- **ICS calendar sync** -- auto-loads `.ics` files, per-file toggle in settings, RRULE support
- Calendar sidebar panel for cockpit and widescreen layouts

### Tasks

- Task Manager (`Ctrl+K`) -- 7 views: Today, Upcoming, All, Done, Calendar, Kanban, Eisenhower
- 5 priority levels, due dates, time estimates (`~30m`), subtask hierarchy, dependencies
- Reschedule, batch reschedule, snooze, pin, undo (10-deep stack), task templates
- Bulk operations, tag/priority filters, focus session launcher, recurring tasks
- Natural language dates (`@next week`, `@friday`), quick-add syntax (`!high #tag ~1h`)
- Task triage with AI-powered priority suggestions
- CLI: `granit todo "Ship v2.0" --due friday --priority high --tag release`

### Goals and Projects

- **Goal Manager** -- active, paused, completed, archived lifecycle with milestones
- Milestone CRUD with due dates, reordering, auto-complete, goal-to-task linking
- Recurring reviews (weekly/monthly/quarterly) with progress sparklines
- **AI Goal Coach** -- velocity tracking, stalled detection, priority recommendations
- **AI Project Insights** -- health analysis, risks, blockers, next actions
- Project dashboards, burndown charts, daily standup generator

### Knowledge Management

- `[[Wikilinks]]` with autocomplete, backlinks panel, and note graph (`Ctrl+G`)
- Tag browser, note outline, bookmarks, reading list with progress tracking
- Semantic search (AI embeddings), dataview queries, mind map view
- Canvas/whiteboard, Zettelkasten ID generator, note versioning with git diffs
- Smart connections (AI-powered related notes), knowledge gaps analysis
- Thread Weaver for connecting disparate notes into narratives

### AI Integration

- **19 AI Bots** (`Ctrl+R`) in 6 categories -- all optimized for small local models (0.5B-3B)
- **Deep Dive Research Agent** -- runs Claude Code in the background to research topics and create structured notes
- Plan My Day, AI Scheduler, Smart Task Triage, Writing Coach, Daily/Weekly Review
- Ghost Writer with 32-entry completion cache
- Auto-tagger, auto-linker, summarizer, flashcard generator, tone adjuster, action item extractor
- Providers: **Ollama** (local, default), **OpenAI**, **Nous**, or offline fallback
- **Ollama management** -- install, start/stop, and pull models from settings
- Production reliability: auto-retry, HTTP cancellation, timeout handling, token-budget checks

### Habits and Productivity

- Habit tracker with streaks and completion charts
- Pomodoro timer with configurable session goals, focus sessions (25/45/60/90 min)
- Daily planner with time-blocked schedule and clipboard export
- Daily/weekly review with AI synthesis, morning routine wizard
- Smart daily note templates with 15+ variables (`{{overdue_tasks}}`, `{{active_goals}}`, etc.)
- Command palette (`Ctrl+X`) with 145+ commands across 11 categories
- Clock in/out time tracking with project tagging and weekly reports
- Scratchpad, clipboard manager, journal prompts, reminders, workspace snapshots

### Sync and Export

- **Git** -- built-in overlay with status, log, diff, commit, push, pull, author config; `granit sync` CLI
- **Nextcloud** WebDAV sync with auto-sync option
- Export to HTML, PDF (pandoc), plain text; blog publisher (Medium, GitHub)
- Note encryption (AES-256-GCM), import from Obsidian/Logseq/Notion
- Backup and restore with timestamped zip archives

### Customization

- **38 themes** including 5 accessibility themes (high-contrast, deuteranopia, protanopia, tritanopia)
- Live theme editor with 16 color roles and custom theme JSON
- **13 layouts** -- default, writer, reading, dashboard, zen, cockpit, stacked, cornell, focus, preview, presenter, kanban, widescreen
- 4 icon themes (unicode, nerd font, emoji, ASCII), 16 core plugins
- Language-agnostic plugin system with Lua scripting API

---

## Installation

### From source (recommended)

```bash
git clone https://github.com/artaeon/granit.git
cd granit
go install ./cmd/granit/

# Ensure ~/go/bin is in your PATH
export PATH="$HOME/go/bin:$PATH"
```

### One-liner

```bash
go install github.com/artaeon/granit/cmd/granit@latest
```

### Build from source with custom binary location

```bash
git clone https://github.com/artaeon/granit.git
cd granit
go build -o bin/granit ./cmd/granit/
sudo cp bin/granit /usr/local/bin/
```

### Requirements

| Requirement | Version | Notes |
|------------|---------|-------|
| **Go** | 1.24+ | Required for building |
| **Git** | Any | Required for git sync features |
| **Platform** | Linux, macOS | Terminal with 256-color support recommended |

### Optional Dependencies

| Package | Purpose |
|---------|---------|
| [Ollama](https://ollama.ai) | Local AI (19 bots, Ghost Writer, research agent) |
| aspell | Spell checking |
| pandoc | PDF export |
| xclip / wl-copy | System clipboard integration |
| Claude Code | Deep Dive Research Agent |

See [docs/INSTALLATION.md](docs/INSTALLATION.md) for desktop app, AUR package, and cross-compilation.

---

## Quick Start

```bash
# Create and open a vault
granit init my-vault
granit my-vault

# Or open any directory of Markdown files
granit ~/Documents/notes

# Subsequent launches re-open the last vault
granit
```

### First steps inside Granit

| Step | Shortcut | What it does |
|------|----------|--------------|
| 1 | `Ctrl+N` | Create a new note with template picker |
| 2 | `Ctrl+K` | Open task manager -- add your first task |
| 3 | `Ctrl+L` | Open calendar -- view month/week/day |
| 4 | `Ctrl+R` | AI bots -- auto-tag, summarize, flashcards |
| 5 | `Ctrl+X` | Command palette -- 145+ commands at your fingertips |
| 6 | `Ctrl+,` | Settings -- configure theme, AI, layout |
| 7 | `Alt+M` | Morning routine -- plan your day with goals |
| 8 | `Ctrl+G` | Note graph -- visualize connections |

### Daily workflow

```bash
# Morning: plan your day
granit                    # opens vault, Alt+M for morning routine

# During the day: quick capture from any terminal
granit note "Idea: refactor auth module"
granit todo "Review PR #42" --due today --priority high

# Evening: review and sync
granit review             # daily review
granit sync               # commit and push changes
```

---

## Keyboard Shortcuts

### Core Navigation

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Cycle between panels |
| `F1` / `F2` / `F3` | Focus sidebar / editor / backlinks |
| `Ctrl+P` | Quick open (fuzzy file search) |
| `Ctrl+J` | Quick switch between recent files |
| `Ctrl+N` | New note with template picker |
| `Ctrl+S` | Save current note |
| `Ctrl+E` | Toggle edit / reading mode |
| `Ctrl+Q` | Quit |

### Overlays and Tools

| Key | Action |
|-----|--------|
| `Ctrl+X` | Command palette (145+ commands) |
| `Ctrl+K` | Task manager |
| `Ctrl+L` | Calendar |
| `Ctrl+R` | AI bots |
| `Ctrl+G` | Note graph visualization |
| `Ctrl+T` | Tag browser |
| `Ctrl+O` | Note outline (headings) |
| `Ctrl+B` | Bookmarks and recent notes |
| `Ctrl+W` | Canvas / whiteboard |
| `Ctrl+,` | Settings |
| `Ctrl+Z` | Focus / zen mode |
| `F4` | Rename current note |
| `F5` / `Alt+?` | Help overlay |

### Editing

| Key | Action |
|-----|--------|
| `Ctrl+F` | Find in file |
| `Ctrl+H` | Find and replace |
| `Ctrl+D` | Multi-cursor: select word / next occurrence |
| `Ctrl+U` | Undo |
| `Ctrl+Y` | Redo |
| `Ctrl+Shift+Up/Down` | Add cursor above/below |

### Daily Workflow

| Key | Action |
|-----|--------|
| `Alt+M` | Morning routine / plan my day |
| `Alt+J` | Daily jot |
| `Alt+E` | Evening review |
| `Alt+H` | Dashboard |
| `Alt+L` | Layout picker |

See [docs/KEYBINDINGS.md](docs/KEYBINDINGS.md) for the complete reference including Vim mode, task manager, calendar, canvas, and goals keybindings.

---

## CLI Reference

Granit includes a full CLI for quick actions from your shell.

### Vault Management

```bash
granit                          # Open last vault or vault selector
granit <path>                   # Open vault at path
granit init <path>              # Initialize a new vault
granit scan <path>              # Print vault statistics (--json)
granit list <path>              # List notes (--json, --paths, --tags)
granit list --vaults            # List all known vaults
granit config                   # Show configuration paths and values
granit backup <path>            # Create timestamped zip backup
```

### Notes and Tasks

```bash
granit note "your thought"      # Quick capture to Inbox.md
granit todo "task" --due friday --priority high --tag work
granit capture "idea"           # Append to inbox.md
echo "idea" | granit clip       # Capture from stdin
granit daily <path>             # Open today's daily note
```

### Sync and Review

```bash
granit sync <path>              # Pull, commit, push (one command)
granit sync -m "weekly update"  # Custom commit message
granit today                    # Print today's dashboard
granit review                   # Daily review
granit review --week --md       # Weekly review as markdown
granit review --save            # Save review to vault
```

### Time Tracking

```bash
granit clock in --project "Study Go"
granit clock out
granit clock status
granit clock log --week
```

### Search

```bash
granit search "TODO" <path>       # Full-text search
granit search --regex "#+\s" .    # Regex search
granit query 'tag:project' <path> # Metadata query
```

### Other

```bash
granit serve <path> --port 8080   # Serve vault as read-only website
granit export --format html --all # Export notes
granit import --from obsidian src # Import from other apps
granit remind "Start work" --at 07:00 --daily
granit completion bash            # Shell completions
granit man | man -l -             # View man page
```

---

## AI Setup

### Ollama (recommended -- local, private, free)

```bash
# Install Ollama
curl -fsSL https://ollama.ai/install.sh | sh

# Pull a small, fast model
ollama pull qwen2.5:0.5b
ollama serve
```

Or use the built-in wizard: `Ctrl+,` > Setup Ollama. You can install, start/stop Ollama, and pull models directly from settings.

### OpenAI

Add to your vault's `.granit.json` or global config:

```json
{
  "ai_provider": "openai",
  "openai_key": "sk-...",
  "openai_model": "gpt-4o-mini"
}
```

### Claude Code (Deep Dive Research Agent)

Install [Claude Code](https://docs.anthropic.com/en/docs/claude-code) for the Deep Dive Research Agent, which runs in the background to research any topic and create structured notes in your vault.

### AI Bot Categories

| Category | Bots |
|----------|------|
| **Writing** | Summarizer, tone adjuster, writing coach, ghost writer |
| **Organization** | Auto-tagger, auto-linker, flashcard generator, action items |
| **Research** | Deep dive agent, note enhancer, vault analyzer, daily digest |
| **Planning** | Plan my day, task triage, AI scheduler, goal coach |
| **Analysis** | Smart connections, knowledge gaps, project insights |

See [docs/AI-GUIDE.md](docs/AI-GUIDE.md) for all providers, model recommendations, and troubleshooting.

---

## Configuration

Granit uses layered JSON configuration. Per-vault settings override global.

| Scope | Path |
|-------|------|
| Global | `~/.config/granit/config.json` |
| Per-vault | `<vault>/.granit.json` |
| Vault registry | `~/.config/granit/vaults.json` |
| Custom themes | `~/.config/granit/themes/` |
| Plugins | `~/.config/granit/plugins/` |

All settings are editable from `Ctrl+,` in the TUI.

### Key Settings

```json
{
  "theme": "catppuccin-mocha",
  "layout": "default",
  "vim_mode": false,
  "ai_provider": "ollama",
  "ollama_model": "qwen2.5:0.5b",
  "ghost_writer": false,
  "auto_save": false,
  "git_auto_sync": false,
  "line_numbers": true,
  "word_wrap": false,
  "show_icons": true,
  "sort_by": "name"
}
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `GRANIT_VAULT` | Default vault path (used when no path given) |
| `EDITOR` | Preferred external editor for shell-out |

See [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for the full reference with all options.

---

## Architecture

Granit is a monolithic Go application built on the [Bubble Tea](https://github.com/charmbracelet/bubbletea) TUI framework.

```
cmd/granit/          Entry point, CLI commands
internal/
  config/            Configuration loading and persistence
  tui/               All TUI components (~170 files)
  vault/             Vault scanning, indexing, search
desktop/             Wails desktop app (Go + Svelte)
docs/                Documentation and website
assets/              Screenshots and GIFs
```

### Tech Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.24+ |
| TUI Framework | [Bubble Tea](https://github.com/charmbracelet/bubbletea) |
| Styling | [Lip Gloss](https://github.com/charmbracelet/lipgloss) |
| Syntax Highlighting | [Chroma](https://github.com/alecthomas/chroma) |
| Desktop App | [Wails](https://wails.io) + Svelte |
| AI | Ollama, OpenAI, Claude Code |

### Codebase Statistics

| Metric | Value |
|--------|-------|
| Lines of Go | 166,000+ |
| Test cases | 1,971 |
| TUI components | 170+ files |
| Commands | 145+ |
| Themes | 38 |
| Layouts | 13 |
| Core plugins | 16 |
| AI bots | 19 |

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for a deep dive into the overlay system, message routing, and component design.

---

## Contributing

Contributions are welcome! Granit is a large codebase, so here are some guidelines to help you get started.

### Getting Started

```bash
# Clone and build
git clone https://github.com/artaeon/granit.git
cd granit
go build -o bin/granit ./cmd/granit/

# Run tests
go test ./...

# Run a specific test package
go test ./internal/tui/ -run TestName
```

### Project Structure

- `cmd/granit/` -- Entry point and CLI commands. Start here to understand the app flow.
- `internal/tui/` -- All TUI components. Each overlay is a self-contained file (e.g., `calendar.go`, `taskmanager.go`).
- `internal/vault/` -- Vault scanning, note parsing, search indexing.
- `internal/config/` -- Configuration loading, saving, and per-vault overrides.

### Guidelines

- **One feature per PR.** Keep changes focused and reviewable.
- **Add tests** for new features. Run `go test ./...` before submitting.
- **Follow existing patterns.** Look at similar overlays for structure reference.
- **No unnecessary dependencies.** Granit ships as a single binary -- keep it that way.

### Areas for Contribution

- Bug fixes and performance improvements
- New themes (add to `internal/tui/themes.go`)
- New AI bot prompts (see `internal/tui/bots.go`)
- Documentation improvements
- Accessibility improvements
- Test coverage for untested components

---

## Documentation

| Document | Description |
|----------|-------------|
| [Features](docs/FEATURES.md) | Complete feature guide with examples |
| [Keybindings](docs/KEYBINDINGS.md) | All keyboard shortcuts by context |
| [AI Guide](docs/AI-GUIDE.md) | AI setup, providers, bot reference |
| [Configuration](docs/CONFIGURATION.md) | All config options with defaults |
| [Themes](docs/THEMES.md) | 38 themes + custom theme creation |
| [Installation](docs/INSTALLATION.md) | Build, install, desktop app, dependencies |
| [Plugins](docs/PLUGINS.md) | Plugin development and Lua API |
| [Architecture](docs/ARCHITECTURE.md) | Codebase structure and design |
| [Changelog](CHANGELOG.md) | Version history and release notes |

---

## Acknowledgments

Built with these excellent open-source projects:

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) -- TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) -- Terminal styling
- [Chroma](https://github.com/alecthomas/chroma) -- Syntax highlighting
- [Catppuccin](https://github.com/catppuccin/catppuccin) -- Color palette inspiration

---

## License

MIT License. See [LICENSE](LICENSE).

---

<p align="center">
  <strong>Your knowledge. Your terminal. Your rules.</strong>
  <br><br>
  <a href="#installation">Get Started</a> &bull;
  <a href="https://github.com/artaeon/granit/issues">Report a Bug</a> &bull;
  <a href="https://github.com/artaeon/granit/discussions">Discussions</a>
</p>
