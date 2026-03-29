package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Alignment constants for table columns.
const (
	alignLeft   = 0
	alignCenter = 1
	alignRight  = 2
)

// TableEditor is an overlay component for visually editing markdown tables.
type TableEditor struct {
	active bool
	width  int
	height int

	// Table data
	headers    []string
	rows       [][]string
	alignments []int // 0=left, 1=center, 2=right

	// Cursor
	curRow  int // -1 = header row
	curCol  int
	editing bool
	editBuf string

	// Source tracking
	startLine int // line in editor where table starts
	endLine   int // line in editor where table ends

	// Scrolling
	scroll int

	// Result tracking
	confirmed bool
}

// NewTableEditor creates a new TableEditor overlay.
func NewTableEditor() TableEditor {
	return TableEditor{}
}

// IsActive returns whether the table editor overlay is currently open.
func (te *TableEditor) IsActive() bool {
	return te.active
}

// Open parses a markdown table from the content around cursorLine and activates the overlay.
func (te *TableEditor) Open(content []string, cursorLine int) {
	if len(content) == 0 {
		return
	}

	// Clamp cursor line
	if cursorLine < 0 {
		cursorLine = 0
	}
	if cursorLine >= len(content) {
		cursorLine = len(content) - 1
	}

	// Check if cursor line is part of a table (contains |)
	if !isTableLine(content[cursorLine]) {
		return
	}

	// Find table boundaries by scanning up and down from the cursor
	te.startLine = cursorLine
	for te.startLine > 0 && isTableLine(content[te.startLine-1]) {
		te.startLine--
	}
	te.endLine = cursorLine
	for te.endLine < len(content)-1 && isTableLine(content[te.endLine+1]) {
		te.endLine++
	}

	// Parse the table lines
	tableLines := content[te.startLine : te.endLine+1]
	if len(tableLines) < 2 {
		// Need at least a header and separator row
		return
	}

	// Parse header row
	te.headers = parseCells(tableLines[0])
	if len(te.headers) == 0 {
		return
	}
	numCols := len(te.headers)

	// Parse separator row (second line) for alignments
	te.alignments = make([]int, numCols)
	if len(tableLines) >= 2 && isSeparatorLine(tableLines[1]) {
		sepCells := parseCells(tableLines[1])
		for i := 0; i < numCols && i < len(sepCells); i++ {
			te.alignments[i] = parseAlignment(sepCells[i])
		}
	}

	// Parse data rows (skip header and separator)
	te.rows = nil
	startIdx := 2
	if len(tableLines) >= 2 && !isSeparatorLine(tableLines[1]) {
		// No separator line — treat everything after header as data
		startIdx = 1
	}
	for i := startIdx; i < len(tableLines); i++ {
		cells := parseCells(tableLines[i])
		// Pad or truncate to match column count
		row := normalizeRow(cells, numCols)
		te.rows = append(te.rows, row)
	}

	te.curRow = -1
	te.curCol = 0
	te.editing = false
	te.editBuf = ""
	te.confirmed = false
	te.scroll = 0
	te.active = true
}

// OpenNew creates a blank 3-column, 2-row table for insertion at the given line.
func (te *TableEditor) OpenNew(insertLine int) {
	te.headers = []string{"Column 1", "Column 2", "Column 3"}
	te.alignments = []int{alignLeft, alignLeft, alignLeft}
	te.rows = [][]string{{"", "", ""}, {"", "", ""}}
	te.startLine = insertLine
	te.endLine = insertLine - 1 // sentinel: endLine < startLine means "insert mode"
	te.curRow = -1
	te.curCol = 0
	te.editing = false
	te.confirmed = false
	te.scroll = 0
	te.active = true
}

// Close deactivates the table editor overlay.
func (te *TableEditor) Close() {
	te.active = false
	te.editing = false
}

// visibleDataRows returns the number of data rows that can be displayed
// in the overlay given the current height.
func (te *TableEditor) visibleDataRows() int {
	overhead := 17 // title, borders, header, separator, alignment row, help, padding
	avail := te.height - overhead
	if avail < 3 {
		avail = 3
	}
	return avail
}

// SetSize sets the available width and height for the overlay.
func (te *TableEditor) SetSize(w, h int) {
	te.width = w
	te.height = h
}

