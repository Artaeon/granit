# Granit — Hero Demo GIF (README showcase)
# Shows: launch, splash dismiss, sidebar navigation, open note, view mode toggle,
#        command palette, quick search, quit
# Duration: ~25 seconds

Output assets/hero.gif

Set Shell "bash"
Set FontSize 14
Set Width 1200
Set Height 700
Set Padding 10
Set TypingSpeed 50ms

# ── Launch Granit with the demo vault ────────────────────────────────────────
Sleep 500ms
Type "cd /home/raphael/Projects/obsidian-alternative && ~/go/bin/granit demo-vault"
Enter
Sleep 3s

# ── Dismiss splash screen ────────────────────────────────────────────────────
Type " "
Sleep 2s

# ── Navigate the sidebar ─────────────────────────────────────────────────────
Type "j"
Sleep 500ms
Type "j"
Sleep 500ms

# ── Open the selected note ───────────────────────────────────────────────────
Enter
Sleep 1s

# ── Toggle to rendered view mode ─────────────────────────────────────────────
Ctrl+E
Sleep 2s

# ── Toggle back to edit mode ─────────────────────────────────────────────────
Ctrl+E
Sleep 1s

# ── Open the command palette ─────────────────────────────────────────────────
Ctrl+X
Sleep 1.5s

# ── Close the command palette ────────────────────────────────────────────────
Escape
Sleep 1s

# ── Quit ──────────────────────────────────────────────────────────────────────
Ctrl+Q
Sleep 500ms
