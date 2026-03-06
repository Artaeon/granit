---
title: Architecture
tags: [docs, technical]
created: 2024-01-01
---

# Architecture

Granit is built with Go and the Bubble Tea TUI framework.

## Components

### Vault Engine
The core engine scans Markdown files, parses [[Features|wikilinks]] and YAML frontmatter, and builds an in-memory index.

### TUI Layer
Built with Bubble Tea (Charm.sh), the interface consists of:
- **Sidebar** — File tree with fuzzy search
- **Editor** — Markdown editor with highlighting
- **Backlinks** — Incoming and outgoing link panel
- **Status Bar** — Mode, active note, vault info

### Daily Notes
A standalone module for creating and managing daily notes.

## File Format

All notes are plain Markdown with optional YAML frontmatter:

```yaml
---
title: Note Title
tags: [tag1, tag2]
created: 2024-01-01
---
```

Links use Obsidian's wikilink syntax: `[[Note Name]]` or `[[Note Name|Display Text]]`.

See [[Welcome]] to get started.
Related: [[Features]], [[Keyboard Shortcuts]]
