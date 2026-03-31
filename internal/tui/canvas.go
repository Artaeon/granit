package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CanvasCard represents a note card placed on the canvas.
type CanvasCard struct {
	Title    string `json:"title"`
	NotePath string `json:"note_path"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Width    int    `json:"width"`
	Color    int    `json:"color"`
}

// canvasCardJSON is used for clean JSON marshalling with separate x/y fields.
type canvasCardJSON struct {
	Title    string `json:"title"`
	NotePath string `json:"note_path"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Width    int    `json:"width"`
	Color    int    `json:"color,omitempty"`
}

// CanvasConnection represents a line drawn between two cards.
type CanvasConnection struct {
	FromIdx int `json:"from"`
	ToIdx   int `json:"to"`
}

// canvasFileData is the JSON structure saved to disk.
type canvasFileData struct {
	Cards       []canvasCardJSON   `json:"cards"`
	Connections []CanvasConnection `json:"connections"`
}

// canvasMode tracks the current interaction mode.
type canvasMode int

const (
	canvasModeNormal  canvasMode = iota
	canvasModeInput              // typing a new card title
	canvasModeMove               // repositioning a card
	canvasModeConnect            // drawing a connection from a source card
)

// canvasZoom tracks the zoom level.
type canvasZoom int

const (
	canvasZoomNormal   canvasZoom = iota // 3-line cards (title only)
	canvasZoomCompact                    // 1-line cards (title only, no border)
	canvasZoomExpanded                   // 5-line cards (title + note path)
)

const (
	canvasCardWidth     = 22
	canvasCardHeight    = 3
	canvasCardMinWidth  = 12
	canvasCardMaxWidth  = 40
	canvasCardNumColors = 6
)

// canvasColorTable maps color index to lipgloss color.
func canvasColorTable() []lipgloss.Color {
	return []lipgloss.Color{blue, green, yellow, peach, mauve, red}
}

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
	moveIdx   int // index of the card being moved
	moveOrigX int
	moveOrigY int

	// Connect mode state
	connectFrom int // index of the source card

	// Zoom state
	zoom canvasZoom

	// Vault path for persistence
	vaultPath string
}

// NewCanvas returns an initialised, inactive Canvas.
func NewCanvas() Canvas {
	return Canvas{
		selected:    -1,
		moveIdx:     -1,
		connectFrom: -1,
		zoom:        canvasZoomNormal,
	}
}

// SetSize updates the overlay dimensions.
func (c *Canvas) SetSize(width, height int) {
	c.width = width
	c.height = height
}

// SetVaultPath stores the vault path for persistence.
func (c *Canvas) SetVaultPath(path string) {
	c.vaultPath = path
}

// canvasFilePath returns the path to the canvas JSON file.
func (c *Canvas) canvasFilePath() string {
	if c.vaultPath == "" {
		return ""
	}
	return filepath.Join(c.vaultPath, ".granit", "canvas.json")
}

