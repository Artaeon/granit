# Granit — Command Palette Showcase
# Shows: open command palette, fuzzy search through commands, select one
# Duration: ~15 seconds

Output assets/command-palette.gif

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

# ── Open a note for context ──────────────────────────────────────────────────
Type "j"
Sleep 300ms
Enter
Sleep 1s

# ── Open command palette ─────────────────────────────────────────────────────
Ctrl+X
Sleep 2s

# ── Search for different commands ─────────────────────────────────────────────
Type "graph"
Sleep 1500ms

# Clear and try another
Backspace
Backspace
Backspace
Backspace
Backspace
Sleep 300ms

Type "task"
Sleep 1500ms

Backspace
Backspace
Backspace
Backspace
Sleep 300ms

Type "theme"
Sleep 1500ms

Backspace
Backspace
Backspace
Backspace
Backspace
Sleep 300ms

Type "export"
Sleep 1500ms

# ── Close ─────────────────────────────────────────────────────────────────────
Escape
Sleep 1s

# ── Quit ─────────────────────────────────────────────────────────────────────
Ctrl+Q
Sleep 500ms
