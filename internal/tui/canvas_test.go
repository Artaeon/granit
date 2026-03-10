package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------- helpers ----------

func cvKeyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "delete":
		return tea.KeyMsg{Type: tea.KeyDelete}
	default:
		if len(s) == 1 {
			return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
		}
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// newTestCanvas creates an active canvas with no persistence (empty vaultPath)
// so that Open/Close/Save/Load don't touch the filesystem.
func newTestCanvas() Canvas {
	c := NewCanvas()
	c.SetSize(120, 40)
	// Don't call Open() because it tries to Load from disk.
	// Instead, activate manually:
	c.active = true
	c.mode = canvasModeNormal
	c.result = ""
	c.inputBuf = ""
	c.selected = c.cardAtCursor()
	return c
}

// ---------- NewCanvas ----------

func TestCanvasNewCanvas(t *testing.T) {
	c := NewCanvas()
	if c.active {
		t.Fatal("expected inactive after construction")
	}
	if c.selected != -1 {
		t.Fatalf("expected selected=-1, got %d", c.selected)
	}
	if c.moveIdx != -1 {
		t.Fatalf("expected moveIdx=-1, got %d", c.moveIdx)
	}
	if c.connectFrom != -1 {
		t.Fatalf("expected connectFrom=-1, got %d", c.connectFrom)
	}
	if c.zoom != canvasZoomNormal {
		t.Fatalf("expected zoom=canvasZoomNormal, got %d", c.zoom)
	}
	if len(c.cards) != 0 {
		t.Fatalf("expected 0 cards, got %d", len(c.cards))
	}
	if len(c.connections) != 0 {
		t.Fatalf("expected 0 connections, got %d", len(c.connections))
	}
}

// ---------- Open / Close / IsActive ----------

func TestCanvasOpenCloseIsActive(t *testing.T) {
	c := NewCanvas()
	c.SetSize(120, 40)
	// Don't set vaultPath so Open/Close skip file I/O.

	if c.IsActive() {
		t.Fatal("should start inactive")
	}

	c.Open()
	if !c.IsActive() {
		t.Fatal("should be active after Open")
	}
	if c.mode != canvasModeNormal {
		t.Fatalf("expected normal mode after Open, got %d", c.mode)
	}

	c.Close()
	if c.IsActive() {
		t.Fatal("should be inactive after Close")
	}
}

// ---------- SetSize ----------

func TestCanvasSetSize(t *testing.T) {
	c := NewCanvas()
	c.SetSize(100, 50)
	if c.width != 100 || c.height != 50 {
		t.Fatalf("expected 100x50, got %dx%d", c.width, c.height)
	}
}

// ---------- Card creation — adding a card via input mode ----------

func TestCanvasCardCreation(t *testing.T) {
	c := newTestCanvas()

	// Press 'n' to enter input mode
	c, _ = c.Update(cvKeyMsg("n"))
	if c.mode != canvasModeInput {
		t.Fatalf("expected input mode, got %d", c.mode)
	}

	// Type a title
	for _, ch := range "MyNote" {
		c, _ = c.Update(cvKeyMsg(string(ch)))
	}
	if c.inputBuf != "MyNote" {
		t.Fatalf("expected inputBuf=MyNote, got %q", c.inputBuf)
	}

	// Press enter to create the card
	c, _ = c.Update(cvKeyMsg("enter"))
	if c.mode != canvasModeNormal {
		t.Fatalf("should return to normal mode, got %d", c.mode)
	}
	if len(c.cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(c.cards))
	}
	if c.cards[0].Title != "MyNote" {
		t.Errorf("expected title=MyNote, got %q", c.cards[0].Title)
	}
	if c.cards[0].NotePath != "MyNote.md" {
		t.Errorf("expected notePath=MyNote.md, got %q", c.cards[0].NotePath)
	}
	if c.cards[0].Width != canvasCardWidth {
		t.Errorf("expected width=%d, got %d", canvasCardWidth, c.cards[0].Width)
	}
}

// ---------- Card creation — empty title cancelled ----------

func TestCanvasCardCreationEmptyTitle(t *testing.T) {
	c := newTestCanvas()
	c, _ = c.Update(cvKeyMsg("n"))
	// Enter without typing anything
	c, _ = c.Update(cvKeyMsg("enter"))
	if len(c.cards) != 0 {
		t.Fatalf("empty title should not create a card, got %d cards", len(c.cards))
	}
}

