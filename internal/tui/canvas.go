package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CanvasCard represents a note card placed on the canvas.
type CanvasCard struct {
	Title    string
	NotePath string
	X, Y     int
	Width    int
}

// CanvasConnection represents a line drawn between two cards.
type CanvasConnection struct {
	FromIdx, ToIdx int
}

// canvasMode tracks the current interaction mode.
type canvasMode int

const (
	canvasModeNormal  canvasMode = iota
	canvasModeInput              // typing a new card title
	canvasModeMove               // repositioning a card
	canvasModeConnect            // drawing a connection from a source card
)

const (
	canvasCardWidth  = 16
	canvasCardHeight = 3
)

// Canvas is a terminal-based whiteboard overlay where users can place,
// move, connect and open note cards on a 2-D grid.
type Canvas struct {
	active bool
	width  int
	height int

	cards       []CanvasCard
	connections []CanvasConnection

	cursorX int
	cursorY int

	mode     canvasMode
	selected int    // index of the card under the cursor (-1 = none)
	result   string // note path chosen via Enter

	// Input mode state (adding a new card)
	inputBuf string

	// Move mode state
	moveIdx    int // index of the card being moved
	moveOrigX  int
	moveOrigY  int

	// Connect mode state
	connectFrom int // index of the source card
}

// NewCanvas returns an initialised, inactive Canvas.
func NewCanvas() Canvas {
	return Canvas{
		selected:    -1,
		moveIdx:     -1,
		connectFrom: -1,
	}
}

// SetSize updates the overlay dimensions.
func (c *Canvas) SetSize(width, height int) {
	c.width = width
	c.height = height
}

// Open activates the canvas overlay.
func (c *Canvas) Open() {
	c.active = true
	c.mode = canvasModeNormal
	c.result = ""
	c.inputBuf = ""
	c.selected = c.cardAtCursor()
}

// Close deactivates the canvas overlay.
func (c *Canvas) Close() {
	c.active = false
	c.mode = canvasModeNormal
}

// IsActive returns true when the overlay is visible.
func (c *Canvas) IsActive() bool {
	return c.active
}

// SelectedNote returns (and clears) the note path the user selected with Enter.
func (c *Canvas) SelectedNote() string {
	s := c.result
	c.result = ""
	return s
}

// gridWidth returns the usable grid width inside the overlay.
func (c *Canvas) gridWidth() int {
	w := c.width*2/3 - 6
	if w < 40 {
		w = 40
	}
	return w
}

// gridHeight returns the usable grid height inside the overlay.
func (c *Canvas) gridHeight() int {
	h := c.height - 10
	if h < 10 {
		h = 10
	}
	return h
}

// cardAtCursor returns the index of the card whose bounding box contains the
// cursor, or -1 if none.
func (c *Canvas) cardAtCursor() int {
	for i, card := range c.cards {
		if c.cursorX >= card.X && c.cursorX < card.X+card.Width &&
			c.cursorY >= card.Y && c.cursorY < card.Y+canvasCardHeight {
			return i
		}
	}
	return -1
}

// clampCursor keeps the cursor within the visible grid.
func (c *Canvas) clampCursor() {
	gw := c.gridWidth()
	gh := c.gridHeight()
	if c.cursorX < 0 {
		c.cursorX = 0
	}
	if c.cursorY < 0 {
		c.cursorY = 0
	}
	if c.cursorX >= gw {
		c.cursorX = gw - 1
	}
	if c.cursorY >= gh {
		c.cursorY = gh - 1
	}
}

// ---------- Update ----------

func (c Canvas) Update(msg tea.Msg) (Canvas, tea.Cmd) {
	if !c.active {
		return c, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch c.mode {
		case canvasModeInput:
			return c.updateInput(msg)
		case canvasModeMove:
			return c.updateMove(msg)
		case canvasModeConnect:
			return c.updateConnect(msg)
		default:
			return c.updateNormal(msg)
		}
	}
	return c, nil
}

