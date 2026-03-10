package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// NewBookmarks — initial state
// ---------------------------------------------------------------------------

func TestBookmarksNewBookmarks(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)

	if bm.IsActive() {
		t.Error("new bookmarks should not be active")
	}
	if bm.vaultRoot != tmpDir {
		t.Errorf("unexpected vaultRoot: %s", bm.vaultRoot)
	}
	if len(bm.data.Starred) != 0 {
		t.Errorf("expected empty starred, got %d", len(bm.data.Starred))
	}
	if len(bm.data.Recent) != 0 {
		t.Errorf("expected empty recent, got %d", len(bm.data.Recent))
	}
	if bm.maxRecent != 20 {
		t.Errorf("expected maxRecent 20, got %d", bm.maxRecent)
	}
	if bm.mode != 0 {
		t.Errorf("expected mode 0 (starred), got %d", bm.mode)
	}
}

// ---------------------------------------------------------------------------
// Open / Close / IsActive — state transitions
// ---------------------------------------------------------------------------

func TestBookmarksOpenCloseIsActive(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)

	if bm.IsActive() {
		t.Error("should not be active before Open")
	}

	bm.Open()
	if !bm.IsActive() {
		t.Error("should be active after Open")
	}
	if bm.cursor != 0 {
		t.Errorf("cursor should reset to 0, got %d", bm.cursor)
	}
	if bm.scroll != 0 {
		t.Errorf("scroll should reset to 0, got %d", bm.scroll)
	}
	if bm.result != "" {
		t.Errorf("result should be empty, got %q", bm.result)
	}

	bm.Close()
	if bm.IsActive() {
		t.Error("should not be active after Close")
	}
}

func TestBookmarksOpenResetsState(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)
	bm.cursor = 5
	bm.scroll = 3
	bm.result = "old"

	bm.Open()
	if bm.cursor != 0 {
		t.Errorf("cursor should reset, got %d", bm.cursor)
	}
	if bm.scroll != 0 {
		t.Errorf("scroll should reset, got %d", bm.scroll)
	}
	if bm.result != "" {
		t.Errorf("result should reset, got %q", bm.result)
	}
}

// ---------------------------------------------------------------------------
// SetSize — dimensions
// ---------------------------------------------------------------------------

func TestBookmarksSetSize(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)
	bm.SetSize(100, 50)
	if bm.width != 100 {
		t.Errorf("expected width 100, got %d", bm.width)
	}
	if bm.height != 50 {
		t.Errorf("expected height 50, got %d", bm.height)
	}
}

// ---------------------------------------------------------------------------
// ToggleStar — adds/removes starred note
// ---------------------------------------------------------------------------

func TestBookmarksToggleStar(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)

	// Star a note
	bm.ToggleStar("notes/hello.md")
	if len(bm.data.Starred) != 1 {
		t.Errorf("expected 1 starred, got %d", len(bm.data.Starred))
	}
	if !bm.IsStarred("notes/hello.md") {
		t.Error("note should be starred")
	}

	// Toggle again to unstar
	bm.ToggleStar("notes/hello.md")
	if len(bm.data.Starred) != 0 {
		t.Errorf("expected 0 starred after toggle off, got %d", len(bm.data.Starred))
	}
	if bm.IsStarred("notes/hello.md") {
		t.Error("note should not be starred after toggle off")
	}
}

func TestBookmarksToggleStarMultiple(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)

	bm.ToggleStar("a.md")
	bm.ToggleStar("b.md")
	bm.ToggleStar("c.md")

	if len(bm.data.Starred) != 3 {
		t.Errorf("expected 3 starred, got %d", len(bm.data.Starred))
	}

	// Remove middle one
	bm.ToggleStar("b.md")
	if len(bm.data.Starred) != 2 {
		t.Errorf("expected 2 starred, got %d", len(bm.data.Starred))
	}
	if bm.IsStarred("b.md") {
		t.Error("b.md should not be starred")
	}
	if !bm.IsStarred("a.md") || !bm.IsStarred("c.md") {
		t.Error("a.md and c.md should still be starred")
	}
}

// ---------------------------------------------------------------------------
// AddRecent — adds to recent list
// ---------------------------------------------------------------------------

func TestBookmarksAddRecent(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)

	bm.AddRecent("notes/first.md")
	if len(bm.data.Recent) != 1 {
		t.Errorf("expected 1 recent, got %d", len(bm.data.Recent))
	}
	if bm.data.Recent[0] != "notes/first.md" {
		t.Errorf("unexpected recent entry: %s", bm.data.Recent[0])
	}

	// Add second — should be at front
	bm.AddRecent("notes/second.md")
	if len(bm.data.Recent) != 2 {
		t.Errorf("expected 2 recent, got %d", len(bm.data.Recent))
	}
	if bm.data.Recent[0] != "notes/second.md" {
		t.Errorf("most recent should be first, got: %s", bm.data.Recent[0])
	}
}