// ---------- Card creation — esc cancels input mode ----------

func TestCanvasCardCreationEscCancel(t *testing.T) {
	c := newTestCanvas()
	c, _ = c.Update(cvKeyMsg("n"))
	c, _ = c.Update(cvKeyMsg("T"))
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if c.mode != canvasModeNormal {
		t.Fatalf("expected normal mode after esc, got %d", c.mode)
	}
	if len(c.cards) != 0 {
		t.Fatal("esc should cancel card creation")
	}
}

// ---------- Card creation — backspace in input mode ----------

func TestCanvasCardInputBackspace(t *testing.T) {
	c := newTestCanvas()
	c, _ = c.Update(cvKeyMsg("n"))
	c, _ = c.Update(cvKeyMsg("A"))
	c, _ = c.Update(cvKeyMsg("B"))
	c, _ = c.Update(cvKeyMsg("backspace"))
	if c.inputBuf != "A" {
		t.Fatalf("expected inputBuf=A after backspace, got %q", c.inputBuf)
	}
}

// ---------- Card movement — move mode changes card position ----------

func TestCanvasCardMovement(t *testing.T) {
	c := newTestCanvas()

	// Add a card at cursor position (0,0)
	c.cards = append(c.cards, CanvasCard{
		Title: "Movable", NotePath: "movable.md",
		X: 0, Y: 0, Width: canvasCardWidth,
	})
	c.selected = c.cardAtCursor()

	// Enter move mode: press 'm'
	c, _ = c.Update(cvKeyMsg("m"))
	if c.mode != canvasModeMove {
		t.Fatalf("expected move mode, got %d", c.mode)
	}
	if c.moveIdx != 0 {
		t.Fatalf("expected moveIdx=0, got %d", c.moveIdx)
	}

	origX := c.cards[0].X
	origY := c.cards[0].Y

	// Move right
	c, _ = c.Update(cvKeyMsg("right"))
	if c.cards[0].X != origX+1 {
		t.Fatalf("expected X=%d after right, got %d", origX+1, c.cards[0].X)
	}

	// Move down
	c, _ = c.Update(cvKeyMsg("down"))
	if c.cards[0].Y != origY+1 {
		t.Fatalf("expected Y=%d after down, got %d", origY+1, c.cards[0].Y)
	}

	// Confirm with enter
	c, _ = c.Update(cvKeyMsg("enter"))
	if c.mode != canvasModeNormal {
		t.Fatalf("expected normal mode after enter, got %d", c.mode)
	}
}

// ---------- Card movement — esc reverts position ----------

func TestCanvasCardMoveEscReverts(t *testing.T) {
	c := newTestCanvas()
	c.cards = append(c.cards, CanvasCard{
		Title: "Revert", NotePath: "revert.md",
		X: 5, Y: 5, Width: canvasCardWidth,
	})
	// Move cursor to card position
	c.cursorX = 5
	c.cursorY = 5
	c.selected = c.cardAtCursor()

	c, _ = c.Update(cvKeyMsg("m"))
	c, _ = c.Update(cvKeyMsg("right"))
	c, _ = c.Update(cvKeyMsg("right"))
	c, _ = c.Update(cvKeyMsg("down"))

	// Esc should revert
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if c.cards[0].X != 5 || c.cards[0].Y != 5 {
		t.Fatalf("esc should revert position to (5,5), got (%d,%d)", c.cards[0].X, c.cards[0].Y)
	}
}

// ---------- Card selection — cursor navigation between cards ----------

func TestCanvasCardSelection(t *testing.T) {
	c := newTestCanvas()
	c.cards = append(c.cards, CanvasCard{
		Title: "Card0", NotePath: "card0.md",
		X: 0, Y: 0, Width: canvasCardWidth,
	})
	c.cards = append(c.cards, CanvasCard{
		Title: "Card1", NotePath: "card1.md",
		X: 30, Y: 0, Width: canvasCardWidth,
	})
	c.cursorX = 0
	c.cursorY = 0
	c.selected = c.cardAtCursor()

	if c.selected != 0 {
		t.Fatalf("expected selected=0 at origin, got %d", c.selected)
	}

	// Move cursor right past first card to second card
	for i := 0; i < 30; i++ {
		c, _ = c.Update(cvKeyMsg("right"))
	}
	if c.selected != 1 {
		t.Fatalf("expected selected=1 after moving to card1, got %d", c.selected)
	}
}

// ---------- Card deletion — removing a card ----------