func (c Canvas) updateNormal(msg tea.KeyMsg) (Canvas, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		c.active = false
		return c, nil

	// Navigation
	case "up", "k":
		c.cursorY--
		c.clampCursor()
		c.selected = c.cardAtCursor()
	case "down", "j":
		c.cursorY++
		c.clampCursor()
		c.selected = c.cardAtCursor()
	case "left", "h":
		c.cursorX--
		c.clampCursor()
		c.selected = c.cardAtCursor()
	case "right", "l":
		c.cursorX++
		c.clampCursor()
		c.selected = c.cardAtCursor()

	// Add new card
	case "n":
		c.mode = canvasModeInput
		c.inputBuf = ""

	// Delete card at cursor
	case "d":
		idx := c.cardAtCursor()
		if idx >= 0 {
			c.removeCard(idx)
			c.selected = c.cardAtCursor()
		}

	// Move mode
	case "m":
		idx := c.cardAtCursor()
		if idx >= 0 {
			c.mode = canvasModeMove
			c.moveIdx = idx
			c.moveOrigX = c.cards[idx].X
			c.moveOrigY = c.cards[idx].Y
		}

	// Connect mode
	case "c":
		idx := c.cardAtCursor()
		if idx >= 0 {
			c.mode = canvasModeConnect
			c.connectFrom = idx
		}

	// Open selected card
	case "enter":
		if c.selected >= 0 && c.selected < len(c.cards) {
			c.result = c.cards[c.selected].NotePath
			c.active = false
		}
		return c, nil
	}
	return c, nil
}

func (c Canvas) updateInput(msg tea.KeyMsg) (Canvas, tea.Cmd) {
	switch msg.String() {
	case "esc":
		c.mode = canvasModeNormal
	case "enter":
		title := strings.TrimSpace(c.inputBuf)
		if title != "" {
			card := CanvasCard{
				Title:    title,
				NotePath: title + ".md",
				X:        c.cursorX,
				Y:        c.cursorY,
				Width:    canvasCardWidth,
			}
			c.cards = append(c.cards, card)
			c.selected = len(c.cards) - 1
		}
		c.inputBuf = ""
		c.mode = canvasModeNormal
	case "backspace":
		if len(c.inputBuf) > 0 {
			c.inputBuf = c.inputBuf[:len(c.inputBuf)-1]
		}
	default:
		ch := msg.String()
		if len(ch) == 1 && ch[0] >= 32 && ch[0] <= 126 {
			c.inputBuf += ch
		}
	}
	return c, nil
}

func (c Canvas) updateMove(msg tea.KeyMsg) (Canvas, tea.Cmd) {
	if c.moveIdx < 0 || c.moveIdx >= len(c.cards) {
		c.mode = canvasModeNormal
		return c, nil
	}
	switch msg.String() {
	case "esc":
		// Cancel: restore original position
		c.cards[c.moveIdx].X = c.moveOrigX
		c.cards[c.moveIdx].Y = c.moveOrigY
		c.mode = canvasModeNormal
	case "enter":
		c.mode = canvasModeNormal
	case "up", "k":
		if c.cards[c.moveIdx].Y > 0 {
			c.cards[c.moveIdx].Y--
			c.cursorY--
			c.clampCursor()
		}
	case "down", "j":
		c.cards[c.moveIdx].Y++
		c.cursorY++
		c.clampCursor()
	case "left", "h":
		if c.cards[c.moveIdx].X > 0 {
			c.cards[c.moveIdx].X--
			c.cursorX--
			c.clampCursor()
		}
	case "right", "l":
		c.cards[c.moveIdx].X++
		c.cursorX++
		c.clampCursor()
	}
	c.selected = c.cardAtCursor()
	return c, nil
}

func (c Canvas) updateConnect(msg tea.KeyMsg) (Canvas, tea.Cmd) {
	switch msg.String() {
	case "esc":
		c.connectFrom = -1
		c.mode = canvasModeNormal
	case "up", "k":
		c.cursorY--
		c.clampCursor()
		c.selected = c.cardAtCursor()
	case "down", "j":
		c.cursorY++
		c.clampCursor()
		c.selected = c.cardAtCursor()
	case "left", "h":
		c.cursorX--
		c.clampCursor()
		c.selected = c.cardAtCursor()
	case "right", "l":
		c.cursorX++
		c.clampCursor()
		c.selected = c.cardAtCursor()
	case "c", "enter":
		target := c.cardAtCursor()
		if target >= 0 && target != c.connectFrom {
			// Avoid duplicate connections
			if !c.connectionExists(c.connectFrom, target) {
				c.connections = append(c.connections, CanvasConnection{
					FromIdx: c.connectFrom,
					ToIdx:   target,
				})
			}
		}
		c.connectFrom = -1
		c.mode = canvasModeNormal
	}
	return c, nil
}

// removeCard deletes a card and fixes up connection indices.
func (c *Canvas) removeCard(idx int) {
	// Remove connections that reference this card
	var kept []CanvasConnection
	for _, conn := range c.connections {
		if conn.FromIdx == idx || conn.ToIdx == idx {
			continue
		}
		adj := conn
		if adj.FromIdx > idx {
			adj.FromIdx--
		}
		if adj.ToIdx > idx {
			adj.ToIdx--
		}
		kept = append(kept, adj)
	}
	c.connections = kept
	c.cards = append(c.cards[:idx], c.cards[idx+1:]...)
}