// Save persists the current canvas state to disk.
func (c *Canvas) Save() {
	fp := c.canvasFilePath()
	if fp == "" {
		return
	}

	dir := filepath.Dir(fp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	cards := make([]canvasCardJSON, len(c.cards))
	for i, card := range c.cards {
		cards[i] = canvasCardJSON(card)
	}

	data := canvasFileData{
		Cards:       cards,
		Connections: c.connections,
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(fp, raw, 0o600)
}

// Load restores canvas state from disk.
func (c *Canvas) Load() {
	fp := c.canvasFilePath()
	if fp == "" {
		return
	}

	raw, err := os.ReadFile(fp)
	if err != nil {
		return
	}

	var data canvasFileData
	if err := json.Unmarshal(raw, &data); err != nil {
		return
	}

	c.cards = make([]CanvasCard, len(data.Cards))
	for i, cj := range data.Cards {
		c.cards[i] = CanvasCard(cj)
	}
	c.connections = data.Connections
	if c.connections == nil {
		c.connections = []CanvasConnection{}
	}
}

// Open activates the canvas overlay.
func (c *Canvas) Open() {
	c.active = true
	c.mode = canvasModeNormal
	c.result = ""
	c.inputBuf = ""
	c.Load()
	c.selected = c.cardAtCursor()
}

// Close deactivates the canvas overlay.
func (c *Canvas) Close() {
	c.Save()
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

// canvasOverlayWidth returns the overlay width clamped to reasonable bounds.
func (c *Canvas) canvasOverlayWidth() int {
	w := c.width * 4 / 5
	if w < 60 {
		w = 60
	}
	if w > 160 {
		w = 160
	}
	return w
}

// canvasGridWidth returns the usable grid width inside the overlay.
func (c *Canvas) canvasGridWidth() int {
	return c.canvasOverlayWidth() - 6
}

// canvasGridHeight returns the usable grid height inside the overlay.
func (c *Canvas) canvasGridHeight() int {
	h := c.height - 10
	if h < 10 {
		h = 10
	}
	return h
}

// cardHeightForZoom returns the card height for the current zoom level.
func (c *Canvas) cardHeightForZoom() int {
	switch c.zoom {
	case canvasZoomCompact:
		return 1
	case canvasZoomExpanded:
		return 5
	default:
		return canvasCardHeight // 3
	}
}

// cardAtCursor returns the index of the card whose bounding box contains the
// cursor, or -1 if none.
func (c *Canvas) cardAtCursor() int {
	ch := c.cardHeightForZoom()
	for i, card := range c.cards {
		if c.cursorX >= card.X && c.cursorX < card.X+card.Width &&
			c.cursorY >= card.Y && c.cursorY < card.Y+ch {
			return i
		}
	}
	return -1
}

// clampCursor keeps the cursor within the visible grid.
func (c *Canvas) clampCursor() {
	gw := c.canvasGridWidth()
	gh := c.canvasGridHeight()
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
		c.Save()
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

	// Delete card at cursor (d, x, or delete)
	case "d", "x", "delete":
		idx := c.cardAtCursor()
		if idx >= 0 {
			c.removeCard(idx)
			c.selected = c.cardAtCursor()
			c.Save()
		}

	// Move mode
	case "m":
		idx := c.cardAtCursor()
		if idx < 0 || idx >= len(c.cards) {
			c.mode = canvasModeNormal
			return c, nil
		}
		c.mode = canvasModeMove
		c.moveIdx = idx
		c.moveOrigX = c.cards[idx].X
		c.moveOrigY = c.cards[idx].Y

	// Connect mode (L for link)
	case "L":
		idx := c.cardAtCursor()
		if idx >= 0 {
			c.mode = canvasModeConnect
			c.connectFrom = idx
		}

	// Cycle card color
	case "c":
		idx := c.cardAtCursor()
		if idx >= 0 {
			c.cards[idx].Color = (c.cards[idx].Color + 1) % canvasCardNumColors
			c.Save()
		}

	// Resize cards
	case "+", "=":
		idx := c.cardAtCursor()
		if idx >= 0 && c.cards[idx].Width < canvasCardMaxWidth {
			c.cards[idx].Width++
			c.Save()
		}
	case "-", "_":
		idx := c.cardAtCursor()
		if idx >= 0 && c.cards[idx].Width > canvasCardMinWidth {
			c.cards[idx].Width--
			c.Save()
		}

	// Zoom level
	case "z":
		switch c.zoom {
		case canvasZoomNormal:
			c.zoom = canvasZoomCompact
		case canvasZoomCompact:
			c.zoom = canvasZoomExpanded
		default:
			c.zoom = canvasZoomNormal
		}

	// Open selected card
	case "enter":
		if c.selected >= 0 && c.selected < len(c.cards) {
			c.result = c.cards[c.selected].NotePath
			c.Save()
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
				Color:    0,
			}
			c.cards = append(c.cards, card)
			c.selected = len(c.cards) - 1
			c.Save()
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
		c.cards[c.moveIdx].X = c.moveOrigX
		c.cards[c.moveIdx].Y = c.moveOrigY
		c.mode = canvasModeNormal
	case "enter":
		c.mode = canvasModeNormal
		c.Save()
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
	case "L", "enter":
		target := c.cardAtCursor()
		if target >= 0 && target != c.connectFrom {
			if !c.connectionExists(c.connectFrom, target) {
				c.connections = append(c.connections, CanvasConnection{
					FromIdx: c.connectFrom,
					ToIdx:   target,
				})
				c.Save()
			}
		}
		c.connectFrom = -1
		c.mode = canvasModeNormal
	}
	return c, nil
}

// removeCard deletes a card and fixes up connection indices.
func (c *Canvas) removeCard(idx int) {
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

// connectionExists returns true if a connection already links the two cards.
func (c *Canvas) connectionExists(a, b int) bool {
	for _, conn := range c.connections {
		if (conn.FromIdx == a && conn.ToIdx == b) ||
			(conn.FromIdx == b && conn.ToIdx == a) {
			return true
		}
	}
	return false
}

// connectionsForCard returns the indices of cards connected to/from the given card.
func (c *Canvas) connectionsForCard(idx int) []int {
	seen := map[int]bool{}
	for _, conn := range c.connections {
		if conn.FromIdx == idx && !seen[conn.ToIdx] {
			seen[conn.ToIdx] = true
		}
		if conn.ToIdx == idx && !seen[conn.FromIdx] {
			seen[conn.FromIdx] = true
		}
	}
	result := make([]int, 0, len(seen))
	for k := range seen {
		result = append(result, k)
	}
	return result
}

// ---------- View ----------

func (c Canvas) View() string {
	overlayW := c.canvasOverlayWidth()
	gw := c.canvasGridWidth()
	gh := c.canvasGridHeight()

	var b strings.Builder

	// Header
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  ◫ Canvas")
	b.WriteString(title)

	// Zoom indicator
	var zoomLabel string
	switch c.zoom {
	case canvasZoomCompact:
		zoomLabel = " [compact]"
	case canvasZoomExpanded:
		zoomLabel = " [expanded]"
	default:
		zoomLabel = ""
	}
	if zoomLabel != "" {
		b.WriteString(DimStyle.Render(zoomLabel))
	}

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
			DimStyle.Render(" — arrows to reposition, Enter to confirm, Esc cancel") + "\n")
	case canvasModeConnect:
		fromLabel := ""
		if c.connectFrom >= 0 && c.connectFrom < len(c.cards) {
			fromLabel = c.cards[c.connectFrom].Title
		}
		b.WriteString(lipgloss.NewStyle().Foreground(yellow).Bold(true).
			Render("  CONNECT") +
			DimStyle.Render(" from \""+fromLabel+"\" — move to target, L/Enter link") + "\n")
	default:
		info := DimStyle.Render("  " + smallNum(len(c.cards)) + " cards  " +
			smallNum(len(c.connections)) + " connections")
		b.WriteString(info + "\n")
	}

	// Build the 2-D grid and render rows
	rows := c.renderGrid(gw, gh)
	for y := 0; y < len(rows); y++ {
		b.WriteString(rows[y])
		b.WriteString("\n")
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", overlayW-6)))
	b.WriteString("\n")
	b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
		{"arrows", "move"}, {"n", "add"}, {"d/x", "del"}, {"m", "move"}, {"L", "connect"},
		{"c", "color"}, {"+/-", "resize"}, {"z", "zoom"}, {"Enter", "open"}, {"Esc", "close"},
	}))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(overlayW).
		Background(mantle)

	return border.Render(b.String())
}

