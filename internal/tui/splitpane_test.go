package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSplitPaneFilterNotes(t *testing.T) {
	sp := NewSplitPane()
	sp.SetNotes([]string{
		"notes/hello.md",
		"notes/world.md",
		"journal/2026-03-01.md",
		"journal/2026-03-02.md",
		"README.md",
	})

	t.Run("empty query returns all notes", func(t *testing.T) {
		sp.pickQuery = ""
		sp.filterNotes()
		if len(sp.filteredNotes) != 5 {
			t.Fatalf("expected 5 notes, got %d", len(sp.filteredNotes))
		}
	})

	t.Run("substring match filters correctly", func(t *testing.T) {
		sp.pickQuery = "journal"
		sp.filterNotes()
		if len(sp.filteredNotes) != 2 {
			t.Fatalf("expected 2 notes matching 'journal', got %d", len(sp.filteredNotes))
		}
		for _, n := range sp.filteredNotes {
			if n != "journal/2026-03-01.md" && n != "journal/2026-03-02.md" {
				t.Errorf("unexpected note in results: %s", n)
			}
		}
	})

	t.Run("case insensitive matching", func(t *testing.T) {
		sp.pickQuery = "README"
		sp.filterNotes()
		if len(sp.filteredNotes) != 1 {
			t.Fatalf("expected 1 note matching 'README', got %d", len(sp.filteredNotes))
		}
		if sp.filteredNotes[0] != "README.md" {
			t.Errorf("expected README.md, got %s", sp.filteredNotes[0])
		}
	})

	t.Run("no matches returns empty", func(t *testing.T) {
		sp.pickQuery = "nonexistent"
		sp.filterNotes()
		if len(sp.filteredNotes) != 0 {
			t.Fatalf("expected 0 notes, got %d", len(sp.filteredNotes))
		}
	})

	t.Run("cursor clamped when filter reduces list", func(t *testing.T) {
		sp.pickQuery = ""
		sp.filterNotes()
		sp.pickCursor = 4 // point to last of 5
		sp.pickQuery = "hello"
		sp.filterNotes()
		// Only 1 result, cursor should be clamped to 0
		if sp.pickCursor != 0 {
			t.Errorf("expected cursor at 0, got %d", sp.pickCursor)
		}
	})
}

func TestSplitPanePickerNavigation(t *testing.T) {
	sp := NewSplitPane()
	sp.SetSize(120, 40)
	sp.SetNotes([]string{"a.md", "b.md", "c.md", "d.md"})
	sp.Open()

	if !sp.picking {
		t.Fatal("expected picking mode after Open")
	}

	t.Run("down moves cursor", func(t *testing.T) {
		sp.pickCursor = 0
		sp, _ = sp.Update(tea.KeyMsg{Type: tea.KeyDown})
		if sp.pickCursor != 1 {
			t.Errorf("expected cursor at 1, got %d", sp.pickCursor)
		}
	})

	t.Run("up moves cursor back", func(t *testing.T) {
		sp.pickCursor = 2
		sp, _ = sp.Update(tea.KeyMsg{Type: tea.KeyUp})
		if sp.pickCursor != 1 {
			t.Errorf("expected cursor at 1, got %d", sp.pickCursor)
		}
	})

	t.Run("up at top stays at 0", func(t *testing.T) {
		sp.pickCursor = 0
		sp, _ = sp.Update(tea.KeyMsg{Type: tea.KeyUp})
		if sp.pickCursor != 0 {
			t.Errorf("expected cursor at 0, got %d", sp.pickCursor)
		}
	})

	t.Run("down at bottom stays at last", func(t *testing.T) {
		sp.pickCursor = 3
		sp, _ = sp.Update(tea.KeyMsg{Type: tea.KeyDown})
		if sp.pickCursor != 3 {
			t.Errorf("expected cursor at 3, got %d", sp.pickCursor)
		}
	})

	t.Run("typing filters and resets cursor", func(t *testing.T) {
		sp.pickCursor = 2
		// Type 'a' to filter to just "a.md"
		sp, _ = sp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
		if sp.pickQuery != "a" {
			t.Errorf("expected pickQuery 'a', got %q", sp.pickQuery)
		}
		if sp.pickCursor != 0 {
			t.Errorf("expected cursor reset to 0, got %d", sp.pickCursor)
		}
		if len(sp.filteredNotes) != 1 {
			t.Errorf("expected 1 filtered note, got %d", len(sp.filteredNotes))
		}
	})

	t.Run("backspace removes last character", func(t *testing.T) {
		sp.pickQuery = "ab"
		sp.filterNotes()
		sp, _ = sp.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		if sp.pickQuery != "a" {
			t.Errorf("expected pickQuery 'a' after backspace, got %q", sp.pickQuery)
		}
	})
}

