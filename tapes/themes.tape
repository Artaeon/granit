# Granit — Theme Switching Showcase
# Shows: open settings, navigate to theme, switch between several themes
# Duration: ~15 seconds

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

# ── Open Settings overlay ───────────────────────────────────────────────────
Ctrl+,
Sleep 1500ms

# ── Navigate down to the Theme setting ───────────────────────────────────────
# Theme is the 13th item (after editor settings); navigate to it
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

# ── Cycle through themes (Enter or right arrow cycles string options) ────────

# Switch to tokyo-night
Enter
Sleep 1200ms

# Switch to gruvbox-dark
Enter
Sleep 1200ms

# Switch to nord
Enter
Sleep 1200ms

# Switch to dracula
Enter
Sleep 1200ms

# Switch to rose-pine
Enter
Sleep 1200ms

# ── Close settings ──────────────────────────────────────────────────────────
Escape
Sleep 1s

# ── Quit ─────────────────────────────────────────────────────────────────────
Ctrl+Q
Sleep 1s