// renderGrid renders all cards, connections and the cursor into styled row strings.
func (c Canvas) renderGrid(gw, gh int) []string {
	type cell struct {
		ch    rune
		color lipgloss.Color
		bold  bool
	}

	// Initialize grid with dot-pattern background
	cells := make([][]cell, gh)
	for y := 0; y < gh; y++ {
		cells[y] = make([]cell, gw)
		for x := 0; x < gw; x++ {
			if x%4 == 0 && y%2 == 0 {
				cells[y][x] = cell{ch: '·', color: surface1}
			} else {
				cells[y][x] = cell{ch: ' ', color: surface1}
			}
		}
	}

	setCell := func(x, y int, ch rune, color lipgloss.Color, bold bool) {
		if x >= 0 && x < gw && y >= 0 && y < gh {
			cells[y][x] = cell{ch: ch, color: color, bold: bold}
		}
	}

	cardH := c.cardHeightForZoom()
	colors := canvasColorTable()

	// Draw connections first (behind cards)
	for _, conn := range c.connections {
		if conn.FromIdx < 0 || conn.FromIdx >= len(c.cards) ||
			conn.ToIdx < 0 || conn.ToIdx >= len(c.cards) {
			continue
		}
		from := c.cards[conn.FromIdx]
		to := c.cards[conn.ToIdx]
		fx := from.X + from.Width/2
		fy := from.Y + cardH/2
		tx := to.X + to.Width/2
		ty := to.Y + cardH/2
		c.drawConnection(fx, fy, tx, ty, gw, gh, setCell)
	}

	// Draw cards
	for i, card := range c.cards {
		borderColor := colors[card.Color%canvasCardNumColors]
		isSelected := (i == c.selected)
		if isSelected {
			borderColor = mauve
		}
		if c.mode == canvasModeMove && i == c.moveIdx {
			borderColor = peach
		}

		cw := card.Width
		if cw < canvasCardMinWidth {
			cw = canvasCardWidth
		}

		// Connection indicators: show linked card indices
		connIndicator := ""
		linked := c.connectionsForCard(i)
		if len(linked) > 0 {
			parts := make([]string, len(linked))
			for ci, li := range linked {
				parts[ci] = fmt.Sprintf("%d", li)
			}
			connIndicator = " \u2192" + strings.Join(parts, ",")
		}

		// Title text fitting
		innerW := cw - 4
		if innerW < 1 {
			innerW = 1
		}

		displayTitle := card.Title
		// Append connection indicator to title if there's room
		titleWithConn := displayTitle
		if connIndicator != "" {
			titleWithConn = displayTitle + connIndicator
		}

		switch c.zoom {
		case canvasZoomCompact:
			// Single-line compact card: " Title "
			compactTitle := titleWithConn
			maxLen := cw - 2
			if maxLen < 1 {
				maxLen = 1
			}
			if r := []rune(compactTitle); len(r) > maxLen {
				if maxLen > 3 {
					compactTitle = string(r[:maxLen-3]) + "..."
				} else {
					compactTitle = string(r[:maxLen])
				}
			}
			titleColor := text
			if isSelected {
				titleColor = mauve
			}
			setCell(card.X, card.Y, '[', borderColor, isSelected)
			for ti, tch := range compactTitle {
				setCell(card.X+1+ti, card.Y, tch, titleColor, isSelected)
			}
			// Pad remaining
			for px := len(compactTitle) + 1; px < cw-1; px++ {
				setCell(card.X+px, card.Y, ' ', text, false)
			}
			setCell(card.X+cw-1, card.Y, ']', borderColor, isSelected)

		case canvasZoomExpanded:
			// 5-line expanded card: border, title, note path, connection info, border
			fitStr := func(s string, w int) string {
				r := []rune(s)
				if len(r) > w {
					if w > 3 {
						return string(r[:w-3]) + "..."
					}
					return string(r[:w])
				}
				for len(r) < w {
					r = append(r, ' ')
				}
				return string(r)
			}
			titleColor := text
			if isSelected {
				titleColor = mauve
			}
			// Top border
			setCell(card.X, card.Y, '\u256D', borderColor, isSelected)
			for dx := 1; dx < cw-1; dx++ {
				setCell(card.X+dx, card.Y, '\u2500', borderColor, isSelected)
			}
			setCell(card.X+cw-1, card.Y, '\u256E', borderColor, isSelected)
			// Row 1: Title
			setCell(card.X, card.Y+1, '\u2502', borderColor, isSelected)
			setCell(card.X+1, card.Y+1, ' ', text, false)
			dt := fitStr(displayTitle, innerW)
			for ti, tch := range dt {
				setCell(card.X+2+ti, card.Y+1, tch, titleColor, isSelected)
			}
			setCell(card.X+cw-2, card.Y+1, ' ', text, false)
			setCell(card.X+cw-1, card.Y+1, '\u2502', borderColor, isSelected)
			// Row 2: Note path
			setCell(card.X, card.Y+2, '\u2502', borderColor, isSelected)
			setCell(card.X+1, card.Y+2, ' ', text, false)
			np := fitStr(card.NotePath, innerW)
			for ti, tch := range np {
				setCell(card.X+2+ti, card.Y+2, tch, overlay0, false)
			}
			setCell(card.X+cw-2, card.Y+2, ' ', text, false)
			setCell(card.X+cw-1, card.Y+2, '\u2502', borderColor, isSelected)
			// Row 3: Connection indicators or empty
			setCell(card.X, card.Y+3, '\u2502', borderColor, isSelected)
			setCell(card.X+1, card.Y+3, ' ', text, false)
			ci := ""
			if connIndicator != "" {
				ci = connIndicator
			}
			ciStr := fitStr(ci, innerW)
			for ti, tch := range ciStr {
				setCell(card.X+2+ti, card.Y+3, tch, overlay0, false)
			}
			setCell(card.X+cw-2, card.Y+3, ' ', text, false)
			setCell(card.X+cw-1, card.Y+3, '\u2502', borderColor, isSelected)
			// Bottom border
			setCell(card.X, card.Y+4, '\u2570', borderColor, isSelected)
			for dx := 1; dx < cw-1; dx++ {
				setCell(card.X+dx, card.Y+4, '\u2500', borderColor, isSelected)
			}
			setCell(card.X+cw-1, card.Y+4, '\u256F', borderColor, isSelected)

		default:
			// Normal 3-line card
			dtNormal := titleWithConn
			if dtRunes := []rune(dtNormal); len(dtRunes) > innerW {
				if innerW > 3 {
					dtNormal = string(dtRunes[:innerW-3]) + "..."
				} else {
					dtNormal = string(dtRunes[:innerW])
				}
			}
			for len([]rune(dtNormal)) < innerW {
				dtNormal += " "
			}

			titleColor := text
			if isSelected {
				titleColor = mauve
			}

			// Top border: ╭──────────────────╮
			setCell(card.X, card.Y, '\u256D', borderColor, isSelected)
			for dx := 1; dx < cw-1; dx++ {
				setCell(card.X+dx, card.Y, '\u2500', borderColor, isSelected)
			}
			setCell(card.X+cw-1, card.Y, '\u256E', borderColor, isSelected)

			// Middle row: │ Title            │
			setCell(card.X, card.Y+1, '\u2502', borderColor, isSelected)
			setCell(card.X+1, card.Y+1, ' ', text, false)
			for ti, tch := range dtNormal {
				setCell(card.X+2+ti, card.Y+1, tch, titleColor, isSelected)
			}
			setCell(card.X+cw-2, card.Y+1, ' ', text, false)
			setCell(card.X+cw-1, card.Y+1, '\u2502', borderColor, isSelected)

			// Bottom border: ╰──────────────────╯
			setCell(card.X, card.Y+2, '\u2570', borderColor, isSelected)
			for dx := 1; dx < cw-1; dx++ {
				setCell(card.X+dx, card.Y+2, '\u2500', borderColor, isSelected)
			}
			setCell(card.X+cw-1, card.Y+2, '\u256F', borderColor, isSelected)
		}
	}

	// Draw cursor on top
	if c.cursorX >= 0 && c.cursorX < gw && c.cursorY >= 0 && c.cursorY < gh {
		existing := cells[c.cursorY][c.cursorX]
		if existing.ch == ' ' || existing.ch == '\u00B7' {
			setCell(c.cursorX, c.cursorY, '\u2588', mauve, true)
		} else {
			cells[c.cursorY][c.cursorX].color = mauve
			cells[c.cursorY][c.cursorX].bold = true
		}
	}

	// Render cell grid into styled strings, batching consecutive same-styled chars
	rows := make([]string, gh)
	for y := 0; y < gh; y++ {
		var row strings.Builder
		x := 0
		for x < gw {
			// Start a new batch
			startColor := cells[y][x].color
			startBold := cells[y][x].bold
			var batch strings.Builder
			batch.WriteRune(cells[y][x].ch)
			x++
			// Extend batch while style is the same
			for x < gw && cells[y][x].color == startColor && cells[y][x].bold == startBold {
				batch.WriteRune(cells[y][x].ch)
				x++
			}
			// Render the batch
			style := lipgloss.NewStyle().Foreground(startColor)
			if startBold {
				style = style.Bold(true)
			}
			row.WriteString(style.Render(batch.String()))
		}
		rows[y] = row.String()
	}
	return rows
}

// drawConnection renders an orthogonal path between two points with ASCII line chars.
func (c Canvas) drawConnection(fx, fy, tx, ty, gw, gh int, setCell func(int, int, rune, lipgloss.Color, bool)) {
	lineColor := overlay0

	dx := 1
	if tx < fx {
		dx = -1
	}
	dy := 1
	if ty < fy {
		dy = -1
	}

	// Horizontal segment using ─
	for x := fx; x != tx; x += dx {
		setCell(x, fy, '\u2500', lineColor, false)
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

	// Vertical segment using │
	for y := fy; y != ty; y += dy {
		if y == fy && fx != tx {
			continue
		}
		setCell(tx, y, '\u2502', lineColor, false)
	}

	// Arrow head at destination
	if ty != fy {
		if dy > 0 {
			setCell(tx, ty, '\u2193', lineColor, false) // ↓ (down arrow using ↓ is not in basic set, use →)
		} else {
			setCell(tx, ty, '\u2191', lineColor, false) // ↑
		}
	} else if tx != fx {
		if dx > 0 {
			setCell(tx, ty, '\u2192', lineColor, false) // →
		} else {
			setCell(tx, ty, '\u2190', lineColor, false) // ←
		}
	}
}
