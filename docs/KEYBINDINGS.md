# Granit — Keyboard shortcut reference

Complete reference for keyboard shortcuts in Granit. The web app and
the terminal UI have separate bindings; both are listed here.

In all tables below `Mod` means the platform-default modifier — `Cmd`
on macOS, `Ctrl` on Linux and Windows. CodeMirror's keymap parser
expands `Mod-X` into both, so `Mod-S` saves on every platform.

---

## Table of contents

- [Web app](#web-app)
  - [Global](#web--global)
  - [Notes editor](#web--notes-editor)
  - [Notes page](#web--notes-page)
  - [Tasks page](#web--tasks-page)
  - [Calendar page](#web--calendar-page)
  - [History panel](#web--history-panel)
  - [Print preview](#web--print-preview)
- [Terminal UI](#terminal-ui)
  - [Global navigation](#tui--global-navigation)
  - [File operations](#tui--file-operations)
  - [Editor](#tui--editor)
  - [Views and tools](#tui--views-and-tools)
  - [Vim mode](#tui--vim-mode)
  - [Task manager](#tui--task-manager)
  - [Calendar](#tui--calendar)
  - [Canvas](#tui--canvas)

---

## Web app

### Web — Global

| Shortcut | Action |
|---|---|
| `Mod-K` | Open the command palette (search across notes, tasks, settings) |
| `Mod-P` | Quick switcher (notes-only fast file open) |
| `Mod-Shift-P` | Browser print dialog (when a print preview is open it captures `Mod-P` for itself) |
| `Mod-Shift-O` | Jump back to the last opened note (note tray) |
| `Esc` | Close the current overlay / drawer |

The command palette and quick switcher live in
`web/src/lib/components/CommandPalette.svelte`. The palette has two
modes — full search (Mod-K) and notes-only (Mod-P).

The note tray (slim "last opened note" bar at the bottom of every
page, and the matching `Mod-Shift-O` shortcut) lives in
`web/src/lib/components/NoteTray.svelte` with state in
`web/src/lib/stores/open-note.ts`. Toggle the tray on / off in
**Settings → General → Note tray**.

### Web — Notes editor

CodeMirror 6 bindings registered in `web/src/lib/editor/Editor.svelte`
and the extension files under `web/src/lib/editor/`.

#### Markdown formatting

| Shortcut | Action |
|---|---|
| `Mod-B` | Toggle bold (`**...**`) |
| `Mod-I` | Toggle italic (`*...*`) |
| `Mod-Shift-I` | Toggle italic with underscore (`_..._`) |
| `Mod-K` | Insert / wrap as a markdown link (uses clipboard if it looks like a URL) |
| `Mod-`` ` `` | Toggle inline code |

#### Headings, lists, quotes

| Shortcut | Action |
|---|---|
| `Mod-Alt-1` … `Mod-Alt-6` | Set the line to a heading of that level |
| `Mod-Alt-0` | Strip any heading / list / quote prefix |
| `Mod-Shift-7` | Toggle ordered list (number prefix) |
| `Mod-Shift-8` | Toggle bullet list (`*`) |
| `Mod-Shift-9` | Toggle blockquote |

#### Tasks

| Shortcut | Action |
|---|---|
| `Mod-Shift-Enter` | Insert `- [ ] ` checklist item |
| `Mod-Enter` | Toggle the checkbox state on the current line |

#### Save + edit

| Shortcut | Action |
|---|---|
| `Mod-S` | Save the current note immediately (auto-save also runs on idle) |
| `Mod-F` | Open CodeMirror's find panel |
| `Mod-Shift-F` | Find and replace |
| `Mod-Z` | Undo |
| `Mod-Shift-Z` | Redo |
| `Tab` / `Shift-Tab` | Indent / unindent |

#### Editor power features

| Shortcut | Action |
|---|---|
| `[[` | Trigger wikilink autocomplete |
| `#` | Trigger tag autocomplete |
| `/` | Trigger snippet / block-completion picker |
| `Mod-Shift-X` | Extract selection into a new note (opens dialog with editable title + folder picker) |
| `Mod-Shift-A` | Send the selection to the AI (opens Ask AI panel) |

When a CodeMirror autocomplete picker is open, auto-save is paused so
the picker isn't disrupted by an external value change.

### Web — Notes page

When focus is in the page chrome (not the editor itself):

| Shortcut | Action |
|---|---|
| `Mod-/` | Cycle the view mode (edit → split → preview → edit) |
| `?` | Open the keyboard-shortcuts help overlay |
| `Esc` | Close help / overlays |

### Web — Tasks page

When focus is on the page (not inside an input):

| Shortcut | Action |
|---|---|
| `j` | Move cursor down to the next task |
| `k` | Move cursor up to the previous task |
| `x` | Toggle selection on the cursor task (multi-select) |
| `d` | Toggle done on the cursor task |
| `e` | Open the task detail drawer |
| `p` | Cycle priority on the cursor task |
| `Esc` | Clear multi-selection |
| `?` | Open / close the keyboard-shortcuts help overlay |

### Web — Calendar page

Mirrors Google Calendar bindings so muscle memory carries over.

| Shortcut | Action |
|---|---|
| `t` | Jump to today |
| `j` or `n` | Next period (week / month / etc., depending on view) |
| `k` or `p` | Previous period |
| `d` | Day view |
| `w` | Week view |
| `x` | 3-day view |
| `m` | Month view |
| `y` | Year view |
| `a` | Agenda view |
| `?` | Toggle the calendar shortcuts help overlay |
| `Esc` | Close create / edit drawer |

Bindings disable when an input has focus or when a creation / detail
drawer is open. The drawers own their own keyboard surface (Enter to
submit, Esc to close).

On touch devices, swipe left / right on the grid to advance / go back.

### Web — History panel

| Shortcut | Action |
|---|---|
| `Arrow Down` / `Arrow Up` | Move the selection through the version list |
| `Esc` | Close the history panel |

### Web — Print preview

| Shortcut | Action |
|---|---|
| `Mod-P` | Print (handled inside the overlay so the global `Mod-P` doesn't open the quick switcher first) |
| `Esc` | Close the preview |

---

## Terminal UI

The TUI uses Bubble Tea bindings. Bindings below are the canonical
defaults; some can be remapped via the settings overlay or
`config.json`.

### TUI — Global navigation

| Shortcut | Action |
|---|---|
| `Tab` | Cycle focus to next panel (sidebar → editor → backlinks) |
| `Shift+Tab` | Cycle focus to previous panel |
| `F1` / `Alt+1` | Focus the file sidebar |
| `F2` / `Alt+2` | Focus the editor |
| `F3` / `Alt+3` | Focus the backlinks panel |
| `Ctrl+Tab` | Switch to next open tab |
| `Ctrl+Shift+Tab` | Switch to previous open tab |
| `Ctrl+1`–`Ctrl+9` | Jump to tab by position |
| `Alt+Left` | Navigate back in history |
| `Alt+Right` | Navigate forward in history |
| `Ctrl+/` | Search Everything (notes, tasks, goals, habits) |
| `Esc` | Close current overlay / return to sidebar |
| `PgUp` / `PgDn` | Scroll page up / down |

### TUI — File operations

| Shortcut | Action |
|---|---|
| `Ctrl+P` | Quick open (fuzzy file search) |
| `Ctrl+N` | Create new note (template picker) |
| `Ctrl+S` | Save current note |
| `Ctrl+V` | Paste from system clipboard |
| `F4` | Rename current note |
| `Ctrl+X` | Open command palette |
| `Ctrl+Q` / `Ctrl+C` | Quit Granit |

### TUI — Editor

| Shortcut | Action |
|---|---|
| `Ctrl+E` | Toggle between view and edit mode |
| `Ctrl+U` | Undo |
| `Ctrl+Y` | Redo |
| `Ctrl+F` | Find in current file |
| `Ctrl+H` | Find and replace in current file |
| `Ctrl+D` | Select word under cursor / add cursor at next occurrence |
| `Ctrl+Shift+Up` | Add cursor on the line above |
| `Ctrl+Shift+Down` | Add cursor on the line below |
| `Esc` | Clear multi-cursors |
| `[[` | Trigger wikilink autocomplete |
| `/` | Trigger snippet expansion |
| `Tab` / `Shift+Tab` | Accept ghost-writer suggestion / indent / unindent |
| `Alt+Shift+Left` / `Alt+Shift+Right` | Reorder current tab |
| `Alt+W` | Close current tab |

### TUI — Views and tools

| Shortcut | Action |
|---|---|
| `Ctrl+G` | Open note graph visualisation |
| `Ctrl+T` | Open tag browser |
| `Ctrl+O` | Open note heading outline |
| `Ctrl+B` | Open bookmarks + recent notes |
| `Ctrl+J` | Quick switch between recent files |
| `Ctrl+W` | Open canvas / whiteboard |
| `Ctrl+L` | Open calendar view |
| `Ctrl+R` | Open AI bots panel |
| `Ctrl+K` | Open task manager |
| `Ctrl+Z` | Toggle focus / zen mode |
| `Ctrl+,` | Open settings panel |
| `F5` | Show help / keyboard shortcuts |

### TUI — Vim mode

Enable Vim mode via Settings ("Vim Mode") or the command palette
("Toggle Vim Mode"). The status bar shows the current mode (`NORMAL`,
`INSERT`, `VISUAL`, `COMMAND`).

#### Cursor movement

| Key | Action |
|---|---|
| `h` / `j` / `k` / `l` | Left / down / up / right |
| `w` / `b` / `e` | Word forward / back / end |
| `0` / `$` | Start / end of line |
| `gg` / `G` | Top / bottom of buffer |

#### Operators (combine with motions)

| Key | Action |
|---|---|
| `d` | Delete |
| `c` | Change |
| `y` | Yank |
| `p` | Paste |
| `dd` / `yy` | Delete / yank line |
| `ciw` | Change inner word |
| `5dd` | Delete 5 lines |

#### Visual

| Key | Action |
|---|---|
| `v` | Character-wise visual |
| `V` | Line-wise visual |

#### Command-line

| Key | Action |
|---|---|
| `:w` | Save |
| `:q` | Quit |
| `:wq` | Save and quit |
| `:{n}` | Jump to line `n` |

### TUI — Task manager

| Shortcut | Action |
|---|---|
| `j` / `k` | Move cursor down / up |
| `x` | Toggle selection |
| `d` | Toggle done |
| `e` | Edit task detail |
| `p` | Cycle priority |
| `=` | Filter by cursor's project |
| `Ctrl+D` / `Delete` | Delete cursor task |
| `Esc` | Clear selection / close panel |

### TUI — Calendar

| Shortcut | Action |
|---|---|
| `t` | Today |
| `j` / `n` | Next period |
| `k` / `p` | Previous period |
| `d` / `w` / `m` / `y` / `a` | Day / week / month / year / agenda |

### TUI — Canvas

| Shortcut | Action |
|---|---|
| `n` | New node |
| `Tab` | Cycle nodes |
| `Enter` | Edit node label |
| `d` | Delete node |
| `Arrow keys` | Move selected node |

---

## Discovering more

- The web app's quickest discovery surface is the command palette
  (`Mod-K`) — every action that has a name is listed there.
- The TUI's command palette (`Ctrl+X`) is similarly the source of
  truth for "what can I do right now"; press `F5` for the help
  overlay.

If a shortcut listed here is wrong or missing, please open an issue
or PR — bindings drift faster than docs.