// connectionExists returns true if a connection already links the two cards
// (in either direction).
func (c *Canvas) connectionExists(a, b int) bool {
	for _, conn := range c.connections {
		if (conn.FromIdx == a && conn.ToIdx == b) ||
			(conn.FromIdx == b && conn.ToIdx == a) {
			return true
		}
	}
	return false
}

// ---------- View ----------

func (c Canvas) View() string {
	overlayW := c.width * 2 / 3
	if overlayW < 50 {
		overlayW = 50
	}
	if overlayW > 120 {
		overlayW = 120
	}

	gw := overlayW - 6 // account for border + padding
	if gw < 40 {
		gw = 40
	}
	gh := c.gridHeight()

	var b strings.Builder

	// Header
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  Canvas")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", overlayW-6)))
	b.WriteString("\n")

	// Mode indicator
	switch c.mode {
	case canvasModeInput:
		prompt := lipgloss.NewStyle().Foreground(green).Bold(true).Render("  New card: ")
		inputText := lipgloss.NewStyle().Foreground(text).Render(c.inputBuf)
		cursor := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("_")
		b.WriteString(prompt + inputText + cursor + "\n")
	case canvasModeMove:
		b.WriteString(lipgloss.NewStyle().Foreground(peach).Bold(true).
			Render("  MOVE MODE") +
			DimStyle.Render(" — arrows to reposition, Enter to confirm, Esc to cancel") + "\n")
	case canvasModeConnect:
		fromLabel := ""
		if c.connectFrom >= 0 && c.connectFrom < len(c.cards) {
			fromLabel = c.cards[c.connectFrom].Title
		}
		b.WriteString(lipgloss.NewStyle().Foreground(yellow).Bold(true).
			Render("  CONNECT") +
			DimStyle.Render(" from \""+fromLabel+"\" — move to target, c/Enter to link, Esc to cancel") + "\n")
	default:
		info := DimStyle.Render("  " + smallNum(len(c.cards)) + " cards  " +
			smallNum(len(c.connections)) + " connections")
		b.WriteString(info + "\n")
	}

	// Build the 2-D grid buffer
	grid := c.buildGrid(gw, gh)

	for y := 0; y < gh; y++ {
		row := grid[y]
		// Trim row to grid width in runes (each cell is one rune in the
		// plain buffer; styling is applied per-cell below).
		if len(row) > gw {
			row = row[:gw]
		}
		b.WriteString(row)
		if y < gh-1 {
			b.WriteString("\n")
		}
	}

	// Footer
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", overlayW-6)))
	b.WriteString("\n")
	footer := "  arrows: move  n: add  d: delete  m: move  c: connect  Enter: open  Esc: close"
	b.WriteString(DimStyle.Render(footer))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(blue).
		Padding(1, 2).
		Width(overlayW).
		Background(mantle)

	return border.Render(b.String())
}