func TestCanvasCardDeletion(t *testing.T) {
	c := newTestCanvas()
	c.cards = append(c.cards, CanvasCard{
		Title: "Delete me", NotePath: "deleteme.md",
		X: 0, Y: 0, Width: canvasCardWidth,
	})
	c.cursorX = 0
	c.cursorY = 0
	c.selected = c.cardAtCursor()

	// Delete with 'd'
	c, _ = c.Update(cvKeyMsg("d"))
	if len(c.cards) != 0 {
		t.Fatalf("expected 0 cards after deletion, got %d", len(c.cards))
	}
}

// ---------- Card deletion — fixes connection indices ----------

func TestCanvasCardDeletionFixesConnections(t *testing.T) {
	c := newTestCanvas()
	c.cards = append(c.cards, CanvasCard{Title: "A", X: 0, Y: 0, Width: canvasCardWidth})
	c.cards = append(c.cards, CanvasCard{Title: "B", X: 30, Y: 0, Width: canvasCardWidth})
	c.cards = append(c.cards, CanvasCard{Title: "C", X: 60, Y: 0, Width: canvasCardWidth})

	// Connection from B(1) to C(2)
	c.connections = append(c.connections, CanvasConnection{FromIdx: 1, ToIdx: 2})

	// Delete A (index 0) — should shift B->0, C->1, connection 1->2 becomes 0->1
	c.removeCard(0)
	if len(c.cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(c.cards))
	}
	if len(c.connections) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(c.connections))
	}
	if c.connections[0].FromIdx != 0 || c.connections[0].ToIdx != 1 {
		t.Errorf("expected conn 0->1, got %d->%d", c.connections[0].FromIdx, c.connections[0].ToIdx)
	}
}

// ---------- Card deletion — removes connections involving deleted card ----------

func TestCanvasCardDeletionRemovesRelatedConnections(t *testing.T) {
	c := newTestCanvas()
	c.cards = append(c.cards, CanvasCard{Title: "A", X: 0, Y: 0, Width: canvasCardWidth})
	c.cards = append(c.cards, CanvasCard{Title: "B", X: 30, Y: 0, Width: canvasCardWidth})

	c.connections = append(c.connections, CanvasConnection{FromIdx: 0, ToIdx: 1})

	// Delete A — connection involving A should be removed
	c.removeCard(0)
	if len(c.connections) != 0 {
		t.Fatalf("expected 0 connections after deleting involved card, got %d", len(c.connections))
	}
}

// ---------- Connection creation — linking two cards ----------

func TestCanvasConnectionCreation(t *testing.T) {
	c := newTestCanvas()
	c.cards = append(c.cards, CanvasCard{
		Title: "Source", NotePath: "source.md",
		X: 0, Y: 0, Width: canvasCardWidth,
	})
	c.cards = append(c.cards, CanvasCard{
		Title: "Target", NotePath: "target.md",
		X: 30, Y: 0, Width: canvasCardWidth,
	})
	c.cursorX = 0
	c.cursorY = 0
	c.selected = c.cardAtCursor()

	// Enter connect mode with 'L'
	c, _ = c.Update(cvKeyMsg("L"))
	if c.mode != canvasModeConnect {
		t.Fatalf("expected connect mode, got %d", c.mode)
	}
	if c.connectFrom != 0 {
		t.Fatalf("expected connectFrom=0, got %d", c.connectFrom)
	}

	// Move to target card
	for i := 0; i < 30; i++ {
		c, _ = c.Update(cvKeyMsg("right"))
	}

	// Confirm connection with enter
	c, _ = c.Update(cvKeyMsg("enter"))
	if c.mode != canvasModeNormal {
		t.Fatalf("expected normal mode after connecting, got %d", c.mode)
	}
	if len(c.connections) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(c.connections))
	}
	if c.connections[0].FromIdx != 0 || c.connections[0].ToIdx != 1 {
		t.Errorf("expected conn 0->1, got %d->%d",
			c.connections[0].FromIdx, c.connections[0].ToIdx)
	}
}

// ---------- Connection — duplicate connections prevented ----------

