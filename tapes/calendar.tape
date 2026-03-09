# Granit — Calendar View Showcase
# Shows: open calendar, month view, navigate months, switch to week/agenda view
# Duration: ~18 seconds

Output assets/calendar.gif

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

# ── Open Calendar ─────────────────────────────────────────────────────────────
Ctrl+L
Sleep 2s

# ── Navigate months ───────────────────────────────────────────────────────────
Type "l"
Sleep 1s
Type "l"
Sleep 1s
Type "h"
Sleep 1s

# ── Switch to week view ──────────────────────────────────────────────────────
Type "2"
Sleep 2s

# ── Switch to agenda view ─────────────────────────────────────────────────────
Type "4"
Sleep 2s

# ── Switch back to month view ─────────────────────────────────────────────────
Type "1"
Sleep 1s

# ── Close calendar ────────────────────────────────────────────────────────────
Escape
Sleep 1s

# ── Quit ─────────────────────────────────────────────────────────────────────
Ctrl+Q
Sleep 500ms
