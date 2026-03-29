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
  <strong>A blazing-fast, AI-powered knowledge manager for terminal and desktop -- fully Obsidian compatible</strong>
</p>

<p align="center">
  <a href="#installation"><img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go Version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue?style=for-the-badge" alt="License"></a>
  <img src="https://img.shields.io/badge/Platform-Linux%20%7C%20macOS-lightgrey?style=for-the-badge" alt="Platform">
  <img src="https://img.shields.io/badge/TUI-164k%2B%20Lines-orange?style=for-the-badge" alt="Codebase">
  <img src="https://img.shields.io/badge/Desktop-Wails%20v2-9B59B6?style=for-the-badge" alt="Desktop App">
  <img src="https://img.shields.io/badge/Themes-40-purple?style=for-the-badge" alt="Themes">
</p>

<p align="center">
  <a href="#features">Features</a> &bull;
  <a href="#installation">Installation</a> &bull;
  <a href="#quick-start">Quick Start</a> &bull;
  <a href="#cli-commands">CLI Commands</a> &bull;
  <a href="#keyboard-shortcuts">Shortcuts</a> &bull;
  <a href="#ai-setup">AI Setup</a> &bull;
  <a href="#nextcloud-sync">Nextcloud Sync</a> &bull;
  <a href="#configuration">Config</a> &bull;
  <a href="#architecture">Architecture</a>
</p>

---

Granit is a **free, open-source** personal knowledge management system built in Go. It runs as a full-featured **terminal TUI** or as a native **desktop application** (Wails v2 + Svelte). It reads and writes standard Markdown with YAML frontmatter and `[[wikilinks]]`, keeping your vault **fully compatible** with Obsidian, Logseq, and any other Markdown-based tool.

**No Electron. No browser. No subscriptions. Just your terminal -- or a lightweight native window.**

| | |
|---|---|
| **240+ source files** | 164,000+ lines of Go |
| **40 themes** | 33 dark + 7 light, plus custom theme editor |
| **20+ AI features** | Ollama, OpenAI, Nous, Claude Code, or offline fallback |
| **8 layouts** | Default, Writer, Minimal, Reading, Dashboard, Zen, Taskboard, Research |
| **16 core plugins** | Enable/disable modules individually |
| **Full Vim mode** | Normal, Insert, Visual, Command -- ex commands, dot repeat, macros |
| **Desktop app** | Wails v2 with Svelte frontend |
| **Obsidian compatible** | `[[wikilinks]]`, YAML frontmatter, same folder structure |
| **Zero telemetry** | Your data stays local. Always. |

---

## Screenshots

<p align="center">
  <img src="assets/hero.gif" alt="Granit TUI" width="800"><br>
  <sub>Terminal UI -- the desktop app (Wails v2 + Svelte) provides the same features in a native window</sub>
</p>

---

## Features

### Editor

- Syntax-highlighted Markdown with Chroma (200+ languages)
- Full Vim modal editing -- Normal, Insert, Visual, Command modes with macros and dot repeat
- Multi-cursor editing, heading folding, split pane view, visual table editor
- Find and replace in-file and global across the vault
- Ghost Writer -- inline AI writing suggestions (Tab to accept)
- 18 built-in snippets, wikilink `[[` autocomplete, rendered view mode (`Ctrl+E`)
- Mermaid diagram rendering, auto-close brackets, smart indentation

### Knowledge Management

- `[[Wikilinks]]` with autocomplete, backlinks panel, and note graph (`Ctrl+G`)
- Tag browser (`Ctrl+T`), note outline (`Ctrl+O`), bookmarks and recents (`Ctrl+B`)
- Reading list with progress tracking (to-read / reading / completed) and star ratings
- Link suggestions -- TF-IDF similarity-powered "Suggested" tab in backlinks panel
- Smart Connections, semantic search (AI embeddings), Knowledge Graph AI
- Dataview queries -- SQL-like: `TABLE title, tags FROM "folder" WHERE tags CONTAINS "project" SORT date DESC`
- Mind map view, canvas/whiteboard (`Ctrl+W`), Zettelkasten ID generator
- Note versioning timeline with git-powered diffs and snapshot restore