func TestCanvasConnectionDuplicate(t *testing.T) {
	c := newTestCanvas()
	c.cards = append(c.cards, CanvasCard{Title: "A", X: 0, Y: 0, Width: canvasCardWidth})
	c.cards = append(c.cards, CanvasCard{Title: "B", X: 30, Y: 0, Width: canvasCardWidth})
	c.connections = append(c.connections, CanvasConnection{FromIdx: 0, ToIdx: 1})

	// connectionExists should detect existing connection
	if !c.connectionExists(0, 1) {
		t.Fatal("should detect existing connection 0->1")
	}
	// Also detect reverse direction
	if !c.connectionExists(1, 0) {
		t.Fatal("should detect existing connection 1->0 (reverse)")
	}
}

// ---------- Connection — self-connection prevented ----------

func TestCanvasConnectionSelf(t *testing.T) {
	c := newTestCanvas()
	c.cards = append(c.cards, CanvasCard{
		Title: "Self", X: 0, Y: 0, Width: canvasCardWidth,
	})
	c.cursorX = 0
	c.cursorY = 0
	c.selected = c.cardAtCursor()

	// Enter connect mode
	c, _ = c.Update(cvKeyMsg("L"))
	// Try to connect to same card (cursor is still on it)
	c, _ = c.Update(cvKeyMsg("enter"))
	if len(c.connections) != 0 {
		t.Fatal("self-connection should not be created")
	}
}

// ---------- Empty canvas — no cards, no crash ----------

func TestCanvasEmpty(t *testing.T) {
	c := newTestCanvas()

	// Navigation on empty canvas
	c, _ = c.Update(cvKeyMsg("up"))
	c, _ = c.Update(cvKeyMsg("down"))
	c, _ = c.Update(cvKeyMsg("left"))
	c, _ = c.Update(cvKeyMsg("right"))

	// Enter on empty canvas
	c, _ = c.Update(cvKeyMsg("enter"))

	// Delete on empty canvas
	c, _ = c.Update(cvKeyMsg("d"))

	// Move on empty canvas (no card under cursor)
	c, _ = c.Update(cvKeyMsg("m"))

	// Connect on empty canvas
	c, _ = c.Update(cvKeyMsg("L"))

	// View should not crash
	_ = c.View()
}

// ---------- Bounds checking — cursor clamping ----------

func TestCanvasCursorBounds(t *testing.T) {
	c := newTestCanvas()

	// Move cursor way to the left (should clamp at 0)
	for i := 0; i < 20; i++ {
		c, _ = c.Update(cvKeyMsg("left"))
	}
	if c.cursorX < 0 {
		t.Fatalf("cursorX should not go negative, got %d", c.cursorX)
	}

	// Move cursor way up (should clamp at 0)
	for i := 0; i < 20; i++ {
		c, _ = c.Update(cvKeyMsg("up"))
	}
	if c.cursorY < 0 {
		t.Fatalf("cursorY should not go negative, got %d", c.cursorY)
	}

	// Move cursor far right
	gw := c.canvasGridWidth()
	for i := 0; i < gw+20; i++ {
		c, _ = c.Update(cvKeyMsg("right"))
	}
	if c.cursorX >= gw {
		t.Fatalf("cursorX=%d should be < gridWidth=%d", c.cursorX, gw)
	}

	// Move cursor far down
	gh := c.canvasGridHeight()
	for i := 0; i < gh+20; i++ {
		c, _ = c.Update(cvKeyMsg("down"))
	}
	if c.cursorY >= gh {
		t.Fatalf("cursorY=%d should be < gridHeight=%d", c.cursorY, gh)
	}
}

// ---------- Mode switching — normal vs move vs connect vs input ----------

func TestCanvasModeSwitching(t *testing.T) {
	c := newTestCanvas()
	c.cards = append(c.cards, CanvasCard{
		Title: "Card", NotePath: "card.md",
		X: 0, Y: 0, Width: canvasCardWidth,
	})
	c.cursorX = 0
	c.cursorY = 0
	c.selected = c.cardAtCursor()

	// Normal -> input mode via 'n'
	c, _ = c.Update(cvKeyMsg("n"))
	if c.mode != canvasModeInput {
		t.Fatalf("expected input mode, got %d", c.mode)
	}
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if c.mode != canvasModeNormal {
		t.Fatalf("expected normal mode, got %d", c.mode)
	}

	// Normal -> move mode via 'm'
	c, _ = c.Update(cvKeyMsg("m"))
	if c.mode != canvasModeMove {
		t.Fatalf("expected move mode, got %d", c.mode)
	}
	c, _ = c.Update(cvKeyMsg("enter"))
	if c.mode != canvasModeNormal {
		t.Fatalf("expected normal mode after move enter, got %d", c.mode)
	}

	// Normal -> connect mode via 'L'
	c, _ = c.Update(cvKeyMsg("L"))
	if c.mode != canvasModeConnect {
		t.Fatalf("expected connect mode, got %d", c.mode)
	}
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if c.mode != canvasModeNormal {
		t.Fatalf("expected normal mode after connect esc, got %d", c.mode)
	}
}