func TestSplitPaneSetNotesAndSelection(t *testing.T) {
	sp := NewSplitPane()
	sp.SetSize(120, 40)

	notes := []string{"notes/alpha.md", "notes/beta.md", "notes/gamma.md"}
	sp.SetNotes(notes)
	sp.Open()

	t.Run("SetNotes populates allNotes", func(t *testing.T) {
		if len(sp.allNotes) != 3 {
			t.Fatalf("expected 3 allNotes, got %d", len(sp.allNotes))
		}
	})

	t.Run("filtered notes match all on open", func(t *testing.T) {
		if len(sp.filteredNotes) != 3 {
			t.Fatalf("expected 3 filteredNotes, got %d", len(sp.filteredNotes))
		}
	})

	t.Run("enter selects note and exits picker", func(t *testing.T) {
		sp.pickCursor = 1 // "notes/beta.md"
		var cmd tea.Cmd
		sp, cmd = sp.Update(tea.KeyMsg{Type: tea.KeyEnter})

		if sp.picking {
			t.Error("expected picking to be false after selection")
		}
		if sp.rightNotePath != "notes/beta.md" {
			t.Errorf("expected rightNotePath 'notes/beta.md', got %q", sp.rightNotePath)
		}
		if sp.GetRightNotePath() != "notes/beta.md" {
			t.Errorf("GetRightNotePath should return 'notes/beta.md', got %q", sp.GetRightNotePath())
		}
		// Command should produce a splitPanePickMsg
		if cmd == nil {
			t.Fatal("expected a non-nil command after selection")
		}
		msgResult := cmd()
		pickMsg, ok := msgResult.(splitPanePickMsg)
		if !ok {
			t.Fatalf("expected splitPanePickMsg, got %T", msgResult)
		}
		if pickMsg.notePath != "notes/beta.md" {
			t.Errorf("expected notePath 'notes/beta.md', got %q", pickMsg.notePath)
		}
	})

	t.Run("p key re-opens picker in normal mode", func(t *testing.T) {
		sp.picking = false
		sp, _ = sp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		if !sp.picking {
			t.Error("expected picking mode after pressing p")
		}
	})
}

func TestSplitPaneScrollBounds(t *testing.T) {
	sp := NewSplitPane()
	sp.SetSize(120, 17) // visibleHeight = 17 - 7 = 10
	sp.Open()
	sp.picking = false

	// Create a left pane with 20 lines
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "line content"
	}
	sp.SetLeftContent("test.md", lines)
	sp.focus = 0

	t.Run("scroll down respects max", func(t *testing.T) {
		sp.left.scroll = 0
		// Scroll down 15 times — should cap at 10 (20 lines - 10 visible)
		for i := 0; i < 15; i++ {
			sp, _ = sp.Update(tea.KeyMsg{Type: tea.KeyDown})
		}
		if sp.left.scroll != 10 {
			t.Errorf("expected scroll capped at 10, got %d", sp.left.scroll)
		}
	})

	t.Run("scroll up respects min", func(t *testing.T) {
		sp.left.scroll = 2
		for i := 0; i < 5; i++ {
			sp, _ = sp.Update(tea.KeyMsg{Type: tea.KeyUp})
		}
		if sp.left.scroll != 0 {
			t.Errorf("expected scroll capped at 0, got %d", sp.left.scroll)
		}
	})

	t.Run("page down respects max", func(t *testing.T) {
		sp.left.scroll = 8
		sp, _ = sp.Update(tea.KeyMsg{Type: tea.KeyPgDown})
		// Jump = 10/2 = 5, 8+5 = 13 > 10, so capped at 10
		if sp.left.scroll != 10 {
			t.Errorf("expected scroll capped at 10 after pgdown, got %d", sp.left.scroll)
		}
	})

	t.Run("page up respects min", func(t *testing.T) {
		sp.left.scroll = 3
		sp, _ = sp.Update(tea.KeyMsg{Type: tea.KeyPgUp})
		// Jump = 5, 3-5 = -2, capped at 0
		if sp.left.scroll != 0 {
			t.Errorf("expected scroll capped at 0 after pgup, got %d", sp.left.scroll)
		}
	})

	t.Run("short content prevents scrolling", func(t *testing.T) {
		shortLines := []string{"one", "two", "three"}
		sp.SetLeftContent("short.md", shortLines)
		sp.left.scroll = 0
		sp, _ = sp.Update(tea.KeyMsg{Type: tea.KeyDown})
		if sp.left.scroll != 0 {
			t.Errorf("expected no scrolling for short content, got scroll %d", sp.left.scroll)
		}
	})
}

