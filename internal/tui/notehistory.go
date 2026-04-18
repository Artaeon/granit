package tui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// noteHistoryResultMsg carries the result of an async git command for note history.
type noteHistoryResultMsg struct {
	action string // "log", "diff", "snapshot"
	output string
	err    error
}

// historyEntry represents a single parsed commit from git log.
type historyEntry struct {
	Hash      string
	ShortHash string
	Author    string
	TimeAgo   string
	Subject   string
}

// NoteHistory shows git history for the currently active note with a visual
// timeline and inline diff viewer / snapshot viewer.
type NoteHistory struct {
	OverlayBase
	vaultRoot string
	notePath  string

	entries []historyEntry
	cursor  int
	scroll  int

	// View modes: 0=timeline, 1=diff, 2=snapshot
	viewMode int

	// Diff/snapshot content
	diffContent     string
	snapshotContent string
	contentScroll   int

	statusMsg string
	noGit     bool
}

// NewNoteHistory creates a new NoteHistory component.
func NewNoteHistory() NoteHistory {
	return NoteHistory{}
}

// OpenForNote activates the overlay for a specific note and fetches its git log.
func (nh *NoteHistory) OpenForNote(vaultRoot, notePath string) {
	nh.Activate()
	nh.vaultRoot = vaultRoot
	nh.notePath = notePath
	nh.entries = nil
	nh.cursor = 0
	nh.scroll = 0
	nh.viewMode = 0
	nh.diffContent = ""
	nh.snapshotContent = ""
	nh.contentScroll = 0
	nh.statusMsg = ""
	nh.noGit = false
}

// FetchLog returns a tea.Cmd that asynchronously fetches git log for the note.
// Call this right after OpenForNote to populate the timeline.
func (nh *NoteHistory) FetchLog() tea.Cmd {
	notePath := nh.notePath
	vaultRoot := nh.vaultRoot
	return func() tea.Msg {
		cmd := exec.Command("git", "log", "--follow",
			"--format=%H|%h|%an|%ar|%s",
			"--", notePath,
		)
		cmd.Dir = vaultRoot
		out, err := cmd.CombinedOutput()
		return noteHistoryResultMsg{
			action: "log",
			output: string(out),
			err:    err,
		}
	}
}

// fetchDiff returns a tea.Cmd that runs git diff between two commits for the note.
func (nh *NoteHistory) fetchDiff(hash1, hash2 string) tea.Cmd {
	notePath := nh.notePath
	vaultRoot := nh.vaultRoot
	return func() tea.Msg {
		cmd := exec.Command("git", "diff", hash1, hash2, "--", notePath)
		cmd.Dir = vaultRoot
		out, err := cmd.CombinedOutput()
		return noteHistoryResultMsg{
			action: "diff",
			output: string(out),
			err:    err,
		}
	}
}

// fetchSnapshot returns a tea.Cmd that retrieves the file content at a specific commit.
func (nh *NoteHistory) fetchSnapshot(hash string) tea.Cmd {
	notePath := nh.notePath
	vaultRoot := nh.vaultRoot
	return func() tea.Msg {
		cmd := exec.Command("git", "show", hash+":"+notePath)
		cmd.Dir = vaultRoot
		out, err := cmd.CombinedOutput()
		return noteHistoryResultMsg{
			action: "snapshot",
			output: string(out),
			err:    err,
		}
	}
}

// parseNoteHistoryLog parses git log output in "%H|%h|%an|%ar|%s" format.
func parseNoteHistoryLog(output string) []historyEntry {
	output = strings.TrimRight(output, "\n")
	if output == "" {
		return nil
	}
	lines := strings.Split(output, "\n")
	var entries []historyEntry
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 5 {
			continue
		}
		entries = append(entries, historyEntry{
			Hash:      parts[0],
			ShortHash: parts[1],
			Author:    parts[2],
			TimeAgo:   parts[3],
			Subject:   parts[4],
		})
	}
	return entries
}

// visibleHeight returns the number of lines available for content display.
func (nh NoteHistory) visibleHeight() int {
	h := nh.height - 14
	if h < 5 {
		h = 5
	}
	return h
}

// overlayWidth calculates the overlay panel width.
func (nh NoteHistory) overlayWidth() int {
	w := nh.width * 2 / 3
	if w < 60 {
		w = 60
	}
	if w > 100 {
		w = 100
	}
	return w
}

