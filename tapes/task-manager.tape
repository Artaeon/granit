# Granit — Task Manager Showcase
# Shows: open Ctrl+K task manager, switch views, add task, set priority, set due date
# Duration: ~20 seconds

Output assets/task-manager.gif

Set Shell "bash"
Set Width 1200
Set Height 700
Set FontSize 14
Set Padding 20
Set TypingSpeed 50ms

# ── Launch Granit ────────────────────────────────────────────────────────────
Sleep 500ms
Type "~/go/bin/granit demo-vault"
Enter
Sleep 4s

# ── Open Task Manager ───────────────────────────────────────────────────────
Ctrl+K
Sleep 1500ms

# ── Browse the Today view (default) ─────────────────────────────────────────
Type "j"
Sleep 400ms
Type "j"
Sleep 400ms
Type "j"
Sleep 400ms

# ── Switch to Upcoming view (tab 2) ─────────────────────────────────────────
Type "2"
Sleep 1500ms

# ── Switch to Kanban view (tab 6) ───────────────────────────────────────────
Type "6"
Sleep 2s

# ── Switch to Calendar view (tab 5) ─────────────────────────────────────────
Type "5"
Sleep 1500ms

# Navigate calendar months
Type "l"
Sleep 800ms
Type "h"
Sleep 800ms

# ── Switch back to All tasks (tab 3) ────────────────────────────────────────
Type "3"
Sleep 1s

# ── Add a new task ───────────────────────────────────────────────────────────
Type "a"
Sleep 500ms
Type "Review pull request for auth module"
Enter
Sleep 1500ms

# ── Set priority on a task (cycle with p) ────────────────────────────────────
Type "p"
Sleep 800ms
Type "p"
Sleep 800ms

# ── Set a due date ───────────────────────────────────────────────────────────
Type "d"
Sleep 500ms
# Confirm the date
Enter
Sleep 1s

# ── Close task manager ──────────────────────────────────────────────────────
Escape
Sleep 1s

# ── Quit ─────────────────────────────────────────────────────────────────────
Ctrl+Q
Sleep 1s
