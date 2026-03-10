# Granit Roadmap

## Current Status: v0.x — Active Development

Granit is under active development. Contributions are welcome for any item below.

---

## Near-Term

### Code Quality
- [ ] Split `app.go` into focused modules (`app_update.go`, `app_view.go`, `app_commands.go`, `app_helpers.go`)
- [ ] Introduce overlay registry pattern to replace manual if-chains
- [ ] Add `golangci-lint` to CI pipeline
- [ ] Increase test coverage for core update loop
- [ ] Centralize message types into `messages.go`

### Platform Support
- [ ] Windows support (terminal compatibility, path handling)
- [ ] FreeBSD / OpenBSD testing

### Distribution
- [ ] Homebrew formula
- [ ] Nix package
- [ ] Snap / Flatpak
- [ ] Docker image for `granit serve`

---

## Mid-Term

### Features
- [ ] Plugin marketplace / registry with `granit plugin search`
- [ ] Dataview query language improvements (aggregates, rollups)
- [ ] Inline math/LaTeX rendering (terminal-compatible)
- [ ] Recurring task engine with automatic scheduling
- [ ] Boolean search operators (AND / OR / NOT)
- [ ] Regex file search in sidebar
- [ ] Bulk file operations (rename, move, tag patterns)

### Import / Export
- [ ] Notion import
- [ ] Evernote import (ENEX format)
- [ ] Logseq graph import improvements
- [ ] Export to Obsidian-compatible vault

### AI
- [ ] Streaming AI responses (token-by-token display)
- [ ] Additional local model support (llama.cpp direct)
- [ ] AI-powered graph clustering
- [ ] Context-aware completions using vault knowledge

---

## Long-Term

### Collaboration
- [ ] Shared vault editing via CRDT / operational transform
- [ ] Real-time multi-user cursors
- [ ] Comments and annotations

### Mobile
- [ ] Companion mobile app (read/capture)
- [ ] SSH-based remote vault access

### Performance
- [ ] Persistent on-disk search index
- [ ] Lazy overlay initialization (only when first opened)
- [ ] Virtual scrolling for 10k+ note vaults

### Ecosystem
- [ ] Public plugin API documentation
- [ ] Theme marketplace
- [ ] Community template library
- [ ] Language Server Protocol for external editor integration

---

## Contributing

Pick any unchecked item and open a PR! See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup. Issues labeled `good first issue` are great starting points.