// Update handles keyboard input and async results for the note history overlay.
func (nh NoteHistory) Update(msg tea.Msg) (NoteHistory, tea.Cmd) {
	if !nh.active {
		return nh, nil
	}

	switch msg := msg.(type) {
	case noteHistoryResultMsg:
		return nh.handleResult(msg)
	case tea.KeyMsg:
		switch nh.viewMode {
		case 1:
			return nh.updateDiffView(msg)
		case 2:
			return nh.updateSnapshotView(msg)
		default:
			return nh.updateTimeline(msg)
		}
	}
	return nh, nil
}

// handleResult processes async git command results.
func (nh NoteHistory) handleResult(msg noteHistoryResultMsg) (NoteHistory, tea.Cmd) {
	switch msg.action {
	case "log":
		if msg.err != nil {
			// Check if this is a non-git repo
			errStr := msg.err.Error()
			outStr := strings.ToLower(msg.output)
			if strings.Contains(outStr, "not a git repository") ||
				strings.Contains(errStr, "not a git repository") {
				nh.noGit = true
				nh.statusMsg = "Not a git repository"
			} else {
				nh.statusMsg = "Error: " + msg.err.Error()
			}
			nh.entries = nil
		} else {
			nh.entries = parseNoteHistoryLog(msg.output)
			if len(nh.entries) == 0 {
				nh.statusMsg = "No history found for this note"
			} else {
				nh.statusMsg = ""
			}
		}

	case "diff":
		if msg.err != nil {
			nh.statusMsg = "Diff error: " + msg.err.Error()
			nh.viewMode = 0
		} else {
			raw := strings.TrimRight(msg.output, "\n")
			if raw == "" {
				nh.diffContent = "(no changes in this commit)"
			} else {
				nh.diffContent = raw
			}
			nh.contentScroll = 0
			nh.viewMode = 1
			nh.statusMsg = ""
		}

	case "snapshot":
		if msg.err != nil {
			nh.statusMsg = "Snapshot error: " + msg.err.Error()
			nh.viewMode = 0
		} else {
			nh.snapshotContent = msg.output
			nh.contentScroll = 0
			nh.viewMode = 2
			nh.statusMsg = ""
		}
	}
	return nh, nil
}

// updateTimeline handles keys in the timeline (list) view.
func (nh NoteHistory) updateTimeline(msg tea.KeyMsg) (NoteHistory, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		nh.active = false
		return nh, nil

	case "up", "k":
		if nh.cursor > 0 {
			nh.cursor--
			if nh.cursor < nh.scroll {
				nh.scroll = nh.cursor
			}
		}
		return nh, nil

	case "down", "j":
		if nh.cursor < len(nh.entries)-1 {
			nh.cursor++
			// Each entry takes 4 lines in the timeline, calculate visible entries
			visEntries := nh.visibleHeight() / 4
			if visEntries < 1 {
				visEntries = 1
			}
			if nh.cursor >= nh.scroll+visEntries {
				nh.scroll = nh.cursor - visEntries + 1
			}
		}
		return nh, nil

	case "enter", "d":
		// Show diff between selected commit and previous one
		if len(nh.entries) > 0 && nh.cursor < len(nh.entries) {
			selected := nh.entries[nh.cursor]
			if nh.cursor+1 < len(nh.entries) {
				// Diff between this commit and the previous (older) one
				prev := nh.entries[nh.cursor+1]
				return nh, nh.fetchDiff(prev.Hash, selected.Hash)
			}
			// This is the oldest commit; diff against empty tree
			emptyTree := "4b825dc642cb6eb9a060e54bf899d8b2b04e3c72"
			return nh, nh.fetchDiff(emptyTree, selected.Hash)
		}
		return nh, nil

	case "s":
		// Show snapshot of file at selected commit
		if len(nh.entries) > 0 && nh.cursor < len(nh.entries) {
			return nh, nh.fetchSnapshot(nh.entries[nh.cursor].Hash)
		}
		return nh, nil
	}
	return nh, nil
}

// updateDiffView handles keys in the diff view.
func (nh NoteHistory) updateDiffView(msg tea.KeyMsg) (NoteHistory, tea.Cmd) {
	switch msg.String() {
	case "esc":
		nh.viewMode = 0
		nh.diffContent = ""
		nh.contentScroll = 0
		return nh, nil

	case "up", "k":
		if nh.contentScroll > 0 {
			nh.contentScroll--
		}
		return nh, nil

	case "down", "j":
		lines := strings.Split(nh.diffContent, "\n")
		maxScroll := len(lines) - nh.visibleHeight()
		if maxScroll < 0 {
			maxScroll = 0
		}
		if nh.contentScroll < maxScroll {
			nh.contentScroll++
		}
		return nh, nil

	case "n":
		// Next commit
		if nh.cursor < len(nh.entries)-1 {
			nh.cursor++
			return nh, nh.triggerDiff()
		}
		return nh, nil

	case "p":
		// Previous commit
		if nh.cursor > 0 {
			nh.cursor--
			return nh, nh.triggerDiff()
		}
		return nh, nil
	}
	return nh, nil
}