### Task Management

- Task Manager (`Ctrl+K`) -- 7 views: Today, Upcoming, All, Done, Calendar, Kanban, Eisenhower Matrix
- 5 priority levels, date picker, cross-vault scanning
- Subtask hierarchy -- indentation-based nesting with expand/collapse (`e`)
- Task dependencies -- `depends:` syntax with blocked indicators
- Time estimation -- `~30m` / `~2h` syntax with workload totals
- Per-task time tracking -- actual logged time badge (green under estimate, red over)
- Reschedule (`r`) -- quick move to tomorrow, next Monday, +1 week, +1 month
- Batch reschedule (`R`) -- walk through all overdue tasks with quick date picks
- Task snooze (`z`) -- hide for 1h, 4h, or tomorrow 9am; auto-reappears on expiry
- Pinned tasks (`W`) -- pin to top of all views, persisted across sessions
- Task notes (`n`) -- attach freeform notes to any task, with note icon indicator
- Auto-priority (`A`) -- heuristic scorer based on deadline, dependencies, project
- Undo (`u`) -- 10-deep undo stack for any task modification
- Task templates (`T`/`t`) -- save tasks as reusable templates, create from template
- Task archiving (`X`) -- move completed tasks >30 days old to Archive/
- Advanced sort (`s`) -- by priority, due date, A-Z, source, or tag
- Bulk operations (`v`) -- select multiple tasks, batch toggle/date/priority
- Tag and priority filters (`#`, `P`) with visual badges
- Focus session launcher (`f`) -- start a timed session directly from a task
- Recurring tasks -- daily, weekly, monthly auto-creation
- Custom kanban columns -- configurable via settings with tag-based routing
- Eisenhower Matrix -- 2x2 urgency/importance grid (DO / SCHEDULE / DELEGATE / ELIMINATE)
- Overdue grouping -- Today view splits overdue (red) from today's tasks
- Project matching -- auto-assign tasks to projects by folder or tag
- Quick-add syntax -- `@tomorrow !high #tag ~1h` parsed from Ctrl+T quick capture
- Natural language dates -- `@next week`, `@end of month`, `@in 3 days`, `@next friday`
- Help overlay (`?`) -- full keybinding reference inside task manager
- Task filtering by config -- exclude folders, require tags, hide completed
- CLI task add: `granit todo "Ship v2.0" --due friday --priority high --tag release`

### Projects and Goals

- **Goal Manager** (command palette > "Goals") -- standalone goal tracking with milestones
  - 4 views: Active, By Category, Timeline, Completed
  - Goal lifecycle: active → paused → completed → archived
  - Milestone CRUD with auto-complete (all done → goal done)
  - Categories, target dates, overdue detection, progress bars
  - Goal notes, creation wizard (title → date → category)
- Project Mode -- 9 categories with dashboards and completion stats
- Goal tracking with milestones and progress bars
- Project health dashboard -- velocity tracking, traffic-light status indicator
- Goal burndown charts -- ASCII chart with ideal vs actual milestone pace
- Cross-project dashboard -- overview of all projects with health indicators
- AI project planner -- automated project breakdown with milestones
- Daily standup generator from git commits, modified notes, and completed tasks

### Habits and Productivity

- Habit tracker with 7-day streaks, pomodoro timer, focus sessions (25/45/60/90 min)
- Daily planner with time-blocked schedule and multi-hour blocks (30m to 3h)
- Copy daily plan to clipboard (`c` in planner, or command palette > "Copy Daily Plan")
- Smart daily note template -- `{{overdue_tasks}}`, `{{today_tasks}}`, `{{today_habits}}`, `{{today_schedule}}`, `{{active_goals}}`
- Daily review -- guided 5-phase end-of-day review with rescheduling
- Weekly review -- structured weekly reflection overlay
- Command palette (`Ctrl+X`) with 80+ commands
- Scratchpad overlay for quick floating notes (auto-saved)
- Quick capture with inbox count badge in status bar
- Journal prompts (100+), clipboard manager (50-entry history)
- Clock in/out time tracking with project tags and weekly logs
- Workspace snapshots -- save and restore named TUI layouts
- Reminders with daily/weekdays/once scheduling
- Dashboard -- overdue warnings, habit widget, writing streaks, quick stats

