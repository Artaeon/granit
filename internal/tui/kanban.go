package tui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

// KanbanCard represents a single task card on the Kanban board.
type KanbanCard struct {
	Text   string
	Source string // note path where this task lives
	Line   int    // line number in source note
	Done   bool
}

// KanbanColumn represents a column on the Kanban board.
type KanbanColumn struct {
	Title string
	Cards []KanbanCard
}

// Kanban is an overlay component that displays tasks from vault notes as a
// Kanban board with Backlog, In Progress, and Done columns.
type Kanban struct {
	active bool
	width  int
	height int

	columns    []KanbanColumn // default: "Backlog", "In Progress", "Done"
	colCursor  int            // which column is selected
	cardCursor int            // which card in the column is selected

	allCards []KanbanCard // all parsed cards

	// Drag state
	dragging bool
	dragCard *KanbanCard

	// Toggle result — consumed by the host after Update
	pendingToggle  bool
	toggleNotePath string
	toggleLine     int
	toggleNewDone  bool
}

// NewKanban creates a new Kanban overlay with the three default columns.
func NewKanban() Kanban {
	return Kanban{
		columns: []KanbanColumn{
			{Title: "Backlog"},
			{Title: "In Progress"},
			{Title: "Done"},
		},
	}
}

// IsActive reports whether the Kanban overlay is currently displayed.
func (kb *Kanban) IsActive() bool { return kb.active }

// Open activates the Kanban overlay.
func (kb *Kanban) Open() {
	kb.active = true
	kb.colCursor = 0
	kb.cardCursor = 0
	kb.dragging = false
	kb.dragCard = nil
	kb.pendingToggle = false
}

// Close deactivates the Kanban overlay.
func (kb *Kanban) Close() { kb.active = false }

// SetSize stores the available terminal dimensions for rendering.
func (kb *Kanban) SetSize(w, h int) {
	kb.width = w
	kb.height = h
}

// SetTasks parses all task items from the provided note contents and
// distributes them into the three default columns.
func (kb *Kanban) SetTasks(noteContents map[string]string) {
	kb.allCards = nil

	// Sort note paths for deterministic ordering.
	paths := make([]string, 0, len(noteContents))
	for p := range noteContents {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, path := range paths {
		content := noteContents[path]
		lines := strings.Split(content, "\n")
		for lineIdx, line := range lines {
			trimmed := strings.TrimSpace(line)

			var card KanbanCard
			if strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
				card = KanbanCard{
					Text:   strings.TrimSpace(trimmed[6:]),
					Source: path,
					Line:   lineIdx + 1, // 1-based
					Done:   true,
				}
			} else if strings.HasPrefix(trimmed, "- [ ] ") {
				card = KanbanCard{
					Text:   strings.TrimSpace(trimmed[6:]),
					Source: path,
					Line:   lineIdx + 1,
					Done:   false,
				}
			} else {
				continue
			}
			kb.allCards = append(kb.allCards, card)
		}
	}

	// Distribute cards into columns.
	kb.columns[0].Cards = nil // Backlog
	kb.columns[1].Cards = nil // In Progress
	kb.columns[2].Cards = nil // Done

	for _, card := range kb.allCards {
		switch {
		case card.Done:
			kb.columns[2].Cards = append(kb.columns[2].Cards, card)
		case kbHasWipTag(card.Text):
			kb.columns[1].Cards = append(kb.columns[1].Cards, card)
		default:
			kb.columns[0].Cards = append(kb.columns[0].Cards, card)
		}
	}

	// Clamp cursors.
	kb.kbClampCursors()
}

// GetToggleResult returns a pending toggle action, if any.
// The caller should update the source note content accordingly.
func (kb *Kanban) GetToggleResult() (notePath string, line int, newDone bool, ok bool) {
	if !kb.pendingToggle {
		return "", 0, false, false
	}
	kb.pendingToggle = false
	return kb.toggleNotePath, kb.toggleLine, kb.toggleNewDone, true
}

// ---------------------------------------------------------------------------
// Update (value receiver per project convention)
// ---------------------------------------------------------------------------