func TestSplitPaneVisibleHeight(t *testing.T) {
	sp := NewSplitPane()

	t.Run("normal height", func(t *testing.T) {
		sp.SetSize(120, 40)
		got := sp.visibleHeight()
		expected := 40 - 7 // = 33
		if got != expected {
			t.Errorf("expected visibleHeight %d, got %d", expected, got)
		}
	})

	t.Run("minimum height clamped to 1", func(t *testing.T) {
		sp.SetSize(120, 5)
		got := sp.visibleHeight()
		if got != 1 {
			t.Errorf("expected visibleHeight 1 for small terminal, got %d", got)
		}
	})

	t.Run("exact boundary", func(t *testing.T) {
		sp.SetSize(120, 8) // 8 - 7 = 1
		got := sp.visibleHeight()
		if got != 1 {
			t.Errorf("expected visibleHeight 1, got %d", got)
		}
	})

	t.Run("height equals chrome", func(t *testing.T) {
		sp.SetSize(120, 7) // 7 - 7 = 0, clamped to 1
		got := sp.visibleHeight()
		if got != 1 {
			t.Errorf("expected visibleHeight 1 (clamped), got %d", got)
		}
	})

	t.Run("large height", func(t *testing.T) {
		sp.SetSize(200, 100)
		got := sp.visibleHeight()
		expected := 100 - 7
		if got != expected {
			t.Errorf("expected visibleHeight %d, got %d", expected, got)
		}
	})
}

func TestSplitPaneEscBehavior(t *testing.T) {
	t.Run("esc with query clears query", func(t *testing.T) {
		sp := NewSplitPane()
		sp.SetSize(120, 40)
		sp.SetNotes([]string{"a.md", "b.md"})
		sp.Open()
		sp.pickQuery = "test"
		sp, _ = sp.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if sp.pickQuery != "" {
			t.Errorf("expected empty query after esc, got %q", sp.pickQuery)
		}
		if !sp.picking {
			t.Error("expected still picking after clearing query")
		}
		if !sp.active {
			t.Error("expected overlay still active")
		}
	})

	t.Run("esc with no query and no selection closes overlay", func(t *testing.T) {
		sp := NewSplitPane()
		sp.SetSize(120, 40)
		sp.SetNotes([]string{"a.md", "b.md"})
		sp.Open()
		sp, _ = sp.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if sp.active {
			t.Error("expected overlay closed when esc with no query and no prior selection")
		}
	})

	t.Run("esc with prior selection returns to split view", func(t *testing.T) {
		sp := NewSplitPane()
		sp.SetSize(120, 40)
		sp.SetNotes([]string{"a.md", "b.md"})
		sp.Open()
		sp.rightNotePath = "a.md"
		sp, _ = sp.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if !sp.active {
			t.Error("expected overlay still active")
		}
		if sp.picking {
			t.Error("expected picker closed, returning to split view")
		}
	})

	t.Run("esc in normal mode closes overlay", func(t *testing.T) {
		sp := NewSplitPane()
		sp.SetSize(120, 40)
		sp.Open()
		sp.picking = false
		sp, _ = sp.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if sp.active {
			t.Error("expected overlay closed on esc in normal mode")
		}
	})
}

func TestSplitPaneTabSwitchesFocus(t *testing.T) {
	sp := NewSplitPane()
	sp.SetSize(120, 40)
	sp.Open()
	sp.picking = false

	if sp.focus != 0 {
		t.Fatalf("expected initial focus 0, got %d", sp.focus)
	}

	sp, _ = sp.Update(tea.KeyMsg{Type: tea.KeyTab})
	if sp.focus != 1 {
		t.Errorf("expected focus 1 after tab, got %d", sp.focus)
	}

	sp, _ = sp.Update(tea.KeyMsg{Type: tea.KeyTab})
	if sp.focus != 0 {
		t.Errorf("expected focus 0 after second tab, got %d", sp.focus)
	}
}

func TestSplitPaneInactiveNoOp(t *testing.T) {
	sp := NewSplitPane()
	sp, cmd := sp.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Error("expected nil command for inactive split pane")
	}
	view := sp.View()
	if view != "" {
		t.Error("expected empty view for inactive split pane")
	}
}