// ---------- Zoom cycling ----------

func TestCanvasZoomCycling(t *testing.T) {
	c := newTestCanvas()

	if c.zoom != canvasZoomNormal {
		t.Fatalf("expected initial zoom=normal, got %d", c.zoom)
	}

	c, _ = c.Update(cvKeyMsg("z"))
	if c.zoom != canvasZoomCompact {
		t.Fatalf("expected zoom=compact, got %d", c.zoom)
	}

	c, _ = c.Update(cvKeyMsg("z"))
	if c.zoom != canvasZoomExpanded {
		t.Fatalf("expected zoom=expanded, got %d", c.zoom)
	}

	c, _ = c.Update(cvKeyMsg("z"))
	if c.zoom != canvasZoomNormal {
		t.Fatalf("expected zoom=normal after full cycle, got %d", c.zoom)
	}
}

// ---------- Card color cycling ----------

func TestCanvasCardColorCycling(t *testing.T) {
	c := newTestCanvas()
	c.cards = append(c.cards, CanvasCard{
		Title: "Colored", X: 0, Y: 0, Width: canvasCardWidth, Color: 0,
	})
	c.cursorX = 0
	c.cursorY = 0
	c.selected = c.cardAtCursor()

	c, _ = c.Update(cvKeyMsg("c"))
	if c.cards[0].Color != 1 {
		t.Fatalf("expected color=1, got %d", c.cards[0].Color)
	}

	// Cycle all the way around
	for i := 0; i < canvasCardNumColors-1; i++ {
		c, _ = c.Update(cvKeyMsg("c"))
	}
	if c.cards[0].Color != 0 {
		t.Fatalf("expected color=0 after full cycle, got %d", c.cards[0].Color)
	}
}

// ---------- Card resize ----------

func TestCanvasCardResize(t *testing.T) {
	c := newTestCanvas()
	c.cards = append(c.cards, CanvasCard{
		Title: "Resize", X: 0, Y: 0, Width: canvasCardWidth,
	})
	c.cursorX = 0
	c.cursorY = 0
	c.selected = c.cardAtCursor()

	origWidth := c.cards[0].Width

	// Increase width
	c, _ = c.Update(cvKeyMsg("+"))
	if c.cards[0].Width != origWidth+1 {
		t.Fatalf("expected width=%d, got %d", origWidth+1, c.cards[0].Width)
	}

	// Decrease width
	c, _ = c.Update(cvKeyMsg("-"))
	if c.cards[0].Width != origWidth {
		t.Fatalf("expected width=%d, got %d", origWidth, c.cards[0].Width)
	}

	// Cannot shrink below minimum
	for i := 0; i < 50; i++ {
		c, _ = c.Update(cvKeyMsg("-"))
	}
	if c.cards[0].Width < canvasCardMinWidth {
		t.Fatalf("width %d should not go below min %d", c.cards[0].Width, canvasCardMinWidth)
	}

	// Cannot grow past maximum
	for i := 0; i < 100; i++ {
		c, _ = c.Update(cvKeyMsg("+"))
	}
	if c.cards[0].Width > canvasCardMaxWidth {
		t.Fatalf("width %d should not exceed max %d", c.cards[0].Width, canvasCardMaxWidth)
	}
}

// ---------- SelectedNote — returns and clears ----------

func TestCanvasSelectedNote(t *testing.T) {
	c := newTestCanvas()
	c.cards = append(c.cards, CanvasCard{
		Title: "Pick", NotePath: "pick.md",
		X: 0, Y: 0, Width: canvasCardWidth,
	})
	c.cursorX = 0
	c.cursorY = 0
	c.selected = c.cardAtCursor()

	// Enter selects the note and closes canvas
	c, _ = c.Update(cvKeyMsg("enter"))
	if c.IsActive() {
		t.Fatal("canvas should close after enter on card")
	}

	first := c.SelectedNote()
	if first != "pick.md" {
		t.Fatalf("expected pick.md, got %q", first)
	}
	second := c.SelectedNote()
	if second != "" {
		t.Fatalf("second call should return empty, got %q", second)
	}
}

