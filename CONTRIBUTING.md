# Contributing to Granit

Welcome, and thank you for considering a contribution to Granit. Granit is a
free, open-source terminal knowledge manager written in Go. It uses the Charm
ecosystem (Bubble Tea, Lip Gloss) for its TUI and aims to provide a fast,
keyboard-driven alternative to Obsidian -- fully compatible with standard
Markdown vaults.

This guide covers everything you need to get started.

---

## Development Setup

### Prerequisites

- Go 1.23 or later
- Git

### Clone and Build

```bash
git clone https://github.com/artaeon/granit.git
cd granit
go build -o granit ./cmd/granit
```

If your Go toolchain lives at a custom path (e.g. `~/go-sdk/go/bin/go`),
substitute that for `go` in the commands above.

### Run

```bash
./granit ~/your-vault      # open a specific vault
./granit                   # open the vault selector
```

### Verify

```bash
go vet ./...
go test ./...
```

Both commands must pass before you submit a pull request.

---

## Project Structure

```
cmd/granit/
  main.go                  CLI entry point, vault selector

internal/config/
  config.go                JSON config (global + per-vault)
  vaults.go                Vault list persistence
  import.go                Obsidian config importer

internal/vault/
  vault.go                 Vault scanning and note storage
  parser.go                Markdown, frontmatter, and wikilink parser
  index.go                 Backlink and link index

internal/tui/
  app.go                   Main Bubble Tea model
  editor.go                Text editor with multi-cursor
  renderer.go              Markdown rendering (view mode)
  sidebar.go               File tree sidebar
  styles.go                Package-level color vars and style definitions
  themes.go                Theme structs (28 built-in themes)
  command.go               Command palette
  vim.go                   Vim modal editing
  ...                      One file per overlay/feature
```

All TUI components live in `internal/tui/` and follow the Bubble Tea
`Model` / `Update` / `View` pattern.

---

## Code Conventions

### Receiver Rules for Overlays

Overlays (tag browser, graph view, git overlay, etc.) follow a consistent
receiver convention:

- **Value receivers** for `Update` and `View` -- these return a new copy of
  the overlay struct, matching the Bubble Tea pattern.
- **Pointer receivers** for helpers -- `Open`, `Close`, `SetSize`, `IsActive`,
  and any internal methods that mutate state.

```go
// Value receivers -- Bubble Tea interface
func (t TagBrowser) Update(msg tea.Msg) (TagBrowser, tea.Cmd) { ... }
func (t TagBrowser) View() string { ... }

// Pointer receivers -- helpers
func (t *TagBrowser) Open()                     { ... }
func (t *TagBrowser) Close()                    { ... }
func (t *TagBrowser) SetSize(w, h int)          { ... }
func (t *TagBrowser) IsActive() bool            { ... }
```

### Styling and Colors

- The default theme is **Catppuccin Mocha**.
- Package-level color variables live in `internal/tui/styles.go` (e.g. `mauve`,
  `blue`, `surface1`, `base`). These are overwritten by `ApplyTheme()` when the
  user switches themes.
- Use these variables instead of hard-coding hex values so that every theme
  works correctly.

### Dependencies

Keep third-party dependencies minimal. The current set is:

- **Bubble Tea** -- TUI framework
- **Lip Gloss** -- styling
- **GopherLua** -- Lua scripting engine

Do not add new dependencies without discussion in an issue first.

### UI Icons -- ASCII Only

Use ASCII-only characters for icons and decorators in UI components. Avoid
Unicode symbols (arrows, bullets, box-drawing characters beyond basic borders)
that break or render inconsistently across terminal emulators. The project
provides multiple icon sets (Unicode, Nerd Font, Emoji, ASCII) and the default
must remain legible everywhere.

---

## Adding a New Overlay

Overlays are modal panels that draw on top of the main editor. Every overlay
follows the same pattern. To add one:

1. Create a new file in `internal/tui/`, e.g. `myoverlay.go`.

2. Define a struct with at least an `active` flag and dimensions:

```go
type MyOverlay struct {
    active bool
    width  int
    height int
}
```