### Calendar

- Month, week, 3-day, agenda, and year views (`Ctrl+L`)
- Week time grid -- hourly rows with 7 day columns showing events and planner blocks
- Calendar sidebar panel
- Task badges on calendar dates
- Quick event creation from calendar

### AI Integration

- **9 AI Bots** (`Ctrl+R`) -- tagger, linker, summarizer, Q&A, writing assistant, titles, action items, MOC generator, daily digest
- AI Chat, AI Compose, Thread Weaver, Ghost Writer, Deep Dive Research (Claude Code)
- AI Writing Coach, AI Smart Scheduler, Vault Refactor, Daily Briefing
- Quiz Mode and Flashcards (SM-2 spaced repetition) for active recall
- Providers: **Nous**, **Ollama**, **OpenAI**, or zero-setup offline fallback

### Sync

- **Nextcloud** WebDAV sync with auto-sync option
- **Git integration** -- built-in overlay with status, log, diff, commit, push, pull
- `granit sync` -- pull, commit, push in one CLI command
- Per-note git history with colored diffs and version restore

### Study and Language Learning

- Flashcards with SM-2 spaced repetition, auto-generated quizzes
- Language learning -- vocabulary tracker for 9 languages with practice mode
- Learning dashboard with streaks, mastery tracking, and level distribution

### Import, Export, and Publishing

- Import from other note formats
- Trash/recycle bin with restore for deleted notes
- Export to HTML, plain text, or PDF (via pandoc); bulk export entire vault
- Static site publisher with search, tag pages, and wikilink resolution
- Blog publisher -- Medium and GitHub with token persistence
- Note encryption -- AES-256-GCM with PBKDF2

### Desktop App

- Native desktop application built with **Wails v2**
- **Svelte** frontend with reactive UI components
- Full feature parity with the TUI -- same Go backend
- File tree sidebar, editor, overlays, and keyboard shortcuts

---

## Installation

### Requirements

