package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------- helpers ----------

func blKeyMsg(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		if len(s) == 1 {
			return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
		}
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// ---------- NewBacklinks ----------

func TestBacklinksNewBacklinks(t *testing.T) {
	bl := NewBacklinks()
	if bl.cursor != 0 {
		t.Fatalf("expected cursor=0, got %d", bl.cursor)
	}
	if bl.focused {
		t.Fatal("expected unfocused")
	}
	if bl.mode != 0 {
		t.Fatalf("expected mode=0 (incoming), got %d", bl.mode)
	}
	if len(bl.incoming) != 0 {
		t.Fatalf("expected empty incoming, got %d", len(bl.incoming))
	}
	if len(bl.outgoing) != 0 {
		t.Fatalf("expected empty outgoing, got %d", len(bl.outgoing))
	}
}

// ---------- SetSize ----------

func TestBacklinksSetSize(t *testing.T) {
	bl := NewBacklinks()
	bl.SetSize(40, 30)
	if bl.width != 40 || bl.height != 30 {
		t.Fatalf("expected 40x30, got %dx%d", bl.width, bl.height)
	}
}

// ---------- SetLinks — setting links updates lists ----------

func TestBacklinksSetLinks(t *testing.T) {
	bl := NewBacklinks()
	incoming := []BacklinkItem{
		{Path: "note-a.md", Context: "mentions [[target]]", LineNum: 5},
		{Path: "note-b.md", Context: "also links to [[target]]", LineNum: 10},
	}
	outgoing := []BacklinkItem{
		{Path: "ref-1.md", Context: "outgoing link", LineNum: 3},
	}
	bl.SetLinks(incoming, outgoing)

	if len(bl.incoming) != 2 {
		t.Fatalf("expected 2 incoming, got %d", len(bl.incoming))
	}
	if len(bl.outgoing) != 1 {
		t.Fatalf("expected 1 outgoing, got %d", len(bl.outgoing))
	}
	// Cursor should reset
	if bl.cursor != 0 {
		t.Fatalf("expected cursor=0 after SetLinks, got %d", bl.cursor)
	}
}

// ---------- Backlink detection — notes linking TO the active note ----------

func TestBacklinksDetection(t *testing.T) {
	bl := NewBacklinks()
	bl.SetSize(40, 30)
	bl.focused = true

	incoming := []BacklinkItem{
		{Path: "linker.md", Context: "see [[target]]", LineNum: 1},
	}
	bl.SetLinks(incoming, nil)

	items := bl.currentItems()
	if len(items) != 1 {
		t.Fatalf("expected 1 backlink item, got %d", len(items))
	}
	if items[0].Path != "linker.md" {
		t.Errorf("expected path=linker.md, got %q", items[0].Path)
	}
}

// ---------- Outgoing links — toggle shows links FROM the active note ----------

func TestBacklinksOutgoingToggle(t *testing.T) {
	bl := NewBacklinks()
	bl.SetSize(40, 30)
	bl.focused = true

	incoming := []BacklinkItem{{Path: "inc.md"}}
	outgoing := []BacklinkItem{{Path: "out1.md"}, {Path: "out2.md"}}
	bl.SetLinks(incoming, outgoing)

	// Default mode=0 shows incoming
	items := bl.currentItems()
	if len(items) != 1 {
		t.Fatalf("expected 1 incoming item, got %d", len(items))
	}

	// Tab switches to outgoing
	bl, _ = bl.Update(blKeyMsg("tab"))
	if bl.mode != 1 {
		t.Fatalf("expected mode=1 after tab, got %d", bl.mode)
	}
	items = bl.currentItems()
	if len(items) != 2 {
		t.Fatalf("expected 2 outgoing items, got %d", len(items))
	}
}

// ---------- Mode toggle — switch between backlinks/outgoing ----------

func TestBacklinksModeToggle(t *testing.T) {
	bl := NewBacklinks()
	bl.SetSize(40, 30)
	bl.focused = true
	bl.SetLinks(
		[]BacklinkItem{{Path: "a.md"}},
		[]BacklinkItem{{Path: "b.md"}},
	)

	// Tab once -> mode 1
	bl, _ = bl.Update(blKeyMsg("tab"))
	if bl.mode != 1 {
		t.Fatal("expected mode=1")
	}
	// Tab again -> back to mode 0
	bl, _ = bl.Update(blKeyMsg("tab"))
	if bl.mode != 0 {
		t.Fatal("expected mode=0")
	}
}

// ---------- Navigation — up/down through link list ----------

func TestBacklinksNavigation(t *testing.T) {
	bl := NewBacklinks()
	bl.SetSize(40, 30)
	bl.focused = true

	items := []BacklinkItem{
		{Path: "first.md"},
		{Path: "second.md"},
		{Path: "third.md"},
	}
	bl.SetLinks(items, nil)

	// Move down
	bl, _ = bl.Update(blKeyMsg("down"))
	if bl.cursor != 1 {
		t.Fatalf("expected cursor=1 after down, got %d", bl.cursor)
	}

	// Move down with j
	bl, _ = bl.Update(blKeyMsg("j"))
	if bl.cursor != 2 {
		t.Fatalf("expected cursor=2 after j, got %d", bl.cursor)
	}

	// Move up
	bl, _ = bl.Update(blKeyMsg("up"))
	if bl.cursor != 1 {
		t.Fatalf("expected cursor=1 after up, got %d", bl.cursor)
	}

	// Move up with k
	bl, _ = bl.Update(blKeyMsg("k"))
	if bl.cursor != 0 {
		t.Fatalf("expected cursor=0 after k, got %d", bl.cursor)
	}
}

// ---------- Selection — Selected() returns the target note path ----------

func TestBacklinksSelected(t *testing.T) {
	bl := NewBacklinks()
	bl.SetSize(40, 30)
	bl.focused = true

	items := []BacklinkItem{
		{Path: "first.md"},
		{Path: "second.md"},
	}
	bl.SetLinks(items, nil)

	// Select first item
	sel := bl.Selected()
	if sel != "first.md" {
		t.Fatalf("expected first.md, got %q", sel)
	}

	// Move down and select second
	bl, _ = bl.Update(blKeyMsg("down"))
	sel = bl.Selected()
	if sel != "second.md" {
		t.Fatalf("expected second.md, got %q", sel)
	}
}

// ---------- No backlinks — empty list ----------

func TestBacklinksEmptyIncoming(t *testing.T) {
	bl := NewBacklinks()
	bl.SetSize(40, 30)
	bl.focused = true
	bl.SetLinks(nil, []BacklinkItem{{Path: "out.md"}})

	items := bl.currentItems()
	if len(items) != 0 {
		t.Fatalf("expected 0 incoming items, got %d", len(items))
	}

	sel := bl.Selected()
	if sel != "" {
		t.Fatalf("expected empty selection for empty list, got %q", sel)
	}

	// View should not panic
	_ = bl.View()
}

// ---------- No outgoing — empty outgoing list ----------

func TestBacklinksEmptyOutgoing(t *testing.T) {
	bl := NewBacklinks()
	bl.SetSize(40, 30)
	bl.focused = true
	bl.SetLinks([]BacklinkItem{{Path: "inc.md"}}, nil)

	// Switch to outgoing
	bl, _ = bl.Update(blKeyMsg("tab"))
	items := bl.currentItems()
	if len(items) != 0 {
		t.Fatalf("expected 0 outgoing items, got %d", len(items))
	}

	sel := bl.Selected()
	if sel != "" {
		t.Fatalf("expected empty selection for empty outgoing, got %q", sel)
	}

	// View should not panic
	_ = bl.View()
}

// ---------- Self-links — note linking to itself ----------

func TestBacklinksSelfLink(t *testing.T) {
	bl := NewBacklinks()
	bl.SetSize(40, 30)
	bl.focused = true

	// A note links to itself — it appears in both incoming and outgoing
	incoming := []BacklinkItem{{Path: "self.md", Context: "[[self]]"}}
	outgoing := []BacklinkItem{{Path: "self.md", Context: "[[self]]"}}
	bl.SetLinks(incoming, outgoing)

	// Check incoming mode
	items := bl.currentItems()
	if len(items) != 1 || items[0].Path != "self.md" {
		t.Fatal("incoming should show self-link")
	}

	// Switch to outgoing
	bl, _ = bl.Update(blKeyMsg("tab"))
	items = bl.currentItems()
	if len(items) != 1 || items[0].Path != "self.md" {
		t.Fatal("outgoing should show self-link")
	}
}

// ---------- Cursor bounds — doesn't go past end of list ----------

func TestBacklinksCursorBounds(t *testing.T) {
	bl := NewBacklinks()
	bl.SetSize(40, 30)
	bl.focused = true

	items := []BacklinkItem{{Path: "a.md"}, {Path: "b.md"}}
	bl.SetLinks(items, nil)

	// Move down past end
	for i := 0; i < 10; i++ {
		bl, _ = bl.Update(blKeyMsg("down"))
	}
	if bl.cursor != 1 {
		t.Fatalf("cursor should clamp at 1, got %d", bl.cursor)
	}

	// Move up past start
	for i := 0; i < 10; i++ {
		bl, _ = bl.Update(blKeyMsg("up"))
	}
	if bl.cursor != 0 {
		t.Fatalf("cursor should clamp at 0, got %d", bl.cursor)
	}
}

// ---------- Tab resets cursor ----------

func TestBacklinksTabResetsCursor(t *testing.T) {
	bl := NewBacklinks()
	bl.SetSize(40, 30)
	bl.focused = true

	items := []BacklinkItem{{Path: "a.md"}, {Path: "b.md"}, {Path: "c.md"}}
	bl.SetLinks(items, items)

	// Move cursor down
	bl, _ = bl.Update(blKeyMsg("down"))
	bl, _ = bl.Update(blKeyMsg("down"))
	if bl.cursor != 2 {
		t.Fatalf("expected cursor=2, got %d", bl.cursor)
	}

	// Tab should reset cursor to 0
	bl, _ = bl.Update(blKeyMsg("tab"))
	if bl.cursor != 0 {
		t.Fatalf("expected cursor=0 after tab, got %d", bl.cursor)
	}
}

// ---------- Unfocused ignores input ----------

func TestBacklinksUnfocusedIgnoresInput(t *testing.T) {
	bl := NewBacklinks()
	bl.SetSize(40, 30)
	bl.focused = false // not focused

	items := []BacklinkItem{{Path: "a.md"}, {Path: "b.md"}}
	bl.SetLinks(items, nil)

	bl, _ = bl.Update(blKeyMsg("down"))
	if bl.cursor != 0 {
		t.Fatalf("unfocused backlinks should ignore input, cursor=%d", bl.cursor)
	}
}

// ---------- View renders without panic ----------

func TestBacklinksViewRenders(t *testing.T) {
	bl := NewBacklinks()
	bl.SetSize(50, 30)
	bl.focused = true

	items := []BacklinkItem{
		{Path: "note-a.md", Context: "references [[target]]", LineNum: 5},
		{Path: "note-b.md", Context: "also links to [[target]]", LineNum: 10},
	}
	bl.SetLinks(items, []BacklinkItem{{Path: "out.md"}})

	out := bl.View()
	if out == "" {
		t.Fatal("View should produce non-empty output")
	}

	// Switch to outgoing and view
	bl, _ = bl.Update(blKeyMsg("tab"))
	out = bl.View()
	if out == "" {
		t.Fatal("outgoing View should produce non-empty output")
	}
}

// ---------- formatTabLabel ----------

func TestBacklinksFormatTabLabel(t *testing.T) {
	tests := []struct {
		label string
		count int
		want  string
	}{
		{"Backlinks", 0, "Backlinks"},
		{"Backlinks", 3, "Backlinks 3"},
		{"Outgoing", 12, "Outgoing 12"},
	}
	for _, tt := range tests {
		got := formatTabLabel(tt.label, tt.count)
		if got != tt.want {
			t.Errorf("formatTabLabel(%q, %d) = %q, want %q", tt.label, tt.count, got, tt.want)
		}
	}
}

// ---------- truncateWithLinks ----------

func TestBacklinksTruncateWithLinks(t *testing.T) {
	tests := []struct {
		line     string
		maxWidth int
		wantEnd  string
	}{
		{"short", 20, "short"},
		{"a very long line that exceeds the max width", 10, "..."},
		{"tiny", 3, "..."},
	}
	for _, tt := range tests {
		got := truncateWithLinks(tt.line, tt.maxWidth)
		if len(got) > tt.maxWidth && len(tt.line) > tt.maxWidth {
			t.Errorf("truncateWithLinks(%q, %d) length %d > maxWidth", tt.line, tt.maxWidth, len(got))
		}
		if tt.maxWidth <= 3 && got != "..." {
			t.Errorf("expected '...' for maxWidth<=3, got %q", got)
		}
	}
}
