package tui

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// gitHistoryResultMsg carries the result of an async git history command.
type gitHistoryResultMsg struct {
	action string // "log", "show", "diff", "restore"
	output string
	err    error
}

// gitHistoryCommit represents a single parsed commit entry.
type gitHistoryCommit struct {
	Hash    string
	Author  string
	Date    string
	Message string
}

// GitHistory provides a per-note git history viewer overlay with diff viewing
// and version restore capabilities.
type GitHistory struct {
	active bool
	width  int
	height int

	// Note being viewed
	notePath  string // relative path in vault
	vaultRoot string

	// History data
	commits []gitHistoryCommit // parsed commit entries
	cursor  int
	scroll  int

	// Detail view
	showingDiff bool
	diffLines   []string
	diffScroll  int

	// Restore
	restored    bool
	restoreHash string

	// Error / loading state
	errorMsg string
	loading  bool
}

// NewGitHistory creates a new GitHistory component.
func NewGitHistory() GitHistory {
	return GitHistory{}
}

// IsActive returns whether the git history overlay is currently visible.
func (g *GitHistory) IsActive() bool {
	return g.active
}

// SetSize updates the available dimensions for the overlay.
func (g *GitHistory) SetSize(w, h int) {
	g.width = w
	g.height = h
}

// Close hides the git history overlay.
func (g *GitHistory) Close() {
	g.active = false
	g.showingDiff = false
	g.diffLines = nil
	g.diffScroll = 0
}

// Open activates the overlay for a specific note and kicks off a log fetch.
func (g *GitHistory) Open(notePath, vaultRoot string) tea.Cmd {
	g.active = true
	g.notePath = notePath
	g.vaultRoot = vaultRoot
	g.commits = nil
	g.cursor = 0
	g.scroll = 0
	g.showingDiff = false
	g.diffLines = nil
	g.diffScroll = 0
	g.restored = false
	g.restoreHash = ""
	g.errorMsg = ""
	g.loading = true
	return g.fetchLog()
}

// GetRestoreResult returns the hash of the restored version and resets the
// flag. The caller should check ok and reload the file content when true.
func (g *GitHistory) GetRestoreResult() (hash string, ok bool) {
	if g.restored {
		g.restored = false
		h := g.restoreHash
		g.restoreHash = ""
		return h, true
	}
	return "", false
}

// fetchLog runs git log for the specific file asynchronously.
func (g *GitHistory) fetchLog() tea.Cmd {
	notePath := g.notePath
	vaultRoot := g.vaultRoot
	return func() tea.Msg {
		cmd := exec.Command("git", "log",
			"--format=%H|%an|%ad|%s",
			"--date=short",
			"--", notePath,
		)
		cmd.Dir = vaultRoot
		out, err := cmd.CombinedOutput()
		return gitHistoryResultMsg{
			action: "log",
			output: string(out),
			err:    err,
		}
	}
}

// fetchDiff runs git diff between a commit and its parent for the file.
func (g *GitHistory) fetchDiff(hash string) tea.Cmd {
	notePath := g.notePath
	vaultRoot := g.vaultRoot
	return func() tea.Msg {
		cmd := exec.Command("git", "diff",
			hash+"~1", hash,
			"--", notePath,
		)
		cmd.Dir = vaultRoot
		out, err := cmd.CombinedOutput()
		// If the commit is the initial commit, diff against empty tree
		if err != nil {
			cmd2 := exec.Command("git", "diff",
				"4b825dc642cb6eb9a060e54bf899d8b2b04e3c72",
				hash,
				"--", notePath,
			)
			cmd2.Dir = vaultRoot
			out2, err2 := cmd2.CombinedOutput()
			return gitHistoryResultMsg{
				action: "diff",
				output: string(out2),
				err:    err2,
			}
		}
		return gitHistoryResultMsg{
			action: "diff",
			output: string(out),
			err:    nil,
		}
	}
}

// fetchShow runs git show to retrieve file content at a specific commit.
func (g *GitHistory) fetchShow(hash string) tea.Cmd {
	notePath := g.notePath
	vaultRoot := g.vaultRoot
	return func() tea.Msg {
		cmd := exec.Command("git", "show", hash+":"+notePath)
		cmd.Dir = vaultRoot
		out, err := cmd.CombinedOutput()
		return gitHistoryResultMsg{
			action: "restore",
			output: string(out),
			err:    err,
		}
	}
}

// parseLogOutput parses the git log output into commit structs.
func parseLogOutput(output string) []gitHistoryCommit {
	output = strings.TrimRight(output, "\n")
	if output == "" {
		return nil
	}
	lines := strings.Split(output, "\n")
	var commits []gitHistoryCommit
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}
		commits = append(commits, gitHistoryCommit{
			Hash:    parts[0],
			Author:  parts[1],
			Date:    parts[2],
			Message: parts[3],
		})
	}
	return commits
}

