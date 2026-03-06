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
  <strong>A blazing-fast, AI-powered terminal knowledge manager — fully Obsidian compatible</strong>
</p>

<p align="center">
  <a href="#installation"><img src="https://img.shields.io/badge/Go-1.23+-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go Version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue?style=for-the-badge" alt="License"></a>
  <img src="https://img.shields.io/badge/Platform-Linux%20%7C%20macOS-lightgrey?style=for-the-badge" alt="Platform">
  <img src="https://img.shields.io/badge/Status-Active-brightgreen?style=for-the-badge" alt="Status">
</p>

<p align="center">
  <a href="#features">Features</a> •
  <a href="#installation">Installation</a> •
  <a href="#quick-start">Quick Start</a> •
  <a href="#ai-features">AI Features</a> •
  <a href="#keyboard-shortcuts">Shortcuts</a> •
  <a href="#configuration">Config</a> •
  <a href="#themes">Themes</a> •
  <a href="#contributing">Contributing</a>
</p>

<p align="center">
  <img src="assets/editor.gif" alt="Granit in action" width="800">
</p>

---

Granit is a **free, open-source** terminal-native personal knowledge management system built in Go. It reads and writes standard Markdown with YAML frontmatter and `[[wikilinks]]`, so your vault stays **fully compatible** with Obsidian, Logseq, and any other Markdown-based tool.

**No Electron. No browser. No subscriptions. Just your terminal.**

> **Why Granit?** Obsidian is great, but it's Electron-based, closed-source, and its AI features require a paid subscription. Granit gives you a fast, keyboard-driven alternative with **built-in AI** (local or cloud), **Vim keybindings**, **Git integration**, and **60+ features** — all running natively in your terminal at a fraction of the memory footprint.

---

## Screenshots

<table>
<tr>
<td align="center"><strong>Splash Screen</strong></td>
<td align="center"><strong>Rendered View Mode</strong></td>
</tr>
<tr>
<td><img src="assets/screenshot-splash.png" alt="Splash screen" width="400"></td>
<td><img src="assets/screenshot-viewmode.png" alt="View mode" width="400"></td>
</tr>
<tr>
<td align="center"><strong>3-Panel Editor Layout</strong></td>
<td align="center"><strong>Sidebar + Backlinks</strong></td>
</tr>
<tr>
<td><img src="assets/screenshot-editor.png" alt="Editor layout" width="400"></td>
<td><img src="assets/splash.gif" alt="Launch animation" width="400"></td>
</tr>
</table>

---

## Features

### Core Editor

- **Syntax-highlighted Markdown** — headings, bold, italic, code blocks, blockquotes, lists, checkboxes
- **Wikilinks** — `[[note]]` linking with automatic resolution across the vault
- **Backlinks panel** — see every note that links to the current one, plus outgoing links
- **YAML frontmatter** — parsing and display of tags, dates, and custom fields
- **Rendered view mode** — toggle between raw edit and styled reading with `Ctrl+E`
- **Vim keybindings** — full modal editing (Normal/Insert/Visual/Command) with `hjkl`, `dd`/`yy`/`p`, `:w`/`:q`, dot repeat
- **Multi-cursor editing** — `Ctrl+D` to select word and add cursors at next occurrence
- **Undo/Redo** — full edit history (`Ctrl+U` / `Ctrl+Y`)
- **Find & Replace** — `Ctrl+F` / `Ctrl+H` with match highlighting
- **Smart autocomplete** — inline wikilink popup triggered by `[[` with fuzzy search and preview snippets
- **Auto-close brackets** and smart indentation
- **Line numbers** with active line highlighting
- **Snippet expansion** — 18 built-in snippets (`/date`, `/todo`, `/meeting`, `/table`, etc.)
- **Spell checking** — integrated aspell/hunspell support
- **Focus/Zen mode** — distraction-free writing with `Ctrl+Z`
- **Ghost Writer** — inline AI writing suggestions (Tab to accept)
- **Visual table editor** — edit Markdown tables in a spreadsheet-like interface
- **Mermaid diagrams** — ASCII rendering of flowcharts, sequence diagrams, and pie charts in view mode

### AI-Powered Features

Granit includes **12 AI features** that work with local models (Ollama), OpenAI, or a zero-setup offline fallback:

| Feature | Description |
|---------|-------------|
| **9 AI Bots** | Auto-Tagger, Link Suggester, Summarizer, Q&A, Writing Assistant, Title Suggester, Action Items, MOC Generator, Daily Digest |
| **AI Chat** | Ask questions about your entire vault with context-aware answers |
| **Chat with Note** | AI Q&A focused on the current note |
| **AI Compose** | Generate full notes from a topic prompt |
| **Ghost Writer** | Inline writing suggestions as you type |
| **Thread Weaver** | Synthesize multiple notes into a new essay or summary |
| **Semantic Search** | AI-powered meaning-based vault search using embeddings |
| **Knowledge Graph AI** | Analyze clusters, hubs, orphan notes, and get link suggestions |
| **Auto-Link** | Find unlinked mentions of note titles in your text |
| **Auto-Tag** | Automatically suggest tags on save |
| **Similar Notes** | TF-IDF cosine similarity to find related notes |
| **Quiz Mode** | Auto-generated quizzes from your notes for active recall |
| **Flashcards** | Spaced repetition study (SM-2 algorithm) extracted from your notes |
| **Learning Dashboard** | Track study progress, streaks, and mastery |

### Vault Management

- **Vault selector** — pick from recent vaults or create new ones when launching `granit` without arguments
- **File tree sidebar** with folder expand/collapse and file icons
- **Fuzzy search** (`Ctrl+P`) across all notes
- **Full-text search** — search across all note contents with highlighted results
- **Tag browser** (`Ctrl+T`) — browse and filter notes by tag
- **Graph view** (`Ctrl+G`) — visualize note connections
- **Calendar view** (`Ctrl+L`) — month, week, and agenda views tied to daily notes
- **Bookmarks & recents** (`Ctrl+B`) — star notes and jump to recently opened files
- **Quick switch** (`Ctrl+J`) — fast switching among recent notes
- **Note outline** (`Ctrl+O`) — heading-based document outline
- **Breadcrumb navigation** — `Alt+Left`/`Alt+Right` for browser-style back/forward, pinned tabs
- **Daily notes** — create or open today's note with a single command
- **Vault statistics** — note counts, link density, word counts
- **Trash** — soft-delete with restore
- **Folder management** — create folders and move files
- **File watcher** — auto-detects external changes and refreshes the vault
- **Pomodoro timer** — 25-min focus sessions with break cycles, writing stats tracking
- **System clipboard** — `Ctrl+V` paste with platform-native clipboard access
- **Web clipper** — fetch a URL, convert to Markdown, save as a note

### Git Integration

Built-in git overlay with three views:

- **Status** — modified, added, deleted, and untracked files
- **Log** — recent commit history with colored hashes
- **Diff** — syntax-highlighted diff of unstaged changes
- Quick actions: **commit** (c), **push** (p), **pull** (P), **refresh** (r)
- **Auto-sync** — optional auto commit+push on save, pull on open

### Export & Publishing

- **Export to HTML** — styled document with CSS
- **Export to Plain Text** — Markdown stripped to plain text
- **Export to PDF** — via pandoc (if installed)
- **Bulk HTML export** — all vault notes at once
- **Static site publisher** — export your vault as a complete HTML website with search, tag pages, and wikilink resolution

### Extensibility

- **Plugin system** — language-agnostic scripts with JSON manifests, 6 built-in plugins
- **Lua scripting** — full API access to vault operations (`granit.read_note`, `granit.write_note`, etc.)
- **Dataview queries** — embed live queries in notes using `query` code blocks
- **Obsidian import** — import settings from existing `.obsidian/` directories
- **Canvas / Whiteboard** — visual note arrangement with connections and colors
- **Split panes** — view two notes side by side

### 28 Themes & 4 Icon Sets

Instantly switch between **22 dark** and **6 light** themes from settings. Choose from **Unicode**, **Nerd Font**, **Emoji**, or **ASCII** icon sets.

### 10 Note Templates

Create notes from built-in templates: Standard, Meeting Notes, Project Plan, Weekly Review, Book Notes, Decision Record, Journal Entry, Research Note, Learning/Zettelkasten, and more.

---

## Installation

### Requirements

