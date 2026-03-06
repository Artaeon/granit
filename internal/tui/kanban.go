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
	pendingToggle    bool
	toggleNotePath   string
	toggleLine       int
	toggleNewDone    bool
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
	if boardWidth < 60 {
		boardWidth = 60
	}
	if boardWidth > 120 {
		boardWidth = 120
	}
	boardHeight := kb.height * 3 / 4
	if boardHeight < 16 {
		boardHeight = 16
	}

	// Column width: divide evenly, minus padding/dividers.
	numCols := len(kb.columns)
	colWidth := (boardWidth - 6 - (numCols - 1)) / numCols // 6 = border+padding
	if colWidth < 16 {
		colWidth = 16
	}

	// Visible card slots per column (subtract header, footer, border space).
	visibleCards := boardHeight - 10
	if visibleCards < 3 {
		visibleCards = 3
	}

	// Render each column.
	renderedCols := make([]string, numCols)
	for ci, col := range kb.columns {
		renderedCols[ci] = kb.kbRenderColumn(ci, col, colWidth, visibleCards)
	}

	// Join columns side by side with vertical dividers.
	divider := lipgloss.NewStyle().Foreground(surface1).Render(
		strings.Repeat("│\n", visibleCards+4))
	dividerLines := strings.Split(divider, "\n")
	_ = dividerLines

	// Use lipgloss.JoinHorizontal to combine columns.
	dividerStr := lipgloss.NewStyle().Foreground(surface1).Render("│")
	_ = dividerStr

	// Build the columns joined with a thin divider.
	var colStrings []string
	for ci, rs := range renderedCols {
		colStrings = append(colStrings, rs)
		if ci < numCols-1 {
			// Build a vertical divider as tall as the column.
			lines := strings.Split(rs, "\n")
			divLines := make([]string, len(lines))
			for i := range divLines {
				divLines[i] = lipgloss.NewStyle().Foreground(surface1).Render(" │ ")
			}
			colStrings = append(colStrings, strings.Join(divLines, "\n"))
		}
	}
	board := lipgloss.JoinHorizontal(lipgloss.Top, colStrings...)

	// Title
	var b strings.Builder
	titleText := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("Kanban Board")
	totalCards := len(kb.allCards)
	doneCount := 0
	for _, c := range kb.allCards {
		if c.Done {
			doneCount++
		}
	}
	statsText := lipgloss.NewStyle().Foreground(overlay0).Render(
		" (" + kbItoa(totalCards) + " tasks, " + kbItoa(doneCount) + " done)")
	b.WriteString("  " + titleText + statsText)
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(
		"  " + strings.Repeat("─", boardWidth-8)))
	b.WriteString("\n\n")

	b.WriteString(board)

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(
		"  " + strings.Repeat("─", boardWidth-8)))
	b.WriteString("\n")
	footer := "  ←→: column  ↑↓: card  Enter/m: move →  M: move ←  x: toggle  Esc: close"
	b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(footer))

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
func (kb Kanban) kbRenderColumn(colIdx int, col KanbanColumn, width, visibleCards int) string {
	var b strings.Builder

	// Column header with colored title.
	headerColor := blue
	switch colIdx {
	case 0:
		headerColor = blue
	case 1:
		headerColor = yellow
	case 2:
		headerColor = green
	}

	isActiveCol := colIdx == kb.colCursor
	titleStyle := lipgloss.NewStyle().Foreground(headerColor).Bold(true)
	countStr := lipgloss.NewStyle().Foreground(overlay0).Render(
		" [" + kbItoa(len(col.Cards)) + "]")

	colIndicator := "  "
	if isActiveCol {
		colIndicator = lipgloss.NewStyle().Foreground(headerColor).Render("▸ ")
	}

	b.WriteString(colIndicator + titleStyle.Render(col.Title) + countStr)
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(
		"  " + strings.Repeat("─", width-4)))
	b.WriteString("\n")

	if len(col.Cards) == 0 {
		emptyMsg := lipgloss.NewStyle().Foreground(overlay0).Render("  (empty)")
		b.WriteString(emptyMsg)
		b.WriteString("\n")
		// Pad remaining lines.
		for i := 1; i < visibleCards; i++ {
			b.WriteString(strings.Repeat(" ", width))
			b.WriteString("\n")
		}
	} else {
		// Compute scroll offset for the active column.
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

			// Card text.
			cardText := kbTruncate(card.Text, width-6)

			// Source note (show just the file name).
			sourceName := kbBaseName(card.Source)
			sourceStr := kbTruncate(sourceName, width-6)

			if isSelected {
				selectedStyle := lipgloss.NewStyle().
					Background(surface0).
					Foreground(peach).
					Bold(true).
					Width(width - 2)

				// Checkbox prefix.
				check := "○ "
				if card.Done {
					check = "● "
				}

				b.WriteString(selectedStyle.Render("  " + check + cardText))
				b.WriteString("\n")
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Foreground(overlay0).
					Width(width - 2).
					Render("    " + sourceStr))
				b.WriteString("\n")
			} else {
				checkStyle := lipgloss.NewStyle().Foreground(yellow)
				if card.Done {
					checkStyle = lipgloss.NewStyle().Foreground(green)
				}
				check := checkStyle.Render("○")
				if card.Done {
					check = checkStyle.Render("●")
				}

				b.WriteString("  " + check + " " + lipgloss.NewStyle().Foreground(text).Render(cardText))
				b.WriteString("\n")
				b.WriteString("    " + lipgloss.NewStyle().Foreground(overlay0).Render(sourceStr))
				b.WriteString("\n")
			}
			linesUsed += 2
		}

		// Pad any remaining vertical space.
		for linesUsed < visibleCards*2 {
			b.WriteString(strings.Repeat(" ", width))
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
	// Find last slash.
	idx := strings.LastIndex(path, "/")
	name := path
	if idx >= 0 {
		name = path[idx+1:]
	}
	// Also handle backslash for Windows-style paths.
	idx = strings.LastIndex(name, "\\")
	if idx >= 0 {
		name = name[idx+1:]
	}
	// Strip .md extension.
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
	// Reverse.
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
	if len(col.Cards) == 0 {
		return
	}
	if kb.cardCursor >= len(col.Cards) {
		return
	}

	card := col.Cards[kb.cardCursor]
	// Remove from current column.
	col.Cards = append(col.Cards[:kb.cardCursor], col.Cards[kb.cardCursor+1:]...)
	// Add to next column.
	next := &kb.columns[kb.colCursor+1]
	next.Cards = append(next.Cards, card)

	// Clamp cursor in current column.
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
	if len(col.Cards) == 0 {
		return
	}
	if kb.cardCursor >= len(col.Cards) {
		return
	}

	card := col.Cards[kb.cardCursor]
	// Remove from current column.
	col.Cards = append(col.Cards[:kb.cardCursor], col.Cards[kb.cardCursor+1:]...)
	// Add to previous column.
	prev := &kb.columns[kb.colCursor-1]
	prev.Cards = append(prev.Cards, card)

	// Clamp cursor in current column.
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

	// Record toggle for the host to pick up.
	kb.pendingToggle = true
	kb.toggleNotePath = card.Source
	kb.toggleLine = card.Line
	kb.toggleNewDone = card.Done
}