// Update handles keyboard input and async results for the git history overlay.
func (g GitHistory) Update(msg tea.Msg) (GitHistory, tea.Cmd) {
	if !g.active {
		return g, nil
	}

	switch msg := msg.(type) {
	case gitHistoryResultMsg:
		return g.handleResult(msg)
	case tea.KeyMsg:
		if g.showingDiff {
			return g.updateDiffMode(msg)
		}
		return g.updateListMode(msg)
	}
	return g, nil
}

// handleResult processes async git command results.
func (g GitHistory) handleResult(msg gitHistoryResultMsg) (GitHistory, tea.Cmd) {
	switch msg.action {
	case "log":
		g.loading = false
		if msg.err != nil {
			g.errorMsg = "Git error: " + msg.err.Error()
			g.commits = nil
		} else {
			g.errorMsg = ""
			g.commits = parseLogOutput(msg.output)
			if len(g.commits) == 0 {
				g.errorMsg = "No history found for this file"
			}
		}
	case "diff":
		g.loading = false
		if msg.err != nil {
			g.errorMsg = "Diff error: " + msg.err.Error()
			g.showingDiff = false
		} else {
			g.errorMsg = ""
			g.showingDiff = true
			g.diffScroll = 0
			raw := strings.TrimRight(msg.output, "\n")
			if raw == "" {
				g.diffLines = []string{"(no changes in this commit for this file)"}
			} else {
				g.diffLines = strings.Split(raw, "\n")
			}
		}
	case "restore":
		if msg.err != nil {
			g.errorMsg = "Restore failed: " + msg.err.Error()
		} else {
			// Write the file content back — the caller (app.go) handles
			// this via GetRestoreResult and reloads the note.
			g.restored = true
			if g.cursor >= 0 && g.cursor < len(g.commits) {
				g.restoreHash = g.commits[g.cursor].Hash
			}
			g.active = false
		}
	}
	return g, nil
}

// updateListMode handles keys when viewing the commit list.
func (g GitHistory) updateListMode(msg tea.KeyMsg) (GitHistory, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		g.active = false
		return g, nil

	case "up", "k":
		if g.cursor > 0 {
			g.cursor--
			if g.cursor < g.scroll {
				g.scroll = g.cursor
			}
		}
		return g, nil

	case "down", "j":
		if g.cursor < len(g.commits)-1 {
			g.cursor++
			visH := g.listVisibleHeight()
			if g.cursor >= g.scroll+visH {
				g.scroll = g.cursor - visH + 1
			}
		}
		return g, nil

	case "enter":
		if len(g.commits) > 0 && g.cursor < len(g.commits) {
			g.loading = true
			hash := g.commits[g.cursor].Hash
			return g, g.fetchDiff(hash)
		}
		return g, nil

	case "r":
		if len(g.commits) > 0 && g.cursor < len(g.commits) {
			g.loading = true
			hash := g.commits[g.cursor].Hash
			return g, g.fetchShow(hash)
		}
		return g, nil
	}
	return g, nil
}

// updateDiffMode handles keys when viewing a diff.
func (g GitHistory) updateDiffMode(msg tea.KeyMsg) (GitHistory, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		g.showingDiff = false
		g.diffLines = nil
		g.diffScroll = 0
		g.errorMsg = ""
		return g, nil

	case "up", "k":
		if g.diffScroll > 0 {
			g.diffScroll--
		}
		return g, nil

	case "down", "j":
		maxScroll := len(g.diffLines) - g.diffVisibleHeight()
		if maxScroll < 0 {
			maxScroll = 0
		}
		if g.diffScroll < maxScroll {
			g.diffScroll++
		}
		return g, nil
	}
	return g, nil
}

// listVisibleHeight returns how many commit rows fit in the list view.
func (g GitHistory) listVisibleHeight() int {
	// header(2) + count(1) + blank(1) + separator(1) + footer(2) + border padding(2)
	h := g.height - 12
	if h < 3 {
		h = 3
	}
	return h
}

// diffVisibleHeight returns how many diff lines fit in the diff view.
func (g GitHistory) diffVisibleHeight() int {
	h := g.height - 10
	if h < 5 {
		h = 5
	}
	return h
}

// overlayWidth calculates the overlay panel width.
func (g GitHistory) overlayWidth() int {
	w := g.width * 2 / 3
	if w < 60 {
		w = 60
	}
	if w > 100 {
		w = 100
	}
	return w
}

// View renders the git history overlay.
func (g GitHistory) View() string {
	if g.showingDiff {
		return g.viewDiff()
	}
	return g.viewList()
}

