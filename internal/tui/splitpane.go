package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// splitPanePickMsg is sent when a note is selected in the split pane picker.
type splitPanePickMsg struct {
	notePath string
}

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

	// Note picker state
	picking       bool
	pickQuery     string
	allNotes      []string
	filteredNotes []string
	pickCursor    int
	rightNotePath string
}

// NewSplitPane returns a SplitPane in its default (inactive) state.
func NewSplitPane() SplitPane {
	return SplitPane{}
}

// IsActive reports whether the split pane overlay is currently visible.
func (sp *SplitPane) IsActive() bool {
	return sp.active
}

// Open activates the split pane overlay and enters note picker mode.
func (sp *SplitPane) Open() {
	sp.active = true
	sp.focus = 0
	sp.left.scroll = 0
	sp.right.scroll = 0
	sp.right.title = ""
	sp.right.lines = nil
	sp.rightNotePath = ""
	sp.openPicker()
}

// Close deactivates the split pane overlay.
func (sp *SplitPane) Close() {
	sp.active = false
	sp.picking = false
}

// SetSize updates the available terminal dimensions.
func (sp *SplitPane) SetSize(width, height int) {
	sp.width = width
	sp.height = height
}

// SetNotes provides the list of vault note paths for the picker.
func (sp *SplitPane) SetNotes(notes []string) {
	sp.allNotes = notes
	sp.filterNotes()
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

// GetRightNotePath returns the path of the note selected for the right pane.
func (sp *SplitPane) GetRightNotePath() string {
	return sp.rightNotePath
}

// openPicker enters note picker mode for the right pane.
func (sp *SplitPane) openPicker() {
	sp.picking = true
	sp.pickQuery = ""
	sp.pickCursor = 0
	sp.filterNotes()
}

// filterNotes updates filteredNotes based on the current pickQuery.
func (sp *SplitPane) filterNotes() {
	if sp.pickQuery == "" {
		sp.filteredNotes = make([]string, len(sp.allNotes))
		copy(sp.filteredNotes, sp.allNotes)
		return
	}
	query := strings.ToLower(sp.pickQuery)
	filtered := make([]string, 0)
	for _, note := range sp.allNotes {
		if strings.Contains(strings.ToLower(note), query) {
			filtered = append(filtered, note)
		}
	}
	sp.filteredNotes = filtered
	// Clamp cursor to valid range
	if sp.pickCursor >= len(sp.filteredNotes) {
		sp.pickCursor = len(sp.filteredNotes) - 1
	}
	if sp.pickCursor < 0 {
		sp.pickCursor = 0
	}
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
		if sp.picking {
			return sp.updatePicker(msg)
		}
		return sp.updateNormal(msg)
	}
	return sp, nil
}

// updatePicker handles key events in picker mode.
func (sp SplitPane) updatePicker(msg tea.KeyMsg) (SplitPane, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if sp.pickQuery != "" {
			// Clear the search query first
			sp.pickQuery = ""
			sp.pickCursor = 0
			sp.filterNotes()
		} else if sp.rightNotePath != "" {
			// If a note was already picked, exit picker back to split view
			sp.picking = false
		} else {
			// No note picked yet and no query, close the whole overlay
			sp.active = false
			sp.picking = false
		}

	case "enter":
		if len(sp.filteredNotes) > 0 && sp.pickCursor < len(sp.filteredNotes) {
			selected := sp.filteredNotes[sp.pickCursor]
			sp.rightNotePath = selected
			sp.picking = false
			return sp, func() tea.Msg {
				return splitPanePickMsg{notePath: selected}
			}
		}

	case "up":
		if sp.pickCursor > 0 {
			sp.pickCursor--
		}

	case "down":
		if sp.pickCursor < len(sp.filteredNotes)-1 {
			sp.pickCursor++
		}

	case "backspace":
		if len(sp.pickQuery) > 0 {
			sp.pickQuery = sp.pickQuery[:len(sp.pickQuery)-1]
			sp.pickCursor = 0
			sp.filterNotes()
		}

	default:
		// Type characters into search query
		key := msg.String()
		if len(key) == 1 && key[0] >= 32 && key[0] <= 126 {
			sp.pickQuery += key
			sp.pickCursor = 0
			sp.filterNotes()
		}
	}
	return sp, nil
}