// Update handles input messages for the table editor. Uses a value receiver
// to match the overlay pattern used elsewhere in this project.
func (te TableEditor) Update(msg tea.Msg) (TableEditor, tea.Cmd) {
	if !te.active {
		return te, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()

		if te.editing {
			return te.updateEditing(key, msg)
		}
		return te.updateNavigating(key)
	}

	return te, nil
}

func (te TableEditor) updateEditing(key string, msg tea.KeyMsg) (TableEditor, tea.Cmd) {
	switch key {
	case "enter":
		// Confirm edit and write buffer back to cell
		te.setCellValue(te.curRow, te.curCol, te.editBuf)
		te.editing = false
		te.editBuf = ""
	case "esc":
		// Cancel edit
		te.editing = false
		te.editBuf = ""
	case "backspace":
		if len(te.editBuf) > 0 {
			te.editBuf = te.editBuf[:len(te.editBuf)-1]
		}
	case "left", "right", "up", "down", "tab", "shift+tab":
		// Ignore navigation keys while editing
	default:
		// Append typed runes
		for _, r := range msg.Runes {
			te.editBuf += string(r)
		}
	}
	return te, nil
}

func (te TableEditor) updateNavigating(key string) (TableEditor, tea.Cmd) {
	numCols := len(te.headers)
	numRows := len(te.rows)

	switch key {
	case "esc", "q":
		te.active = false
		return te, nil

	case "up":
		if te.curRow > -1 {
			te.curRow--
		}
	case "down":
		if te.curRow < numRows-1 {
			te.curRow++
		}
	case "left":
		if te.curCol > 0 {
			te.curCol--
		}
	case "right":
		if te.curCol < numCols-1 {
			te.curCol++
		}

	case "tab":
		// Next cell
		te.curCol++
		if te.curCol >= numCols {
			te.curCol = 0
			te.curRow++
			if te.curRow >= numRows {
				te.curRow = numRows - 1
				te.curCol = numCols - 1
			}
		}
	case "shift+tab":
		// Previous cell
		te.curCol--
		if te.curCol < 0 {
			te.curCol = numCols - 1
			te.curRow--
			if te.curRow < -1 {
				te.curRow = -1
				te.curCol = 0
			}
		}

	case "enter":
		// Start editing current cell
		te.editing = true
		te.editBuf = te.getCellValue(te.curRow, te.curCol)

	case "ctrl+s":
		// Confirm and output markdown
		te.confirmed = true
		te.active = false
		return te, nil

	case "a":
		// Add row after current
		newRow := make([]string, numCols)
		insertIdx := te.curRow + 1
		if insertIdx < 0 {
			insertIdx = 0
		}
		if insertIdx > numRows {
			insertIdx = numRows
		}
		newRows := make([][]string, 0, numRows+1)
		newRows = append(newRows, te.rows[:insertIdx]...)
		newRows = append(newRows, newRow)
		newRows = append(newRows, te.rows[insertIdx:]...)
		te.rows = newRows
		te.curRow = insertIdx

	case "A":
		// Add column after current
		insertCol := te.curCol + 1
		if insertCol > numCols {
			insertCol = numCols
		}
		// Expand headers
		newHeaders := make([]string, 0, numCols+1)
		newHeaders = append(newHeaders, te.headers[:insertCol]...)
		newHeaders = append(newHeaders, "")
		newHeaders = append(newHeaders, te.headers[insertCol:]...)
		te.headers = newHeaders
		// Expand alignments
		newAligns := make([]int, 0, numCols+1)
		newAligns = append(newAligns, te.alignments[:insertCol]...)
		newAligns = append(newAligns, alignLeft)
		newAligns = append(newAligns, te.alignments[insertCol:]...)
		te.alignments = newAligns
		// Expand each row
		for i, row := range te.rows {
			newRow := make([]string, 0, numCols+1)
			r := normalizeRow(row, numCols)
			newRow = append(newRow, r[:insertCol]...)
			newRow = append(newRow, "")
			newRow = append(newRow, r[insertCol:]...)
			te.rows[i] = newRow
		}
		te.curCol = insertCol

	case "d":
		// Delete current row (only if there are data rows and cursor is on a data row)
		if te.curRow >= 0 && numRows > 0 {
			te.rows = append(te.rows[:te.curRow], te.rows[te.curRow+1:]...)
			if te.curRow >= len(te.rows) {
				te.curRow = len(te.rows) - 1
			}
		}

	case "D":
		// Delete current column (must keep at least one column)
		if numCols > 1 {
			col := te.curCol
			// Remove from headers
			te.headers = append(te.headers[:col], te.headers[col+1:]...)
			// Remove from alignments
			te.alignments = append(te.alignments[:col], te.alignments[col+1:]...)
			// Remove from each row
			for i, row := range te.rows {
				r := normalizeRow(row, numCols)
				te.rows[i] = append(r[:col], r[col+1:]...)
			}
			if te.curCol >= len(te.headers) {
				te.curCol = len(te.headers) - 1
			}
		}

	case "l":
		// Set alignment left
		if te.curCol < len(te.alignments) {
			te.alignments[te.curCol] = alignLeft
		}
	case "r":
		// Set alignment right
		if te.curCol < len(te.alignments) {
			te.alignments[te.curCol] = alignRight
		}
	case "c":
		// Set alignment center
		if te.curCol < len(te.alignments) {
			te.alignments[te.curCol] = alignCenter
		}
	}

	// Auto-scroll to keep cursor visible
	vis := te.visibleDataRows()
	if te.curRow >= 0 {
		if te.curRow >= te.scroll+vis {
			te.scroll = te.curRow - vis + 1
		}
		if te.curRow < te.scroll {
			te.scroll = te.curRow
		}
	}

	return te, nil
}

