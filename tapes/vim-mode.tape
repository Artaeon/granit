# Granit — Vim Mode Showcase
# Shows: enable vim mode, hjkl movement, dd/yy/p, :w save, visual mode, insert mode
# Duration: ~20 seconds

Output assets/vim-mode.gif

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

# ── Open a note with content ────────────────────────────────────────────────
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Enter
Sleep 1500ms

# Switch to editor panel
Tab
Sleep 500ms

# ── Enable Vim mode via command palette ─────────────────────────────────────
Ctrl+X
Sleep 1s
Type "vim"
Sleep 1s
Enter
Sleep 1500ms

# ── Normal mode: hjkl navigation ────────────────────────────────────────────
# Move down
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms

# Move up
Type "k"
Sleep 300ms

# Move right
Type "l"
Sleep 200ms
Type "l"
Sleep 200ms
Type "l"
Sleep 200ms

# Move left
Type "h"
Sleep 200ms
Type "h"
Sleep 200ms

# ── Yank a line (yy) and paste (p) ──────────────────────────────────────────
Type "yy"
Sleep 800ms
Type "p"
Sleep 1s

# ── Delete a line (dd) ──────────────────────────────────────────────────────
Type "dd"
Sleep 1s

# ── Visual mode selection ────────────────────────────────────────────────────
Type "v"
Sleep 500ms
Type "l"
Sleep 200ms
Type "l"
Sleep 200ms
Type "l"
Sleep 200ms
Type "l"
Sleep 200ms
Type "l"
Sleep 500ms

# Exit visual mode
Escape
Sleep 500ms

# ── Enter insert mode and type some text ─────────────────────────────────────
Type "i"
Sleep 500ms
Type "Hello from Vim mode! "
Sleep 1s

# Return to normal mode
Escape
Sleep 500ms

# ── Save with :w ─────────────────────────────────────────────────────────────
Type ":w"
Enter
Sleep 1500ms

# ── Quit ─────────────────────────────────────────────────────────────────────
Ctrl+Q
Sleep 1s
