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
  <strong>The terminal-native knowledge manager that replaces Obsidian, Notion, and Todoist.</strong><br>
  <em>Notes, tasks, goals, habits, calendar, 23 AI bots, and daily planning -- one binary, zero subscriptions.</em>
</p>

<p align="center">
  <a href="#installation"><img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go Version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue?style=for-the-badge" alt="License"></a>
  <img src="https://img.shields.io/badge/Platform-Linux%20%7C%20macOS-lightgrey?style=for-the-badge" alt="Platform">
  <img src="https://img.shields.io/badge/Themes-40-purple?style=for-the-badge" alt="Themes">
  <img src="https://img.shields.io/badge/AI%20Bots-23-green?style=for-the-badge" alt="AI Bots">
  <img src="https://img.shields.io/badge/Tests-2072-orange?style=for-the-badge" alt="Tests">
</p>

<p align="center">
  <a href="#features">Features</a> &bull;
  <a href="#who-is-granit-for">Use Cases</a> &bull;
  <a href="#screenshots">Screenshots</a> &bull;
  <a href="#installation">Installation</a> &bull;
  <a href="#quick-start">Quick Start</a> &bull;
  <a href="#a-day-with-granit">Daily Workflow</a> &bull;
  <a href="#keyboard-shortcuts">Shortcuts</a> &bull;
  <a href="#cli-reference">CLI</a> &bull;
  <a href="#ai-setup">AI Setup</a> &bull;
  <a href="#configuration">Config</a> &bull;
  <a href="#granit-vs-the-alternatives">Compare</a> &bull;
  <a href="#contributing">Contributing</a> &bull;
  <a href="docs/">Docs</a>
</p>

---

## What is Granit?

Granit is a **free, open-source** knowledge management system built entirely in Go. A single ~32 MB binary gives you a full-featured note editor, task manager, goal tracker, habit tracker, calendar with time-blocking, 23 AI bots, and daily planning workflows -- all inside your terminal.

Your notes are plain Markdown files with `[[wikilinks]]` and YAML frontmatter. **Fully compatible** with Obsidian, Logseq, and any Markdown editor. Bring your existing vault or start fresh.

**No Electron. No browser. No subscriptions. No telemetry. No vendor lock-in.**

```
granit                          # open your vault
granit note "ship the release"  # quick capture from shell
granit sync                     # git pull + commit + push
granit today                    # print today's tasks and habits
```

> **How much are you paying for productivity tools?**
>
> | Tool | Monthly Cost | What Granit Replaces |
> |------|-------------|---------------------|
> | Obsidian Sync + Publish | $16/mo | Notes, wikilinks, graph, canvas, sync |
> | Obsidian AI | $14/mo | 23 local AI bots, free |
> | Todoist Pro | $5/mo | Task manager with 7 views + Eisenhower + Kanban |
> | Notion | $10/mo | Database, calendar, kanban, wiki |
> | Habitica / Streaks | $5/mo | Habit tracker with streaks |
> | **Total** | **$50/mo** | **Granit: $0/mo, forever** |

---

## Why Granit?

Most knowledge tools force a trade-off: **power vs speed**, **features vs simplicity**, **local vs connected**. Granit refuses to choose.

- **Instant startup.** Your vault loads in ~12ms. No splash screens, no spinners, no loading bars. Open Granit, start typing. It's as fast as `vim` but with a full productivity suite behind it.
- **Everything in one place.** Notes, tasks, goals, habits, calendar, journal, AI -- they all live together. No more context-switching between five different apps to plan your day.
- **Keyboard-first.** 168 commands, all reachable from your keyboard. 17 layouts to match how you work. Full Vim mode with macros, text objects, and dot repeat. Your hands never leave the keyboard.
- **AI that runs locally.** 23 AI bots work with models as small as 0.5B parameters via Ollama. Auto-tag your notes, generate flashcards, plan your day, triage tasks -- all without sending data to the cloud.
- **Your data, your files.** Plain `.md` files in a folder. Sync with git, Nextcloud, or Dropbox. Encrypt sensitive notes with AES-256. No proprietary database, no cloud dependency, no account required.
- **Obsidian compatible.** Use your existing Obsidian vault. `[[Wikilinks]]`, backlinks, graph view, canvas, dataview queries, YAML frontmatter -- it all works. Switch to Granit without changing a single file.