// buildGrid renders all cards, connections and the cursor into a slice of
// styled strings (one per row).
func (c Canvas) buildGrid(gw, gh int) []string {
	// cellChar stores the character at each grid position.
	// cellColor stores an optional lipgloss.Color; empty means default.
	type cell struct {
		ch    rune
		color lipgloss.Color
		bold  bool
	}

	cells := make([][]cell, gh)
	for y := 0; y < gh; y++ {
		cells[y] = make([]cell, gw)
		for x := 0; x < gw; x++ {
			// Subtle dot pattern background
			if x%4 == 0 && y%2 == 0 {
				cells[y][x] = cell{ch: '.', color: surface0}
			} else {
				cells[y][x] = cell{ch: ' ', color: surface0}
			}
		}
	}

	// Helper to set a cell safely.
	setCell := func(x, y int, ch rune, color lipgloss.Color, bold bool) {
		if x >= 0 && x < gw && y >= 0 && y < gh {
			cells[y][x] = cell{ch: ch, color: color, bold: bold}
		}
	}

	// Draw connections first (behind cards).
	for _, conn := range c.connections {
		if conn.FromIdx < 0 || conn.FromIdx >= len(c.cards) ||
			conn.ToIdx < 0 || conn.ToIdx >= len(c.cards) {
			continue
		}
		from := c.cards[conn.FromIdx]
		to := c.cards[conn.ToIdx]

		// Centre points of each card.
		fx := from.X + from.Width/2
		fy := from.Y + canvasCardHeight/2
		tx := to.X + to.Width/2
		ty := to.Y + canvasCardHeight/2

		c.drawConnection(fx, fy, tx, ty, gw, gh, setCell)
	}

	// Draw cards.
	cardColors := []lipgloss.Color{blue, green, peach, yellow, red, mauve}
	for i, card := range c.cards {
		borderColor := cardColors[i%len(cardColors)]
		isSelected := (i == c.selected)
		if isSelected {
			borderColor = mauve
		}
		isMoving := (c.mode == canvasModeMove && i == c.moveIdx)
		if isMoving {
			borderColor = peach
		}

		cw := card.Width
		if cw < 5 {
			cw = canvasCardWidth
		}

		// Truncate title to fit inside the box borders (2 border chars + 2 padding).
		innerW := cw - 4
		if innerW < 1 {
			innerW = 1
		}
		displayTitle := card.Title
		if len(displayTitle) > innerW {
			displayTitle = displayTitle[:innerW-3]
			if len(displayTitle) < 0 {
				displayTitle = ""
			}
			displayTitle += "..."
		}
		// Pad title to inner width
		for len(displayTitle) < innerW {
			displayTitle += " "
		}

		// Top border: ╭──...──╮
		setCell(card.X, card.Y, '\u256D', borderColor, isSelected) // ╭
		for dx := 1; dx < cw-1; dx++ {
			setCell(card.X+dx, card.Y, '\u2500', borderColor, isSelected) // ─
		}
		setCell(card.X+cw-1, card.Y, '\u256E', borderColor, isSelected) // ╮

		// Middle row: │ Title │
		setCell(card.X, card.Y+1, '\u2502', borderColor, isSelected)     // │
		setCell(card.X+1, card.Y+1, ' ', text, false)                    // padding
		for ti, tch := range displayTitle {
			clr := text
			if isSelected {
				clr = mauve
			}
			setCell(card.X+2+ti, card.Y+1, tch, clr, isSelected)
		}
		setCell(card.X+cw-2, card.Y+1, ' ', text, false)                // padding
		setCell(card.X+cw-1, card.Y+1, '\u2502', borderColor, isSelected) // │

		// Bottom border: ╰──...──╯
		setCell(card.X, card.Y+2, '\u2570', borderColor, isSelected) // ╰
		for dx := 1; dx < cw-1; dx++ {
			setCell(card.X+dx, card.Y+2, '\u2500', borderColor, isSelected) // ─
		}
		setCell(card.X+cw-1, card.Y+2, '\u256F', borderColor, isSelected) // ╯
	}

	// Draw cursor on top.
	if c.cursorX >= 0 && c.cursorX < gw && c.cursorY >= 0 && c.cursorY < gh {
		existing := cells[c.cursorY][c.cursorX]
		if existing.ch == ' ' || existing.ch == '.' {
			setCell(c.cursorX, c.cursorY, '\u2588', mauve, true) // █
		} else {
			// Highlight the existing character under the cursor.
			cells[c.cursorY][c.cursorX].color = mauve
			cells[c.cursorY][c.cursorX].bold = true
		}
	}

	// Render cell grid into styled strings.
	rows := make([]string, gh)
	for y := 0; y < gh; y++ {
		var row strings.Builder
		for x := 0; x < gw; x++ {
			cl := cells[y][x]
			style := lipgloss.NewStyle().Foreground(cl.color)
			if cl.bold {
				style = style.Bold(true)
			}
			row.WriteString(style.Render(string(cl.ch)))
		}
		rows[y] = row.String()
	}
	return rows
}

// drawConnection renders a simple orthogonal path between two points using
// box-drawing characters.
func (c Canvas) drawConnection(fx, fy, tx, ty, gw, gh int, setCell func(int, int, rune, lipgloss.Color, bool)) {
	lineColor := overlay0

	// We draw an L-shaped path: horizontal from (fx,fy) to (tx,fy), then
	// vertical from (tx,fy) to (tx,ty). Use dashed lines for visual
	// distinction from card borders.
	//
	// Determine direction.
	dx := 1
	if tx < fx {
		dx = -1
	}
	dy := 1
	if ty < fy {
		dy = -1
	}

	// Horizontal segment
	for x := fx; x != tx; x += dx {
		setCell(x, fy, '\u2504', lineColor, false) // ┄ (dashed horizontal)
	}

	// Corner
	if fx != tx && fy != ty {
		var corner rune
		if dx > 0 && dy > 0 {
			corner = '\u2510' // ┐
		} else if dx > 0 && dy < 0 {
			corner = '\u2518' // ┘
		} else if dx < 0 && dy > 0 {
			corner = '\u250C' // ┌
		} else {
			corner = '\u2514' // └
		}
		setCell(tx, fy, corner, lineColor, false)
	}

	// Vertical segment
	for y := fy; y != ty; y += dy {
		if y == fy && fx != tx {
			continue // skip corner cell already drawn
		}
		setCell(tx, y, '\u2506', lineColor, false) // ┆ (dashed vertical)
	}
}
