# Granit — Theme Switching Showcase
# Shows: open settings via command palette, navigate to theme, cycle themes
# Duration: ~20 seconds

Output assets/themes.gif

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

# ── Open a note so theme changes are visible on content ──────────────────────
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

# ── Navigate down to the Theme setting ────────────────────────────────────────
Type "j"
Sleep 200ms
Type "j"
Sleep 200ms
Type "j"
Sleep 200ms
Type "j"
Sleep 200ms
Type "j"
Sleep 200ms
Type "j"
Sleep 200ms
Type "j"
Sleep 200ms
Type "j"
Sleep 200ms
Type "j"
Sleep 200ms
Type "j"
Sleep 200ms
Type "j"
Sleep 200ms
Type "j"
Sleep 200ms

# ── Cycle through themes ──────────────────────────────────────────────────────
Enter
Sleep 1200ms

Enter
Sleep 1200ms

Enter
Sleep 1200ms

Enter
Sleep 1200ms

Enter
Sleep 1200ms

# ── Close settings ───────────────────────────────────────────────────────────
Escape
Sleep 1s

# ── Quit ─────────────────────────────────────────────────────────────────────
Ctrl+Q
Sleep 500ms
