# Granit — Split Pane View Showcase
# Shows: open split pane from command palette, pick second note, Tab between panes
# Duration: ~15 seconds

Output assets/split-pane.gif

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

# ── Open a note first so left pane has content ───────────────────────────────
Type "j"
Sleep 300ms
Enter
Sleep 1500ms

# ── Open command palette and select Split View ──────────────────────────────
Ctrl+X
Sleep 1s
Type "split"
Sleep 1s
Enter
Sleep 1500ms

# ── Pick a second note from the file picker ─────────────────────────────────
# Type to filter notes
Type "machine"
Sleep 1s

# Select the matching note
Enter
Sleep 2s

# ── Switch focus between left and right panes with Tab ───────────────────────
Tab
Sleep 1s

# Scroll in the right pane
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Sleep 500ms

# Switch back to left pane
Tab
Sleep 1s

# Scroll in the left pane
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Sleep 1s

# ── Close split pane ────────────────────────────────────────────────────────
Escape
Sleep 1s

# ── Quit ─────────────────────────────────────────────────────────────────────
Ctrl+Q
Sleep 1s
