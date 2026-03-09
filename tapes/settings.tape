# Granit — Settings Panel Showcase
# Shows: open settings via command palette, browse categories, toggle options
# Duration: ~15 seconds

Output assets/settings.gif

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

# ── Open a note for background context ────────────────────────────────────────
Type "j"
Sleep 300ms
Enter
Sleep 1s

# ── Open Settings via command palette ─────────────────────────────────────────
Ctrl+X
Sleep 1s
Type "settings"
Sleep 800ms
Enter
Sleep 2s

# ── Browse settings ──────────────────────────────────────────────────────────
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms

Sleep 1s

# ── Toggle a boolean setting ──────────────────────────────────────────────────
Enter
Sleep 800ms
Type "j"
Sleep 300ms
Enter
Sleep 800ms

# ── Browse more settings ──────────────────────────────────────────────────────
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms

Sleep 1s

# ── Close settings ───────────────────────────────────────────────────────────
Escape
Sleep 1s

# ── Quit ─────────────────────────────────────────────────────────────────────
Ctrl+Q
Sleep 500ms