// viewList renders the commit list view.
func (g GitHistory) viewList() string {
	width := g.overlayWidth()
	innerW := width - 6

	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render("  Git History: " + g.notePath))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")

	if g.loading {
		b.WriteString(DimStyle.Render("  Loading..."))
		b.WriteString("\n")
	} else if g.errorMsg != "" && len(g.commits) == 0 {
		b.WriteString(DimStyle.Render("  " + g.errorMsg))
		b.WriteString("\n")
	} else if len(g.commits) == 0 {
		b.WriteString(DimStyle.Render("  No history"))
		b.WriteString("\n")
	} else {
		// Commit count
		countStyle := lipgloss.NewStyle().Foreground(overlay0)
		b.WriteString(countStyle.Render(fmt.Sprintf("  %d commits", len(g.commits))))
		b.WriteString("\n\n")

		visH := g.listVisibleHeight()
		end := g.scroll + visH
		if end > len(g.commits) {
			end = len(g.commits)
		}

		hashStyle := lipgloss.NewStyle().Foreground(yellow)
		dateStyle := lipgloss.NewStyle().Foreground(subtext0)
		msgStyle := lipgloss.NewStyle().Foreground(text)
		selectedHash := lipgloss.NewStyle().Foreground(peach).Bold(true)
		selectedDate := lipgloss.NewStyle().Foreground(peach)
		selectedMsg := lipgloss.NewStyle().Foreground(peach).Bold(true)
		selectedBg := lipgloss.NewStyle().Background(surface0)

		for i := g.scroll; i < end; i++ {
			c := g.commits[i]
			shortHash := c.Hash
			if len(shortHash) > 7 {
				shortHash = shortHash[:7]
			}

			if i == g.cursor {
				indicator := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render(ThemeAccentBar)
				line := fmt.Sprintf(" %s  %s  %s",
					selectedHash.Render(shortHash),
					selectedDate.Render(c.Date),
					selectedMsg.Render(truncateMsg(c.Message, innerW-30)),
				)
				rendered := selectedBg.Width(innerW).Render("  " + indicator + line)
				b.WriteString(rendered)
			} else {
				line := fmt.Sprintf("     %s  %s  %s",
					hashStyle.Render(shortHash),
					dateStyle.Render(c.Date),
					msgStyle.Render(truncateMsg(c.Message, innerW-30)),
				)
				b.WriteString(line)
			}
			b.WriteString("\n")
		}
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")

	b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
		{"Enter", "view diff"}, {"r", "restore"}, {"Esc", "close"},
	}))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// viewDiff renders the diff detail view.
func (g GitHistory) viewDiff() string {
	width := g.overlayWidth()
	innerW := width - 6

	var b strings.Builder

	// Title with commit info
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	var diffTitle string
	if g.cursor >= 0 && g.cursor < len(g.commits) {
		c := g.commits[g.cursor]
		shortHash := c.Hash
		if len(shortHash) > 7 {
			shortHash = shortHash[:7]
		}
		diffTitle = fmt.Sprintf("  Diff: %s — %s", shortHash, c.Message)
	} else {
		diffTitle = "  Diff"
	}
	b.WriteString(titleStyle.Render(diffTitle))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")

	if g.loading {
		b.WriteString(DimStyle.Render("  Loading..."))
		b.WriteString("\n")
	} else if g.errorMsg != "" {
		errStyle := lipgloss.NewStyle().Foreground(red).Bold(true)
		b.WriteString("  " + errStyle.Render(g.errorMsg))
		b.WriteString("\n")
	} else {
		visH := g.diffVisibleHeight()

		maxScroll := len(g.diffLines) - visH
		if maxScroll < 0 {
			maxScroll = 0
		}
		scroll := g.diffScroll
		if scroll > maxScroll {
			scroll = maxScroll
		}

		end := scroll + visH
		if end > len(g.diffLines) {
			end = len(g.diffLines)
		}

		addStyle := lipgloss.NewStyle().Foreground(green)
		delStyle := lipgloss.NewStyle().Foreground(red)
		hunkStyle := lipgloss.NewStyle().Foreground(surface2)
		fileStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)

		for i := scroll; i < end; i++ {
			line := g.diffLines[i]
			var styled string

			switch {
			case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
				styled = fileStyle.Render(line)
			case strings.HasPrefix(line, "@@"):
				styled = hunkStyle.Render(line)
			case strings.HasPrefix(line, "+"):
				styled = addStyle.Render(line)
			case strings.HasPrefix(line, "-"):
				styled = delStyle.Render(line)
			case strings.HasPrefix(line, "diff "):
				styled = lipgloss.NewStyle().Foreground(peach).Bold(true).Render(line)
			default:
				styled = DimStyle.Render(line)
			}
			b.WriteString("  " + styled)
			b.WriteString("\n")
		}

		// Scroll indicator
		if len(g.diffLines) > visH {
			pct := 0
			if maxScroll > 0 {
				pct = scroll * 100 / maxScroll
			}
			scrollInfo := lipgloss.NewStyle().Foreground(overlay0).
				Render(fmt.Sprintf("  %d%%  (%d/%d lines)", pct, scroll+visH, len(g.diffLines)))
			b.WriteString(scrollInfo)
			b.WriteString("\n")
		}
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")

	b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
		{"Esc", "back to list"},
	}))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// truncateMsg truncates a commit message to fit within maxLen characters.
func truncateMsg(msg string, maxLen int) string {
	if maxLen < 5 {
		maxLen = 5
	}
	if len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen-3] + "..."
}
