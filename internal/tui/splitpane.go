package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// paneState holds the content and scroll position for one side of the split.
type paneState struct {
	title  string
	lines  []string
	scroll int
}

// SplitPane displays two notes side by side with independent scrolling.
type SplitPane struct {
	active bool
	width  int
	height int
	focus  int // 0 = left, 1 = right
	left   paneState
	right  paneState
}

// NewSplitPane returns a SplitPane in its default (inactive) state.
func NewSplitPane() SplitPane {
	return SplitPane{}
}

// IsActive reports whether the split pane overlay is currently visible.
func (sp *SplitPane) IsActive() bool {
	return sp.active
}

// Open activates the split pane overlay.
func (sp *SplitPane) Open() {
	sp.active = true
	sp.focus = 0
	sp.left.scroll = 0
	sp.right.scroll = 0
}

// Close deactivates the split pane overlay.
func (sp *SplitPane) Close() {
	sp.active = false
}

// SetSize updates the available terminal dimensions.
func (sp *SplitPane) SetSize(width, height int) {
	sp.width = width
	sp.height = height
}

// SetLeftContent loads content into the left pane.
func (sp *SplitPane) SetLeftContent(title string, lines []string) {
	sp.left.title = title
	sp.left.lines = lines
	sp.left.scroll = 0
}

// SetRightContent loads content into the right pane.
func (sp *SplitPane) SetRightContent(title string, lines []string) {
	sp.right.title = title
	sp.right.lines = lines
	sp.right.scroll = 0
}

// focusedPane returns a pointer to the currently focused pane.
func (sp *SplitPane) focusedPane() *paneState {
	if sp.focus == 0 {
		return &sp.left
	}
	return &sp.right
}

// visibleHeight returns the number of content lines that fit in one pane.
func (sp *SplitPane) visibleHeight() int {
	// Total height minus: top border (1) + title row (1) + separator (1)
	// + bottom border (1) + help footer (1) + padding (2) = 7 lines of chrome.
	h := sp.height - 7
	if h < 1 {
		h = 1
	}
	return h
}

// Update handles key events while the split pane is active.
func (sp SplitPane) Update(msg tea.Msg) (SplitPane, tea.Cmd) {
	if !sp.active {
		return sp, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			sp.active = false

		case "tab":
			sp.focus = (sp.focus + 1) % 2

		case "down", "j":
			pane := sp.focusedPane()
			maxScroll := len(pane.lines) - sp.visibleHeight()
			if maxScroll < 0 {
				maxScroll = 0
			}
			if pane.scroll < maxScroll {
				pane.scroll++
			}

		case "up", "k":
			pane := sp.focusedPane()
			if pane.scroll > 0 {
				pane.scroll--
			}

		case "pgdown", "ctrl+d":
			pane := sp.focusedPane()
			jump := sp.visibleHeight() / 2
			if jump < 1 {
				jump = 1
			}
			maxScroll := len(pane.lines) - sp.visibleHeight()
			if maxScroll < 0 {
				maxScroll = 0
			}
			pane.scroll += jump
			if pane.scroll > maxScroll {
				pane.scroll = maxScroll
			}

		case "pgup", "ctrl+u":
			pane := sp.focusedPane()
			jump := sp.visibleHeight() / 2
			if jump < 1 {
				jump = 1
			}
			pane.scroll -= jump
			if pane.scroll < 0 {
				pane.scroll = 0
			}
		}
	}
	return sp, nil
}

// View renders the full split pane overlay.
func (sp SplitPane) View() string {
	if !sp.active {
		return ""
	}

	// Divider takes 1 column plus 2 columns of padding around it.
	const dividerWidth = 3
	// Border + padding on each side: left border (1) + pad (1) + pad (1) + right border (1) = 4.
	const outerChrome = 4

	totalInner := sp.width - outerChrome - dividerWidth
	if totalInner < 10 {
		totalInner = 10
	}
	paneWidth := totalInner / 2

	visH := sp.visibleHeight()

	// --- render individual panes ---
	leftView := sp.renderPane(sp.left, paneWidth, visH, sp.focus == 0)
	rightView := sp.renderPane(sp.right, paneWidth, visH, sp.focus == 1)

	// --- vertical divider ---
	dividerStyle := lipgloss.NewStyle().Foreground(surface0)
	dividerLines := make([]string, visH+2) // +2 for title row + separator
	for i := range dividerLines {
		dividerLines[i] = dividerStyle.Render(" │ ")
	}
	divider := strings.Join(dividerLines, "\n")

	// --- combine panes ---
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftView, divider, rightView)

	// --- footer ---
	footer := DimStyle.Render("  Tab: switch pane  j/k: scroll  Ctrl+D/U: page  Esc: close")

	content := body + "\n" + footer

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Width(sp.width - 2).
		Height(sp.height - 2).
		Padding(0, 1).
		Background(mantle)

	return border.Render(content)
}

// renderPane renders one side of the split view.
func (sp SplitPane) renderPane(pane paneState, width, visH int, focused bool) string {
	var b strings.Builder

	// --- title ---
	titleStr := pane.title
	if titleStr == "" {
		titleStr = "(empty)"
	}
	maxTitleLen := width - 4
	if maxTitleLen < 5 {
		maxTitleLen = 5
	}
	if len(titleStr) > maxTitleLen {
		titleStr = titleStr[:maxTitleLen-3] + "..."
	}

	var titleRendered string
	if focused {
		titleRendered = lipgloss.NewStyle().
			Foreground(mauve).
			Bold(true).
			Render(titleStr)
	} else {
		titleRendered = lipgloss.NewStyle().
			Foreground(overlay0).
			Render(titleStr)
	}
	b.WriteString(titleRendered)
	b.WriteString("\n")

	// --- separator under title ---
	sepColor := surface0
	if focused {
		sepColor = mauve
	}
	sep := lipgloss.NewStyle().Foreground(sepColor).Render(strings.Repeat("─", width))
	b.WriteString(sep)
	b.WriteString("\n")

	// --- content lines ---
	end := pane.scroll + visH
	if end > len(pane.lines) {
		end = len(pane.lines)
	}

	lineCount := 0
	for i := pane.scroll; i < end; i++ {
		line := pane.lines[i]
		// Truncate long lines to fit pane width.
		if len(line) > width {
			line = line[:width]
		}
		b.WriteString(NormalItemStyle.Render(line))
		lineCount++
		if lineCount < visH {
			b.WriteString("\n")
		}
	}

	// Pad remaining empty lines so both panes have equal height.
	for lineCount < visH {
		if lineCount > 0 {
			b.WriteString("\n")
		}
		b.WriteString("")
		lineCount++
	}

	// --- scroll indicator ---
	if len(pane.lines) > visH {
		pos := ""
		if pane.scroll == 0 {
			pos = "TOP"
		} else if pane.scroll >= len(pane.lines)-visH {
			pos = "END"
		} else {
			pct := (pane.scroll * 100) / (len(pane.lines) - visH)
			pos = fmt.Sprintf("%d%%", pct)
		}
		indicator := DimStyle.Render(fmt.Sprintf(" %s (%d lines)", pos, len(pane.lines)))
		// Replace the last line with indicator appended.
		b.WriteString("\n")
		b.WriteString(indicator)
	}

	return lipgloss.NewStyle().Width(width).Render(b.String())
}
