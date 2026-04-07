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
</p>

<p align="center">
  <a href="#features">Features</a> &bull;
  <a href="#installation">Installation</a> &bull;
  <a href="#quick-start">Quick Start</a> &bull;
  <a href="#keyboard-shortcuts">Shortcuts</a> &bull;
  <a href="#ai-setup">AI Setup</a> &bull;
  <a href="#configuration">Config</a> &bull;
  <a href="docs/">Documentation</a>
</p>

---

Granit is a **free, open-source** knowledge management system built entirely in Go. A single 32 MB binary that replaces Obsidian, Notion, and Todoist. Your notes are plain Markdown files with `[[wikilinks]]` and YAML frontmatter -- **fully compatible** with Obsidian, Logseq, and any Markdown editor.

**No Electron. No browser. No subscriptions. No telemetry.**

```
granit                          # open your vault
granit note "ship the release"  # quick capture from shell
granit sync                     # git pull + commit + push
```

---

## Why Granit

| | |
|---|---|
| **Full Vim mode** | Modal editing with text objects, macros, dot repeat, and ex commands |
| **25+ AI features** | 19 bots, goal coach, task triage, plan my day -- works with local models |
| **Calendar with time-blocking** | Weekly grid, goal tracking, task scheduling, ICS sync |
| **7 task views** | Today, Upcoming, All, Done, Calendar, Kanban, Eisenhower Matrix |
| **Goal manager** | Milestones with due dates, reviews, progress sparklines, AI coaching |
| **38 themes** | Catppuccin, Tokyo Night, Gruvbox, Nord, Dracula, and 33 more |
| **Obsidian compatible** | `[[wikilinks]]`, backlinks, graph, canvas, dataview queries |
| **Zero dependencies** | Single static binary, works offline, syncs with git or Nextcloud |

---

## Features

### Editor

- Syntax-highlighted Markdown with Chroma (200+ languages)
- Full Vim modal editing -- Normal, Insert, Visual, Command modes with macros and dot repeat
- Multi-cursor editing, heading folding, split pane view, visual table editor
- Ghost Writer -- inline AI writing suggestions (Tab to accept)
- **Reading mode** (`Ctrl+E`) -- distraction-free view with reading-width column, clean headings, progress bar
- Find and replace (in-file and global), 18 built-in snippets, `[[` autocomplete
- Mermaid diagram rendering, auto-close brackets, smart indentation

### Calendar

- 6 views: month, week, 3-day, 1-day, agenda, year (`Ctrl+L`)
- **Full-width weekly grid** -- hourly time slots with column separators, today highlighted
- **Current time marker** -- green `▸HH:MM` line across the grid
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

### AI Integration

- **19 AI Bots** (`Ctrl+R`) in 6 categories -- all optimized for small local models (0.5B-3B)
- Plan My Day, AI Scheduler, Smart Task Triage, Writing Coach, Daily/Weekly Review
- Ghost Writer with 32-entry completion cache
- Providers: **Ollama** (local, default), **OpenAI**, **Nous**, or offline fallback
- **Ollama management** -- install, start/stop, and pull models from settings
- Production reliability: auto-retry, HTTP cancellation, timeout handling, token-budget checks

### Habits and Productivity

- Habit tracker with streaks, pomodoro timer, focus sessions (25/45/60/90 min)
- Daily planner with time-blocked schedule, daily/weekly review with AI synthesis
- Smart daily note templates with 15+ variables (`{{overdue_tasks}}`, `{{active_goals}}`, etc.)
- Command palette (`Ctrl+X`) with 145+ commands, scratchpad, clipboard manager
- Clock in/out time tracking, journal prompts, reminders, workspace snapshots

### Sync and Export

- **Git** -- built-in overlay with status, log, diff, commit, push, pull; `granit sync` CLI
- **Nextcloud** WebDAV sync with auto-sync option
- Export to HTML, PDF (pandoc), plain text; blog publisher (Medium, GitHub)
- Note encryption (AES-256-GCM), import from other formats

### Customization