// View renders the table editor overlay. Uses a value receiver to match
// the overlay pattern used elsewhere in this project.
func (te TableEditor) View() string {
	if !te.active || len(te.headers) == 0 {
		return ""
	}

	numCols := len(te.headers)
	overlayWidth := te.width * 2 / 3
	if overlayWidth < 50 {
		overlayWidth = 50
	}
	if overlayWidth > 100 {
		overlayWidth = 100
	}

	// Compute column widths based on content
	colWidths := te.computeColWidths(overlayWidth - 4 - numCols - 1) // account for borders and separators

	var b strings.Builder

	// Title
	titleIcon := lipgloss.NewStyle().Foreground(blue).Render(IconEditChar)
	titleText := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" Table Editor")
	b.WriteString("  " + titleIcon + titleText)
	b.WriteString("\n")
	innerWidth := overlayWidth - 8
	if innerWidth < 20 {
		innerWidth = 20
	}
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", innerWidth)))
	b.WriteString("\n\n")

	// Compute the table width from column widths
	tableWidth := 1 // leading border
	for _, cw := range colWidths {
		tableWidth += cw + 3 // cell padding (1 space each side) + separator
	}

	// Top border: ╭─┬─╮
	b.WriteString("  ")
	b.WriteString(te.renderTopBorder(colWidths))
	b.WriteString("\n")

	// Header row
	b.WriteString("  ")
	b.WriteString(te.renderRow(te.headers, colWidths, -1))
	b.WriteString("\n")

	// Separator row with alignment indicators: ├─┼─┤
	b.WriteString("  ")
	b.WriteString(te.renderSeparator(colWidths))
	b.WriteString("\n")

	// Data rows (with vertical scrolling)
	vis := te.visibleDataRows()
	startIdx := te.scroll
	if startIdx < 0 {
		startIdx = 0
	}
	endIdx := startIdx + vis
	if endIdx > len(te.rows) {
		endIdx = len(te.rows)
	}
	for rowIdx := startIdx; rowIdx < endIdx; rowIdx++ {
		b.WriteString("  ")
		b.WriteString(te.renderRow(te.rows[rowIdx], colWidths, rowIdx))
		b.WriteString("\n")
	}

	// Bottom border: ╰─┴─╯
	b.WriteString("  ")
	b.WriteString(te.renderBottomBorder(colWidths))
	b.WriteString("\n")

	// Scroll indicator
	if len(te.rows) > vis {
		scrollInfo := fmt.Sprintf("  Rows %d-%d of %d", startIdx+1, endIdx, len(te.rows))
		b.WriteString(DimStyle.Render(scrollInfo))
		b.WriteString("\n")
	}

	// Alignment indicator line
	b.WriteString("\n")
	alignInfo := "  Align: "
	for i, a := range te.alignments {
		label := "L"
		switch a {
		case alignCenter:
			label = "C"
		case alignRight:
			label = "R"
		}
		style := DimStyle
		if i == te.curCol {
			style = lipgloss.NewStyle().Foreground(peach).Bold(true)
		}
		if i > 0 {
			alignInfo += DimStyle.Render(" ")
		}
		alignInfo += style.Render(label)
	}
	b.WriteString(alignInfo)
	b.WriteString("\n")

	// Help bar
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", innerWidth)))
	b.WriteString("\n")
	if te.editing {
		b.WriteString(te.renderEditHelp())
	} else {
		b.WriteString(te.renderNavHelp())
	}

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(overlayWidth).
		Background(mantle)

	return border.Render(b.String())
}