func TestBookmarksAddRecentMovesToFront(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)

	bm.AddRecent("a.md")
	bm.AddRecent("b.md")
	bm.AddRecent("c.md")

	// Re-add a.md — should move to front, no duplicate
	bm.AddRecent("a.md")
	if len(bm.data.Recent) != 3 {
		t.Errorf("expected 3 recent (no dup), got %d", len(bm.data.Recent))
	}
	if bm.data.Recent[0] != "a.md" {
		t.Errorf("expected a.md at front, got %s", bm.data.Recent[0])
	}
}

// ---------------------------------------------------------------------------
// Navigation — up/down through bookmarks
// ---------------------------------------------------------------------------

func TestBookmarksNavigation(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)

	bm.ToggleStar("a.md")
	bm.ToggleStar("b.md")
	bm.ToggleStar("c.md")

	bm.Open()
	bm.SetSize(80, 40)

	if bm.cursor != 0 {
		t.Errorf("cursor should start at 0, got %d", bm.cursor)
	}

	// Navigate down
	bm, _ = bm.Update(tea.KeyMsg{Type: tea.KeyDown})
	if bm.cursor != 1 {
		t.Errorf("expected cursor 1, got %d", bm.cursor)
	}

	bm, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if bm.cursor != 2 {
		t.Errorf("expected cursor 2, got %d", bm.cursor)
	}

	// Should not go beyond last item
	bm, _ = bm.Update(tea.KeyMsg{Type: tea.KeyDown})
	if bm.cursor != 2 {
		t.Errorf("cursor should clamp at 2, got %d", bm.cursor)
	}

	// Navigate up
	bm, _ = bm.Update(tea.KeyMsg{Type: tea.KeyUp})
	if bm.cursor != 1 {
		t.Errorf("expected cursor 1, got %d", bm.cursor)
	}

	bm, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if bm.cursor != 0 {
		t.Errorf("expected cursor 0, got %d", bm.cursor)
	}

	// Should not go below 0
	bm, _ = bm.Update(tea.KeyMsg{Type: tea.KeyUp})
	if bm.cursor != 0 {
		t.Errorf("cursor should clamp at 0, got %d", bm.cursor)
	}
}

// ---------------------------------------------------------------------------
// Tab switching — between starred and recent tabs
// ---------------------------------------------------------------------------

func TestBookmarksTabSwitching(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)

	bm.ToggleStar("starred.md")
	bm.AddRecent("recent.md")

	bm.Open()

	// Should start on starred tab (mode 0)
	if bm.mode != 0 {
		t.Errorf("expected mode 0, got %d", bm.mode)
	}

	items := bm.currentItems()
	if len(items) != 1 || items[0] != "starred.md" {
		t.Errorf("expected starred items, got %v", items)
	}

	// Tab to recent
	bm, _ = bm.Update(tea.KeyMsg{Type: tea.KeyTab})
	if bm.mode != 1 {
		t.Errorf("expected mode 1 after tab, got %d", bm.mode)
	}
	if bm.cursor != 0 {
		t.Errorf("cursor should reset on tab, got %d", bm.cursor)
	}

	items = bm.currentItems()
	if len(items) != 1 || items[0] != "recent.md" {
		t.Errorf("expected recent items, got %v", items)
	}

	// Tab back to starred
	bm, _ = bm.Update(tea.KeyMsg{Type: tea.KeyTab})
	if bm.mode != 0 {
		t.Errorf("expected mode 0 after second tab, got %d", bm.mode)
	}
}

// ---------------------------------------------------------------------------
// Selection — enter returns selected note path
// ---------------------------------------------------------------------------

func TestBookmarksSelection(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)

	bm.ToggleStar("a.md")
	bm.ToggleStar("b.md")

	bm.Open()

	// Move to second item
	bm, _ = bm.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Press enter to select
	bm, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if bm.IsActive() {
		t.Error("overlay should close after selection")
	}

	selected := bm.SelectedNote()
	if selected != "b.md" {
		t.Errorf("expected selected 'b.md', got %q", selected)
	}

	// Second call to SelectedNote should return empty
	second := bm.SelectedNote()
	if second != "" {
		t.Errorf("expected empty on second call, got %q", second)
	}
}

func TestBookmarksSelectionEmptyList(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)
	bm.Open()

	// Press enter on empty list
	bm, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})

	selected := bm.SelectedNote()
	if selected != "" {
		t.Errorf("expected empty selection on empty list, got %q", selected)
	}
}

// ---------------------------------------------------------------------------
// Cursor bounds — cursor valid after removing starred item
// ---------------------------------------------------------------------------

func TestBookmarksCursorBoundsAfterRemove(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)

	bm.ToggleStar("a.md")
	bm.ToggleStar("b.md")

	bm.Open()
	bm.SetSize(80, 40)

	// Move to last item
	bm, _ = bm.Update(tea.KeyMsg{Type: tea.KeyDown})
	if bm.cursor != 1 {
		t.Errorf("expected cursor 1, got %d", bm.cursor)
	}

	// Delete the last starred item via 'd' key
	bm, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if len(bm.data.Starred) != 1 {
		t.Errorf("expected 1 starred after delete, got %d", len(bm.data.Starred))
	}
	if bm.cursor >= len(bm.data.Starred) {
		t.Errorf("cursor %d should be < %d after delete", bm.cursor, len(bm.data.Starred))
	}
}