---

## Who is Granit for?

### Developers and Engineers

You live in the terminal. Your IDE is there, your git is there, your builds run there. Why should your notes be trapped in Electron? Granit fits into your existing workflow -- `tmux` split, SSH session, or standalone. Use the CLI to capture ideas without leaving your current context. Git sync keeps everything versioned automatically.

```bash
# Quick capture without leaving your terminal workflow
granit note "Bug: race condition in auth middleware -- see PR #847"
granit todo "Fix flaky test in CI" --due tomorrow --priority high --tag backend
```

### Researchers and Students

Granit is a Zettelkasten and study tool in one. `[[Wikilinks]]` connect your ideas into a knowledge graph. The AI flashcard generator turns your notes into study material. Semantic search finds connections you didn't know existed. The reading list tracks papers and books with progress bars.

### Writers and Content Creators

Focus Mode strips away everything except your words. Ghost Writer offers inline AI suggestions as you type. 17 layouts let you switch between drafting, editing, and outlining. Export to HTML, PDF, or publish directly to Medium and GitHub Pages.

### Productivity Enthusiasts

If you've tried every app from Todoist to Notion to Obsidian and still feel like something's missing, Granit might be it. Morning routine, evening review, habit tracking with streaks, Pomodoro timer, goal manager with milestones, clock-in/out time tracking, Eisenhower Matrix, and Kanban boards -- all without paying for a single subscription.

### Teams on a Budget

Granit is MIT-licensed and free forever. Sync through a shared git repo or Nextcloud server. Every team member gets the full feature set. No "Pro" tier, no per-seat pricing, no feature gates.

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
  <br><em>23 AI bots running locally via Ollama -- auto-tag, summarize, flashcards, and more</em>
</p>

<p align="center">
  <img src="assets/themes.gif" alt="Themes" width="800">
  <br><em>40 built-in themes including accessibility themes and a live theme editor</em>
</p>

---

## Features

### Editor

Granit's editor isn't a glorified textarea -- it's a real code-grade text editor built for Markdown.

- Syntax-highlighted Markdown with Chroma (200+ languages in fenced code blocks)
- Full Vim modal editing -- Normal, Insert, Visual, Command modes with macros, marks, registers, and dot repeat
- Multi-cursor editing (`Ctrl+D` to select next occurrence, `Ctrl+Shift+Up/Down` to add cursors)
- Heading folding, split pane view, visual table editor
- Ghost Writer -- inline AI writing suggestions (Tab to accept) with 32-entry completion cache
- **Reading mode** (`Ctrl+E`) -- distraction-free rendered view with reading-width column, progress bar, and smooth scrolling
- Find and replace (in-file and global with regex), 18 built-in snippets, smart bracket auto-close
- `[[` wikilink autocomplete with fuzzy matching across all vault files
- Mermaid diagram rendering, footnotes, callout blocks, transclusion, and smart indentation
- Spell checking (aspell), clipboard manager with history, and unlimited undo/redo
- Word count, reading time, and writing statistics in the status bar

> **Vim Mode:** If you're a Vim user, Granit feels like home. Normal, Insert, Visual, and Command modes. Text objects (`diw`, `ci"`, `da(`), macros (`q` to record, `@` to play), marks (`m` + letter), search highlighting with `n`/`N`, dot repeat, `:%s` substitution, and more. Toggle with `Ctrl+,` > Vim Mode or the command palette.

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

Tasks live inside your notes as Markdown checkboxes. Granit finds them all and gives you powerful views to organize them.

- Task Manager (`Ctrl+K`) -- 7 views:

  | View | What it shows |
  |------|--------------|
  | **Today** | Due today + overdue, sorted by priority |
  | **Upcoming** | Next 7 days grouped by date |
  | **All** | Every task with filters and sort |
  | **Done** | Completed tasks with completion dates |
  | **Calendar** | Tasks on a date grid |
  | **Kanban** | Drag between columns by status |
  | **Eisenhower** | 2x2 urgent/important matrix |