- **38 themes** including 5 accessibility themes (high-contrast, deuteranopia, protanopia, tritanopia)
- Live theme editor with 16 color roles and custom theme JSON
- **13 layouts** -- default, writer, reading, dashboard, zen, cockpit, stacked, cornell, focus, preview, presenter, kanban, widescreen
- 4 icon themes (unicode, nerd font, emoji, ASCII), 16 core plugins
- Language-agnostic plugin system with Lua scripting API

---

## Installation

```bash
# Clone and install
git clone https://github.com/artaeon/granit.git
cd granit
go install ./cmd/granit/

# Ensure ~/go/bin is in your PATH
export PATH="$HOME/go/bin:$PATH"
```

**Requirements:** Go 1.24+, Git, Linux or macOS.

**Optional:** [Ollama](https://ollama.ai) (local AI), aspell (spell check), pandoc (PDF export), xclip/wl-copy (clipboard).

See [docs/INSTALLATION.md](docs/INSTALLATION.md) for desktop app, AUR package, and cross-compilation.

---

## Quick Start

```bash
granit init my-vault    # create a new vault
granit my-vault         # open it
granit                  # re-opens last vault
```

**First steps:**
1. `Ctrl+N` -- create a new note
2. `Ctrl+K` -- open task manager
3. `Ctrl+L` -- open calendar
4. `Ctrl+R` -- use AI bots
5. `Ctrl+X` -- command palette (145+ commands)
6. `Ctrl+,` -- settings

---

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Tab` | Cycle panels |
| `Ctrl+P` | Quick open (fuzzy search) |
| `Ctrl+N` | New note |
| `Ctrl+S` | Save |
| `Ctrl+E` | Toggle reading mode |
| `Ctrl+K` | Task manager |
| `Ctrl+L` | Calendar |
| `Ctrl+R` | AI bots |
| `Ctrl+G` | Note graph |
| `Ctrl+X` | Command palette |
| `Ctrl+,` | Settings |
| `Ctrl+Q` | Quit |

See [docs/KEYBINDINGS.md](docs/KEYBINDINGS.md) for the complete reference (Vim mode, task manager, calendar, canvas, goals).

---

## AI Setup

### Ollama (recommended, local)

```bash
curl -fsSL https://ollama.ai/install.sh | sh
ollama pull qwen2.5:0.5b
ollama serve
```

Or use the built-in wizard: `Ctrl+,` > Setup Ollama. You can also start/stop Ollama directly from settings.

### OpenAI

```json
{ "ai_provider": "openai", "openai_key": "sk-...", "openai_model": "gpt-4o-mini" }
```

See [docs/AI-GUIDE.md](docs/AI-GUIDE.md) for all providers and troubleshooting.

---

## Configuration

Layered JSON configuration. Per-vault settings override global.

| Scope | Path |
|-------|------|
| Global | `~/.config/granit/config.json` |
| Per-vault | `<vault>/.granit.json` |
| Custom themes | `~/.config/granit/themes/` |
| Plugins | `~/.config/granit/plugins/` |

All settings editable from `Ctrl+,`. Key options: `theme`, `layout`, `vim_mode`, `ai_provider`, `ghost_writer`, `auto_save`.

See [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for the full reference.

---

## Documentation

| Document | Description |
|----------|-------------|
| [Features](docs/FEATURES.md) | Complete feature guide |
| [Keybindings](docs/KEYBINDINGS.md) | All keyboard shortcuts |
| [AI Guide](docs/AI-GUIDE.md) | AI setup, providers, bots |
| [Configuration](docs/CONFIGURATION.md) | All config options |
| [Themes](docs/THEMES.md) | 38 themes + custom theme guide |
| [Installation](docs/INSTALLATION.md) | Build, install, dependencies |
| [Plugins](docs/PLUGINS.md) | Plugin development guide |
| [Architecture](docs/ARCHITECTURE.md) | Codebase structure |
| [Changelog](CHANGELOG.md) | Version history |

---

## License

MIT License. See [LICENSE](LICENSE).

<p align="center">
  <sub>Your knowledge. Your terminal. Your rules.</sub>
</p>
