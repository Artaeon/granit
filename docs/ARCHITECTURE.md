# Granit — Architecture Overview

> Technical architecture reference for developers and contributors.

---

## Table of Contents

- [Project Structure](#project-structure)
- [Bubble Tea Model/Update/View Pattern](#bubble-tea-modelupdateview-pattern)
- [Overlay System](#overlay-system)
- [Configuration System](#configuration-system)
- [Vault Scanning and Lazy Loading](#vault-scanning-and-lazy-loading)
- [Plugin System](#plugin-system)
- [AI Provider Abstraction](#ai-provider-abstraction)
- [Theme System](#theme-system)
- [Layout System](#layout-system)

---

## Project Structure

```
granit/
├── cmd/granit/
│   ├── main.go                     CLI entry point, subcommands, vault selector
│   └── manpage.go                  Roff-formatted man page generator
│
├── internal/
│   ├── config/
│   │   ├── config.go               JSON configuration (global + per-vault, layered)
│   │   ├── vaults.go               Vault list persistence (~/.config/granit/vaults.json)
│   │   └── import.go               Obsidian .obsidian/ settings importer
│   │
│   ├── daily/
│   │   └── (daily note utilities)
│   │
│   ├── vault/
│   │   ├── vault.go                Vault scanning with lazy loading
│   │   ├── parser.go               Markdown, YAML frontmatter, wikilink parser
│   │   └── index.go                Backlink and forward-link index builder
│   │
│   └── tui/                        All TUI components (Bubble Tea)
│       │
│       │── Core Application
│       ├── app.go                  Main Bubble Tea Model (~4800 lines)
│       ├── editor.go               Text editor with multi-cursor, undo/redo (~1240 lines)
│       ├── renderer.go             Markdown rendering for view mode
│       ├── sidebar.go              File tree sidebar with fuzzy search
│       ├── filetree.go             Collapsible folder hierarchy
│       ├── statusbar.go            Status bar (AI indicator, pomodoro, tasks)
│       ├── splash.go               Animated ASCII splash screen
│       │
│       │── Styling & Theming
│       ├── styles.go               Global mutable style/color variables, icon themes
│       ├── themes.go               38 built-in Theme structs + ApplyTheme()
│       ├── customtheme.go          Custom theme JSON loading/saving
│       ├── themeeditor.go          Live theme editor overlay
│       ├── layouts.go              13 panel layout definitions + helpers + Alt+L picker
│       │
│       │── Navigation & Search
│       ├── command.go              Command palette (145+ commands, 11 categories) with fuzzy filter
│       ├── quickswitch.go          Fast file switcher (Ctrl+J)
│       ├── contentsearch.go        Full-text vault search
│       ├── globalreplace.go        Global search & replace across vault
│       ├── nlsearch.go             Natural language AI-powered search
│       ├── breadcrumb.go           Breadcrumb navigation + pinned tabs
│       ├── tabbar.go               Tab bar management
│       │
│       │── Editor Enhancements
│       ├── vim.go                  Vim modal editing (Normal/Insert/Visual/Command)
│       ├── syntaxhl.go             Language-aware code block highlighting
│       ├── folding.go              Collapsible heading/code fold state
│       ├── footnotes.go            Footnote parsing and rendering
│       ├── snippets.go             18 built-in snippets with placeholder expansion
│       ├── slashmenu.go            Slash command menu (/ trigger)
│       ├── autocomplete.go         Wikilink [[ autocomplete popup
│       ├── linkcomplete.go         Link completion helpers
│       ├── spellcheck.go           aspell/hunspell integration
│       ├── findreplace.go          Find & replace in file (Ctrl+F / Ctrl+H)
│       ├── tableeditor.go          Visual markdown table editor
│       ├── encryption.go           AES-256-GCM note encryption
│       ├── frontmatteredit.go      Structured frontmatter property editor
│       ├── ghostwriter.go          Inline AI writing suggestions
│       ├── clipboard.go            System clipboard integration
│       │
│       │── Overlays & Panels
│       ├── settings.go             Settings overlay (30+ options + Ollama wizard)
│       ├── help.go                 Help overlay (keyboard shortcuts)
│       ├── graph.go                Note graph visualization
│       ├── tags.go                 Tag browser overlay
│       ├── outline.go              Note heading outline (Ctrl+O)
│       ├── bookmarks.go            Starred + recent notes (Ctrl+B)
│       ├── focusmode.go            Zen mode centered editor (Ctrl+Z)
│       ├── stats.go                Vault statistics with bar charts
│       ├── templates.go            10 built-in note templates (Ctrl+N)
│       ├── trash.go                Recycle bin with restore
│       ├── backlinks.go            Backlinks/outgoing links panel
│       ├── backlinkpreview.go      Live wikilink hover preview popup
│       ├── notepreview.go          Note preview popup (floating)
│       ├── imageview.go            Image manager + terminal preview
│       │
│       │── Knowledge Management
│       ├── calendar.go             Calendar (month/week/agenda/year views)
│       ├── timeline.go             Chronological note timeline
│       ├── canvas.go               Visual 2D note canvas (Ctrl+W)
│       ├── splitpane.go            Side-by-side note view
│       ├── taskmanager.go          Task manager with kanban, priorities
│       ├── kanban.go               Kanban board view
│       ├── mindmap.go              ASCII mind map from headings/links
│       ├── dataview.go             Dataview queries (frontmatter filter/sort)
│       ├── workspace.go            Named workspace layout persistence
│       ├── zettelkasten.go         Zettelkasten note creation
│       ├── smartconnect.go         TF-IDF content similarity engine
│       ├── linkassist.go           Unlinked mention finder + batch linking
│       │
│       │── Productivity
│       ├── pomodoro.go             Pomodoro focus timer
│       ├── focussession.go         Guided focus sessions with scratchpad
│       ├── timetracker.go          Per-note/task time tracking
│       ├── recurringtasks.go       Daily/weekly/monthly recurring tasks
│       ├── habits.go               Habit & goal tracker with streaks
│       ├── dailyplanner.go         Time-blocked daily schedule
│       ├── aischeduler.go          AI-powered schedule generation
│       ├── standup.go              Daily standup generator
│       ├── quickcapture.go         Quick capture floating input
│       ├── journalprompts.go       100+ daily reflection prompts
│       ├── clipmanager.go          Clipboard manager (50-entry history)
│       ├── dashboard.go            Vault dashboard home screen
│       ├── scratchpad.go           Persistent floating scratchpad
│       ├── projectmode.go          Project management overlay
│       ├── writingstats.go         Writing statistics with charts
│       │
│       │── AI Features (25+ features, all via AIConfig.Chat())
│       ├── aiconfig.go             Shared AIConfig struct + Chat() hub (Ollama/OpenAI/Nous/Nerve)
│       ├── bots.go                 19 AI bots in 6 categories (Ollama/OpenAI/local)
│       ├── aichat.go               Vault-wide AI chat with context search
│       ├── composer.go             AI note composer
│       ├── threadweaver.go         Multi-note AI synthesis
│       ├── autotag.go              Auto-tagger + note chat
│       ├── autolink.go             Auto-link suggestion engine
│       ├── embeddings.go           Semantic search with AI embeddings
│       ├── knowledgegraph.go       Knowledge graph AI analysis
│       ├── similarity.go           TF-IDF note similarity
│       ├── vaultrefactor.go        AI vault reorganization
│       ├── dailybriefing.go        DEEPCOVEN morning briefing
│       ├── devotional.go           AI scripture devotional (goals + daily verse)
│       ├── tasktriage.go           Smart task triage with stale detection
│       ├── planmyday.go            AI daily schedule generation
│       ├── aischeduler.go          AI schedule optimizer with preferences
│       ├── blogdraft.go            Multi-stage AI blog writer
│       ├── aitemplates.go          AI template generator (9 types)
│       ├── research.go             Claude Code research agent + analyzer
│       ├── writingcoach.go         AI writing coach + persona
│       ├── flashcards.go           Spaced repetition (SM-2 algorithm)
│       ├── quiz.go                 Auto-generated quizzes
│       ├── learndash.go            Learning progress dashboard
│       ├── languagelearn.go        Language learning module
│       │
│       │── Git & Export
│       ├── git.go                  Git integration overlay (status/log/diff)
│       ├── githistory.go           Per-note git history with diff/restore
│       ├── autosync.go             Automatic git commit+push on save
│       ├── export.go               Note export (HTML, text, PDF, bulk)
│       ├── publish.go              Static site publisher
│       ├── blogpublish.go          Blog publisher (Medium + GitHub)
│       │
│       │── Extensibility
│       ├── plugins.go              Plugin system + manager overlay (~736 lines)
│       ├── lua.go                  Lua scripting engine (GopherLua)
│       ├── luaoverlay.go           Lua script selector overlay
│       │
│       │── Utility
│       ├── toast.go                Toast notification system
│       ├── watcher.go              File system watcher
│       ├── diagrams.go             Custom diagram engine (6 types)
│       ├── mermaid.go              Mermaid diagram ASCII renderer
│       ├── notehistory.go          Note versioning timeline
│       ├── vaultswitch.go          In-app multi-vault switcher
│       └── vaultselector.go        Full-screen vault selector UI
│
├── example-vault/                  Example vault with sample notes
├── aur/
│   └── PKGBUILD                   Arch Linux AUR package definition
├── vhs/                            VHS tape files for terminal recordings
├── assets/                         Screenshots, GIFs, logos
├── Makefile                        Build, install, test, cross-compile
├── PKGBUILD                        Root PKGBUILD for Arch Linux
├── go.mod                          Go module definition
├── go.sum                          Go module checksums
├── CHANGELOG.md                    Release changelog
├── CONTRIBUTING.md                 Contribution guidelines
├── LICENSE                         MIT License
└── README.md                       Project overview and quick start
```

---

## Bubble Tea Model/Update/View Pattern

Granit is built on [Bubble Tea](https://github.com/charmbracelet/bubbletea), Charm's Elm-architecture framework for Go terminal applications.

### The Main Model

The central `Model` struct in `app.go` (~4800 lines) holds all application state:

```go
type Model struct {
    // Core state
    vault       *vault.Vault
    index       *vault.Index
    config      config.Config

    // Editor state
    content     []string
    cursor      int
    col         int
    viewMode    bool

    // Sub-components (overlays and panels)
    sidebar     Sidebar
    backlinks   BacklinksPanel
    settings    Settings
    commandPal  CommandPalette
    bots        BotPanel
    // ... 30+ overlay components
}
```

### Update Cycle

Every keypress, mouse event, or async message flows through the `Update` method:

```
User Input → tea.Msg → Model.Update() → Updated Model + tea.Cmd
```

The Update method follows a strict priority order:

1. **Window size messages** — resize all components
2. **Active overlays** — highest-priority overlay consumes the input first
3. **Vim mode** — if enabled, processes keys through the VimState
4. **Panel-specific handling** — based on which panel has focus
5. **Global shortcuts** — Ctrl+key combinations

### View Rendering

The `View` method composes the final screen from panels and overlays:

```
Model.View() → Sidebar | Editor | Backlinks → Overlay on top → Status bar
```

### Async Operations (tea.Cmd)

Long-running operations (AI calls, file I/O, git commands) return `tea.Cmd` functions that execute asynchronously and deliver results as messages:

```go
func queryOllama(prompt string) tea.Cmd {
    return func() tea.Msg {
        resp, err := http.Post(...)
        return ollamaResultMsg{response: resp, err: err}
    }
}
```

This keeps the UI responsive — the event loop never blocks on I/O.

---

## Overlay System

Overlays are modal UI components that appear on top of the main layout. Granit uses a priority-ordered overlay system.

### Overlay Priority

When multiple overlays could theoretically be active, the `Update` method checks them in priority order. The first active overlay consumes the input:

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    // 1. Command palette (highest priority)
    if m.commandPal.IsActive() { ... }
    // 2. Settings
    if m.settings.IsActive() { ... }
    // 3. Research agent
    if m.research.IsActive() { ... }
    // 4. Other overlays...
    // N. Normal editor input (lowest priority)
}
```

### Value vs Pointer Receivers

Granit follows a convention for receiver types:

- **Overlay `Update` and `View` methods** use **value receivers** — the overlay is copied and returned, following Bubble Tea's immutable update pattern:
  ```go
  func (cp CommandPalette) Update(msg tea.Msg) (CommandPalette, tea.Cmd)
  func (cp CommandPalette) View() string
  ```

- **Helper/mutation methods** use **pointer receivers** — for methods that configure the overlay:
  ```go
  func (cp *CommandPalette) Open()
  func (cp *CommandPalette) SetSize(width, height int)
  ```

### Overlay Lifecycle

1. **Open:** A method like `Open()` sets `active = true` and initializes state
2. **Update:** Each `Update` call processes input and returns a new overlay state
3. **View:** The `View` method renders the overlay as a string
4. **Close:** Setting `active = false` removes the overlay from rendering; some overlays return a result (e.g., `CommandPalette.Result()`)

### Composing Overlays in View

The main `View` method renders the base layout first, then overlays on top using Lip Gloss positioning:

```go
func (m Model) View() string {
    base := renderPanels(m)   // sidebar | editor | backlinks
    if m.commandPal.IsActive() {
        overlay := m.commandPal.View()
        base = placeCenter(base, overlay)
    }
    return base + statusBar
}
```

---

## Configuration System

### Layered Configuration

Granit uses a two-layer JSON configuration system:

```
Global:     ~/.config/granit/config.json
Per-vault:  <vault>/.granit.json
```

Per-vault settings **override** global settings. Both files use the same JSON schema.

### Config Loading

```go
func LoadForVault(vaultRoot string) Config {
    cfg := Load()  // Load global first

    // Override with vault-specific config
    vaultPath := VaultConfigPath(vaultRoot)
    if data, err := os.ReadFile(vaultPath); err == nil {
        json.Unmarshal(data, &cfg)  // Overlay on top of global
    }

    return cfg
}
```

Fields not present in the per-vault config retain their global values because `json.Unmarshal` only overwrites fields that exist in the JSON.

### Config Struct

The `Config` struct contains all settings with JSON tags:

```go
type Config struct {
    Editor    EditorConfig       `json:"editor"`
    Theme     string             `json:"theme"`
    Layout    string             `json:"layout"`
    VimMode   bool               `json:"vim_mode"`
    AIProvider string            `json:"ai_provider"`
    CorePlugins map[string]bool  `json:"core_plugins"`
    // ... 30+ fields
}
```

### Settings Persistence

When the user changes a setting in the Settings overlay, the config is saved immediately:

1. Setting change in `Settings.Update()` modifies `s.config`
2. On overlay close, `app.go` calls `config.Save()` or `config.SaveToVault()`
3. Theme changes call `ApplyTheme()` for immediate visual update

### Vault List

A separate file tracks known vaults:

```
~/.config/granit/vaults.json
```

```go
type VaultList struct {
    Vaults   []VaultEntry `json:"vaults"`
    LastUsed string       `json:"last_used"`
}

type VaultEntry struct {
    Path     string `json:"path"`
    Name     string `json:"name"`
    LastOpen string `json:"last_open"`
}
```

Vaults are automatically registered when opened and can be removed via the vault selector.

---

## Vault Scanning and Lazy Loading

### Vault Structure

```go
type Vault struct {
    root  string
    notes map[string]*Note  // relative path → Note
}

type Note struct {
    Path        string
    Content     string
    Frontmatter map[string]interface{}
    Links       []string  // outgoing [[wikilinks]]
    loaded      bool      // lazy loading flag
}
```

### Scanning

`vault.Scan()` walks the vault directory tree:

1. Finds all `.md` files (skipping hidden directories like `.git`, `.granit`)
2. Reads frontmatter and extracts links from each note
3. Stores notes in a map keyed by relative path

### Lazy Loading

For large vaults (1000+ notes), full content reading at startup would be slow. Instead:

1. **Scan phase:** Only file paths, frontmatter, and links are read
2. **On open:** Full content is loaded when a note is first opened
3. **Indexing:** The backlink index is built from the scan data (which includes links)

This enables sub-second startup even with thousands of notes.

### Backlink Index

```go
type Index struct {
    backlinks map[string][]string  // target → [source notes]
}
```

The index is built by iterating all notes and mapping each outgoing link to its source. This powers the backlinks panel and the graph view.

---

## Plugin System

### Directory Structure

```
~/.config/granit/plugins/          # Global plugins
    my-plugin/
        plugin.json                # Manifest
        run.sh                     # Script(s)

<vault>/.granit/plugins/           # Vault-local plugins
    vault-plugin/
        plugin.json
        process.py
```

### Manifest Format (plugin.json)

```json
{
  "name": "My Plugin",
  "description": "Does something useful",
  "version": "1.0.0",
  "author": "Author Name",
  "enabled": true,
  "commands": [
    {
      "label": "Run My Plugin",
      "description": "Processes the current note",
      "run": "python3 process.py"
    }
  ],
  "hooks": {
    "on_save": "python3 on_save.py",
    "on_open": "",
    "on_create": "",
    "on_delete": ""
  }
}
```

### Hooks

| Hook | Triggered When |
|------|---------------|
| `on_save` | A note is saved |
| `on_open` | A note is opened |
| `on_create` | A new note is created |
| `on_delete` | A note is deleted |

### Execution Environment

Plugins receive context via environment variables:

| Variable | Description |
|----------|-------------|
| `GRANIT_NOTE_PATH` | Absolute path to the current note |
| `GRANIT_NOTE_NAME` | Note filename without extension |
| `GRANIT_VAULT_PATH` | Absolute path to the vault root |

The current note's content is passed via **stdin**.

### Output Protocol

Plugin output is parsed line by line:

| Prefix | Action |
|--------|--------|
| `MSG:text` | Display `text` as a status message |
| `CONTENT:base64` | Replace the editor content with the base64-decoded text |
| `INSERT:base64` | Insert the base64-decoded text at the cursor position |

### Execution Limits

- **Timeout:** 10 seconds (via `context.WithTimeout`)
- **Isolation:** Plugins run as child processes; they cannot directly access Granit's memory
- **Language agnostic:** Any executable works (bash, Python, Ruby, Go, etc.)

---

## AI Provider Abstraction

### Provider Architecture

All AI features route through the centralized `AIConfig.Chat(systemPrompt, userPrompt)` method in `aiconfig.go`:

```go
func (c AIConfig) Chat(systemPrompt, userPrompt string) (string, error) {
    switch c.Provider {
    case "openai":
        return c.chatOpenAI(systemPrompt, userPrompt)
    case "nous":
        return c.NewNous().Chat(prompt)
    case "nerve":
        return c.NewNerve().Chat(systemPrompt, userPrompt, 120*time.Second)
    default: // "ollama", "local"
        return c.chatOllama(systemPrompt, userPrompt)
    }
}
```

Shared HTTP clients (`aiHTTPClient`, `ghostHTTPClient`) provide connection pooling across all AI calls.

### Ollama HTTP Protocol

```
POST {ollama_url}/api/chat
Content-Type: application/json

{
  "model": "qwen2.5:1.5b",
  "messages": [
    {"role": "system", "content": "..."},
    {"role": "user", "content": "..."}
  ],
  "stream": false
}

Response:
{
  "message": {"content": "..."}
}
```

### OpenAI HTTP Protocol

```
POST https://api.openai.com/v1/chat/completions
Authorization: Bearer sk-...
Content-Type: application/json

{
  "model": "gpt-4o-mini",
  "messages": [
    {"role": "system", "content": "..."},
    {"role": "user", "content": "..."}
  ]
}

Response:
{
  "choices": [
    {"message": {"content": "..."}}
  ]
}
```

### Local Fallback

The local provider uses Go-native algorithms:

- **Keyword extraction:** Tokenize → remove stopwords → count frequencies → return top N
- **Topic detection:** Pattern matching against predefined topic lists
- **Summarization:** Extract sentences with highest keyword density
- **Similarity:** TF-IDF vectors with cosine similarity scoring
- **Tagging:** Map keywords to tag suggestions using a topic-tag table

### Async Pattern

All AI calls are wrapped in `tea.Cmd` for async execution:

```go
func queryOllamaCmd(prompt, url, model string, kind botKind) tea.Cmd {
    return func() tea.Msg {
        body := ollamaRequest{Model: model, Prompt: prompt, Stream: false}
        resp, err := http.Post(url+"/api/generate", "application/json", marshal(body))
        if err != nil {
            return ollamaResultMsg{err: err, botKind: kind}
        }
        var result ollamaResponse
        json.NewDecoder(resp.Body).Decode(&result)
        return ollamaResultMsg{response: result.Response, botKind: kind}
    }
}
```

### Model Lifecycle

- **On startup:** No model is loaded (on-demand loading)
- **First AI call:** Ollama loads the model into memory
- **During session:** Model stays in memory for fast responses
- **On exit:** Granit calls `ollama stop <model>` to free memory

---

## Theme System

### Theme Structure

Each theme defines 16 color roles and 5 style properties:

```go
type Theme struct {
    Name string

    // Accent colors (7)
    Primary   lipgloss.Color  // Main accent: headings, borders, selection
    Secondary lipgloss.Color  // Links, H2 headings
    Accent    lipgloss.Color  // Selection highlight, line numbers
    Warning   lipgloss.Color  // Yellow accents
    Success   lipgloss.Color  // Green: checkmarks, success states
    Error     lipgloss.Color  // Red: errors, deletions
    Info      lipgloss.Color  // Blue/cyan: info, hints

    // Text hierarchy (3)
    Text    lipgloss.Color  // Main text color
    Subtext lipgloss.Color  // Secondary text
    Dim     lipgloss.Color  // Dimmed text, comments

    // Surface hierarchy (6)
    Surface2 lipgloss.Color  // Line numbers
    Surface1 lipgloss.Color  // Unfocused borders
    Surface0 lipgloss.Color  // Code backgrounds, highlights
    Base     lipgloss.Color  // Main background
    Mantle   lipgloss.Color  // Status bar background
    Crust    lipgloss.Color  // Help bar background

    // Style properties (5)
    Border        string  // "rounded", "double", "thick", "normal", "hidden"
    Density       string  // "compact", "normal", "spacious"
    AccentBar     string  // Sidebar selection indicator character
    Separator     string  // Horizontal separator character
    LinkUnderline bool    // Whether links are underlined
}
```

### Color Roles Explained

| Role | Purpose | Example Usage |
|------|---------|---------------|
| `Primary` | Main accent color | H1 headings, focused borders, selection bars |
| `Secondary` | Secondary accent | Links, H2 headings, wikilinks |
| `Accent` | Warm accent | Active line numbers, peach highlights |
| `Warning` | Caution color | Yellow markers, warnings |
| `Success` | Positive state | Completed checkboxes, success messages |
| `Error` | Negative state | Errors, deletions, red markers |
| `Info` | Informational | H3 headings, info callouts |
| `Text` | Primary text | Body text, editor content |
| `Subtext` | Secondary text | Subtitles, descriptions |
| `Dim` | Tertiary text | Comments, hints, disabled items |
| `Surface2` | Lightest surface | Line number column |
| `Surface1` | Mid surface | Unfocused panel borders |
| `Surface0` | Darkest surface | Code block backgrounds |
| `Base` | Main background | Editor, sidebar, panels |
| `Mantle` | Status area bg | Status bar |
| `Crust` | Help area bg | Help bar, tooltips |

### Runtime Theme Switching

`ApplyTheme(name)` performs a complete runtime theme switch:

1. Looks up the theme (custom themes take priority over built-in)
2. Updates all package-level color variables (`mauve`, `blue`, `peach`, etc.)
3. Updates style properties (`ThemeBorder`, `ThemeDensity`, etc.)
4. Rebuilds every Lip Gloss style variable (`SidebarStyle`, `EditorStyle`, etc.)

This is instantaneous — no restart required.

### Custom Themes

Custom themes are stored as JSON in `~/.config/granit/themes/`:

```json
{
  "name": "my-theme",
  "primary": "#FF6B6B",
  "secondary": "#4ECDC4",
  "accent": "#FFE66D",
  "warning": "#F7DC6F",
  "success": "#27AE60",
  "error": "#E74C3C",
  "info": "#3498DB",
  "text": "#ECF0F1",
  "subtext": "#BDC3C7",
  "dim": "#7F8C8D",
  "surface2": "#4A4A4A",
  "surface1": "#3A3A3A",
  "surface0": "#2A2A2A",
  "base": "#1A1A1A",
  "mantle": "#141414",
  "crust": "#0E0E0E",
  "border": "rounded",
  "density": "normal",
  "accent_bar": "┃",
  "separator": "─",
  "link_underline": true
}
```

Custom themes override built-in themes with the same name.

---

## Layout System

### Layout Definitions

13 layouts define different panel arrangements, selectable via `Alt+L` picker with ASCII previews:

| Layout | Panels | Description |
|--------|--------|-------------|
| `default` | 3 | Sidebar + Editor + Backlinks |
| `writer` | 2 | Sidebar + Editor |
| `reading` | 2 | Editor + Backlinks (no sidebar) |
| `dashboard` | 4 | Sidebar + Editor + Outline + Backlinks |
| `zen` | 1 | Centered editor, no chrome |
| `cockpit` | 4 | Sidebar + Editor + Calendar & Tasks |
| `stacked` | 4 | Sidebar + Editor + bottom panels (IDE-like) |
| `cornell` | 2 | Editor + Notes panel (study layout) |
| `focus` | 2 | Sidebar + wide centered editor |
| `preview` | 2 | Editor + live markdown preview |
| `presenter` | 1 | Full-screen rendered markdown |
| `kanban` | 3 | Sidebar + Editor + mini Kanban board |
| `widescreen` | 5 | Sidebar + Outline + Editor + Backlinks + Calendar |

### Layout Helpers

```go
LayoutHasSidebar(layout string) bool    // Does the layout show the sidebar?
LayoutHasBacklinks(layout string) bool  // Does the layout show backlinks?
LayoutHasOutline(layout string) bool    // Does the layout show the outline?
LayoutPanelCount(layout string) int     // How many panels?
LayoutDescription(layout string) string // Human-readable description
```

### Adaptive Degradation

Granit automatically adjusts the layout based on terminal width:

| Terminal Width | Behavior |
|---------------|----------|
| < 80 columns | Forces Minimal layout (editor only) |
| 80-119 columns | Multi-panel layouts downgraded to Writer or Minimal |
| 120-159 columns | 4+ panel layouts (dashboard, cockpit, stacked, widescreen) downgraded to Default |
| 160+ columns | Uses configured layout as-is |

This ensures usability on any terminal size, including mobile terminals.

### Panel Width Allocation

In the main `View` method, available width is allocated proportionally:

- **Sidebar:** ~25% of width (minimum 20 columns)
- **Editor:** Remaining width after sidebar and backlinks
- **Backlinks:** ~25% of width (minimum 20 columns)
- **Outline:** ~15% of width (in Dashboard layout only)