func (kb Kanban) Update(msg tea.Msg) (Kanban, tea.Cmd) {
	if !kb.active {
		return kb, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			kb.active = false
			return kb, nil

		// Column navigation
		case "left", "h":
			if kb.colCursor > 0 {
				kb.colCursor--
				kb.kbClampCardCursor()
			}
		case "right", "l":
			if kb.colCursor < len(kb.columns)-1 {
				kb.colCursor++
				kb.kbClampCardCursor()
			}

		// Card navigation
		case "up", "k":
			if kb.cardCursor > 0 {
				kb.cardCursor--
			}
		case "down", "j":
			col := kb.columns[kb.colCursor]
			if kb.cardCursor < len(col.Cards)-1 {
				kb.cardCursor++
			}

		// Move card to next column
		case "m", "enter":
			kb.kbMoveCardForward()

		// Move card to previous column
		case "M":
			kb.kbMoveCardBackward()

		// Toggle done state
		case "x":
			kb.kbToggleDone()
		}
	}

	return kb, nil
}

// ---------------------------------------------------------------------------
// View (value receiver per project convention)
// ---------------------------------------------------------------------------

func (kb Kanban) View() string {
	// Determine board dimensions.
	boardWidth := kb.width * 3 / 4
	if boardWidth < 70 {
		boardWidth = 70
	}
	if boardWidth > 130 {
		boardWidth = 130
	}
	boardHeight := kb.height * 3 / 4
	if boardHeight < 16 {
		boardHeight = 16
	}

	// Inner content width (subtract border + padding: 2 border + 4 padding = 6)
	innerWidth := boardWidth - 6
	if innerWidth < 60 {
		innerWidth = 60
	}

	numCols := len(kb.columns)
	// Column widths — divide inner width evenly, accounting for dividers
	dividerWidth := 3 // " │ "
	totalDividers := (numCols - 1) * dividerWidth
	colWidth := (innerWidth - totalDividers) / numCols
	if colWidth < 18 {
		colWidth = 18
	}

	// Visible card slots per column
	visibleCards := (boardHeight - 10) / 2 // each card takes 2 lines
	if visibleCards < 2 {
		visibleCards = 2
	}

	// Column colors
	colColors := []lipgloss.Color{blue, yellow, green}
	colIcons := []string{"○", "◉", "●"}

	// Render each column
	renderedCols := make([]string, numCols)
	for ci, col := range kb.columns {
		renderedCols[ci] = kb.kbRenderColumn(ci, col, colWidth, visibleCards, colColors[ci], colIcons[ci])
	}

	// Join columns with dividers
	var colStrings []string
	for ci, rs := range renderedCols {
		colStrings = append(colStrings, rs)
		if ci < numCols-1 {
			// Vertical divider matching column height
			lines := strings.Split(rs, "\n")
			divLines := make([]string, len(lines))
			divStyle := lipgloss.NewStyle().Foreground(surface1)
			for i := range divLines {
				divLines[i] = divStyle.Render(" │ ")
			}
			colStrings = append(colStrings, strings.Join(divLines, "\n"))
		}
	}
	board := lipgloss.JoinHorizontal(lipgloss.Top, colStrings...)

	// Build final content
	var b strings.Builder

	// Title bar
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render("  Kanban Board"))

	// Stats
	totalCards := len(kb.allCards)
	doneCount := 0
	for _, c := range kb.allCards {
		if c.Done {
			doneCount++
		}
	}
	statsStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString(statsStyle.Render("  " + kbItoa(totalCards) + " tasks, " + kbItoa(doneCount) + " done"))

	// Progress indicator
	if totalCards > 0 {
		pct := doneCount * 100 / totalCards
		pctStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
		b.WriteString(pctStyle.Render("  " + kbItoa(pct) + "%"))
	}
	b.WriteString("\n")

	// Top rule
	ruleStyle := lipgloss.NewStyle().Foreground(surface1)
	b.WriteString(ruleStyle.Render("  " + strings.Repeat("─", innerWidth-4)))
	b.WriteString("\n\n")

	// Board
	b.WriteString(board)
	b.WriteString("\n\n")

	// Bottom rule
	b.WriteString(ruleStyle.Render("  " + strings.Repeat("─", innerWidth-4)))
	b.WriteString("\n")

	// Footer with keybinds
	footerStyle := lipgloss.NewStyle().Foreground(overlay0)
	keyStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	sepStyle := lipgloss.NewStyle().Foreground(surface1)
	sep := sepStyle.Render(" │ ")

	b.WriteString("  ")
	b.WriteString(keyStyle.Render("←→") + footerStyle.Render(" column") + sep)
	b.WriteString(keyStyle.Render("↑↓") + footerStyle.Render(" card") + sep)
	b.WriteString(keyStyle.Render("m") + footerStyle.Render(" move →") + sep)
	b.WriteString(keyStyle.Render("M") + footerStyle.Render(" move ←") + sep)
	b.WriteString(keyStyle.Render("x") + footerStyle.Render(" toggle") + sep)
	b.WriteString(keyStyle.Render("Esc") + footerStyle.Render(" close"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(boardWidth).
		Background(mantle)

	return border.Render(b.String())
}

// ---------------------------------------------------------------------------
// Internal helpers (kb-prefixed to avoid collisions)
// ---------------------------------------------------------------------------

// kbRenderColumn renders a single Kanban column as a string block.
func (kb Kanban) kbRenderColumn(colIdx int, col KanbanColumn, width, visibleCards int, colColor lipgloss.Color, icon string) string {
	var b strings.Builder

	isActiveCol := colIdx == kb.colCursor

	// Column header
	titleStyle := lipgloss.NewStyle().Foreground(colColor).Bold(true)
	countStyle := lipgloss.NewStyle().Foreground(surface2)

	indicator := "  "
	if isActiveCol {
		indicator = lipgloss.NewStyle().Foreground(colColor).Bold(true).Render("▸ ")
	}

	b.WriteString(indicator)
	b.WriteString(lipgloss.NewStyle().Foreground(colColor).Render(icon))
	b.WriteString(" ")
	b.WriteString(titleStyle.Render(col.Title))
	b.WriteString(countStyle.Render(" " + kbItoa(len(col.Cards))))
	b.WriteString("\n")

	// Column underline
	underColor := surface1
	if isActiveCol {
		underColor = colColor
	}
	b.WriteString("  ")
	b.WriteString(lipgloss.NewStyle().Foreground(underColor).Render(strings.Repeat("─", width-4)))
	b.WriteString("\n")

	if len(col.Cards) == 0 {
		emptyStyle := lipgloss.NewStyle().Foreground(surface2).Italic(true)
		b.WriteString("  ")
		b.WriteString(emptyStyle.Render("No tasks"))
		b.WriteString("\n")
		// Pad remaining lines
		for i := 1; i < visibleCards*2; i++ {
			b.WriteString("\n")
		}
	} else {
		// Compute scroll offset for the active column
		scrollOffset := 0
		if isActiveCol && kb.cardCursor >= visibleCards {
			scrollOffset = kb.cardCursor - visibleCards + 1
		}

		end := scrollOffset + visibleCards
		if end > len(col.Cards) {
			end = len(col.Cards)
		}

		linesUsed := 0
		for i := scrollOffset; i < end; i++ {
			card := col.Cards[i]
			isSelected := isActiveCol && i == kb.cardCursor

			cardText := kbTruncate(card.Text, width-6)
			sourceName := kbBaseName(card.Source)
			sourceStr := kbTruncate(sourceName, width-8)

			if isSelected {
				// Selected card — highlighted background
				checkIcon := "○"
				checkColor := colColor
				if card.Done {
					checkIcon = "●"
					checkColor = green
				}

				selBg := lipgloss.NewStyle().
					Background(surface0).
					Width(width - 2)

				checkSt := lipgloss.NewStyle().
					Background(surface0).
					Foreground(checkColor).
					Bold(true)

				textSt := lipgloss.NewStyle().
					Background(surface0).
					Foreground(text).
					Bold(true)

				srcSt := lipgloss.NewStyle().
					Background(surface0).
					Foreground(overlay0)

				_ = selBg
				line1 := "  " + checkSt.Render(checkIcon) + " " + textSt.Render(cardText)
				line2 := "    " + srcSt.Render(sourceStr)

				b.WriteString(lipgloss.NewStyle().Background(surface0).Width(width-2).Render(line1))
				b.WriteString("\n")
				b.WriteString(lipgloss.NewStyle().Background(surface0).Width(width-2).Render(line2))
				b.WriteString("\n")
			} else {
				// Normal card
				checkIcon := "○"
				checkColor := surface2
				if card.Done {
					checkIcon = "●"
					checkColor = green
				}

				checkSt := lipgloss.NewStyle().Foreground(checkColor)
				textSt := lipgloss.NewStyle().Foreground(text)
				srcSt := lipgloss.NewStyle().Foreground(surface2)

				b.WriteString("  " + checkSt.Render(checkIcon) + " " + textSt.Render(cardText))
				b.WriteString("\n")
				b.WriteString("    " + srcSt.Render(sourceStr))
				b.WriteString("\n")
			}
			linesUsed += 2
		}

		// Scroll indicator
		if len(col.Cards) > visibleCards {
			remaining := len(col.Cards) - end
			if remaining > 0 {
				moreStyle := lipgloss.NewStyle().Foreground(surface2).Italic(true)
				b.WriteString("  " + moreStyle.Render("+" + kbItoa(remaining) + " more"))
				b.WriteString("\n")
				linesUsed++
			}
		}

		// Pad remaining vertical space
		for linesUsed < visibleCards*2 {
			b.WriteString("\n")
			linesUsed++
		}
	}

	return lipgloss.NewStyle().Width(width).Render(b.String())
}

// kbHasWipTag checks whether a task text contains a #wip or #doing tag.
func kbHasWipTag(text string) bool {
	lower := strings.ToLower(text)
	for _, tag := range []string{"#wip", "#doing"} {
		idx := strings.Index(lower, tag)
		if idx < 0 {
			continue
		}
		// Make sure the tag is word-bounded on the right.
		end := idx + len(tag)
		if end >= len(lower) {
			return true
		}
		ch := lower[end]
		if ch == ' ' || ch == '\t' || ch == ',' || ch == '.' || ch == ')' || ch == ']' {
			return true
		}
	}
	return false
}

// kbTruncate truncates s to maxLen characters, appending "..." if truncated.
func kbTruncate(s string, maxLen int) string {
	if maxLen < 4 {
		maxLen = 4
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// kbBaseName returns the file name from a path, without the .md extension.
func kbBaseName(path string) string {
	idx := strings.LastIndex(path, "/")
	name := path
	if idx >= 0 {
		name = path[idx+1:]
	}
	idx = strings.LastIndex(name, "\\")
	if idx >= 0 {
		name = name[idx+1:]
	}
	if strings.HasSuffix(strings.ToLower(name), ".md") {
		name = name[:len(name)-3]
	}
	return name
}

// kbItoa converts an int to a decimal string without importing strconv.
func kbItoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	digits := make([]byte, 0, 10)
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}

// kbClampCursors ensures both colCursor and cardCursor are within bounds.
func (kb *Kanban) kbClampCursors() {
	if kb.colCursor < 0 {
		kb.colCursor = 0
	}
	if kb.colCursor >= len(kb.columns) {
		kb.colCursor = len(kb.columns) - 1
	}
	kb.kbClampCardCursor()
}

// kbClampCardCursor ensures cardCursor is within bounds for the current column.
func (kb *Kanban) kbClampCardCursor() {
	col := kb.columns[kb.colCursor]
	if kb.cardCursor >= len(col.Cards) {
		kb.cardCursor = len(col.Cards) - 1
	}
	if kb.cardCursor < 0 {
		kb.cardCursor = 0
	}
}

// kbMoveCardForward moves the selected card from the current column to the next one.
func (kb *Kanban) kbMoveCardForward() {
	if kb.colCursor >= len(kb.columns)-1 {
		return
	}
	col := &kb.columns[kb.colCursor]
	if len(col.Cards) == 0 || kb.cardCursor >= len(col.Cards) {
		return
	}

	card := col.Cards[kb.cardCursor]
	col.Cards = append(col.Cards[:kb.cardCursor], col.Cards[kb.cardCursor+1:]...)
	next := &kb.columns[kb.colCursor+1]
	next.Cards = append(next.Cards, card)

	if kb.cardCursor >= len(col.Cards) && kb.cardCursor > 0 {
		kb.cardCursor--
	}
}

// kbMoveCardBackward moves the selected card from the current column to the previous one.
func (kb *Kanban) kbMoveCardBackward() {
	if kb.colCursor <= 0 {
		return
	}
	col := &kb.columns[kb.colCursor]
	if len(col.Cards) == 0 || kb.cardCursor >= len(col.Cards) {
		return
	}

	card := col.Cards[kb.cardCursor]
	col.Cards = append(col.Cards[:kb.cardCursor], col.Cards[kb.cardCursor+1:]...)
	prev := &kb.columns[kb.colCursor-1]
	prev.Cards = append(prev.Cards, card)

	if kb.cardCursor >= len(col.Cards) && kb.cardCursor > 0 {
		kb.cardCursor--
	}
}

// kbToggleDone toggles the Done state of the selected card and records the
// toggle result so the host can update the source note.
func (kb *Kanban) kbToggleDone() {
	col := &kb.columns[kb.colCursor]
	if len(col.Cards) == 0 || kb.cardCursor >= len(col.Cards) {
		return
	}

	card := &col.Cards[kb.cardCursor]
	card.Done = !card.Done

	kb.pendingToggle = true
	kb.toggleNotePath = card.Source
	kb.toggleLine = card.Line
	kb.toggleNewDone = card.Done
}
