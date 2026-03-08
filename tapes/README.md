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

Make sure `granit` is built and available at `~/go/bin/granit`, and that the
`demo-vault/` directory exists at the project root.

## Recording a single tape

```bash
vhs tapes/demo.tape
```

The output GIF will be written to `assets/<name>.gif` as specified in each tape file.

## Recording all tapes

```bash
for tape in tapes/*.tape; do
  echo "Recording $tape ..."
  vhs "$tape"
done
```

## Tape inventory

| File | Description | Duration |
|------|-------------|----------|
| `demo.tape` | Hero demo: launch, navigate, edit/view mode, command palette | ~30s |
| `vim-mode.tape` | Vim mode: hjkl, dd/yy/p, visual mode, :w save | ~20s |
| `task-manager.tape` | Task manager: views, add task, priority, due dates | ~20s |
| `ai-features.tape` | AI bots: summarizer, AI compose from palette | ~20s |
| `themes.tape` | Theme switching via settings overlay | ~15s |
| `split-pane.tape` | Split pane: side-by-side notes with Tab switching | ~15s |

## Tips

- Adjust `Set TypingSpeed` to make typing faster or slower.
- Adjust `Sleep` durations to control pacing.
- Use `Set Theme "Catppuccin Mocha"` (or any VHS theme) to change the terminal
  emulator appearance. This is independent of Granit's internal theme.
- GIF files can be large. Consider using `Output assets/demo.webm` for smaller
  WebM video output instead.