// ---------- Esc / q closes canvas ----------

func TestCanvasEscCloses(t *testing.T) {
	c := newTestCanvas()
	c, _ = c.Update(cvKeyMsg("q"))
	if c.IsActive() {
		t.Fatal("canvas should close on 'q'")
	}
}

// ---------- Inactive canvas ignores input ----------

func TestCanvasInactiveIgnoresInput(t *testing.T) {
	c := NewCanvas()
	c.SetSize(120, 40)
	// Not active

	c, _ = c.Update(cvKeyMsg("n"))
	if c.mode != canvasModeNormal {
		t.Fatal("inactive canvas should ignore all input")
	}
	if len(c.cards) != 0 {
		t.Fatal("inactive canvas should not create cards")
	}
}

// ---------- connectionsForCard ----------

func TestCanvasConnectionsForCard(t *testing.T) {
	c := newTestCanvas()
	c.cards = append(c.cards, CanvasCard{Title: "A"})
	c.cards = append(c.cards, CanvasCard{Title: "B"})
	c.cards = append(c.cards, CanvasCard{Title: "C"})

	c.connections = []CanvasConnection{
		{FromIdx: 0, ToIdx: 1},
		{FromIdx: 2, ToIdx: 0},
	}

	linked := c.connectionsForCard(0)
	if len(linked) != 2 {
		t.Fatalf("expected 2 connected cards for card 0, got %d", len(linked))
	}
	// Card 1 has only one connection (from 0)
	linked1 := c.connectionsForCard(1)
	if len(linked1) != 1 {
		t.Fatalf("expected 1 connected card for card 1, got %d", len(linked1))
	}
}

// ---------- View renders without panic ----------

func TestCanvasViewRenders(t *testing.T) {
	c := newTestCanvas()
	c.cards = append(c.cards, CanvasCard{
		Title: "A", NotePath: "a.md",
		X: 2, Y: 2, Width: canvasCardWidth, Color: 0,
	})
	c.cards = append(c.cards, CanvasCard{
		Title: "B", NotePath: "b.md",
		X: 30, Y: 5, Width: canvasCardWidth, Color: 3,
	})
	c.connections = []CanvasConnection{{FromIdx: 0, ToIdx: 1}}

	out := c.View()
	if out == "" {
		t.Fatal("View should produce non-empty output")
	}
}

// ---------- cardHeightForZoom ----------

func TestCanvasCardHeightForZoom(t *testing.T) {
	c := newTestCanvas()

	c.zoom = canvasZoomNormal
	if h := c.cardHeightForZoom(); h != canvasCardHeight {
		t.Fatalf("normal zoom: expected %d, got %d", canvasCardHeight, h)
	}

	c.zoom = canvasZoomCompact
	if h := c.cardHeightForZoom(); h != 1 {
		t.Fatalf("compact zoom: expected 1, got %d", h)
	}

	c.zoom = canvasZoomExpanded
	if h := c.cardHeightForZoom(); h != 5 {
		t.Fatalf("expanded zoom: expected 5, got %d", h)
	}
}

// ---------- Move mode with invalid moveIdx ----------

func TestCanvasMoveInvalidIdx(t *testing.T) {
	c := newTestCanvas()
	// Manually set invalid move state
	c.mode = canvasModeMove
	c.moveIdx = -1

	// Should return to normal mode without panic
	c, _ = c.Update(cvKeyMsg("right"))
	if c.mode != canvasModeNormal {
		t.Fatalf("expected normal mode with invalid moveIdx, got %d", c.mode)
	}
}

// ---------- Connect mode esc cancels ----------

func TestCanvasConnectEscCancels(t *testing.T) {
	c := newTestCanvas()
	c.cards = append(c.cards, CanvasCard{Title: "A", X: 0, Y: 0, Width: canvasCardWidth})
	c.cursorX = 0
	c.cursorY = 0
	c.selected = c.cardAtCursor()

	c, _ = c.Update(cvKeyMsg("L"))
	if c.mode != canvasModeConnect {
		t.Fatalf("expected connect mode, got %d", c.mode)
	}

	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if c.mode != canvasModeNormal {
		t.Fatalf("expected normal mode after esc, got %d", c.mode)
	}
	if c.connectFrom != -1 {
		t.Fatalf("connectFrom should be -1, got %d", c.connectFrom)
	}
	if len(c.connections) != 0 {
		t.Fatal("no connections should be created on cancel")
	}
}