- **Go 1.23+** ([install Go](https://go.dev/doc/install))
- **Git** (for cloning and git features)
- Linux or macOS (Windows support planned)

### Quick Install (Recommended)

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
source ~/.bashrc  # or source ~/.zshrc
```

### System-wide Install

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

```bash
cd granit
git pull
go install ./cmd/granit/
```

### Optional Dependencies

| Tool | Purpose | Required? |
|------|---------|-----------|
| **Ollama** | Local AI (recommended) | No — local fallback works offline |
| **aspell** or **hunspell** | Spell checking | No |
| **pandoc** | PDF export | No |
| **xclip**, **xsel**, or **wl-copy** | System clipboard (Linux) | No — clipboard features degrade gracefully |
| **Git** | Version control features | No — git features are optional |

---

## Quick Start

```bash
# Open the vault selector (pick from recent vaults or create new):
granit

# Open a specific vault:
granit ~/Notes

# Open with explicit command:
granit open ~/Notes

# Create/open today's daily note:
granit daily ~/Notes

# Scan a vault and print stats:
granit scan ~/Notes

# Print version:
granit version
```

### First Steps

1. Run `granit` in any directory with `.md` files — or create a new vault from the selector.
2. Use `Tab` or `F1`/`F2`/`F3` to switch between sidebar, editor, and backlinks.
3. Press `Ctrl+N` to create a new note (pick from 10 templates).
4. Type `[[` in the editor to start a wikilink — autocomplete suggests matching notes.
5. Press `Ctrl+E` to toggle between edit and rendered view mode.
6. Press `Ctrl+S` to save. Enable auto-save in settings (`Ctrl+,`).
7. Press `Ctrl+X` to open the **command palette** — access all 60+ commands from one place.

---

## AI Features

Granit supports three AI providers. The **local** provider works out of the box with no setup.

### Ollama (Recommended for Local AI)

Granit includes a **built-in setup wizard**. Open settings (`Ctrl+,`), select **"Setup Ollama"**, and press Enter. The wizard installs Ollama and pulls your chosen model automatically.

Or set up manually:

```bash
# Install Ollama
curl -fsSL https://ollama.ai/install.sh | sh

# Pull a model
ollama pull qwen2.5:0.5b

# Start the server
ollama serve
```

#### Model Recommendations

| RAM | Model | Quality |
|-----|-------|---------|
| 4 GB | `qwen2.5:0.5b` | Fast, lightweight |
| 8 GB | `qwen2.5:1.5b` or `phi3:mini` | Good balance |
| 16 GB | `qwen2.5:3b` or `phi3.5:3.8b` | High quality |
| 32 GB+ | `llama3.2` or `mistral` | Best quality |

When Granit exits, it automatically unloads the Ollama model to free memory.

### OpenAI

```json
{
  "ai_provider": "openai",
  "openai_key": "sk-...",
  "openai_model": "gpt-4o-mini"
}
```

Available models: `gpt-4o-mini`, `gpt-4o`, `gpt-4.1-mini`, `gpt-4.1-nano`.

### Local Fallback

The default `"local"` provider uses keyword matching, stopword filtering, and topic detection — no network calls, no API keys, works offline.

---

## Keyboard Shortcuts

### Navigation

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Cycle between panels |
| `F1` / `F2` / `F3` | Focus sidebar / editor / backlinks |
| `Alt+Left` / `Alt+Right` | Navigate back / forward in history |
| `Esc` | Return to sidebar / close overlay |
| `j` / `k` / Arrows | Navigate up/down |
| `Enter` | Open selected file or link |

### File Operations

| Key | Action |
|-----|--------|
| `Ctrl+P` | Quick open (fuzzy search) |
| `Ctrl+N` | Create new note (template picker) |
| `Ctrl+S` | Save current note |
| `Ctrl+V` | Paste from system clipboard |
| `F4` | Rename current note |
| `Ctrl+X` | Command palette (all commands) |

### Editor

| Key | Action |
|-----|--------|
| `Ctrl+E` | Toggle view/edit mode |
| `Ctrl+U` / `Ctrl+Y` | Undo / Redo |
| `Ctrl+F` | Find in file |
| `Ctrl+H` | Find and replace |
| `Ctrl+D` | Select word / multi-cursor |
| `Ctrl+K` | Delete to end of line |
| `[[` | Trigger wikilink autocomplete |
| `Tab` | Accept ghost writer suggestion / indent |

### Views & Tools

| Key | Action |
|-----|--------|
| `Ctrl+G` | Note graph |
| `Ctrl+T` | Tag browser |
| `Ctrl+O` | Note outline |
| `Ctrl+B` | Bookmarks & recent |
| `Ctrl+J` | Quick switch files |
| `Ctrl+W` | Canvas / whiteboard |
| `Ctrl+L` | Calendar view |
| `Ctrl+R` | AI bots |
| `Ctrl+Z` | Focus / zen mode |
| `Ctrl+,` | Settings |
| `F5` | Help / keyboard shortcuts |
| `Ctrl+Q` | Quit |

### Vim Mode

When enabled (settings or command palette), the editor uses full modal keybindings:

| Mode | Keys |
|------|------|
| **Normal** | `h`/`j`/`k`/`l`, `w`/`b`/`e`, `0`/`$`, `gg`/`G`, `dd`/`yy`/`p`, `u`/`Ctrl+R`, `i`/`a`/`o`/`O`, `.` repeat |
| **Insert** | All keys pass through; `Esc` returns to Normal |
| **Visual** | Movement extends selection; `d` deletes, `y` yanks |
| **Command** | `:w` save, `:q` quit, `:wq` save+quit, `:{n}` go to line |

---

## Configuration

Granit uses a layered JSON config:

| Scope | Path |
|-------|------|
| Global | `~/.config/granit/config.json` |
| Per-vault | `<vault>/.granit.json` |
| Vault list | `~/.config/granit/vaults.json` |

Per-vault settings override global. All settings can be changed from the built-in settings panel (`Ctrl+,`).

<details>
<summary><strong>All Configuration Options</strong></summary>

```json
{
  "editor": {
    "tab_size": 4,
    "insert_tabs": false,
    "auto_indent": true
  },
  "theme": "catppuccin-mocha",
  "icon_theme": "unicode",
  "layout": "default",
  "sidebar_position": "left",
  "show_icons": true,
  "show_help": true,
  "show_splash": true,
  "compact_mode": false,
  "line_numbers": true,
  "word_wrap": false,
  "highlight_current_line": true,
  "auto_close_brackets": true,
  "auto_save": false,
  "auto_refresh": true,
  "confirm_delete": true,
  "default_view_mode": false,
  "vim_mode": false,
  "ghost_writer": false,
  "auto_tag": false,
  "daily_notes_folder": "",
  "daily_note_template": "",
  "git_auto_sync": false,
  "ai_provider": "local",
  "ollama_model": "qwen2.5:0.5b",
  "ollama_url": "http://localhost:11434",
  "openai_key": "",
  "openai_model": "gpt-4o-mini"
}
```

| Option | Default | Description |
|--------|---------|-------------|
| `theme` | `catppuccin-mocha` | Color theme (28 available) |
| `icon_theme` | `unicode` | `unicode`, `nerd`, `emoji`, or `ascii` |
| `layout` | `default` | `default` (3-panel), `writer` (2-panel), `minimal` (editor only) |
| `vim_mode` | `false` | Enable Vim-style modal editing |
| `ghost_writer` | `false` | Enable inline AI writing suggestions |
| `auto_tag` | `false` | Auto-suggest tags on save |
| `git_auto_sync` | `false` | Auto commit+push on save, pull on open |
| `ai_provider` | `local` | `local`, `ollama`, or `openai` |

</details>

---

## Themes

### Dark Themes (22)

| Theme | Description |
|-------|-------------|
| `catppuccin-mocha` | Warm, pastel dark (default) |
| `catppuccin-frappe` | Mid-tone Catppuccin |
| `catppuccin-macchiato` | Deep Catppuccin |
| `tokyo-night` | Inspired by Tokyo at night |
| `gruvbox-dark` | Retro, earthy warm tones |
| `nord` | Arctic, cool blue palette |
| `dracula` | Classic dark with vivid accents |
| `solarized-dark` | Ethan Schoonover's dark palette |
| `rose-pine` | Muted, elegant dark |
| `everforest-dark` | Nature-inspired greens |
| `kanagawa` | Inspired by Hokusai |
| `one-dark` | Atom's iconic dark theme |
| `github-dark` | GitHub dark mode |
| `ayu-dark` | Minimal, deep dark |
| `palenight` | Material Design dark |
| `synthwave-84` | Neon retro synthwave |
| `nightfox` | Cool, refined dark |
| `vesper` | Warm amber on deep brown |
| `poimandres` | Cool teal and pastels |
| `moonlight` | Soft blue-purple moonlit |
| `vitesse-dark` | Minimal, modern green |
| `oxocarbon` | IBM Carbon-inspired |

### Light Themes (6)

| Theme | Description |
|-------|-------------|
| `catppuccin-latte` | Warm, pastel light |
| `solarized-light` | Ethan Schoonover's light |
| `rose-pine-dawn` | Elegant, warm light |
| `github-light` | GitHub light mode |
| `ayu-light` | Clean, bright light |
| `min-light` | Ultra-minimal bright |

---

## Architecture

```
granit/
  cmd/granit/
    main.go                 CLI entry point, vault selector
  internal/
    config/
      config.go             JSON configuration (global + per-vault)
      vaults.go             Vault list persistence
      import.go             Obsidian config importer
    vault/
      vault.go              Vault scanning, note storage
      parser.go             Markdown/frontmatter/wikilink parser
      index.go              Backlink and link index
    tui/
      app.go                Main Bubble Tea model (~2800 lines)
      editor.go             Text editor with multi-cursor
      renderer.go           Markdown rendering for view mode
      sidebar.go            File tree sidebar
      statusbar.go          Status bar with pomodoro indicator
      styles.go             Global style definitions
      themes.go             28 built-in color themes
      command.go            Command palette (60+ actions)
      vim.go                Vim modal editing
      watcher.go            File system polling watcher
      vaultselector.go      Vault selector full-screen UI
      clipboard.go          System clipboard + web clipper
      pomodoro.go           Pomodoro focus timer
      breadcrumb.go         Back/forward navigation + pinned tabs
      linkcomplete.go       Wikilink autocomplete popup
      ghostwriter.go        Inline AI writing suggestions
      threadweaver.go       Multi-note AI synthesis
      autotag.go            Auto-tagger + note chat
      embeddings.go         Semantic search with AI embeddings
      tableeditor.go        Visual markdown table editor
      mermaid.go            Mermaid diagram ASCII renderer
      bots.go               AI bot system (9 bots)
      flashcards.go         Spaced repetition (SM-2)
      quizmode.go           Auto-generated quizzes
      learndash.go          Learning dashboard
      aichat.go             Vault-wide AI chat
      composer.go           AI note composer
      knowledgegraph.go     Knowledge graph analysis
      similarity.go         TF-IDF note similarity
      git.go                Git integration overlay
      export.go             Note export (HTML, text, PDF)
      publish.go            Static site publisher
      plugins.go            Plugin system + registry
      lua.go                Lua scripting engine
      calendar.go           Calendar view (month/week/agenda)
      canvas.go             Visual whiteboard
      contentsearch.go      Full-text vault search
      ... and more
```

Built on [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss) by [Charm](https://charm.sh/).

---

## Contributing

Contributions are welcome! Granit is free and open-source software.

### Build & Run

```bash
git clone https://github.com/artaeon/granit.git
cd granit
go build -o granit ./cmd/granit
./granit ~/your-vault
```

### Development Guidelines

- All TUI components live in `internal/tui/` and follow Bubble Tea's `Model`/`Update`/`View` pattern
- Overlays use value receivers for `Update` and `View`, helper components use pointer receivers
- Configuration goes in `internal/config/config.go` + `internal/tui/settings.go`
- Themes are `Theme` structs in `internal/tui/themes.go`
- Keep dependencies minimal (currently: Bubble Tea, Lip Gloss, GopherLua)

### Submitting Changes

1. Fork the repository and create a feature branch
2. Make your changes and verify `go build ./...` and `go vet ./...` pass
3. Open a pull request with a clear description

### Reporting Issues

Found a bug or have a feature request? [Open an issue](https://github.com/artaeon/granit/issues).

---

## License

Granit is released under the [MIT License](LICENSE). Free to use, modify, and distribute.

---

## Acknowledgments

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) & [Lip Gloss](https://github.com/charmbracelet/lipgloss) — the terminal UI framework
- [Charm](https://charm.sh/) — the team behind the Go terminal ecosystem
- [Obsidian](https://obsidian.md/) — inspiration for vault-based knowledge management
- [Catppuccin](https://github.com/catppuccin/catppuccin) — the default color palette
- [GopherLua](https://github.com/yuin/gopher-lua) — Lua scripting support

---

<p align="center">
  <strong>Granit</strong> — your knowledge, your terminal, your rules.<br>
  <sub>Free and open source. No telemetry. No subscriptions. Your data stays local.</sub>
</p>