3. Implement the standard methods:

```go
func (o *MyOverlay) IsActive() bool              { return o.active }
func (o *MyOverlay) Open()                        { o.active = true }
func (o *MyOverlay) Close()                       { o.active = false }
func (o *MyOverlay) SetSize(w, h int)             { o.width = w; o.height = h }

func (o MyOverlay) Update(msg tea.Msg) (MyOverlay, tea.Cmd) {
    // Handle keys, return updated overlay
    return o, nil
}

func (o MyOverlay) View() string {
    // Return rendered string
    return ""
}
```

4. Add a field for the overlay in the main `App` struct in `app.go`.

5. Wire it up:
   - Call `SetSize` in the app's resize handler.
   - Dispatch to its `Update` when `IsActive()` is true.
   - Render its `View` on top of the main layout when active.
   - Add a keybinding or command palette entry to `Open` it.

---

## Adding a New Theme

Themes are defined in `internal/tui/themes.go` as `Theme` structs in the
`builtinThemes` map.

1. Add an entry to `builtinThemes`:

```go
"my-theme": {
    Name:      "my-theme",
    Primary:   lipgloss.Color("#..."),
    Secondary: lipgloss.Color("#..."),
    Accent:    lipgloss.Color("#..."),
    Warning:   lipgloss.Color("#..."),
    Success:   lipgloss.Color("#..."),
    Error:     lipgloss.Color("#..."),
    Info:      lipgloss.Color("#..."),
    Text:      lipgloss.Color("#..."),
    Subtext:   lipgloss.Color("#..."),
    Dim:       lipgloss.Color("#..."),
    Surface2:  lipgloss.Color("#..."),
    Surface1:  lipgloss.Color("#..."),
    Surface0:  lipgloss.Color("#..."),
    Base:      lipgloss.Color("#..."),
    Mantle:    lipgloss.Color("#..."),
    Crust:     lipgloss.Color("#..."),
},
```

2. Fill in every color role. Use the existing themes as a reference for what
   each role is used for (see the comments on the `Theme` struct).

3. The theme will automatically appear in the settings panel. No other wiring
   is required.

---

## Testing

Run all tests:

```bash
go test ./...
```

Run the linter:

```bash
go vet ./...
```

Existing tests live alongside the code they cover (e.g.
`internal/vault/vault_test.go`, `internal/vault/parser_test.go`). When adding
new logic to the `vault` or `config` packages, add corresponding test
functions.

TUI overlay code is harder to unit-test, but at minimum ensure that:

- `go build ./...` succeeds with no errors.
- `go vet ./...` reports no issues.
- Manual smoke-testing covers the happy path of your feature.

---

## Commit Messages

Follow the style used in the project's history. Commits use a short imperative
summary (under ~72 characters) that starts with a verb:

```
Add toast notification system for ephemeral messages
Fix kanban board column alignment and divider rendering
Polish sidebar with accent bar selection and styled search
Overhaul graph view with hub/orphan detection and stats
```

- Start with a capitalized verb: Add, Fix, Polish, Overhaul, Rewrite, Remove.
- Keep the first line concise. Add a blank line and further detail in the body
  if the change is complex.
- Do not use conventional-commit prefixes (feat:, fix:, etc.) -- the project
  does not use them.

---

## Pull Request Process

1. Fork the repository and create a feature branch from `main`.
2. Make your changes in focused, logical commits.
3. Confirm that `go build ./...`, `go vet ./...`, and `go test ./...` all pass.
4. Open a pull request against `main` with a clear title and description:
   - What the change does and why.
   - Screenshots or GIFs if the change is visual.
   - Any new dependencies or configuration options introduced.
5. Be responsive to review feedback. Small, focused PRs are easier to review
   and merge.

---

## Reporting Issues

Found a bug or have a feature idea?
[Open an issue](https://github.com/artaeon/granit/issues) with:

- **Bug reports**: Steps to reproduce, expected vs. actual behavior, terminal
  emulator and OS, and the output of `granit version`.
- **Feature requests**: A clear description of the proposed feature and why it
  would be useful.

---

Thank you for helping make Granit better.
