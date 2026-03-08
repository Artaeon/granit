# Granit — Installation Guide

> How to install, build, update, and uninstall Granit on Linux and macOS.

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Install (Recommended)](#quick-install-recommended)
- [System-Wide Install](#system-wide-install)
- [Go Install (Remote)](#go-install-remote)
- [Arch Linux (AUR)](#arch-linux-aur)
- [Building from Source with Custom Flags](#building-from-source-with-custom-flags)
- [Cross-Compilation](#cross-compilation)
- [Optional Dependencies](#optional-dependencies)
- [Verifying Installation](#verifying-installation)
- [Updating](#updating)
- [Uninstalling](#uninstalling)

---

## Prerequisites

| Requirement | Version | Notes |
|-------------|---------|-------|
| **Go** | 1.23+ | [Install Go](https://go.dev/doc/install) |
| **Git** | Any recent version | For cloning and git features |
| **OS** | Linux or macOS | Windows support planned |

Verify your Go installation:

```bash
go version
# Expected: go version go1.23.x linux/amd64 (or similar)
```

If Go is not in your PATH, you may need to add it:

```bash
export PATH="/usr/local/go/bin:$PATH"
```

---

## Quick Install (Recommended)

Clone the repository and install with `go install`:

```bash
git clone https://github.com/artaeon/granit.git
cd granit
go install ./cmd/granit/
```

This installs the `granit` binary to `~/go/bin/`. Ensure this directory is in your PATH:

```bash
# Add to ~/.bashrc or ~/.zshrc (one-time setup):
export PATH="$HOME/go/bin:$PATH"

# Then reload:
source ~/.bashrc  # or source ~/.zshrc
```

---

## System-Wide Install

Build and install to `/usr/local/bin/` for all users:

```bash
git clone https://github.com/artaeon/granit.git
cd granit
go build -ldflags="-s -w" -o granit ./cmd/granit/
sudo mv granit /usr/local/bin/
```

Or use the included Makefile:

```bash
git clone https://github.com/artaeon/granit.git
cd granit
make build
sudo cp bin/granit /usr/local/bin/
```

---

## Go Install (Remote)

Install directly from the repository without cloning:

```bash
go install github.com/artaeon/granit/cmd/granit@latest
```

This downloads, builds, and installs the latest version to `~/go/bin/`.

---

## Arch Linux (AUR)

Two AUR packages are available:

### Stable Release

```bash
# Using an AUR helper (e.g., yay, paru):
yay -S granit

# Or manually:
git clone https://aur.archlinux.org/granit.git
cd granit
makepkg -si
```

### Development (Git)

Builds from the latest `main` branch:

```bash
yay -S granit-git
```

### PKGBUILD Details

The PKGBUILD uses static compilation with security-hardened flags:

```bash
CGO_ENABLED=0
GOFLAGS="-buildmode=pie -trimpath -mod=readonly -modcacherw"
```

Optional dependencies registered in the PKGBUILD:

- `ollama` — local AI provider
- `aspell` or `hunspell` — spell checking
- `pandoc` — PDF export
- `xclip` — system clipboard (X11)
- `wl-clipboard` — system clipboard (Wayland)
- `git` — version control features

---

## Building from Source with Custom Flags

### Optimized Release Build

```bash
git clone https://github.com/artaeon/granit.git
cd granit
go build -ldflags="-s -w" -o granit ./cmd/granit/
```

The `-s -w` flags strip debug symbols and DWARF information, reducing binary size by ~30%.

### Build with Version Information

Set version metadata at build time using ldflags:

```bash
go build -ldflags="-s -w \
  -X main.version=1.0.0 \
  -X main.commit=$(git rev-parse --short HEAD) \
  -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o granit ./cmd/granit/
```

This embeds version, commit hash, and build date into the binary, visible with `granit version`.

### Static Binary (No CGO)

For maximum portability:

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o granit ./cmd/granit/
```

### PIE Build (Position Independent Executable)

For security-hardened systems:

```bash
CGO_ENABLED=0 go build -buildmode=pie -trimpath \
  -ldflags="-s -w" -o granit ./cmd/granit/
```

### Using the Makefile

The included Makefile provides convenience targets:

```bash
make build          # Build to bin/granit with -s -w
make install        # Build and copy to ~/go/bin/
make run ARGS=~/notes  # Run without installing
make test           # Run all tests
make clean          # Remove build artifacts
make build-all      # Cross-compile for Linux, macOS, Windows
```

---

## Cross-Compilation

Build for multiple platforms from any OS:

```bash
# Linux (amd64)
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o granit-linux-amd64 ./cmd/granit/

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o granit-darwin-arm64 ./cmd/granit/

# macOS (Intel)
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o granit-darwin-amd64 ./cmd/granit/

# Windows (amd64)
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o granit-windows-amd64.exe ./cmd/granit/
```

Or use the Makefile:

```bash
make build-all
# Creates: bin/granit-linux-amd64, bin/granit-darwin-arm64, bin/granit-windows-amd64.exe
```

---

## Optional Dependencies

Granit works fully without any optional dependencies. These tools enhance specific features:

| Tool | Purpose | Install (Debian/Ubuntu) | Install (Arch) | Install (macOS) |
|------|---------|------------------------|-----------------|-----------------|
| **Ollama** | Local AI models | `curl -fsSL https://ollama.ai/install.sh \| sh` | `yay -S ollama` | `brew install ollama` |
| **aspell** | Spell checking | `sudo apt install aspell` | `sudo pacman -S aspell` | `brew install aspell` |
| **hunspell** | Spell checking (alt) | `sudo apt install hunspell` | `sudo pacman -S hunspell` | `brew install hunspell` |
| **pandoc** | PDF export | `sudo apt install pandoc` | `sudo pacman -S pandoc` | `brew install pandoc` |
| **xclip** | Clipboard (X11) | `sudo apt install xclip` | `sudo pacman -S xclip` | N/A |
| **xsel** | Clipboard (X11, alt) | `sudo apt install xsel` | `sudo pacman -S xsel` | N/A |
| **wl-clipboard** | Clipboard (Wayland) | `sudo apt install wl-clipboard` | `sudo pacman -S wl-clipboard` | N/A |
| **Claude Code** | Deep Dive Research | [Install Claude Code](https://docs.anthropic.com/en/docs/claude-code) | Same | Same |
| **Git** | Version control | `sudo apt install git` | `sudo pacman -S git` | `brew install git` |

### Feature Degradation

Without optional dependencies, Granit degrades gracefully:

- **No Ollama/OpenAI:** AI features use the local fallback (keyword-based analysis, no LLM required)
- **No aspell/hunspell:** Spell check is unavailable; all other editing features work normally
- **No pandoc:** PDF export is unavailable; HTML and text export still work
- **No xclip/xsel/wl-clipboard:** System clipboard paste (`Ctrl+V`) is unavailable; internal copy/paste still works
- **No Claude Code:** Deep Dive Research, Vault Analyzer, Note Enhancer, and Daily Digest are unavailable; all other AI features work
- **No Git:** Git overlay, auto-sync, note history, and standup generator git features are unavailable

---

## Verifying Installation

After installing, verify that Granit is working:

```bash
# Check the binary is found
which granit

# Check the version
granit version
# Expected: Granit v0.1.0 (abc1234, 2026-03-08)

# View help
granit help

# View the man page
granit man | man -l -

# View configuration
granit config

# Test with a vault
granit ~/your-notes-folder
```

### Quick Smoke Test

```bash
# Create a temporary test vault
mkdir /tmp/granit-test
echo "# Hello Granit" > /tmp/granit-test/test.md

# Open it
granit /tmp/granit-test

# Clean up
rm -rf /tmp/granit-test
```

---

## Updating

### From Git Clone

```bash
cd granit  # your clone directory
git pull
go install ./cmd/granit/
```

### From Remote

```bash
go install github.com/artaeon/granit/cmd/granit@latest
```

### Arch Linux (AUR)

```bash
yay -Syu granit    # or granit-git
```

---

## Uninstalling

### Remove the Binary

```bash
# If installed to ~/go/bin/
rm ~/go/bin/granit

# If installed system-wide
sudo rm /usr/local/bin/granit
```

### Remove Configuration (Optional)

```bash
# Remove global configuration
rm -rf ~/.config/granit/

# This removes:
#   ~/.config/granit/config.json    — global settings
#   ~/.config/granit/vaults.json    — vault registry
#   ~/.config/granit/plugins/       — global plugins
#   ~/.config/granit/lua/           — global Lua scripts
#   ~/.config/granit/themes/        — custom themes
```

### Remove Per-Vault Configuration (Optional)

Per-vault configuration files are inside each vault:

```bash
# In each vault directory:
rm .granit.json              # per-vault settings
rm -rf .granit/              # vault-local plugins and lua scripts
rm -rf .granit-trash/        # trash (deleted notes)
```

### Arch Linux

```bash
sudo pacman -R granit
```

### Clean Go Module Cache (Optional)

```bash
go clean -modcache
```

---

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `GRANIT_VAULT` | Default vault path (used when no path is given) | None |
| `EDITOR` | Preferred external editor for shell-out operations | None |
| `HOME` | User home directory (for config location) | System default |

---

## Troubleshooting

### "command not found: granit"

Ensure `~/go/bin/` is in your PATH:

```bash
echo $PATH | tr ':' '\n' | grep go
# Should show /home/youruser/go/bin

# If missing, add to your shell config:
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

### Build Errors

1. Verify Go version: `go version` (need 1.23+)
2. Ensure modules are downloaded: `go mod download`
3. Clear module cache if corrupted: `go clean -modcache && go mod download`

### Permission Denied

If installing system-wide fails:

```bash
# Use sudo for /usr/local/bin
sudo cp bin/granit /usr/local/bin/

# Or install to user directory
cp bin/granit ~/bin/
```