// updateSnapshotView handles keys in the snapshot view.
func (nh NoteHistory) updateSnapshotView(msg tea.KeyMsg) (NoteHistory, tea.Cmd) {
	switch msg.String() {
	case "esc":
		nh.viewMode = 0
		nh.snapshotContent = ""
		nh.contentScroll = 0
		return nh, nil

	case "up", "k":
		if nh.contentScroll > 0 {
			nh.contentScroll--
		}
		return nh, nil

	case "down", "j":
		lines := strings.Split(nh.snapshotContent, "\n")
		maxScroll := len(lines) - nh.visibleHeight()
		if maxScroll < 0 {
			maxScroll = 0
		}
		if nh.contentScroll < maxScroll {
			nh.contentScroll++
		}
		return nh, nil

	case "n":
		// Next commit snapshot
		if nh.cursor < len(nh.entries)-1 {
			nh.cursor++
			return nh, nh.fetchSnapshot(nh.entries[nh.cursor].Hash)
		}
		return nh, nil

	case "p":
		// Previous commit snapshot
		if nh.cursor > 0 {
			nh.cursor--
			return nh, nh.fetchSnapshot(nh.entries[nh.cursor].Hash)
		}
		return nh, nil
	}
	return nh, nil
}

// triggerDiff fires off a diff command for the current cursor position.
func (nh *NoteHistory) triggerDiff() tea.Cmd {
	if nh.cursor < 0 || nh.cursor >= len(nh.entries) {
		return nil
	}
	selected := nh.entries[nh.cursor]
	if nh.cursor+1 < len(nh.entries) {
		prev := nh.entries[nh.cursor+1]
		return nh.fetchDiff(prev.Hash, selected.Hash)
	}
	emptyTree := "4b825dc642cb6eb9a060e54bf899d8b2b04e3c72"
	return nh.fetchDiff(emptyTree, selected.Hash)
}

// View renders the note history overlay.
func (nh NoteHistory) View() string {
	switch nh.viewMode {
	case 1:
		return nh.viewDiff()
	case 2:
		return nh.viewSnapshot()
	default:
		return nh.viewTimeline()
	}
}