- 5 priority levels, due dates, time estimates (`~30m`), subtask hierarchy, dependencies
- Reschedule, batch reschedule, snooze, pin, undo (10-deep stack), task templates
- Bulk operations, tag/priority filters, focus session launcher
- Recurring tasks with auto-creation on completion
- Natural language dates (`@next week`, `@friday`, `@in 3 days`), quick-add syntax (`!high #tag ~1h`)
- **AI task triage** -- suggest priorities based on deadlines, dependencies, and workload
- CLI quick capture:
  ```bash
  granit todo "Ship v2.0" --due friday --priority high --tag release
  granit todo "Buy groceries" --due today --tag personal
  ```

### Goals and Projects

- **Goal Manager** -- active, paused, completed, archived lifecycle with milestones and due dates
- Milestone CRUD with reordering, auto-complete, goal-to-task linking, and progress sparklines
- Recurring reviews (weekly/monthly/quarterly) with progress tracking
- **AI Goal Coach** -- velocity tracking, stalled detection, priority recommendations
- **AI Project Insights** -- health analysis, risks, blockers, next actions
- Project dashboards, burndown charts, and daily standup generator

### Knowledge Management

Your notes are not just files -- they're a connected knowledge graph.

- `[[Wikilinks]]` with real-time autocomplete, backlinks panel, and interactive note graph (`Ctrl+G`)
- **Tag browser** (`Ctrl+T`) -- browse all tags with note counts, drill into tagged notes
- **Note outline** (`Ctrl+O`) -- jump to any heading in the current note
- **Bookmarks** (`Ctrl+B`) -- starred notes and recently opened, with quick navigation
- **Reading list** -- track books, papers, and articles with progress bars and notes
- Semantic search with AI embeddings -- find notes by meaning, not just keywords
- **Dataview queries** -- filter and display notes based on frontmatter metadata
- Canvas/whiteboard with nodes, connections, and spatial layout for visual thinking
- Zettelkasten ID generator for structured note-taking workflows
- Note versioning with git diffs -- see how any note changed over time
- **Smart connections** -- AI finds related notes you didn't explicitly link
- **Knowledge gaps analysis** -- AI identifies topics you've referenced but never written about
- **Thread Weaver** -- connects disparate notes into coherent narratives
- **Auto-linker** -- suggests `[[links]]` you might have missed based on content analysis

### AI Integration

Granit treats AI as a built-in utility, not a paid add-on. Every bot is optimized for small models that run on CPU.

- **23 AI Bots** (`Ctrl+R`) in 6 categories:

  | Category | Bots |
  |----------|------|
  | **Writing** | Summarizer, tone adjuster, writing coach, ghost writer, expand, TLDR |
  | **Organization** | Auto-tagger, auto-linker, flashcard generator, action items, outliner, MOC generator |
  | **Research** | Deep dive agent, note enhancer, vault analyzer, daily digest, key terms |
  | **Planning** | Plan my day, task triage, AI scheduler, goal coach, standup generator |
  | **Analysis** | Smart connections, knowledge gaps, project insights, counter-argument, pros/cons |
  | **Learning** | Explain simply, question generator, flashcards, key concepts |

- **Deep Dive Research Agent** -- runs Claude Code in the background to research topics and create structured notes in your vault. Status indicator in the status bar. ESC to minimize, Ctrl+C to cancel.
- **Ghost Writer** -- inline AI suggestions as you type, Tab to accept. 32-entry completion cache ensures fast suggestions even with slow models.
- Providers: **Ollama** (local, default), **OpenAI**, **Nous**, or offline fallback with graceful degradation
- **Ollama management** -- install, start/stop, and pull models directly from Granit's settings panel
- Production reliability: auto-retry with exponential backoff, HTTP cancellation, timeout handling, token-budget checks, and per-bot system prompts with small-model variants

