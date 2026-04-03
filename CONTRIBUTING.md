# Contributing to Granit

Welcome, and thank you for considering a contribution to **Granit** -- a
terminal-based knowledge manager written in Go with
[Bubble Tea](https://github.com/charmbracelet/bubbletea). Whether you are
fixing a typo, reporting a bug, proposing a new theme, writing a plugin, or
building a major feature, all contributions are welcome and appreciated.

This guide will help you get oriented quickly.

---

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [How to Contribute](#how-to-contribute)
- [Code Style](#code-style)
- [Pull Request Process](#pull-request-process)
- [Architecture Quick Reference](#architecture-quick-reference)
- [Testing](#testing)
- [License](#license)

---

## Code of Conduct

This project is maintained by a small team and a growing community of
contributors. We ask that everyone:

- **Be respectful.** Treat others the way you would want to be treated.
  Disagreements are fine; personal attacks are not.
- **Be constructive.** When reviewing code or discussing ideas, focus on the
  work, not the person. Offer suggestions, not just criticism.
- **Be inclusive.** We welcome contributors of all experience levels and
  backgrounds. If someone is new, help them learn rather than dismissing their
  effort.
- **Assume good intent.** Most misunderstandings are exactly that --
  misunderstandings. Ask for clarification before assuming the worst.

Unacceptable behavior (harassment, trolling, spam) will result in removal from
the project. If you experience or witness such behavior, please reach out to
the maintainers directly.

---

## Getting Started

1. **Fork** the repository on GitHub:
   [github.com/artaeon/granit](https://github.com/artaeon/granit)

2. **Clone** your fork:

   ```bash
   git clone https://github.com/<your-username>/granit.git
   cd granit
   ```

3. **Build** the project:

   ```bash
   go build ./cmd/granit/
   ```

4. **Verify** that everything passes:

   ```bash
   go vet ./...
   go test ./...
   ```

5. **Run** Granit:

   ```bash
   ./granit ~/your-vault      # open a specific vault
   ./granit                   # open the vault selector
   ```

If your Go toolchain lives at a custom path (e.g. `~/go-sdk/go/bin/go`),
substitute that for `go` in the commands above.

---

## Development Setup

### Prerequisites

- **Go 1.23** or later
- **Git**
- A terminal emulator (any modern terminal will do)

### Project Structure

```
cmd/granit/
  main.go                    CLI entry point (open, scan, daily, version, help)

internal/config/
  config.go                  JSON config (global ~/.config/granit/ + per-vault .granit.json)
  vaults.go                  Vault list persistence
  import.go                  Obsidian config importer

internal/vault/
  vault.go                   Vault scanning and note storage
  parser.go                  Markdown, frontmatter, and wikilink parser
  index.go                   Backlink and link index

internal/tui/
  app.go                     Main Bubble Tea model (~2150 lines)
  editor.go                  Text editor with multi-cursor support (~1240 lines)
  renderer.go                Markdown rendering (view mode)
  sidebar.go                 File tree sidebar with fuzzy search
  filetree.go                Collapsible folder hierarchy
  styles.go                  Package-level color vars and style definitions
  themes.go                  Theme structs (38 built-in themes)
  command.go                 Command palette (Ctrl+X)
  settings.go                Settings overlay (Ctrl+,)
  bots.go                    AI bots: Ollama + OpenAI + local fallback
  plugins.go                 Plugin system and manager overlay
  vim.go                     Vim modal editing
  ...                        One file per overlay/feature
```

All TUI components live in `internal/tui/` and follow the Bubble Tea
`Model` / `Update` / `View` pattern.

### Key Directories

| Directory          | Purpose                                      |
| ------------------ | -------------------------------------------- |
| `cmd/granit/`      | CLI entry point and subcommands              |
| `internal/config/` | Configuration loading, saving, and importing |
| `internal/vault/`  | Vault scanning, parsing, and indexing        |
| `internal/tui/`    | All TUI components, overlays, and themes     |

---

## How to Contribute

### Bug Reports

Found a bug? [Open an issue](https://github.com/artaeon/granit/issues/new?template=bug_report.md) with:

- **Steps to reproduce** -- Numbered steps someone else can follow to trigger
  the issue.
- **Expected behavior** -- What you expected to happen.
- **Actual behavior** -- What actually happened (include error messages or
  garbled output if applicable).
- **Environment** -- OS, terminal emulator, Granit version (`granit version`),
  and Go version (`go version`).
- **Screenshots** -- If the issue is visual, a screenshot or recording helps
  enormously.

### Feature Requests

Have an idea? [Open a feature request](https://github.com/artaeon/granit/issues/new?template=feature_request.md).
Focus on the **use case** rather than just the feature itself. Explain:

- What problem are you trying to solve?
- How do you currently work around it (if at all)?
- Why would this benefit other Granit users?

A well-explained use case helps maintainers evaluate and prioritize the
request, even if the final implementation looks different from what you
originally proposed.

### Code Contributions

1. **Fork** the repository and create a feature branch from `main`:

   ```bash
   git checkout -b my-feature
   ```

2. **Make your changes** in focused, logical commits.
3. **Test** your changes (see [Testing](#testing)).
4. **Push** to your fork:

   ```bash
   git push origin my-feature
   ```

5. **Open a pull request** against `main`.

### Documentation Improvements

Documentation fixes -- typos, clarifications, better examples -- are always
welcome. No issue required; just open a PR.

### Theme Contributions

Themes are defined in `internal/tui/themes.go` as `Theme` structs in the
`builtinThemes` map. Each theme has **16 color roles**:

| Role        | Purpose                                        |
| ----------- | ---------------------------------------------- |
| `Primary`   | Primary accent color (selections, highlights)  |
| `Secondary` | Secondary accent                               |
| `Accent`    | Tertiary accent for emphasis                   |
| `Warning`   | Warning indicators                             |
| `Success`   | Success indicators                             |
| `Error`     | Error indicators                               |
| `Info`      | Informational highlights                       |
| `Text`      | Main text color                                |
| `Subtext`   | Dimmer text (descriptions, hints)              |
| `Dim`       | Even dimmer text (disabled items, line numbers) |
| `Surface2`  | Lightest surface (active borders)              |
| `Surface1`  | Mid surface (borders, separators)              |
| `Surface0`  | Dark surface (inactive panels)                 |
| `Base`      | Main background                                |
| `Mantle`    | Slightly darker background (sidebars)          |
| `Crust`     | Darkest background (status bar)                |

To add a theme:

1. Add an entry to `builtinThemes` in `themes.go` with all 16 color roles
   filled in. Use existing themes as a reference.
2. The theme will automatically appear in the settings panel. No other wiring
   is required.
3. Test your theme across multiple overlays (editor, sidebar, command palette,
   settings, graph view) to ensure readability and contrast.

### Plugin Contributions

Granit supports external plugins. Plugins live in:

- `~/.config/granit/plugins/<name>/` (global)
- `<vault>/.granit/plugins/<name>/` (per-vault)

Each plugin has a `plugin.json` manifest defining its name, description,
version, commands, and hooks (`on_save`, `on_open`, `on_create`, `on_delete`).
Scripts receive context via environment variables (`GRANIT_NOTE_PATH`,
`GRANIT_NOTE_NAME`, `GRANIT_VAULT_PATH`) and note content via stdin.

See the plugin system documentation or existing plugins for examples.

---

## Code Style

### Follow Existing Patterns

Granit has a consistent internal style. Before writing new code, read a few
existing files in `internal/tui/` to absorb the patterns.

### Receiver Rules for Overlays

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

- **No hardcoded colors outside `styles.go` and `themes.go`.** All color
  values must come from the package-level style variables (`mauve`, `peach`,
  `green`, `blue`, `surface1`, `base`, etc.) defined in `styles.go`.
- These variables are overwritten at runtime by `ApplyTheme()` when the user
  switches themes. Hardcoding hex values will break theme support.
- The default theme is **Catppuccin Mocha**.

### UI Icons

Use ASCII-only characters for the default icon set. The project provides
multiple icon sets (Unicode, Nerd Font, Emoji, ASCII) and the default must
remain legible in any terminal.

### Dependencies

Keep third-party dependencies minimal. The current set is:

- **Bubble Tea** -- TUI framework
- **Lip Gloss** -- styling
- **GopherLua** -- Lua scripting engine

Do not add new dependencies without discussion in an issue first.

### Commits

- Make **individual commits per logical change**. Each commit should be a
  self-contained, coherent unit.
- Use a short imperative summary (under ~72 characters) starting with a
  capitalized verb:

  ```
  Add toast notification system for ephemeral messages
  Fix kanban board column alignment and divider rendering
  Polish sidebar with accent bar selection and styled search
  ```

- Do not use conventional-commit prefixes (`feat:`, `fix:`, etc.) -- the
  project does not use them.

### Before Submitting

Always run these commands and ensure they pass:

```bash
go vet ./...
go test ./...
go build ./cmd/granit/
```

---

## Pull Request Process

1. **Fork** the repository and create a feature branch from `main`.
2. Make your changes in focused, logical commits.
3. Confirm that `go build ./...`, `go vet ./...`, and `go test ./...` all
   pass.
4. Open a pull request against `main` with:
   - **A clear title and description** -- explain what the change does and
     why.
   - **A link to the related issue**, if one exists.
   - **Screenshots or GIFs** for any UI changes. Visual changes are much
     easier to review with a before/after comparison.
   - **A note about new dependencies or configuration options**, if any were
     introduced.
5. Ensure CI passes (build + vet + test).
6. Be responsive to review feedback. Small, focused PRs are easier to review
   and merge.

---

## Architecture Quick Reference

### Overlay System

Overlays are modal panels rendered on top of the main editor. They share a
consistent pattern:

- A struct with `active bool`, `width int`, `height int` fields.
- `Open()` / `Close()` / `IsActive()` / `SetSize()` on pointer receivers.
- `Update()` / `View()` on value receivers.
- Wired into `app.go`: dispatched in `Update` when active, rendered in `View`
  on top of the main layout, and triggered via keybindings or the command
  palette.

Overlays are checked in priority order in both `Update` and `View` -- the
first active overlay wins input focus.

### Config Layering

Configuration is layered:

1. **Global config** -- `~/.config/granit/config.json` (user-wide defaults).
2. **Per-vault config** -- `<vault>/.granit.json` (vault-specific overrides).

Per-vault settings take precedence over global settings.

### AI Provider Abstraction

The AI/bots system supports three backends:

- **Ollama** -- Local LLM via HTTP (`/api/generate`).
- **OpenAI** -- Remote API via HTTP (`/v1/chat/completions`).
- **Local fallback** -- Keyword analysis, stopword filtering, pattern matching
  (no external dependencies).

The active provider is set via the `AIProvider` config key (`"local"`,
`"ollama"`, or `"openai"`). All AI calls are dispatched asynchronously via
`tea.Cmd` to keep the TUI responsive.

### Three Layouts

Granit adapts its layout based on terminal width:

- **Default** (width >= 120) -- 3-panel: sidebar + editor + backlinks.
- **Writer** (width 80--119) -- 2-panel: sidebar + editor.
- **Minimal** (width < 80) -- Editor only.

---

## Testing

### Running Tests

```bash
go test ./...          # run all tests
go test ./internal/vault/  # run tests for a specific package
go vet ./...           # static analysis
```

### Where Test Files Go

Test files live alongside the code they cover, following Go convention:

- `internal/vault/vault_test.go`
- `internal/vault/parser_test.go`
- `internal/config/config_test.go`

### Testing Conventions

- When adding new logic to the `vault` or `config` packages, add
  corresponding test functions.
- TUI overlay code is harder to unit-test, but at minimum ensure that:
  - `go build ./...` succeeds with no errors.
  - `go vet ./...` reports no issues.
  - Manual smoke-testing covers the happy path of your feature.
- Test function names should be descriptive:
  `TestParseWikiLink`, `TestFrontmatterExtraction`, etc.

---

## License

Granit is licensed under the [MIT License](LICENSE). By submitting a
contribution (code, documentation, themes, plugins, or otherwise), you agree
that your contribution will be licensed under the same MIT License that covers
the project.

---

Thank you for helping make Granit better.