// viewTimeline renders the visual timeline of commits.
func (nh NoteHistory) viewTimeline() string {
	width := nh.overlayWidth()
	innerW := width - 6

	var b strings.Builder

	// Title
	noteBase := filepath.Base(nh.notePath)
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render("  Note History: " + noteBase))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")

	if nh.noGit {
		errStyle := lipgloss.NewStyle().Foreground(red).Bold(true)
		b.WriteString("\n")
		b.WriteString("  " + errStyle.Render("Not a git repository"))
		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("  Initialize a git repo in your vault to track note history."))
		b.WriteString("\n")
	} else if nh.statusMsg != "" && len(nh.entries) == 0 {
		b.WriteString("\n")
		b.WriteString("  " + DimStyle.Render(nh.statusMsg))
		b.WriteString("\n")
	} else if len(nh.entries) == 0 {
		b.WriteString("\n")
		b.WriteString("  " + DimStyle.Render("Loading..."))
		b.WriteString("\n")
	} else {
		// Commit count
		countStyle := lipgloss.NewStyle().Foreground(overlay0)
		b.WriteString(countStyle.Render(fmt.Sprintf("  %d version(s)", len(nh.entries))))
		b.WriteString("\n\n")

		// Each timeline entry takes ~4 lines, calculate visible entries
		visH := nh.visibleHeight()
		visEntries := visH / 4
		if visEntries < 1 {
			visEntries = 1
		}

		end := nh.scroll + visEntries
		if end > len(nh.entries) {
			end = len(nh.entries)
		}

		nodeSelected := lipgloss.NewStyle().Foreground(mauve).Bold(true)
		nodeNormal := lipgloss.NewStyle().Foreground(overlay0)
		connectorStyle := lipgloss.NewStyle().Foreground(surface2)
		timeStyle := lipgloss.NewStyle().Foreground(teal)
		hashStyle := lipgloss.NewStyle().Foreground(yellow)
		subjectStyle := lipgloss.NewStyle().Foreground(text)
		authorStyle := lipgloss.NewStyle().Foreground(overlay0)

		connector := connectorStyle.Render("\u2502")

		for i := nh.scroll; i < end; i++ {
			entry := nh.entries[i]

			// Node marker
			var node string
			if i == nh.cursor {
				node = nodeSelected.Render("\u25cf")
			} else {
				node = nodeNormal.Render("\u25cb")
			}

			// Truncate subject if needed
			subject := entry.Subject
			maxSubjectLen := innerW - 30
			if maxSubjectLen < 10 {
				maxSubjectLen = 10
			}
			subject = TruncateDisplay(subject, maxSubjectLen)

			// Line 1: node + time ago + short hash
			timePart := timeStyle.Render(entry.TimeAgo)
			hashPart := hashStyle.Render(entry.ShortHash)
			line1 := fmt.Sprintf("  %s %s \u2014 %s", node, timePart, hashPart)

			if i == nh.cursor {
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Width(innerW).
					Render(line1))
			} else {
				b.WriteString(line1)
			}
			b.WriteString("\n")

			// Line 2: connector + subject
			subjectPart := subjectStyle.Render(subject)
			if i == nh.cursor {
				subjectPart = lipgloss.NewStyle().Foreground(lavender).Bold(true).Render(subject)
			}
			b.WriteString(fmt.Sprintf("  %s %s", connector, subjectPart))
			b.WriteString("\n")

			// Line 3: connector + author
			authorPart := authorStyle.Render("Author: " + entry.Author)
			b.WriteString(fmt.Sprintf("  %s %s", connector, authorPart))
			b.WriteString("\n")

			// Line 4: connector (blank spacer) unless last entry
			if i < end-1 {
				b.WriteString(fmt.Sprintf("  %s", connector))
				b.WriteString("\n")
			}
		}

		// Scroll indicator
		if len(nh.entries) > visEntries {
			b.WriteString("\n")
			pct := 0
			maxScroll := len(nh.entries) - visEntries
			if maxScroll > 0 {
				pct = nh.scroll * 100 / maxScroll
			}
			scrollInfo := lipgloss.NewStyle().Foreground(overlay0).
				Render(fmt.Sprintf("  %d%% (%d/%d)", pct, nh.cursor+1, len(nh.entries)))
			b.WriteString(scrollInfo)
		}
	}

	// Footer
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")

	if len(nh.entries) > 0 && !nh.noGit {
		b.WriteString(nh.renderFooterKeys([]footerKey{
			{"j/k", "navigate"},
			{"Enter/d", "diff"},
			{"s", "snapshot"},
			{"Esc", "close"},
		}))
	} else {
		b.WriteString(nh.renderFooterKeys([]footerKey{
			{"Esc", "close"},
		}))
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// viewDiff renders the diff view with colored additions and deletions.
func (nh NoteHistory) viewDiff() string {
	width := nh.overlayWidth()
	innerW := width - 6

	var b strings.Builder

	// Title with commit info
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	var diffTitle string
	if nh.cursor >= 0 && nh.cursor < len(nh.entries) {
		entry := nh.entries[nh.cursor]
		diffTitle = fmt.Sprintf("  Diff: %s \u2014 %s", entry.ShortHash, entry.Subject)
		diffTitle = TruncateDisplay(diffTitle, innerW)
	} else {
		diffTitle = "  Diff"
	}
	b.WriteString(titleStyle.Render(diffTitle))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")

	if nh.statusMsg != "" {
		errStyle := lipgloss.NewStyle().Foreground(red).Bold(true)
		b.WriteString("  " + errStyle.Render(nh.statusMsg))
		b.WriteString("\n")
	} else {
		lines := strings.Split(nh.diffContent, "\n")
		visH := nh.visibleHeight()

		maxScroll := len(lines) - visH
		if maxScroll < 0 {
			maxScroll = 0
		}
		scroll := nh.contentScroll
		if scroll > maxScroll {
			scroll = maxScroll
		}

		endLine := scroll + visH
		if endLine > len(lines) {
			endLine = len(lines)
		}

		addStyle := lipgloss.NewStyle().Foreground(green)
		delStyle := lipgloss.NewStyle().Foreground(red)
		hunkStyle := lipgloss.NewStyle().Foreground(blue)
		fileStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)

		for i := scroll; i < endLine; i++ {
			line := TruncateDisplay(lines[i], innerW-4)

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
				styled = NormalItemStyle.Render(line)
			}
			b.WriteString("  " + styled)
			b.WriteString("\n")
		}

		// Scroll indicator
		if len(lines) > visH {
			pct := 0
			if maxScroll > 0 {
				pct = scroll * 100 / maxScroll
			}
			scrollInfo := lipgloss.NewStyle().Foreground(overlay0).
				Render(fmt.Sprintf("  %d%% (%d/%d lines)", pct, scroll+visH, len(lines)))
			b.WriteString(scrollInfo)
		}
	}

	// Footer
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")
	b.WriteString(nh.renderFooterKeys([]footerKey{
		{"j/k", "scroll"},
		{"n/p", "next/prev commit"},
		{"Esc", "back"},
	}))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// viewSnapshot renders the file content at a specific commit with basic highlighting.
