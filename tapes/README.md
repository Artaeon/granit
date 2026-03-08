# VHS Tape Files for Granit

Terminal demo recordings scripted with [VHS](https://github.com/charmbracelet/vhs) from Charm.

## Prerequisites

Install VHS (requires Go and ffmpeg/ttyd):

```bash
go install github.com/charmbracelet/vhs@latest
```

On Arch Linux you can also install from the AUR:

```bash
yay -S vhs
```

You also need **ffmpeg** installed for GIF encoding:

```bash
# Debian/Ubuntu
sudo apt install ffmpeg

# Arch Linux
sudo pacman -S ffmpeg

# macOS
brew install ffmpeg
```

Make sure `granit` is built and available at `~/go/bin/granit`, and that the
`demo-vault/` directory exists at the project root.

## Recording all tapes

The easiest way to record all tapes at once:

```bash
./tapes/record.sh
```

This script checks all prerequisites, then runs VHS on every `.tape` file in the `tapes/` directory. Output GIFs are written to `assets/`.

## Recording a single tape

```bash
# Via the helper script (pass tape name without .tape extension):
./tapes/record.sh hero

# Or directly with VHS:
vhs tapes/hero.tape
```

The output GIF will be written to `assets/<name>.gif` as specified in each tape file.

## Tape inventory

| File | Output | Description | Duration |
|------|--------|-------------|----------|
| `hero.tape` | `assets/hero.gif` | README hero image: launch, splash, navigate, view mode, command palette | ~25s |
| `demo.tape` | `assets/demo.gif` | Full demo: launch, navigate, edit/view mode, command palette search | ~30s |
| `vim-mode.tape` | `assets/vim-mode.gif` | Vim mode: hjkl, dd/yy/p, visual mode, :w save | ~20s |
| `task-manager.tape` | `assets/task-manager.gif` | Task manager: views, add task, priority, due dates | ~20s |
| `ai-features.tape` | `assets/ai-features.gif` | AI bots: summarizer, AI compose from palette | ~20s |
| `themes.tape` | `assets/themes.gif` | Theme switching via settings overlay | ~15s |
| `split-pane.tape` | `assets/split-pane.gif` | Split pane: side-by-side notes with Tab switching | ~15s |

## Tips

- Adjust `Set TypingSpeed` to make typing faster or slower.
- Adjust `Sleep` durations to control pacing.
- Use `Set Theme "Catppuccin Mocha"` (or any VHS theme) to change the terminal
  emulator appearance. This is independent of Granit's internal theme.
- GIF files can be large. Consider using `Output assets/demo.webm` for smaller
  WebM video output instead.
- If VHS fails with a display error, make sure you have a working display
  (X11/Wayland) or run inside a virtual framebuffer (`Xvfb`).
