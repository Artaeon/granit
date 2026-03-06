# Granit

A fast, terminal-based knowledge manager — fully compatible with Obsidian vaults.

```
   ██████╗ ██████╗  █████╗ ███╗   ██╗██╗████████╗
  ██╔════╝ ██╔══██╗██╔══██╗████╗  ██║██║╚══██╔══╝
  ██║  ███╗██████╔╝███████║██╔██╗ ██║██║   ██║
  ██║   ██║██╔══██╗██╔══██║██║╚██╗██║██║   ██║
  ╚██████╔╝██║  ██║██║  ██║██║ ╚████║██║   ██║
   ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝╚═╝   ╚═╝
```

## Features

- **Vault-compatible** — Opens any Obsidian vault without migration
- **[[WikiLinks]]** — Full wikilink parsing and navigation
- **YAML Frontmatter** — Reads and preserves frontmatter metadata
- **Backlinks Panel** — See incoming and outgoing links at a glance
- **Fuzzy Search** — Fast, fzf-like file navigation
- **Markdown Editor** — Integrated editor with syntax highlighting
- **Daily Notes** — Quick daily note creation with `granit daily`
- **Cross-Platform** — Linux, macOS, Windows

## Install

```bash
git clone https://github.com/artaeon/granit.git
cd granit
make build
```

## Usage

```bash
# Open a vault
granit open ~/my-vault

# Or just pass the path
granit ~/my-vault

# Scan a vault (print stats)
granit scan ~/my-vault

# Open/create today's daily note
granit daily ~/my-vault
```

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Ctrl+1` | Focus file sidebar |
| `Ctrl+2` | Focus editor |
| `Ctrl+3` | Focus backlinks panel |
| `Ctrl+S` | Save current note |
| `Ctrl+Q` | Quit |
| `Tab` | Toggle backlinks/outgoing |
| `j`/`k` | Navigate up/down |
| `Enter` | Open selected |
| Type | Fuzzy search in sidebar |

## Architecture

```
granit/
├── cmd/granit/          # CLI entry point
├── internal/
│   ├── vault/           # Vault engine (scan, parse, index)
│   ├── tui/             # Bubble Tea TUI components
│   └── daily/           # Daily notes
├── go.mod
├── Makefile
└── README.md
```

## Tech Stack

- **Go** with [Bubble Tea](https://github.com/charmbracelet/bubbletea) (Charm.sh)
- **Lip Gloss** for styling
- Local Markdown files (no proprietary database)

## License

MIT