// GetMarkdown outputs the current table as a markdown string.
func (te *TableEditor) GetMarkdown() string {
	if len(te.headers) == 0 {
		return ""
	}

	numCols := len(te.headers)

	// Compute column widths for pretty output
	colWidths := make([]int, numCols)
	for i, h := range te.headers {
		if len(h) > colWidths[i] {
			colWidths[i] = len(h)
		}
	}
	for _, row := range te.rows {
		for i := 0; i < numCols && i < len(row); i++ {
			if len(row[i]) > colWidths[i] {
				colWidths[i] = len(row[i])
			}
		}
	}
	// Minimum width of 3 for separator aesthetics
	for i := range colWidths {
		if colWidths[i] < 3 {
			colWidths[i] = 3
		}
	}

	var b strings.Builder

	// Header row
	b.WriteString("|")
	for i, h := range te.headers {
		b.WriteString(" ")
		b.WriteString(tePadCell(h, colWidths[i], te.getAlignment(i)))
		b.WriteString(" |")
	}
	b.WriteString("\n")

	// Separator row
	b.WriteString("|")
	for i, w := range colWidths {
		a := te.getAlignment(i)
		switch a {
		case alignLeft:
			b.WriteString(" :")
			b.WriteString(strings.Repeat("-", w-1))
			b.WriteString(" |")
		case alignCenter:
			b.WriteString(" :")
			b.WriteString(strings.Repeat("-", w-2))
			b.WriteString(": |")
		case alignRight:
			b.WriteString(" ")
			b.WriteString(strings.Repeat("-", w-1))
			b.WriteString(": |")
		}
	}
	b.WriteString("\n")

	// Data rows
	for _, row := range te.rows {
		b.WriteString("|")
		for i := 0; i < numCols; i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			b.WriteString(" ")
			b.WriteString(tePadCell(cell, colWidths[i], te.getAlignment(i)))
			b.WriteString(" |")
		}
		b.WriteString("\n")
	}

	// Remove trailing newline
	result := b.String()
	return strings.TrimRight(result, "\n")
}

// GetResult returns the markdown result when the user has confirmed, along with
// the line range in the original content that should be replaced.
func (te *TableEditor) GetResult() (markdown string, startLine, endLine int, ok bool) {
	if !te.confirmed {
		return "", 0, 0, false
	}
	te.confirmed = false
	return te.GetMarkdown(), te.startLine, te.endLine, true
}

// ---------------------------------------------------------------------------
// Parsing helpers
// ---------------------------------------------------------------------------

// isTableLine and isSeparatorLine are defined in editor.go and reused here.

