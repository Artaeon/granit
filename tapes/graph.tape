# Granit — Note Graph Showcase
# Shows: open graph view, browse note connections
# Duration: ~12 seconds

Output assets/graph.gif

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

# ── Open Note Graph ──────────────────────────────────────────────────────────
Ctrl+G
Sleep 3s

# ── Navigate the graph ────────────────────────────────────────────────────────
Type "j"
Sleep 500ms
Type "j"
Sleep 500ms
Type "j"
Sleep 500ms
Type "k"
Sleep 500ms

Sleep 2s

# ── Close graph ──────────────────────────────────────────────────────────────
Escape
Sleep 1s

# ── Quit ─────────────────────────────────────────────────────────────────────
Ctrl+Q
Sleep 500ms
