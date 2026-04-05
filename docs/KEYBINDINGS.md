# Granit — Keyboard Shortcut Reference

> Complete reference for every keyboard shortcut in Granit.

---

## Table of Contents

- [Global Navigation](#global-navigation)
- [File Operations](#file-operations)
- [Editor Shortcuts](#editor-shortcuts)
- [Views & Tools](#views--tools)
- [Vim Mode](#vim-mode)
- [Overlay-Specific Shortcuts](#overlay-specific-shortcuts)
- [Task Manager Shortcuts](#task-manager-shortcuts)
- [Calendar Shortcuts](#calendar-shortcuts)
- [Canvas Shortcuts](#canvas-shortcuts)

---

## Global Navigation

| Key | Action |
|-----|--------|
| `Tab` | Cycle focus to next panel (sidebar → editor → backlinks) |
| `Shift+Tab` | Cycle focus to previous panel |
| `F1` | Focus the file sidebar |
| `F2` | Focus the editor |
| `F3` | Focus the backlinks panel |
| `Alt+Left` | Navigate back in history (browser-style) |
| `Alt+Right` | Navigate forward in history |
| `Ctrl+/` | Search Everything (notes, tasks, goals, habits) |
| `Esc` | Close current overlay / return to sidebar |
| `PgUp` | Scroll page up |
| `PgDn` | Scroll page down |

---

## File Operations

| Key | Action |
|-----|--------|
| `Ctrl+P` | Quick open (fuzzy file search) |
| `Ctrl+N` | Create new note (template picker) |
| `Ctrl+S` | Save current note |
| `Ctrl+V` | Paste from system clipboard |
| `F4` | Rename current note |
| `Ctrl+X` | Open command palette (145+ commands, 11 categories) |
| `Ctrl+Q` | Quit Granit |
| `Ctrl+C` | Quit Granit |

---

## Editor Shortcuts

| Key | Action |
|-----|--------|
| `Ctrl+E` | Toggle between view and edit mode |
| `Ctrl+U` | Undo |
| `Ctrl+Y` | Redo |
| `Ctrl+F` | Find in current file |
| `Ctrl+H` | Find and replace in current file |
| `Ctrl+D` | Select word under cursor / add next occurrence to multi-cursor |
| `Ctrl+Shift+Up` | Add cursor on the line above |
| `Ctrl+Shift+Down` | Add cursor on the line below |
| `Esc` | Clear multi-cursors (when multi-cursor is active) |
| `[[` | Trigger wikilink autocomplete |
| `/` | Trigger snippet expansion (e.g., `/date`, `/meeting`) |
| `Tab` | Accept ghost writer suggestion / indent |
| `Shift+Tab` | Unindent |
| `Enter` | New line (with auto-indent) |
| `Backspace` | Delete character before cursor |
| `Delete` | Delete character after cursor |
| `Home` | Move to start of line |
| `End` | Move to end of line |
| `Arrow keys` | Move cursor |
| `Alt+Shift+Left` | Reorder tab left |
| `Alt+Shift+Right` | Reorder tab right |
| `Alt+W` | Close current tab |

---

## Views & Tools

| Key | Action |
|-----|--------|
| `Ctrl+G` | Open note graph visualization |
| `Ctrl+T` | Open tag browser |
| `Ctrl+O` | Open note heading outline |
| `Ctrl+B` | Open bookmarks & recent notes |
| `Ctrl+J` | Quick switch between recent files |
| `Ctrl+W` | Open canvas / whiteboard |
| `Ctrl+L` | Open calendar view |
| `Ctrl+R` | Open AI bots panel |
| `Ctrl+K` | Open task manager |
| `Ctrl+Z` | Toggle focus / zen mode |
| `Ctrl+,` | Open settings panel |
| `F5` | Show help / keyboard shortcuts |
| `Ctrl+X` | Open command palette |

---

## Vim Mode

Enable Vim mode via Settings > "Vim Mode" or Command palette > "Toggle Vim Mode".

When Vim mode is active, the status bar shows the current mode: `NORMAL`, `INSERT`, `VISUAL`, or `COMMAND`.

### Normal Mode

#### Cursor Movement

| Key | Action |
|-----|--------|
| `h` | Move left |
| `j` | Move down |
| `k` | Move up |
| `l` | Move right |
| `w` | Move to start of next word |
| `b` | Move to start of previous word |
| `e` | Move to end of current/next word |
| `0` | Move to start of line |
| `$` | Move to end of line |
| `^` | Move to first non-space character |
| `gg` | Move to top of file |
| `G` | Move to bottom of file |
| `{count}G` | Move to line {count} |
| `{count}gg` | Move to line {count} |
| `H` | Move to top of visible screen |
| `M` | Move to middle of visible screen |
| `L` | Move to bottom of visible screen |
| `Ctrl+D` | Scroll half page down |
| `Ctrl+U` | Scroll half page up |

All movement commands accept a numeric prefix: `5j` moves down 5 lines, `3w` moves forward 3 words.

#### Entering Insert Mode

| Key | Action |
|-----|--------|
| `i` | Insert before cursor |
| `a` | Insert after cursor |
| `I` | Insert at first non-space character of line |
| `A` | Insert at end of line |
| `o` | Open new line below and enter Insert mode |
| `O` | Open new line above and enter Insert mode |

#### Editing (Normal Mode)

| Key | Action |
|-----|--------|
| `dd` | Delete current line |
| `{count}dd` | Delete {count} lines |
| `dw` | Delete to next word |
| `d$` | Delete to end of line |
| `D` | Delete to end of line (same as `d$`) |
| `dj` | Delete current and next line |
| `dk` | Delete current and previous line |
| `dG` | Delete from cursor to end of file |
| `x` | Delete character under cursor |
| `cc` | Change (delete + enter Insert) current line |
| `cw` | Change word |
| `c$` | Change to end of line |
| `C` | Change to end of line (same as `c$`) |
| `yy` | Yank (copy) current line |
| `{count}yy` | Yank {count} lines |
| `yw` | Yank word |
| `y$` | Yank to end of line |
| `yj` | Yank current and next line |
| `yk` | Yank current and previous line |
| `yG` | Yank from cursor to end of file |
| `p` | Paste below current line |
| `P` | Paste above current line |
| `u` | Undo |
| `Ctrl+R` | Redo |
| `J` | Join current line with the next line |
| `.` | Repeat last action |

#### Folding

| Key | Action |
|-----|--------|
| `za` | Toggle fold at cursor |
| `zM` | Fold all sections |
| `zR` | Unfold all sections |

#### Search

| Key | Action |
|-----|--------|
| `/` | Start forward search |
| `?` | Start backward search |
| `n` | Go to next search match |
| `N` | Go to previous search match |

#### Mode Switching

| Key | Action |
|-----|--------|
| `v` or `V` | Enter Visual mode |
| `:` | Enter Command mode |

### Insert Mode

| Key | Action |
|-----|--------|
| `Esc` | Return to Normal mode |
| All other keys | Type normally (passed through to the editor) |

### Visual Mode

Visual mode starts a selection from the cursor position. Moving the cursor extends the selection.

| Key | Action |
|-----|--------|
| `h` / `j` / `k` / `l` | Extend selection in direction |
| `G` | Extend selection to end of file |
| `0` | Extend selection to start of line |
| `$` | Extend selection to end of line |
| `d` or `x` | Delete the selection |
| `y` | Yank (copy) the selection |
| `Esc` | Return to Normal mode (cancel selection) |

### Command Mode

Enter Command mode by pressing `:` in Normal mode. Type a command and press `Enter`.

| Command | Action |
|---------|--------|
| `:w` | Save the current note |
| `:q` | Quit Granit |
| `:wq` | Save and quit |
| `:{number}` | Go to line number (e.g., `:42` goes to line 42) |
| `Esc` | Cancel and return to Normal mode |
| `Backspace` | Delete last character in command buffer |

---

## Overlay-Specific Shortcuts

### Command Palette (`Ctrl+X`)

| Key | Action |
|-----|--------|
| Type text | Filter commands by name or description |
| `Up` / `Ctrl+K` | Move cursor up in results |
| `Down` / `Ctrl+J` | Move cursor down in results |
| `Enter` | Execute selected command |
| `Esc` | Close palette |
| `Backspace` | Delete last filter character |

### Sidebar (Focused)

| Key | Action |
|-----|--------|
| Type text | Fuzzy filter file list |
| `Up` / `k` | Move cursor up |
| `Down` / `j` | Move cursor down |
| `Enter` | Open selected file or expand/collapse folder |
| `Esc` | Clear search filter |

### Settings (`Ctrl+,`)

| Key | Action |
|-----|--------|
| `Up` / `k` | Move cursor up |
| `Down` / `j` | Move cursor down |
| `Enter` / `Space` | Toggle boolean setting or cycle through options |
| `Left` / `Right` | Cycle string option values |
| `Esc` | Close settings and save |

### Graph View (`Ctrl+G`)

| Key | Action |
|-----|--------|
| `Up` / `Down` | Select node |
| `Enter` | Open selected note |
| `Esc` | Close graph |

### Tag Browser (`Ctrl+T`)

| Key | Action |
|-----|--------|
| `Up` / `Down` | Navigate tags/notes |
| `Enter` | Select tag or open note |
| `Esc` | Close tag browser |

### Bookmarks (`Ctrl+B`)

| Key | Action |
|-----|--------|
| `Tab` | Switch between Starred and Recent tabs |
| `Up` / `Down` | Navigate entries |
| `Enter` | Open selected note |
| `Esc` | Close bookmarks |

### Find & Replace (`Ctrl+F` / `Ctrl+H`)

| Key | Action |
|-----|--------|
| Type text | Enter search query |
| `Enter` | Find next match |
| `Tab` | Switch between find and replace fields |
| `Esc` | Close find/replace |

### Outline (`Ctrl+O`)

| Key | Action |
|-----|--------|
| `Up` / `Down` | Navigate headings |
| `Enter` | Jump to selected heading |
| `Esc` | Close outline |

### Quick Switch (`Ctrl+J`)

| Key | Action |
|-----|--------|
| `Up` / `Down` | Navigate recent files |
| `Enter` | Open selected file |
| `Esc` | Close quick switch |

### AI Bots (`Ctrl+R`)

19 AI bots organized into 6 categories (Summarize, Writing, Analysis, Organize, Learning, Vault).

**Bot list:**

| Key | Action |
|-----|--------|
| `Up` / `k` | Select previous bot (wraps at top) |
| `Down` / `j` | Select next bot (wraps at bottom) |
| `Home` | Jump to first bot |
| `End` | Jump to last bot |
| `Enter` | Run selected bot |
| `1`–`9` | Quick-pick first nine visible bots |
| type any letter | Type-to-filter the list (searches name + description) |
| `Backspace` | Remove last filter character |
| `Esc` | Clear filter if set, otherwise close |

**Bot results:**

| Key | Action |
|-----|--------|
| `j` / `k` | Scroll down / up |
| `Down` / `Up` | Scroll down / up |
| `pgdn` / `ctrl+d` | Page down |
| `pgup` / `ctrl+u` | Page up |
| `g` / `home` | Jump to top |
| `G` / `end` | Jump to bottom |
| `c` / `y` | Copy raw AI response to system clipboard |
| `s` | Save result as a note in `<vault>/Bots/` with frontmatter |
| `r` | Re-run the same bot (great for retrying on small models) |
| `Enter` | Apply result (tags/links) or close |
| `Esc` | Back to bot list |

**Bot loading screen:**

| Key | Action |
|-----|--------|
| `Esc` | Cancel the in-flight AI request (actually aborts HTTP, not just UI) |

### Export

| Key | Action |
|-----|--------|
| `Up` / `Down` | Select export format |
| `Enter` | Export in selected format |
| `Esc` | Close export overlay |

### Git Overlay

| Key | Action |
|-----|--------|
| `Tab` | Switch between Status / Log / Diff views |
| `c` | Commit (prompts for message) |
| `p` | Push to remote |
| `P` | Pull from remote |
| `r` | Refresh status |
| `Up` / `Down` | Navigate entries |
| `Esc` | Close git overlay |

### Plugin Manager

| Key | Action |
|-----|--------|
| `Up` / `k` | Move cursor up |
| `Down` / `j` | Move cursor down |
| `Enter` / `Space` | Toggle plugin enabled/disabled |
| `d` | Show plugin detail view |
| `r` | Run first command of selected plugin |
| `i` | Show installable plugins registry |
| `Esc` / `q` | Close plugin manager |

### Trash

| Key | Action |
|-----|--------|
| `Up` / `Down` | Navigate deleted notes |
| `r` | Restore selected note |
| `Esc` | Close trash |

---

## Task Manager Shortcuts

### Navigation & Views

| Key | Action |
|-----|--------|
| `Tab` | Cycle views (Today / Upcoming / All / Done / Calendar / Kanban / Matrix) |
| `1`-`7` | Jump to specific view |
| `j` / `k` | Navigate tasks |
| `Esc`, `q` | Close task manager |

### Task Actions

| Key | Action |
|-----|--------|
| `x`, `Enter` | Toggle task done/undone |
| `u` | Undo task action (10-deep stack; toggle, date, priority, etc.) |
| `n` | Add/edit task note (Enter saves, Esc cancels) |
| `z` | Snooze task (1=1h, 2=4h, 3=tomorrow 9am) |
| `W` | Pin/unpin task (pinned tasks sort to top) |
| `A` | Auto-suggest priority (heuristic based on deadline/deps/project) |
| `R` | Batch reschedule all overdue tasks (Today view only) |
| `a` | Add new task |
| `g` | Jump to task source note |
| `d` | Set/change due date (date picker) |
| `r` | Reschedule (1=tomorrow, 2=Monday, 3=+1wk, 4=+1mo, 5=custom) |
| `p` | Cycle priority level (none/low/med/high/highest) |
| `E` | Set time estimate (1=15m, 2=30m, 3=45m, 4=1h, 5=1.5h, 6=2h) |
| `e` | Expand/collapse subtasks |
| `b` | Add task dependency |
| `f` | Start focus session on task |
| `T` | Save current task as reusable template |
| `t` | Create task from template (1-9 to select) |
| `X` | Archive completed tasks older than 30 days |
| `?` | Show help overlay with all keybindings |

### Filtering & Sorting

| Key | Action |
|-----|--------|
| `/` | Search tasks (supports `#tag` syntax) |
| `#` | Cycle tag filter |
| `P` | Cycle priority filter |
| `s` | Cycle sort mode (priority / due date / A-Z / source / tag) |
| `c` | Clear all active filters |

### Bulk Operations

| Key | Action |
|-----|--------|
| `v` | Enter/exit select mode |
| `Space` | Toggle selection on current task (in select mode) |
| `x` | Bulk toggle done/undone (in select mode) |
| `d` | Bulk set due date (in select mode) |
| `Esc`, `q` | Exit select mode |

### Kanban Board

| Key | Action |
|-----|--------|
| `h` / `l` | Move between columns |
| `j` / `k` | Navigate tasks within a column |
| `x`, `Enter` | Toggle task completion |
| `>` / `<` | Move task to next/previous column |
| `g` | Jump to task source note |
| `a` | Add new task |

### Eisenhower Matrix

| Key | Action |
|-----|--------|
| `7` or `Tab` | Switch to Matrix view |
| Read-only | 2×2 grid: DO (urgent+important), SCHEDULE (important), DELEGATE (urgent), ELIMINATE (neither) |

---

## Goals Manager Shortcuts

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate goals / milestones |
| `Tab` | Cycle views (Active / By Category / Timeline / Completed) |
| `1`-`4` | Jump to specific view |
| `Enter` | Expand goal to show milestones / toggle milestone |
| `a` | Create new goal (title → date → category wizard) |
| `m` | Add milestone to current goal |
| `x` | Toggle goal complete / toggle milestone done |
| `e` | Edit goal title |
| `E` | Edit goal description |
| `n` | Edit goal notes |
| `p` | Pause / resume goal |
| `A` | Archive goal (soft delete) |
| `D` | Delete goal permanently |
| `d` | Delete milestone (when expanded) |
| `!` | Set milestone due date (1=1wk, 2=2wk, 3=1mo, 4=3mo, 0=clear) |
| `J` / `K` | Reorder milestone down / up |
| `t` | Create task from milestone (links with goal:ID) |
| `r` | Set review frequency / write review (when expanded) |
| `C` | Set goal color (7 theme colors) |
| `?` | Help overlay with all keybindings |
| `Esc` | Collapse goal / close overlay |

---

## Daily Planner Shortcuts

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate time slots |
| `a` | Add block (type with 1-4, duration with -/+) |
| `d` | Delete block |
| `Space` | Toggle block done |
| `m` / `Enter` | Move block / assign unscheduled task |
| `f` | Start focus session from block |
| `c` | Copy daily plan to clipboard (includes goals) |
| `S` | Export plan as markdown to Plans/ folder |
| `s` | Save planner to file |
| `[` / `]` | Previous / next day |
| `Tab` | Switch panel (schedule / tasks / habits) |
| `Esc` | Close planner |

---

## Calendar Shortcuts

| Key | Action |
|-----|--------|
| `Left` / `Right` | Previous / next day |
| `Up` / `Down` | Previous / next week |
| `Tab` | Switch between Month / Week / Agenda views |
| `Enter` | Open or create daily note for selected date |
| `n` | Quick add event |
| `[` / `]` | Previous / next month |
| `t` | Jump to today |
| `y` | Toggle year view |
| `Esc` | Close calendar |

---

## Canvas Shortcuts

| Key | Action |
|-----|--------|
| Arrow keys | Move selected card |
| `n` | Add new card |
| `Enter` | Open note for selected card |
| `d` | Delete selected card |
| `c` | Connect two cards |
| `Tab` | Cycle between cards |
| `Esc` | Close canvas |