// parseCells splits a markdown table row into cell values.
func parseCells(line string) []string {
	trimmed := strings.TrimSpace(line)
	// Remove leading and trailing |
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")
	parts := strings.Split(trimmed, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells
}

// parseAlignment determines the alignment from a separator cell like :---, :---:, ---:.
func parseAlignment(sep string) int {
	s := strings.TrimSpace(sep)
	left := strings.HasPrefix(s, ":")
	right := strings.HasSuffix(s, ":")
	if left && right {
		return alignCenter
	}
	if right {
		return alignRight
	}
	return alignLeft
}

// normalizeRow pads or truncates a row to have exactly numCols columns.
func normalizeRow(cells []string, numCols int) []string {
	row := make([]string, numCols)
	for i := 0; i < numCols && i < len(cells); i++ {
		row[i] = cells[i]
	}
	return row
}

// ---------------------------------------------------------------------------
// Cell access helpers
// ---------------------------------------------------------------------------

func (te *TableEditor) getCellValue(row, col int) string {
	if col < 0 || col >= len(te.headers) {
		return ""
	}
	if row == -1 {
		return te.headers[col]
	}
	if row >= 0 && row < len(te.rows) {
		r := te.rows[row]
		if col < len(r) {
			return r[col]
		}
	}
	return ""
}

func (te *TableEditor) setCellValue(row, col int, value string) {
	if col < 0 || col >= len(te.headers) {
		return
	}
	if row == -1 {
		te.headers[col] = value
		return
	}
	if row >= 0 && row < len(te.rows) {
		// Ensure row has enough columns
		for len(te.rows[row]) <= col {
			te.rows[row] = append(te.rows[row], "")
		}
		te.rows[row][col] = value
	}
}

func (te *TableEditor) getAlignment(col int) int {
	if col >= 0 && col < len(te.alignments) {
		return te.alignments[col]
	}
	return alignLeft
}

// ---------------------------------------------------------------------------
// Rendering helpers
// ---------------------------------------------------------------------------

func (te TableEditor) computeColWidths(maxTotal int) []int {
	numCols := len(te.headers)
	if numCols == 0 {
		return nil
	}

	widths := make([]int, numCols)
	// Start with content widths
	for i, h := range te.headers {
		if len(h) > widths[i] {
			widths[i] = len(h)
		}
	}
	for _, row := range te.rows {
		for i := 0; i < numCols && i < len(row); i++ {
			if len(row[i]) > widths[i] {
				widths[i] = len(row[i])
			}
		}
	}

	// Editing buffer might be wider
	if te.editing {
		if te.curCol >= 0 && te.curCol < numCols {
			bufLen := len(te.editBuf) + 1 // +1 for cursor indicator
			if bufLen > widths[te.curCol] {
				widths[te.curCol] = bufLen
			}
		}
	}

	// Minimum width
	for i := range widths {
		if widths[i] < 3 {
			widths[i] = 3
		}
	}

	// Clamp total if needed
	total := 0
	for _, w := range widths {
		total += w
	}
	if maxTotal > 0 && total > maxTotal {
		// Scale down proportionally
		scale := float64(maxTotal) / float64(total)
		for i := range widths {
			widths[i] = int(float64(widths[i]) * scale)
			if widths[i] < 3 {
				widths[i] = 3
			}
		}
	}

	return widths
}

// Box-drawing characters
const (
	boxTopLeft     = "\u256d" // ╭
	boxTopRight    = "\u256e" // ╮
	boxBottomLeft  = "\u2570" // ╰
	boxBottomRight = "\u256f" // ╯
	boxHoriz       = "\u2500" // ─
	boxVert        = "\u2502" // │
	boxTopT        = "\u252c" // ┬
	boxBottomT     = "\u2534" // ┴
	boxLeftT       = "\u251c" // ├
	boxRightT      = "\u2524" // ┤
	boxCross       = "\u253c" // ┼
)

func (te TableEditor) renderTopBorder(colWidths []int) string {
	borderStyle := lipgloss.NewStyle().Foreground(surface2)
	var parts []string
	for _, w := range colWidths {
		parts = append(parts, strings.Repeat(boxHoriz, w+2)) // +2 for cell padding
	}
	return borderStyle.Render(boxTopLeft + strings.Join(parts, boxTopT) + boxTopRight)
}

func (te TableEditor) renderBottomBorder(colWidths []int) string {
	borderStyle := lipgloss.NewStyle().Foreground(surface2)
	var parts []string
	for _, w := range colWidths {
		parts = append(parts, strings.Repeat(boxHoriz, w+2))
	}
	return borderStyle.Render(boxBottomLeft + strings.Join(parts, boxBottomT) + boxBottomRight)
}

func (te TableEditor) renderSeparator(colWidths []int) string {
	borderStyle := lipgloss.NewStyle().Foreground(surface2)
	alignStyle := lipgloss.NewStyle().Foreground(yellow)

	var b strings.Builder
	b.WriteString(borderStyle.Render(boxLeftT))
	for i, w := range colWidths {
		a := te.getAlignment(i)
		lineWidth := w + 2 // cell padding
		switch a {
		case alignLeft:
			b.WriteString(alignStyle.Render(":"))
			b.WriteString(borderStyle.Render(strings.Repeat(boxHoriz, lineWidth-1)))
		case alignCenter:
			b.WriteString(alignStyle.Render(":"))
			b.WriteString(borderStyle.Render(strings.Repeat(boxHoriz, lineWidth-2)))
			b.WriteString(alignStyle.Render(":"))
		case alignRight:
			b.WriteString(borderStyle.Render(strings.Repeat(boxHoriz, lineWidth-1)))
			b.WriteString(alignStyle.Render(":"))
		default:
			b.WriteString(borderStyle.Render(strings.Repeat(boxHoriz, lineWidth)))
		}
		if i < len(colWidths)-1 {
			b.WriteString(borderStyle.Render(boxCross))
		}
	}
	b.WriteString(borderStyle.Render(boxRightT))
	return b.String()
}

func (te TableEditor) renderRow(cells []string, colWidths []int, rowIdx int) string {
	// rowIdx == -1 means this is the header row
	borderStyle := lipgloss.NewStyle().Foreground(surface2)
	headerCellStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	normalCellStyle := lipgloss.NewStyle().Foreground(text)
	cursorCellStyle := lipgloss.NewStyle().Foreground(peach).Background(surface0)
	editCursorStyle := lipgloss.NewStyle().Foreground(peach).Background(surface0).Bold(true)

	numCols := len(te.headers)

	var b strings.Builder
	b.WriteString(borderStyle.Render(boxVert))

	for col := 0; col < numCols; col++ {
		cellValue := ""
		if col < len(cells) {
			cellValue = cells[col]
		}

		w := 3 // fallback
		if col < len(colWidths) {
			w = colWidths[col]
		}

		isCursor := (te.curRow == rowIdx) && (te.curCol == col)

		var rendered string
		if isCursor && te.editing {
			// Show edit buffer with cursor indicator
			displayBuf := te.editBuf + "_"
			padded := tePadCell(displayBuf, w, alignLeft)
			rendered = editCursorStyle.Render(padded)
		} else if isCursor {
			padded := tePadCell(cellValue, w, te.getAlignment(col))
			rendered = cursorCellStyle.Render(padded)
		} else if rowIdx == -1 {
			padded := tePadCell(cellValue, w, te.getAlignment(col))
			rendered = headerCellStyle.Render(padded)
		} else {
			padded := tePadCell(cellValue, w, te.getAlignment(col))
			rendered = normalCellStyle.Render(padded)
		}

		b.WriteString(" ")
		b.WriteString(rendered)
		b.WriteString(" ")
		b.WriteString(borderStyle.Render(boxVert))
	}

	return b.String()
}

func (te TableEditor) renderNavHelp() string {
	line1 := RenderHelpBar([]struct{ Key, Desc string }{
		{"Arrows", "navigate"}, {"Enter", "edit cell"}, {"Tab", "next cell"},
	})
	line2 := RenderHelpBar([]struct{ Key, Desc string }{
		{"a", "add row"}, {"A", "add col"}, {"d", "del row"}, {"D", "del col"},
	})
	line3 := RenderHelpBar([]struct{ Key, Desc string }{
		{"l/c/r", "align"}, {"Ctrl+S", "confirm"}, {"Esc", "close"},
	})
	return line1 + "\n" + line2 + "\n" + line3
}

func (te TableEditor) renderEditHelp() string {
	editLabel := lipgloss.NewStyle().Foreground(green).Bold(true).Render("  EDITING")
	return editLabel + "  " + RenderHelpBar([]struct{ Key, Desc string }{
		{"Enter", "confirm"}, {"Esc", "cancel"}, {"Backspace", "delete"},
	})[2:] // trim leading "  " since editLabel provides spacing
}

// tePadCell pads a string to a given width with the specified alignment.
func tePadCell(s string, width int, alignment int) string {
	if len(s) >= width {
		return s[:width]
	}
	pad := width - len(s)
	switch alignment {
	case alignCenter:
		left := pad / 2
		right := pad - left
		return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
	case alignRight:
		return strings.Repeat(" ", pad) + s
	default: // alignLeft
		return s + strings.Repeat(" ", pad)
	}
}
