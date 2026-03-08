# Granit — Main Hero Demo
# Shows: launch, sidebar navigation, open note, view mode, command palette, quit
# Duration: ~30 seconds

Output assets/demo.gif

Set Shell "bash"
Set Width 1200
Set Height 700
Set FontSize 14
Set Padding 20
Set TypingSpeed 50ms

# ── Launch Granit with the demo vault ────────────────────────────────────────
Sleep 500ms
Type "~/go/bin/granit demo-vault"
Enter
Sleep 4s

# ── Navigate the sidebar ────────────────────────────────────────────────────
# Move down through files/folders
Type "j"
Sleep 400ms
Type "j"
Sleep 400ms
Type "j"
Sleep 400ms
Type "j"
Sleep 400ms

# Open the selected note (Getting Started)
Enter
Sleep 2s

# ── Switch to the editor panel ──────────────────────────────────────────────
Tab
Sleep 500ms

# ── Toggle to rendered view mode ────────────────────────────────────────────
Ctrl+E
Sleep 2s

# Scroll through the rendered markdown
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

# ── Toggle back to edit mode ────────────────────────────────────────────────
Ctrl+E
Sleep 1500ms

# ── Return to sidebar and open another note ─────────────────────────────────
Tab
Sleep 500ms
Type "j"
Sleep 400ms
Type "j"
Sleep 400ms
Enter
Sleep 2s

# ── Open the command palette ────────────────────────────────────────────────
Ctrl+X
Sleep 1500ms

# Search for "graph"
Type "graph"
Sleep 1500ms

# Clear search and try another command
Ctrl+U
Sleep 300ms
Type "export"
Sleep 1500ms

# Close the command palette
Escape
Sleep 1s

# ── Quit Granit ─────────────────────────────────────────────────────────────
Ctrl+Q
Sleep 1s
