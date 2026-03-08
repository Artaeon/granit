# Granit — AI Bots & Features Showcase
# Shows: open AI bots panel, select summarizer, show results, AI compose via palette
# Duration: ~20 seconds

Output assets/ai-features.gif

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

# ── Open a note first so the AI has content to work with ─────────────────────
Type "j"
Sleep 300ms
Enter
Sleep 1500ms

# ── Open AI Bots panel ──────────────────────────────────────────────────────
Ctrl+R
Sleep 1500ms

# ── Navigate to Summarizer (3rd bot in the list) ────────────────────────────
Type "j"
Sleep 400ms
Type "j"
Sleep 400ms

# ── Run the Summarizer ──────────────────────────────────────────────────────
Enter
Sleep 3s

# ── Scroll through results ──────────────────────────────────────────────────
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Sleep 1s

# ── Close AI Bots panel ─────────────────────────────────────────────────────
Escape
Sleep 1s

# ── Open command palette and search for AI features ──────────────────────────
Ctrl+X
Sleep 1s
Type "AI"
Sleep 1500ms

# ── Select AI Compose Note ──────────────────────────────────────────────────
Enter
Sleep 1500ms

# ── Close ────────────────────────────────────────────────────────────────────
Escape
Sleep 1s

# ── Quit ─────────────────────────────────────────────────────────────────────
Ctrl+Q
Sleep 1s