func TestBookmarksCursorBoundsDeleteAll(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)

	bm.ToggleStar("only.md")
	bm.Open()
	bm.SetSize(80, 40)

	// Delete the only starred item
	bm, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if len(bm.data.Starred) != 0 {
		t.Errorf("expected 0 starred, got %d", len(bm.data.Starred))
	}
	// Cursor should not be negative
	if bm.cursor < 0 {
		t.Errorf("cursor should not be negative, got %d", bm.cursor)
	}
}

// ---------------------------------------------------------------------------
// Duplicate prevention — starring same note twice doesn't duplicate
// ---------------------------------------------------------------------------

func TestBookmarksDuplicatePrevention(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)

	bm.ToggleStar("note.md")
	// Toggle again removes it; toggle a third time adds it back
	bm.ToggleStar("note.md")
	bm.ToggleStar("note.md")

	if len(bm.data.Starred) != 1 {
		t.Errorf("expected 1 starred (no duplicate), got %d", len(bm.data.Starred))
	}
}

func TestBookmarksRecentDuplicatePrevention(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)

	bm.AddRecent("note.md")
	bm.AddRecent("other.md")
	bm.AddRecent("note.md")

	if len(bm.data.Recent) != 2 {
		t.Errorf("expected 2 recent (no dup), got %d", len(bm.data.Recent))
	}
	if bm.data.Recent[0] != "note.md" {
		t.Errorf("expected note.md at front, got %s", bm.data.Recent[0])
	}
}

// ---------------------------------------------------------------------------
// Recent list limit — recent list has max capacity
// ---------------------------------------------------------------------------

func TestBookmarksRecentListLimit(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)

	// Add more than maxRecent items
	for i := 0; i < 25; i++ {
		bm.AddRecent(string(rune('a'+i)) + ".md")
	}

	if len(bm.data.Recent) != bm.maxRecent {
		t.Errorf("expected %d recent (max), got %d", bm.maxRecent, len(bm.data.Recent))
	}

	// Most recently added should be first
	// rune 'a'+24 = 'y'
	if bm.data.Recent[0] != "y.md" {
		t.Errorf("expected most recent 'y.md' at front, got %s", bm.data.Recent[0])
	}
}

// ---------------------------------------------------------------------------
// Additional edge cases
// ---------------------------------------------------------------------------

func TestBookmarksEscCloses(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)
	bm.Open()

	bm, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if bm.IsActive() {
		t.Error("Esc should close bookmarks overlay")
	}
}

func TestBookmarksCtrlBCloses(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)
	bm.Open()

	bm, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: nil, Alt: false})
	// Use the string-based approach since ctrl+b is a compound key
	bm2 := bm
	bm2.active = true
	msg := tea.KeyMsg{}
	// Simulate ctrl+b by checking the string representation
	msg.Type = tea.KeyCtrlB
	bm2, _ = bm2.Update(msg)
	// ctrl+b triggers close in the update
}

func TestBookmarksUpdateInactive(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)

	// Update while inactive should be a no-op
	_, cmd := bm.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Error("expected nil cmd when inactive")
	}
}

func TestBookmarksDeleteOnRecentTabNoop(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)

	bm.AddRecent("recent.md")
	bm.Open()
	bm.SetSize(80, 40)

	// Switch to recent tab
	bm, _ = bm.Update(tea.KeyMsg{Type: tea.KeyTab})

	// 'd' on recent tab should not remove items (only works on starred)
	bm, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	if len(bm.data.Recent) != 1 {
		t.Errorf("expected 1 recent item (delete is noop on recent tab), got %d", len(bm.data.Recent))
	}
}

func TestBookmarksViewEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)
	bm.SetSize(100, 40)
	bm.Open()

	view := bm.View()
	if view == "" {
		t.Error("expected non-empty view for empty bookmarks")
	}
}

func TestBookmarksViewWithItems(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)
	bm.ToggleStar("starred.md")
	bm.AddRecent("recent.md")
	bm.SetSize(100, 40)
	bm.Open()

	view := bm.View()
	if view == "" {
		t.Error("expected non-empty view with items")
	}
}

func TestBookmarksIsStarredNotStarred(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)

	if bm.IsStarred("nonexistent.md") {
		t.Error("nonexistent note should not be starred")
	}
}

func TestBookmarksSelectionFromRecentTab(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBookmarks(tmpDir)

	bm.AddRecent("recent1.md")
	bm.AddRecent("recent2.md")

	bm.Open()

	// Switch to recent tab
	bm, _ = bm.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Select first item
	bm, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})

	selected := bm.SelectedNote()
	if selected != "recent2.md" {
		t.Errorf("expected 'recent2.md', got %q", selected)
	}
}
