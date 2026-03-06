package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// FocusMode provides a distraction-free writing experience by centering the
// editor in a narrow column and hiding the sidebar and backlinks panel.
type FocusMode struct {
	active      bool
	width       int
	height      int
	targetWords int // optional word count goal (0 = no goal)
	startWords  int // word count when focus mode started
}

// NewFocusMode returns a FocusMode in its default (inactive) state.
func NewFocusMode() FocusMode {
	return FocusMode{}
}

// SetSize updates the available terminal dimensions.
func (f *FocusMode) SetSize(width, height int) {
	f.width = width
	f.height = height
}

// Open activates focus mode, recording the current word count so that
// progress toward a target can be measured.
func (f *FocusMode) Open(currentWordCount int) {
	f.active = true
	f.startWords = currentWordCount
}

// Close deactivates focus mode.
func (f *FocusMode) Close() {
	f.active = false
}

// IsActive reports whether focus mode is currently engaged.
func (f *FocusMode) IsActive() bool {
	return f.active
}

// SetTargetWords sets an optional word-count goal. Pass 0 to clear the goal.
func (f *FocusMode) SetTargetWords(n int) {
	f.targetWords = n
}

// RenderEditor wraps the editor view in a centered, minimal layout suitable
// for distraction-free writing. It strips surrounding chrome and shows only
// the editor content inside a narrow column with a thin status line at the
// bottom.
func (f *FocusMode) RenderEditor(editorView string, wordCount int) string {
	// --- column geometry ---
	const maxColumn = 80
	colWidth := f.width - 4 // leave room for border + padding
	if colWidth > maxColumn {
		colWidth = maxColumn
	}
	if colWidth < 20 {
		colWidth = 20
	}

	// Available height: total minus border (2) minus status line (1) minus
	// a small top/bottom breathing margin supplied by padding.
	innerHeight := f.height - 4
	if innerHeight < 4 {
		innerHeight = 4
	}

	// --- status line ---
	statusLine := f.buildStatus(wordCount, colWidth)

	// --- truncate / pad editor content to fit ---
	lines := strings.Split(editorView, "\n")
	editorHeight := innerHeight - 1 // reserve one line for status
	if editorHeight < 1 {
		editorHeight = 1
	}
	if len(lines) > editorHeight {
		lines = lines[:editorHeight]
	}
	editorContent := strings.Join(lines, "\n")

	body := editorContent + "\n" + statusLine

	// --- focused panel style ---
	panel := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Foreground(text).
		Width(colWidth).
		Height(innerHeight).
		Padding(0, 1)

	rendered := panel.Render(body)

	// --- center the panel horizontally and vertically ---
	centered := lipgloss.Place(
		f.width,
		f.height,
		lipgloss.Center,
		lipgloss.Center,
		rendered,
	)

	return centered
}

// buildStatus produces the minimal status string shown at the bottom of the
// focus-mode panel: word count and, when a target is set, a small progress
// bar with "X/Y words".
func (f *FocusMode) buildStatus(wordCount int, availWidth int) string {
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)
	countStyle := lipgloss.NewStyle().Foreground(subtext0)

	sep := dimStyle.Render("─")
	ruler := strings.Repeat(sep, availWidth-2)

	var info string
	if f.targetWords > 0 {
		progress := wordCount - f.startWords
		if progress < 0 {
			progress = 0
		}
		info = f.renderProgressBar(progress, f.targetWords, availWidth-2) +
			"  " + countStyle.Render(fmt.Sprintf("%d/%d words", progress, f.targetWords))
	} else {
		info = countStyle.Render(fmt.Sprintf("%d words", wordCount))
	}

	// Compose: ruler on one conceptual line, info right-aligned below it.
	// We put the info on the same line as the ruler, right-aligned.
	infoWidth := lipgloss.Width(info)
	rulerWidth := lipgloss.Width(ruler)
	padding := rulerWidth - infoWidth
	if padding < 0 {
		padding = 0
	}

	return ruler + "\n" + strings.Repeat(" ", padding) + info
}

// renderProgressBar draws a small horizontal bar using block characters,
// coloured with the theme's primary (mauve) and surface colours.
func (f *FocusMode) renderProgressBar(current, target, maxWidth int) string {
	barWidth := maxWidth / 3
	if barWidth < 8 {
		barWidth = 8
	}
	if barWidth > 20 {
		barWidth = 20
	}

	filled := 0
	if target > 0 {
		filled = (current * barWidth) / target
		if filled > barWidth {
			filled = barWidth
		}
	}
	empty := barWidth - filled

	filledStyle := lipgloss.NewStyle().Foreground(mauve)
	emptyStyle := lipgloss.NewStyle().Foreground(surface1)

	bar := filledStyle.Render(strings.Repeat("█", filled)) +
		emptyStyle.Render(strings.Repeat("░", empty))

	return bar
}
