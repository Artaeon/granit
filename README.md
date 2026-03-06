
```
   ██████╗ ██████╗  █████╗ ███╗   ██╗██╗████████╗
  ██╔════╝ ██╔══██╗██╔══██╗████╗  ██║██║╚══██╔══╝
  ██║  ███╗██████╔╝███████║██╔██╗ ██║██║   ██║
  ██║   ██║██╔══██╗██╔══██║██║╚██╗██║██║   ██║
  ╚██████╔╝██║  ██║██║  ██║██║ ╚████║██║   ██║
   ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝╚═╝   ╚═╝
```

### A blazing-fast terminal knowledge manager -- Obsidian compatible

![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go)
![License](https://img.shields.io/badge/License-MIT-blue?style=flat-square)
![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS-lightgrey?style=flat-square)

---

Granit is a terminal-native personal knowledge management system built in Go. It reads and writes standard Markdown with YAML frontmatter and `[[wikilinks]]`, so your vault stays fully compatible with Obsidian, Logseq, and any other Markdown-based tool. No Electron. No browser. Just your terminal.

<!-- Screenshot: main view -->

---

## Table of Contents

- [Features](#features)
- [Screenshots](#screenshots)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Keyboard Shortcuts](#keyboard-shortcuts)
- [Configuration](#configuration)
- [AI Setup](#ai-setup)
- [Themes](#themes)
- [Architecture](#architecture)
- [Contributing](#contributing)
- [License](#license)
- [Acknowledgments](#acknowledgments)

---

## Features

### Core Features

- **Markdown editing** with syntax highlighting (headings, bold, italic, code, blockquotes, lists)
- **Wikilinks** -- `[[note]]` linking with automatic resolution across the vault
- **Backlinks panel** -- see every note that links to the current one, plus outgoing links
- **YAML frontmatter** parsing and display (tags, dates, custom fields)
- **Rendered view mode** -- toggle between raw edit and a styled read view with `Ctrl+E`
- **Daily notes** -- create or open today's note with a single command

### AI-Powered Bots

Nine built-in AI bots that analyze and enhance your notes:

| Bot | Description |
|-----|-------------|
| Auto-Tagger | Suggest tags for the current note |
| Link Suggester | Find related notes in the vault |
| Summarizer | Generate a brief summary |
| Question Bot | Ask questions about your notes |
| Writing Assistant | Suggest writing improvements |
| Title Suggester | Propose better note titles |
| Action Items | Extract todos and action items |
| MOC Generator | Create a Map of Content |
| Daily Digest | Summarize vault activity |

Bots support three providers:

- **Local** -- keyword-based fallback that works offline with zero setup
- **Ollama** -- local LLM inference (built-in setup wizard installs Ollama and pulls a model)
- **OpenAI** -- GPT-4o, GPT-4o-mini, GPT-4.1-mini, GPT-4.1-nano via API

### Vault Management

- **File tree sidebar** with folder expand/collapse and file icons
- **Fuzzy search** (`Ctrl+P`) across all notes
- **Tag browser** (`Ctrl+T`) -- browse and filter notes by tag
- **Graph view** (`Ctrl+G`) -- visualize note connections with incoming/outgoing link counts
- **Calendar view** (`Ctrl+L`) -- month, week, and agenda views tied to daily notes
- **Bookmarks and recents** (`Ctrl+B`) -- star notes and jump to recently opened files
- **Quick switch** (`Ctrl+J`) -- fast file switching among recent notes
- **Note outline** (`Ctrl+O`) -- heading-based document outline for quick navigation
- **Vault statistics** -- note counts, link density, and scan time
- **Trash** -- soft-delete with restore support
- **Folder management** -- create folders and move files between them

### Editor Features

- **Syntax highlighting** for Markdown elements (headings, links, code blocks, lists, checkboxes)
- **Undo / Redo** (`Ctrl+U` / `Ctrl+Y`)
- **Find in file** (`Ctrl+F`) with match highlighting
- **Find and replace** (`Ctrl+H`)
- **Autocomplete** for wikilinks and tags
- **Auto-close brackets** and smart indentation
- **Line numbers** with active line highlighting
- **Cursor navigation** (Home/End, PgUp/PgDown, arrow keys)
- **Multi-cursor editing** -- `Ctrl+D` to select word and add cursors at next occurrence
- **Vim mode** (optional, toggle in settings)
- **Focus / Zen mode** (`Ctrl+Z`) -- distraction-free writing

### Git Integration

Built-in git integration (`Ctrl+X` → Git) with three views:

- **Status** -- see modified, added, deleted, and untracked files
- **Log** -- recent commit history with colored hashes
- **Diff** -- syntax-highlighted diff of unstaged changes

Quick actions: **commit** (c), **push** (p), **pull** (P), **refresh** (r). Switch views with Tab or 1/2/3.

### Export

Export the current note to multiple formats via the command palette:

- **HTML** -- styled document with CSS, saved alongside the note
- **Plain Text** -- Markdown stripped to plain text
- **PDF** -- via pandoc (if installed)
- **Bulk HTML** -- export all vault notes at once

### 28 Themes

Granit ships with 28 built-in color themes covering both dark and light palettes. Switch themes instantly from the settings panel (`Ctrl+,`). See the [full theme list](#themes) below.

### 4 Icon Themes

| Theme | Description |
|-------|-------------|
| `unicode` | Default -- uses Unicode symbols (works everywhere) |
| `nerd` | Nerd Font icons (requires a patched font) |
| `emoji` | Emoji-based icons |
| `ascii` | Plain ASCII characters (maximum compatibility) |

### 10 Note Templates

Press `Ctrl+N` to create a note from a built-in template:

- Blank Note (no template)
- Standard Note (title, date, tags)
- Meeting Notes (attendees, agenda, action items)
- Project Plan (goals, timeline table, tasks)
- Weekly Review (accomplishments, challenges, next week)
- Book Notes (summary, key ideas, quotes)
- Decision Record (context, decision, consequences)
- Journal Entry (mood, gratitude, tomorrow's tasks)
- Research Note (findings, methodology, evidence)
- Learning Note / Zettelkasten (main idea, connections, source)

Templates support `{{title}}` and `{{date}}` placeholders that are filled in automatically.

### Advanced Editor

- **Table editing** -- Tab navigates between cells, Enter adds rows, automatic column alignment
- **Snippet expansion** -- type `/date`, `/todo`, `/meeting`, `/table` etc. and press space to expand (18 built-in snippets)
- **Multi-cursor editing** -- `Ctrl+D` to select word and add cursors at next occurrence
- **Spellcheck** -- integrated aspell/hunspell support via the command palette

### Full-Text Search

Search across all note contents in the vault via the command palette. Results show file, line number, and matching context with highlighted terms.

### Dataview Queries

Embed live queries in your notes using code blocks:

````
```query
from: tags:project
where: status = active
sort: modified desc
fields: title, tags, modified
limit: 10
```
````

Results render as styled tables in view mode.

### Split Panes

View two notes side by side with independent scrolling. Open via the command palette.

### Static Site Publisher

Export your entire vault as a static HTML website with built-in CSS theme, tag pages, search, and wikilink resolution. Open via `Ctrl+X` → Publish Site.

### Auto Git Sync

Enable in settings to automatically commit and push on every save, and pull latest changes when opening a vault. Works with any git-initialized vault.

### Plugin System

Extend Granit with custom plugins. Plugins are language-agnostic scripts that integrate via JSON manifests:

```
~/.config/granit/plugins/
  my-plugin/
    plugin.json    # manifest (name, commands, hooks)
    run.sh         # command script (any language)
```

Plugins can:
- **Register commands** that appear in the command palette
- **Hook into events**: `on_save`, `on_open`, `on_create`, `on_delete`
- **Modify note content**, insert text, or show messages
- Run in any language (bash, Python, Node, etc.)

**Built-in plugin registry** -- press `i` in the Plugin Manager to install from 6 ready-made plugins (word-count, timestamp, backlink-count, reading-time, orphan-finder, daily-summary).

### Lua Scripting

Write Lua scripts that interact with your vault. Place `.lua` files in `<vault>/.granit/lua/` or `~/.config/granit/lua/` and run them from the command palette.

Scripts have full access to the `granit` API:
- `granit.read_note(name)` / `granit.write_note(name, content)` / `granit.list_notes()`
- `granit.set_content(text)` / `granit.insert(text)` / `granit.msg(text)`
- `granit.note_path` / `granit.note_content` / `granit.frontmatter` / `granit.date()`

### Obsidian Import

Import settings from an existing Obsidian vault's `.obsidian/` directory -- theme, vim mode, tab size, daily notes folder, and more. Available in the command palette.

### Canvas / Whiteboard

A visual canvas (`Ctrl+W`) for spatial note arrangement:
- Place note cards, reposition and resize them
- Draw connections between cards
- Cycle card colors and zoom levels
- Auto-saves to `<vault>/.granit/canvas.json`

### Command Palette

Press `Ctrl+X` to open the command palette -- a fuzzy-searchable launcher for every action in Granit.

---

## Screenshots

<!-- Screenshot: main editing view with sidebar, editor, and backlinks -->

<!-- Screenshot: graph view showing note connections -->

<!-- Screenshot: command palette overlay -->

<!-- Screenshot: AI bots panel -->

<!-- Screenshot: calendar view -->

<!-- Screenshot: canvas / whiteboard -->

---

## Installation

### Quick Install (Recommended)

Requires Go 1.23 or later:

```bash
git clone https://github.com/artaeon/granit.git
cd granit
go install ./cmd/granit/
```

This installs the `granit` binary to `~/go/bin/`. Make sure it's in your PATH:

```bash
# Add to ~/.bashrc or ~/.zshrc (one-time setup):
export PATH="$HOME/go/bin:$PATH"

# Then reload:
source ~/.bashrc
```

Now you can run `granit` from anywhere:

```bash
granit            # opens current directory as vault
granit ~/Notes    # opens a specific vault
```

### System-wide Install

If you prefer a system-wide install:

```bash
git clone https://github.com/artaeon/granit.git
cd granit
go build -o granit ./cmd/granit
sudo mv granit /usr/local/bin/
```

### Go Install (Remote)

```bash
go install github.com/artaeon/granit/cmd/granit@latest
```

### Updating

Pull the latest changes and reinstall:

```bash
cd granit
git pull
go install ./cmd/granit/
```

---

## Quick Start

**Open the current directory as a vault:**

```bash
granit
```

**Open a specific vault** (any directory of Markdown files):

```bash
granit ~/Notes
# or
granit open ~/Notes
```

**Create and open today's daily note:**

```bash
granit daily ~/Notes
```

**Scan a vault and print statistics:**

```bash
granit scan ~/Notes
```

**Print version:**

```bash
granit version
```

### First Steps

1. Run `granit` in any directory with `.md` files (or it will create a new vault).
2. Use `Tab` or `F1`/`F2`/`F3` to switch between the sidebar, editor, and backlinks panel.
3. Press `Ctrl+N` to create a new note, or select an existing note from the sidebar.
4. Type `[[` in the editor to start a wikilink -- autocomplete will suggest matching notes.
5. Press `Ctrl+E` to toggle between edit and rendered view mode.
6. Press `Ctrl+S` to save, or enable auto-save in settings (`Ctrl+,`).

---

## Keyboard Shortcuts

### Navigation

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Cycle between panels |
| `F1` | Focus file sidebar |
| `F2` | Focus editor |
| `F3` | Focus backlinks panel |
| `Esc` | Return to sidebar / close overlay |
| `j` / `k` / Arrow keys | Navigate up/down |
| `Enter` | Open selected file or link |

### File Operations

| Key | Action |
|-----|--------|
| `Ctrl+P` | Quick open (fuzzy search) |
| `Ctrl+N` | Create new note |
| `Ctrl+S` | Save current note |
| `F4` | Rename current note |
| `Ctrl+X` | Command palette |

### Editor

| Key | Action |
|-----|--------|
| `Ctrl+E` | Toggle view/edit mode |
| Arrow keys | Move cursor |
| `Home` / `Ctrl+A` | Go to line start |
| `End` / `Ctrl+E` | Go to line end |
| `PgUp` / `PgDown` | Scroll page |
| `Ctrl+U` | Undo |
| `Ctrl+Y` | Redo |
| `Ctrl+K` | Delete to end of line |
| `Ctrl+D` / `Delete` | Delete character forward |
| `Ctrl+D` | Select word / add cursor at next match |
| `Tab` | Insert spaces (configurable tab size) |

### Views and Tools

| Key | Action |
|-----|--------|
| `Ctrl+G` | Show note graph |
| `Ctrl+T` | Browse tags |
| `Ctrl+O` | Show note outline |
| `Ctrl+B` | Bookmarks and recent notes |
| `Ctrl+F` | Find in file |
| `Ctrl+H` | Find and replace |
| `Ctrl+J` | Quick switch files |
| `Ctrl+W` | Canvas / whiteboard |
| `Ctrl+L` | Calendar (month/week/agenda) |
| `Ctrl+R` | AI bots |
| `Ctrl+Z` | Focus / zen mode |
| `Ctrl+,` | Open settings |
| `F5` | Show keyboard shortcuts |

### Sidebar

| Key | Action |
|-----|--------|
| Type characters | Fuzzy filter files |
| `Backspace` | Clear search character |
| `Esc` | Clear search |

### Backlinks Panel

| Key | Action |
|-----|--------|
| `Tab` | Toggle backlinks / outgoing links |
| `Enter` | Navigate to linked note |

### Application

| Key | Action |
|-----|--------|
| `Ctrl+Q` / `Ctrl+C` | Quit Granit |

---

## Configuration

Granit uses a layered JSON configuration system:

| Scope | Path |
|-------|------|
| Global | `~/.config/granit/config.json` |
| Per-vault | `<vault-root>/.granit.json` |

Per-vault settings override global settings. All settings can also be changed from the built-in settings panel (`Ctrl+,`).

### All Configuration Options

```json
{
  "editor": {
    "tab_size": 4,
    "insert_tabs": false,
    "auto_indent": true
  },
  "theme": "catppuccin-mocha",
  "show_help": true,
  "daily_notes_folder": "",
  "daily_note_template": "",
  "auto_close_brackets": true,
  "highlight_current_line": true,
  "show_minimap": false,
  "sidebar_position": "left",
  "show_icons": true,
  "compact_mode": false,
  "icon_theme": "unicode",
  "layout": "default",
  "auto_save": false,
  "show_splash": true,
  "vim_mode": false,
  "line_numbers": true,
  "word_wrap": false,
  "default_view_mode": false,
  "confirm_delete": true,
  "auto_refresh": true,
  "spell_check": false,
  "git_auto_sync": false,
  "ai_provider": "local",
  "ollama_model": "qwen2.5:0.5b",
  "ollama_url": "http://localhost:11434",
  "openai_key": "",
  "openai_model": "gpt-4o-mini",
  "background_bots": false,
  "show_hidden_files": false,
  "sort_by": "name",
  "search_content_by_default": true,
  "max_search_results": 50
}
```

<details>
<summary>Option Reference</summary>

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `editor.tab_size` | int | `4` | Number of spaces per tab |
| `editor.insert_tabs` | bool | `false` | Insert tab characters instead of spaces |
| `editor.auto_indent` | bool | `true` | Automatically indent new lines |
| `theme` | string | `"catppuccin-mocha"` | Color theme name |
| `show_help` | bool | `true` | Show the help bar at the bottom |
| `daily_notes_folder` | string | `""` | Subfolder for daily notes |
| `daily_note_template` | string | `""` | Template for daily notes |
| `auto_close_brackets` | bool | `true` | Auto-close brackets, parens, quotes |
| `highlight_current_line` | bool | `true` | Highlight the active editor line |
| `show_minimap` | bool | `false` | Show editor minimap |
| `sidebar_position` | string | `"left"` | Sidebar position: `"left"` or `"right"` |
| `show_icons` | bool | `true` | Show file/folder icons |
| `compact_mode` | bool | `false` | Reduce padding for smaller terminals |
| `icon_theme` | string | `"unicode"` | Icon set: `"unicode"`, `"nerd"`, `"emoji"`, `"ascii"` |
| `layout` | string | `"default"` | Layout preset: `"default"`, `"writer"`, `"minimal"` |
| `auto_save` | bool | `false` | Save automatically on changes |
| `show_splash` | bool | `true` | Show splash screen on startup |
| `vim_mode` | bool | `false` | Enable Vim-style keybindings |
| `line_numbers` | bool | `true` | Show line numbers in the editor |
| `word_wrap` | bool | `false` | Wrap long lines |
| `default_view_mode` | bool | `false` | Open notes in view mode by default |
| `confirm_delete` | bool | `true` | Ask for confirmation before deleting |
| `auto_refresh` | bool | `true` | Auto-rescan vault for external changes |
| `spell_check` | bool | `false` | Enable spell checking |
| `git_auto_sync` | bool | `false` | Auto commit+push on save, pull on open |
| `ai_provider` | string | `"local"` | AI backend: `"local"`, `"ollama"`, `"openai"` |
| `ollama_model` | string | `"qwen2.5:0.5b"` | Ollama model to use |
| `ollama_url` | string | `"http://localhost:11434"` | Ollama API endpoint |
| `openai_key` | string | `""` | OpenAI API key |
| `openai_model` | string | `"gpt-4o-mini"` | OpenAI model to use |
| `background_bots` | bool | `false` | Auto-analyze notes on save |
| `show_hidden_files` | bool | `false` | Show dotfiles in the sidebar |
| `sort_by` | string | `"name"` | File sort order: `"name"`, `"modified"`, `"created"` |
| `search_content_by_default` | bool | `true` | Search file contents (not just names) |
| `max_search_results` | int | `50` | Maximum fuzzy search results |

</details>

---

## AI Setup

Granit supports three AI providers. The **local** provider works out of the box with no external dependencies -- it uses keyword extraction and heuristics instead of a language model.

### Ollama (Recommended for Local LLM)

Granit includes a built-in Ollama setup wizard. Open settings (`Ctrl+,`), navigate to **"Setup Ollama (install + model)"**, and press Enter. The wizard will:

1. Check if Ollama is installed
2. Install it if missing
3. Pull your selected model

Alternatively, set it up manually:

```bash
# Install Ollama
curl -fsSL https://ollama.ai/install.sh | sh

# Pull a model (smallest, good for tagging and summaries)
ollama pull qwen2.5:0.5b

# Start the Ollama server
ollama serve
```

Then set the provider in your config or via settings:

```json
{
  "ai_provider": "ollama",
  "ollama_model": "qwen2.5:0.5b",
  "ollama_url": "http://localhost:11434"
}
```

When Granit exits, it automatically unloads the running Ollama model to free memory.

#### Model Recommendations by Hardware

| RAM | Recommended Model | Notes |
|-----|-------------------|-------|
| 4 GB | `qwen2.5:0.5b` | Minimal footprint, fast responses |
| 8 GB | `qwen2.5:1.5b` or `phi3:mini` | Good balance of quality and speed |
| 16 GB | `qwen2.5:3b` or `phi3.5:3.8b` | Higher quality output |
| 32 GB+ | `llama3.2` or `mistral` | Best quality, larger context |

All available models in settings: `qwen2.5:0.5b`, `qwen2.5:1.5b`, `qwen2.5:3b`, `phi3:mini`, `phi3.5:3.8b`, `gemma2:2b`, `tinyllama`, `llama3.2`, `llama3.2:1b`, `mistral`, `gemma2`.

### OpenAI

Set your API key and preferred model:

```json
{
  "ai_provider": "openai",
  "openai_key": "sk-...",
  "openai_model": "gpt-4o-mini"
}
```

Available models: `gpt-4o-mini`, `gpt-4o`, `gpt-4.1-mini`, `gpt-4.1-nano`.

### Local Fallback

The default `"local"` provider requires no setup. It uses keyword matching, stopword filtering, and topic detection to provide basic tagging, link suggestions, and summaries without any network calls.

---

## Themes

Granit ships with 28 color themes. Change themes from the settings panel (`Ctrl+,`) or by editing your config file.

### Dark Themes

| Theme | Description |
|-------|-------------|
| `catppuccin-mocha` | Warm, pastel dark theme (default) |
| `catppuccin-frappe` | Mid-tone Catppuccin variant |
| `catppuccin-macchiato` | Deep Catppuccin variant |
| `tokyo-night` | Inspired by Tokyo at night |
| `gruvbox-dark` | Retro, earthy warm tones |
| `nord` | Arctic, cool blue palette |
| `dracula` | Classic dark with vivid accents |
| `solarized-dark` | Ethan Schoonover's dark palette |
| `rose-pine` | Muted, elegant dark palette |
| `everforest-dark` | Nature-inspired greens and browns |
| `kanagawa` | Inspired by Katsushika Hokusai |
| `one-dark` | Atom's iconic dark theme |
| `github-dark` | GitHub's dark mode colors |
| `ayu-dark` | Minimal, deep dark palette |
| `palenight` | Material Design inspired dark |
| `synthwave-84` | Neon retro synthwave aesthetic |
| `nightfox` | Cool, refined dark palette |
| `vesper` | Warm amber on deep brown |
| `poimandres` | Cool teal and pastel accents |
| `moonlight` | Soft blue-purple moonlit palette |
| `vitesse-dark` | Minimal, modern green accents |
| `oxocarbon` | IBM Carbon-inspired cool palette |

### Light Themes

| Theme | Description |
|-------|-------------|
| `catppuccin-latte` | Warm, pastel light theme |
| `solarized-light` | Ethan Schoonover's light palette |
| `rose-pine-dawn` | Elegant, warm light palette |
| `github-light` | GitHub's light mode colors |
| `ayu-light` | Clean, bright light palette |
| `min-light` | Ultra-minimal bright palette |

---

## Architecture

Granit is structured as a standard Go project:

```
granit/
  cmd/granit/
    main.go              CLI entry point, argument parsing
  internal/
    config/
      config.go          JSON configuration (global + per-vault)
      import.go          Obsidian .obsidian/ config importer
    vault/
      vault.go           Vault scanning, note storage
      parser.go          Markdown/frontmatter/wikilink parser
      index.go           Backlink and link index
    daily/
      daily.go           Daily note creation
    tui/
      app.go             Main Bubble Tea model and update loop
      editor.go          Text editor component
      renderer.go        Markdown rendering for view mode
      sidebar.go         File tree sidebar
      filetree.go        Hierarchical file tree data structure
      backlinks.go       Backlinks/outgoing links panel
      statusbar.go       Status bar component
      styles.go          Global style definitions
      themes.go          28 built-in color themes
      command.go         Command palette with all actions
      help.go            Keyboard shortcut overlay
      settings.go        Settings panel with Ollama wizard
      bots.go            AI bot system (Ollama, OpenAI, local)
      templates.go       10 note templates
      git.go             Git integration overlay
      export.go          Note export (HTML, text, PDF)
      plugins.go         Plugin system, manager, and registry
      publish.go         Static site publisher
      lua.go             Lua scripting engine
      luaoverlay.go      Lua script runner overlay
      contentsearch.go   Full-text vault search
      dataview.go        Dataview-lite query system
      spellcheck.go      Spell checking (aspell/hunspell)
      snippets.go        Snippet/template expansion
      splitpane.go       Side-by-side note viewing
      autosync.go        Auto git commit/push/pull
      graph.go           Note connection graph view
      tags.go            Tag browser
      calendar.go        Calendar view (month/week/agenda)
      canvas.go          Visual whiteboard / canvas
      bookmarks.go       Bookmarks and recent files
      outline.go         Heading-based document outline
      findreplace.go     Find and replace
      focusmode.go       Distraction-free writing mode
      quickswitch.go     Quick file switching
      autocomplete.go    Wikilink and tag autocomplete
      trash.go           Soft delete with restore
      splash.go          Startup splash screen
      stats.go           Vault statistics
  go.mod
  go.sum
```

The TUI is built on [Bubble Tea](https://github.com/charmbracelet/bubbletea) (the Elm Architecture for Go terminals) with [Lip Gloss](https://github.com/charmbracelet/lipgloss) for styling, and [GopherLua](https://github.com/yuin/gopher-lua) for Lua scripting support.

---

## Contributing

Contributions are welcome. Here is how to get started:

### Build and Run

```bash
git clone https://github.com/artaeon/granit.git
cd granit
go build -o granit ./cmd/granit
./granit open ~/your-vault
```

### Code Conventions

- All TUI components live in `internal/tui/` and follow the Bubble Tea `Model` / `Update` / `View` pattern.
- Configuration is handled through `internal/config/config.go` -- add new settings there and wire them into `internal/tui/settings.go`.
- Themes are defined as `Theme` structs in `internal/tui/themes.go`. To add a new theme, add an entry to the `builtinThemes` map.
- Keep dependencies minimal. Granit currently depends only on Bubble Tea and Lip Gloss.

### Submitting Changes

1. Fork the repository and create a feature branch.
2. Make your changes and verify that `go build ./...` succeeds.
3. Run `go vet ./...` and fix any issues.
4. Open a pull request with a clear description of what you changed and why.

---

## License

Granit is released under the [MIT License](LICENSE).

---

## Acknowledgments

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) -- the terminal UI framework that makes Granit possible
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) -- CSS-like styling for the terminal
- [Charm](https://charm.sh/) -- the team behind the Go terminal ecosystem
- [Obsidian](https://obsidian.md/) -- the inspiration for vault-based knowledge management and the `[[wikilink]]` format
- [Catppuccin](https://github.com/catppuccin/catppuccin) -- the default color palette

---

<p align="center">
  <strong>Granit</strong> -- your knowledge, your terminal, your rules.
</p>