// updateNormal handles key events in normal (dual-pane viewing) mode.
func (sp SplitPane) updateNormal(msg tea.KeyMsg) (SplitPane, tea.Cmd) {
	switch msg.String() {
	case "esc":
		sp.active = false

	case "tab":
		sp.focus = (sp.focus + 1) % 2

	case "p":
		sp.openPicker()

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
	var rightView string
	if sp.picking {
		rightView = sp.renderPicker(paneWidth, visH)
	} else {
		rightView = sp.renderPane(sp.right, paneWidth, visH, sp.focus == 1)
	}

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
	var footerText string
	if sp.picking {
		footerText = "  Type to search  Up/Down: navigate  Enter: select  Esc: cancel"
	} else {
		footerText = "  Tab: switch pane  j/k: scroll  Ctrl+D/U: page  p: pick note  Esc: close"
	}
	footer := DimStyle.Render(footerText)

	content := body + "\n" + footer

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(OverlayBorderColor).
		Width(sp.width - 2).
		Height(sp.height - 2).
		Padding(0, 1).
		Background(mantle)

	return border.Render(content)
}

// renderPicker renders the note picker UI for the right pane.
func (sp SplitPane) renderPicker(width, visH int) string {
	var b strings.Builder

	// --- title ---
	titleRendered := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("Select Note")
	b.WriteString(titleRendered)
	b.WriteString("\n")

	// --- separator ---
	sep := lipgloss.NewStyle().Foreground(mauve).Render(strings.Repeat("─", width))
	b.WriteString(sep)
	b.WriteString("\n")

	// --- search input ---
	prompt := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("> ")
	queryText := sp.pickQuery
	maxQ := width - 4
	if maxQ > 0 && len(queryText) > maxQ {
		queryText = queryText[len(queryText)-maxQ:]
	}
	cursor := lipgloss.NewStyle().Foreground(mauve).Render("_")
	searchLine := prompt + NormalItemStyle.Render(queryText) + cursor
	b.WriteString(searchLine)
	b.WriteString("\n")

	// --- note list ---
	// Reserve 1 line for search input from visible height
	listHeight := visH - 1
	if listHeight < 1 {
		listHeight = 1
	}

	// Determine visible window of filtered notes based on cursor position
	startIdx := 0
	if sp.pickCursor >= listHeight {
		startIdx = sp.pickCursor - listHeight + 1
	}
	endIdx := startIdx + listHeight
	if endIdx > len(sp.filteredNotes) {
		endIdx = len(sp.filteredNotes)
	}

	lineCount := 0
	for i := startIdx; i < endIdx; i++ {
		note := sp.filteredNotes[i]
		// Truncate to fit
		display := note
		if len(display) > width-2 {
			display = display[:width-5] + "..."
		}

		if i == sp.pickCursor {
			line := SelectedItemStyle.Render("> " + display)
			b.WriteString(line)
		} else {
			line := NormalItemStyle.Render("  " + display)
			b.WriteString(line)
		}
		lineCount++
		if lineCount < listHeight {
			b.WriteString("\n")
		}
	}

	// Pad empty lines
	for lineCount < listHeight {
		if lineCount > 0 {
			b.WriteString("\n")
		}
		b.WriteString("")
		lineCount++
	}

	// Show count
	if len(sp.filteredNotes) > 0 {
		countText := fmt.Sprintf(" %d/%d notes", len(sp.filteredNotes), len(sp.allNotes))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render(countText))
	} else if len(sp.allNotes) > 0 {
		b.WriteString("\n")
		b.WriteString(DimStyle.Render(" No matches"))
	}

	return lipgloss.NewStyle().Width(width).Render(b.String())
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
	titleStr = TruncateDisplay(titleStr, maxTitleLen)

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