- **Go 1.24+** ([install Go](https://go.dev/doc/install))
- **Git** (for cloning and git features)
- Linux or macOS

### TUI (Terminal)

```bash
git clone https://github.com/artaeon/granit.git
cd granit
go install ./cmd/granit/
```

Make sure `~/go/bin` is in your PATH:

```bash
export PATH="$HOME/go/bin:$PATH"
```

### Desktop App (Wails)

```bash
# Install Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Build the desktop app
cd granit/desktop
wails build
```

The built binary will be in `desktop/build/bin/`.

### Optional Dependencies

| Tool | Purpose |
|------|---------|
| **Ollama** | Local AI models |
| **aspell** / **hunspell** | Spell checking |
| **pandoc** | PDF export |
| **xclip** / **wl-copy** | System clipboard (Linux) |
| **Claude Code** | Deep Dive research agent |

---

## Quick Start

```bash
# Launch vault selector:
granit

# Open a specific vault:
granit ~/Notes

# Create today's daily note:
granit daily ~/Notes

# Quick capture a thought:
granit capture "Remember to review PR #42"
```

### First Steps

1. Run `granit` in any directory with `.md` files, or create a new vault from the selector.
2. Use `Tab` or `F1`/`F2`/`F3` to switch between sidebar, editor, and backlinks.
3. Press `Ctrl+N` to create a new note from 10+ templates.
4. Type `[[` in the editor to start a wikilink with autocomplete.
5. Press `Ctrl+E` to toggle between edit and rendered view.
6. Press `Ctrl+X` to open the command palette -- access 70+ commands.
7. Press `Ctrl+K` to open the task manager with Kanban board.

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `granit` | Launch vault selector |
| `granit <path>` | Open a vault directly |
| `granit init [path]` | Initialize a new vault |
| `granit daily [path]` | Create/open today's daily note |
| `granit today [path]` | Print today's dashboard (`--json`) |
| `granit review [path]` | Daily review (`--week`, `--markdown`, `--save`) |
| `granit search <query>` | Search vault content (`--regex`, `--json`) |
| `granit todo <text>` | Add task (`--due`, `--priority`, `--tag`) |
| `granit capture <text>` | Quick-capture to inbox.md |
| `granit sync [path]` | Pull, commit, push (`--dry-run`, `-m "msg"`) |
| `granit clock in/out/log` | Time tracking with project tags |
| `granit remind "text"` | Set reminders (`--at HH:MM`, `--daily`) |
| `granit serve [path]` | Serve vault as read-only website |
| `granit list [path]` | List vault notes (`--json`, `--paths`, `--tags`) |
| `granit query <filter>` | Query notes by tag, folder, or frontmatter (`--json`) |
| `granit import <source>` | Import notes from other formats (`--from`) |
| `granit export [path]` | Export to HTML, text, or JSON |
| `granit backup [path]` | Create timestamped zip backup |
| `granit plugin list` | Plugin management |
| `granit config` | Show configuration |
| `granit man` | Generate man page |
| `granit completion bash` | Shell completions (bash/zsh/fish) |

Run `granit help` for the full command reference.

---

## Keyboard Shortcuts

### Navigation

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Cycle between panels |
| `F1` (`Alt+1`) / `F2` (`Alt+2`) / `F3` (`Alt+3`) | Focus sidebar / editor / backlinks |
| `F4` | Rename current note |
| `F5` (`Alt+?`) | Help overlay |
| `Esc` | Close overlay / return to sidebar |
| `Ctrl+P` | Quick open (fuzzy file search) |
| `Ctrl+J` | Quick switch files |
| `Ctrl+N` | Create new note |
| `Ctrl+1`--`Ctrl+9` | Switch to tab by number |

### Editor

| Key | Action |
|-----|--------|
| `Ctrl+S` | Save |
| `Ctrl+E` | Toggle view/edit mode |
| `Ctrl+U` / `Ctrl+Y` | Undo / Redo |
| `Ctrl+F` / `Ctrl+H` | Find / Find and replace |
| `Ctrl+D` | Multi-cursor: select word / next occurrence |
| `Alt+F` | Toggle fold at cursor |

### Views and Tools

| Key | Action |
|-----|--------|
| `Ctrl+X` | Command palette |
| `Ctrl+K` | Task manager |
| `Ctrl+G` | Note graph |
| `Ctrl+T` | Tag browser |
| `Ctrl+O` | Note outline |
| `Ctrl+B` | Bookmarks and recent |
| `Ctrl+L` | Calendar |
| `Ctrl+R` | AI bots |
| `Ctrl+W` | Canvas / whiteboard |
| `Ctrl+Z` | Focus / zen mode |
| `Ctrl+,` | Settings |
| `Ctrl+Q` | Quit |

### Task Manager (`Ctrl+K`)

| Key | Action |
|-----|--------|
| `j`/`k` | Navigate up/down |
| `x` | Toggle task done/undone |
| `u` | Undo last action |
| `n` | Add/edit task note |
| `z` | Snooze task (1h/4h/tomorrow) |
| `W` | Pin/unpin task |
| `A` | Auto-suggest priority |
| `R` | Batch reschedule overdue (Today view) |
| `T` | Save current task as template |
| `t` | Create task from template |
| `X` | Archive completed tasks >30 days |
| `?` | Help overlay (keybinding reference) |
| `e` | Expand/collapse subtasks |
| `f` | Start focus session on task |
| `g` | Jump to task source note |
| `a` | Add new task |
| `d` | Set due date |
| `r` | Reschedule (tomorrow/Monday/+1wk/+1mo) |
| `E` | Set time estimate (15m-2h) |
| `s` | Cycle sort mode (priority/date/A-Z/source/tag) |
| `b` | Add dependency |
| `v` | Enter bulk select mode |
| `p` | Cycle priority |
| `#` | Cycle tag filter |
| `P` | Cycle priority filter |
| `c` | Clear all filters |
| `/` | Search tasks |
| `Tab` | Switch view (Today/Upcoming/All/Done/Calendar/Kanban/Matrix) |
| `1`-`7` | Jump to specific view |

### Vim Mode

When enabled, the editor supports full modal keybindings: `hjkl` movement, `dd`/`yy`/`p`, `w`/`b`/`e` word motion, `i`/`a`/`o` insert, `.` dot repeat, `q`+register macros, visual selection, and ex commands (`:w`, `:wq`, `:s/old/new/g`, `:%s`, `:set wrap`, `:{line}`).

---

## AI Setup

Granit supports four AI providers. The **local** fallback works offline with no setup.

### Ollama (Recommended)

```bash
curl -fsSL https://ollama.ai/install.sh | sh
ollama pull qwen2.5:0.5b   # 4 GB RAM -- use larger models with more RAM
ollama serve
```

Or use the built-in wizard: Settings (`Ctrl+,`) > Setup Ollama.

### OpenAI

Set in `~/.config/granit/config.json`:

```json
{
  "ai_provider": "openai",
  "openai_key": "sk-...",
  "openai_model": "gpt-4o-mini"
}
```

### Nous (Local Server)

```json
{
  "ai_provider": "nous",
  "nous_url": "http://localhost:3333",
  "nous_api_key": ""
}
```

### Claude Code (Research)

The Deep Dive Research feature uses Claude Code as a research agent. Requires `claude` in PATH.

---

## Nextcloud Sync

Granit can sync your vault via Nextcloud WebDAV.

Configure in `~/.config/granit/config.json` or via Settings (`Ctrl+,`):

```json
{
  "nextcloud_url": "https://your-nextcloud.example.com",
  "nextcloud_user": "username",
  "nextcloud_pass": "app-password",
  "nextcloud_path": "Notes",
  "nextcloud_auto_sync": false
}
```

Generate an app password in Nextcloud under Settings > Security > Devices & sessions. Set `nextcloud_auto_sync` to `true` to sync automatically on save and open.

---

## Configuration

Granit uses layered JSON configuration. Per-vault settings override global settings.

| Scope | Path |
|-------|------|
| Global | `~/.config/granit/config.json` |
| Per-vault | `<vault>/.granit.json` |
| Vault registry | `~/.config/granit/vaults.json` |
| Plugins | `~/.config/granit/plugins/` or `<vault>/.granit/plugins/` |

All settings are editable from the built-in settings panel (`Ctrl+,`).

Key options: `theme` (40 themes), `layout` (8 layouts), `vim_mode`, `auto_save`, `ai_provider` (`local`/`ollama`/`openai`/`nous`), `ghost_writer`, `auto_tag`, `git_auto_sync`, `spell_check`, `icon_theme` (`unicode`/`nerd`/`emoji`/`ascii`), `pomodoro_goal`, `nextcloud_auto_sync`.

Environment variables: `GRANIT_VAULT` (default vault path), `EDITOR` (external editor).

---

## Architecture

Granit is a Go monolith with two frontends sharing the same core packages.

```
cmd/granit/        CLI entry point and subcommands
internal/config/   JSON configuration (global + per-vault)
internal/vault/    Vault scanning, Markdown parser, backlink index
internal/tui/      Terminal UI -- 188 source files on Bubble Tea
desktop/           Wails v2 desktop app (Go API + Svelte frontend)
```

The **TUI** is built on [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss). The **desktop app** uses [Wails v2](https://wails.io/) with a Svelte frontend calling the same Go backend via bindings. A **Lua scripting engine** and **plugin system** (16 core plugins, language-agnostic scripts) provide extensibility.

---

## License

Granit is released under the [MIT License](LICENSE).

---

<p align="center">
  <strong>Granit</strong> -- your knowledge, your terminal, your rules.<br>
  <sub>Free and open source. No telemetry. No subscriptions. Your data stays local.</sub>
</p>