> **Privacy first:** When using Ollama, your notes never leave your machine. All AI processing happens locally. The 0.5B parameter `qwen2.5:0.5b` model runs on any CPU and handles auto-tagging, summarization, and flashcard generation well. Upgrade to 3B-7B for better writing and planning quality.

### Habits and Daily Routines

- Habit tracker with streaks, frequency settings (daily/weekly), and completion history
- Pomodoro timer with configurable session goals, focus sessions (25/45/60/90 min)
- Daily planner with time-blocked schedule and clipboard export
- **Morning Routine** (`Alt+M`) -- scripture, set a goal, select tasks, toggle habits, write thoughts -- saved to daily note
- **Evening Review** (`Alt+E`) -- reflect on accomplishments, audit overdue tasks, plan tomorrow
- **Daily Jot** (`Alt+J`) -- quick timestamped bullets throughout the day
- Smart daily note templates with 15+ variables (`{{overdue_tasks}}`, `{{active_goals}}`, etc.)
- Clock in/out time tracking with project tagging and weekly reports

### Explorer / File Browser

- Hierarchical file tree with folder expand/collapse, file type icons, and daily/weekly note markers
- **Persistent state** -- folder collapse state saved across sessions automatically
- `z` to collapse all folders, `Z` to expand all -- instantly reorganize your view
- `/` to enter fuzzy search mode, filtering across all filenames in real time
- Vim-style navigation: `j`/`k` to move, `h`/`l` to collapse/expand, `Enter` to open
- Tab switching: `Ctrl+Tab` / `Ctrl+Shift+Tab` to cycle, `Ctrl+1-9` to jump by position

### Sync and Export

- **Git** -- built-in overlay with status, log, diff, commit, push, pull, author config; `granit sync` CLI
- **Nextcloud** WebDAV sync with auto-sync option
- Export to HTML, PDF (pandoc), plain text; blog publisher (Medium, GitHub Pages)
- Note encryption (AES-256-GCM), import from Obsidian/Logseq/Notion
- Backup and restore with timestamped zip archives

### Customization

Granit is deeply configurable. Every visual element can be changed without touching code.

- **40 themes** including 5 accessibility themes (high-contrast, deuteranopia, protanopia, tritanopia, monochrome)
  - Popular themes: Catppuccin Mocha/Latte/Frappe/Macchiato, Tokyo Night, Gruvbox, Nord, Dracula, Rosepine, Solarized, One Dark, Kanagawa, Everforest, and more
- **Live theme editor** -- customize all 16 color roles in real-time and export as custom JSON
- **17 layouts** with instant switching (`Alt+L`):

  | Layout | Best for |
  |--------|----------|
  | Default | General note-taking with sidebar + editor + backlinks |
  | Writer | Centered editor with generous margins |
  | Zen | Distraction-free, no panels |
  | Dashboard | Sidebar + editor + calendar panel |
  | Cockpit | Compact layout with status tray |
  | Kanban | Task manager focused with board view |
  | Cornell | Cornell note-taking method layout |
  | Widescreen | Optimized for ultra-wide monitors |
  | + 9 more | Reading, focus, preview, presenter, research, etc. |

- 4 icon themes: unicode (default), nerd font, emoji, ASCII
- 16 core plugins with Lua scripting API for custom workflows
- Command palette (`Ctrl+X`) with 168 commands across 11 categories
- Per-vault configuration overrides -- different themes and settings per vault

### Testing & Reliability

Granit is one of the most thoroughly tested TUI applications in the Go ecosystem.

- **2,072 test cases** covering editor operations, task parsing, calendar logic, AI integration, configuration, vault scanning, and more
- Tests run on every commit with `go test ./...`
- Edge case coverage: Unicode handling, large file performance, concurrent vault access, malformed Markdown, empty vaults
- AI bots include fallback paths for offline/error scenarios -- Granit never crashes because an AI request failed
- Configuration migration handles schema changes across versions
- File watcher detects external changes (edits from other apps) and reloads gracefully

---

## A Day with Granit

Here's what a typical day looks like:

**6:30 AM -- Morning routine** (`Alt+M`)
You open Granit and press `Alt+M`. A Bible verse greets you. You set your goal for the day: "Ship the API refactor." You select today's tasks from your vault, toggle which habits you'll focus on, and write a quick thought. Everything gets saved to your daily note.

**9:00 AM -- Deep work**
You're coding in tmux. An idea hits you mid-flow. Without leaving your terminal: `granit note "Consider event sourcing for audit trail"`. Done. Back to code. Later, you press `Ctrl+R` and ask the auto-linker to connect your new note to related ones.

**12:00 PM -- Quick capture**
Lunch meeting produced action items. You fire off:
```bash
granit todo "Draft API migration guide" --due friday --tag docs
granit todo "Set up staging env" --due tomorrow --priority high
```

**3:00 PM -- Research**
You press `Alt+R` to launch the Deep Dive Research Agent. It runs Claude Code in the background, researching "event sourcing patterns in Go," and creates a structured note in your vault. You keep working while it runs -- the status bar shows progress.

**5:30 PM -- Review and sync**
Press `Alt+E` for the evening review. Granit shows what you accomplished, flags overdue tasks, and helps you plan tomorrow. Then `granit sync` commits and pushes everything.

**Throughout the day** -- `Alt+J` for quick jots. `Ctrl+L` to check your calendar. `Alt+H` for the dashboard with tasks, goals, habits, and stats at a glance.

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
| [Ollama](https://ollama.ai) | Local AI (23 bots, Ghost Writer, research agent) |
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

# Or open any directory of Markdown files (including Obsidian vaults)
granit ~/Documents/notes

# Subsequent launches re-open the last vault
granit
```

### First steps inside Granit

| Step | Shortcut | What it does |
|------|----------|--------------|
| 1 | `Ctrl+N` | Create a new note with template picker |
| 2 | Type `[[` | Link to another note with autocomplete |
| 3 | `Ctrl+K` | Open task manager -- add your first task |
| 4 | `Ctrl+L` | Open calendar -- view month/week/day |
| 5 | `Ctrl+R` | AI bots -- auto-tag, summarize, flashcards |
| 6 | `Ctrl+X` | Command palette -- 168 commands at your fingertips |
| 7 | `Ctrl+,` | Settings -- configure theme, AI, layout |
| 8 | `Alt+M` | Morning routine -- plan your day |
| 9 | `Ctrl+G` | Note graph -- visualize connections |
| 10 | `Alt+L` | Layout picker -- try all 17 layouts |

### Import your existing vault

```bash
# Obsidian vault -- just point Granit at it
granit ~/Documents/ObsidianVault

# Logseq -- works with the pages/ directory
granit ~/Logseq/pages

# Notion export (markdown)
granit import --from notion ~/Downloads/notion-export
```

---

## Keyboard Shortcuts

### Core Navigation

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Cycle between panels |
| `F1` / `F2` / `F3` | Focus sidebar / editor / backlinks |
| `Ctrl+Tab` / `Ctrl+Shift+Tab` | Cycle between open tabs |
| `Ctrl+1-9` | Jump to tab by position |
| `Ctrl+P` | Quick open (fuzzy file search) |
| `Ctrl+J` | Quick switch between recent files |
| `Ctrl+N` | New note with template picker |
| `Ctrl+S` | Save current note |
| `Ctrl+E` | Toggle edit / reading mode |
| `Ctrl+Q` | Quit |

### Explorer / Sidebar

| Key | Action |
|-----|--------|
| `j` / `k` / `Up` / `Down` | Navigate up / down |
| `Enter` / `Space` | Open file or expand/collapse folder |
| `Left` / `h` | Collapse folder or go to parent |
| `Right` / `l` | Expand folder or enter directory |
| `z` | Collapse all folders |
| `Z` | Expand all folders |
| `/` | Search / filter files |
| `Esc` | Clear search, return to tree |

> Folder state persists across sessions automatically.

### Overlays and Tools

| Key | Action |
|-----|--------|
| `Ctrl+X` | Command palette (168 commands) |
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
| `Alt+M` | Morning routine -- scripture, goal, tasks, habits, thoughts |
| `Alt+D` | Today's daily note |
| `Alt+J` | Daily jot (quick timestamped entries) |
| `Alt+E` | Evening review |
| `Alt+H` | Dashboard (tasks, goals, habits, stats) |
| `Alt+W` | Weekly note |
| `Alt+L` | Layout picker |
| `Alt+R` | Deep Dive Research Agent |

See [docs/KEYBINDINGS.md](docs/KEYBINDINGS.md) for the complete reference including Vim mode, task manager, calendar, canvas, and goals keybindings.

---

## CLI Reference

Granit includes a full CLI for quick actions from your shell -- perfect for scripting, cron jobs, and integration with other tools.

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
echo "idea" | granit clip       # Capture from stdin (pipe-friendly)
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
granit completion bash            # Shell completions (bash, zsh, fish)
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

All 23 AI bots are optimized for small models. A 0.5B parameter model running on CPU is enough for auto-tagging, summarization, flashcards, and most features. Larger models (3B-7B) improve quality for writing coach, planning, and research tasks.

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
| **Writing** | Summarizer, tone adjuster, writing coach, ghost writer, expand, TLDR |
| **Organization** | Auto-tagger, auto-linker, flashcard generator, action items, outliner |
| **Research** | Deep dive agent, note enhancer, vault analyzer, daily digest, key terms |
| **Planning** | Plan my day, task triage, AI scheduler, goal coach, standup generator |
| **Analysis** | Smart connections, knowledge gaps, project insights, counter-argument, pros/cons |

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

## Granit vs the Alternatives

| Feature | Granit | Obsidian | Notion | Logseq |
|---------|--------|----------|--------|--------|
| **Price** | **Free forever (MIT)** | $50/yr + $8/mo sync | $8-10/mo | Free |
| **Terminal native** | Yes | No (Electron) | No (web) | No (Electron) |
| **Open source** | Yes (MIT) | No | No | Yes (AGPL) |
| **Startup time** | ~12ms | ~2s | ~3s | ~2s |
| **Vim mode** | Full (macros, objects, marks) | Plugin | No | Plugin |
| **AI integration** | 23 bots, local or cloud | $14/mo add-on | Built-in (cloud only) | Plugin |
| **Task manager** | 7 views + Eisenhower + Kanban | Plugin | Built-in | Built-in |
| **Calendar** | 6 views + time-blocking | Plugin | Built-in | No |
| **Goal tracking** | Built-in + AI coaching | No | No | No |
| **Habit tracking** | Built-in with streaks | No | Template | No |
| **Daily routines** | Morning + Evening + Jot | No | Template | Journal |
| **Themes** | 40 built-in + editor | Community | Limited | Community |
| **Layouts** | 17 built-in | 1 | 1 | 1 |
| **Offline** | Always | Paid | No | Yes |
| **Data format** | Plain Markdown | Plain Markdown | Proprietary | Markdown/EDN |
| **Binary size** | ~32 MB | ~300 MB | N/A | ~250 MB |
| **Memory usage** | ~30 MB | ~500 MB+ | ~500 MB+ | ~400 MB+ |

---

## Architecture

Granit is a monolithic Go application built on the [Bubble Tea](https://github.com/charmbracelet/bubbletea) TUI framework.

```
cmd/granit/          Entry point, CLI commands
internal/
  config/            Configuration loading and persistence
  tui/               All TUI components (~219 files)
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
| Lines of Go | 187,000+ |
| Test cases | 2,072 |
| TUI components | 219 files |
| Commands | 168 |
| Themes | 40 |
| Layouts | 17 |
| Core plugins | 16 |
| AI bots | 23 |

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
| [Themes](docs/THEMES.md) | 40 themes + custom theme creation |
| [Installation](docs/INSTALLATION.md) | Build, install, desktop app, dependencies |
| [Plugins](docs/PLUGINS.md) | Plugin development and Lua API |
| [Architecture](docs/ARCHITECTURE.md) | Codebase structure and design |
| [Changelog](CHANGELOG.md) | Version history and release notes |

---

## Frequently Asked Questions

<details>
<summary><strong>Can I use my existing Obsidian vault?</strong></summary>

Yes. Point Granit at your Obsidian vault directory and everything works: `[[wikilinks]]`, backlinks, YAML frontmatter, tags, daily notes, and canvas files. You don't need to migrate, convert, or change anything. Both apps can use the same vault simultaneously.
</details>

<details>
<summary><strong>Does Granit require an internet connection?</strong></summary>

No. Granit works fully offline. AI features use local models via Ollama (also offline). Cloud AI providers (OpenAI, Claude) are optional. Git and Nextcloud sync only connect when you explicitly run sync.
</details>

<details>
<summary><strong>How does Granit compare to Neovim with plugins?</strong></summary>

Granit gives you a fully integrated system out of the box -- no plugin hunting, no configuration hell, no breaking updates. Tasks, calendar, goals, habits, AI, and daily routines all work together seamlessly. Think of it as Neovim's editing DNA combined with Notion's feature set, in a single binary.
</details>

<details>
<summary><strong>What AI models work with Granit?</strong></summary>

Any Ollama-compatible model works. We recommend `qwen2.5:0.5b` for fast, lightweight AI on CPU. Larger models like `llama3.1:8b` or `mistral:7b` give better results for writing and planning tasks. OpenAI models (GPT-4o, GPT-4o-mini) also work. The Deep Dive Research Agent uses Claude Code.
</details>

<details>
<summary><strong>Is there a GUI / desktop version?</strong></summary>

Yes. Granit includes a Wails-based desktop app (Go + Svelte) in the `desktop/` directory. However, the terminal version is the primary interface and most actively developed. See [docs/INSTALLATION.md](docs/INSTALLATION.md) for desktop build instructions.
</details>

<details>
<summary><strong>How do I sync between devices?</strong></summary>

Three options: (1) `granit sync` uses git -- pull, commit, push in one command. (2) Nextcloud WebDAV sync is built-in. (3) Any folder-based sync (Dropbox, Syncthing, rsync) works since notes are plain files. Git is recommended because it gives you version history and conflict resolution.
</details>

<details>
<summary><strong>Can I write plugins?</strong></summary>

Yes. Granit has a Lua scripting API and 16 core plugins included. Plugins can hook into save, open, create, and delete events. See [docs/PLUGINS.md](docs/PLUGINS.md) for the full API reference and examples.
</details>

<details>
<summary><strong>What terminals work best?</strong></summary>

Any terminal with 256-color support works. Recommended: Alacritty, Kitty, Ghostty, WezTerm, or iTerm2. Works well in tmux and screen. Nerd Font icons are supported but optional (Unicode icons are the default).
</details>

---

## Roadmap

Granit is actively developed. Here's what's coming:

- [ ] **Mobile companion app** -- read and capture on the go, sync via git
- [ ] **Collaborative editing** -- real-time shared vaults over WebSocket
- [ ] **PDF annotation** -- highlight and annotate PDFs inline
- [ ] **Spaced repetition** -- integrated SRS with the flashcard generator
- [ ] **Email integration** -- clip emails to notes, send notes as email
- [ ] **Voice notes** -- record and transcribe with Whisper
- [ ] **Advanced dataview** -- SQL-like queries across all vault metadata
- [ ] **Web clipper** -- save web pages as clean Markdown
- [ ] **Multi-vault search** -- search across all vaults from one interface
- [ ] **Theme marketplace** -- share and discover community themes

Want to help? See [Contributing](#contributing) or browse [open issues](https://github.com/artaeon/granit/issues).

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
</p>

<p align="center">
  <a href="#installation"><img src="https://img.shields.io/badge/GET%20STARTED-Install%20Now-b48ead?style=for-the-badge" alt="Get Started"></a>
</p>

<p align="center">
  <a href="https://github.com/artaeon/granit/issues">Report a Bug</a> &bull;
  <a href="https://github.com/artaeon/granit/discussions">Discussions</a> &bull;
  <a href="https://github.com/artaeon/granit">Star on GitHub</a>
</p>

<p align="center">
  <sub>If Granit helps you, consider giving it a star. It helps others discover the project.</sub>
</p>