func (nh NoteHistory) viewSnapshot() string {
	width := nh.overlayWidth()
	innerW := width - 6

	var b strings.Builder

	// Title with commit info
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	var snapTitle string
	if nh.cursor >= 0 && nh.cursor < len(nh.entries) {
		entry := nh.entries[nh.cursor]
		snapTitle = fmt.Sprintf("  Snapshot: %s \u2014 %s", entry.ShortHash, entry.TimeAgo)
	} else {
		snapTitle = "  Snapshot"
	}
	b.WriteString(titleStyle.Render(snapTitle))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")

	if nh.statusMsg != "" {
		errStyle := lipgloss.NewStyle().Foreground(red).Bold(true)
		b.WriteString("  " + errStyle.Render(nh.statusMsg))
		b.WriteString("\n")
	} else {
		lines := strings.Split(nh.snapshotContent, "\n")
		visH := nh.visibleHeight()

		maxScroll := len(lines) - visH
		if maxScroll < 0 {
			maxScroll = 0
		}
		scroll := nh.contentScroll
		if scroll > maxScroll {
			scroll = maxScroll
		}

		endLine := scroll + visH
		if endLine > len(lines) {
			endLine = len(lines)
		}

		headingStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
		boldStyle := lipgloss.NewStyle().Foreground(text).Bold(true)
		codeStyle := lipgloss.NewStyle().Foreground(green)
		linkStyleSnap := lipgloss.NewStyle().Foreground(blue)
		normalStyle := lipgloss.NewStyle().Foreground(text)

		inCodeBlock := false

		for i := scroll; i < endLine; i++ {
			line := lines[i]

			// Truncate long lines, then derive trimmed once on the
			// truncated form (the prior raw-line trim was dead — it was
			// overwritten before any read).
			line = TruncateDisplay(line, innerW-4)
			trimmed := strings.TrimSpace(line)

			var styled string
			switch {
			case strings.HasPrefix(trimmed, "```"):
				inCodeBlock = !inCodeBlock
				styled = codeStyle.Render(line)
			case inCodeBlock:
				styled = codeStyle.Render(line)
			case strings.HasPrefix(trimmed, "#"):
				styled = headingStyle.Render(line)
			case strings.HasPrefix(trimmed, "**") && strings.HasSuffix(trimmed, "**"):
				styled = boldStyle.Render(line)
			case strings.Contains(trimmed, "[[") && strings.Contains(trimmed, "]]"):
				styled = linkStyleSnap.Render(line)
			case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* "):
				styled = normalStyle.Render(line)
			case strings.HasPrefix(trimmed, "> "):
				styled = lipgloss.NewStyle().Foreground(yellow).Render(line)
			default:
				styled = normalStyle.Render(line)
			}
			b.WriteString("  " + styled)
			b.WriteString("\n")
		}

		// Scroll indicator
		if len(lines) > visH {
			pct := 0
			if maxScroll > 0 {
				pct = scroll * 100 / maxScroll
			}
			scrollInfo := lipgloss.NewStyle().Foreground(overlay0).
				Render(fmt.Sprintf("  %d%% (%d/%d lines)", pct, scroll+visH, len(lines)))
			b.WriteString(scrollInfo)
		}
	}

	// Footer
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")
	b.WriteString(nh.renderFooterKeys([]footerKey{
		{"j/k", "scroll"},
		{"n/p", "next/prev commit"},
		{"Esc", "back"},
	}))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// footerKey pairs a key label with its description.
type footerKey struct {
	key  string
	desc string
}

// renderFooterKeys renders a styled footer with key hints.
func (nh NoteHistory) renderFooterKeys(keys []footerKey) string {
	pairs := make([]struct{ Key, Desc string }, len(keys))
	for i, fk := range keys {
		pairs[i] = struct{ Key, Desc string }{fk.key, fk.desc}
	}
	return RenderHelpBar(pairs)
}
