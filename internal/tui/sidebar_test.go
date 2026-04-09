package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFuzzyMatch_ExactMatch(t *testing.T) {
	if !fuzzyMatch("README", "README") {
		t.Error("fuzzyMatch(\"README\", \"README\") = false; want true")
	}
}

func TestFuzzyMatch_SubstringMatch(t *testing.T) {
	// fuzzyMatch works on character subsequence, so "read" should match
	// against the lowercased "readme.md".
	if !fuzzyMatch("readme.md", "read") {
		t.Error("fuzzyMatch(\"readme.md\", \"read\") = false; want true")
	}
}

func TestFuzzyMatch_CaseInsensitive(t *testing.T) {
	// The sidebar lowercases both sides before calling fuzzyMatch, so we
	// test the same way here.
	str := "readme.md"
	pattern := "readme"
	if !fuzzyMatch(str, pattern) {
		t.Errorf("fuzzyMatch(%q, %q) = false; want true", str, pattern)
	}
}

func TestFuzzyMatch_NoMatch(t *testing.T) {
	if fuzzyMatch("readme", "xyz") {
		t.Error("fuzzyMatch(\"readme\", \"xyz\") = true; want false")
	}
}

func TestFuzzyMatch_EmptyQuery(t *testing.T) {
	// An empty pattern should match everything (all 0 pattern chars consumed).
	if !fuzzyMatch("readme.md", "") {
		t.Error("fuzzyMatch(\"readme.md\", \"\") = false; want true")
	}
	if !fuzzyMatch("", "") {
		t.Error("fuzzyMatch(\"\", \"\") = false; want true")
	}
}

func TestIsHiddenPath_DotFile(t *testing.T) {
	if !isHiddenPath(".gitignore") {
		t.Error("isHiddenPath(\".gitignore\") = false; want true")
	}
}

func TestIsHiddenPath_DotDir(t *testing.T) {
	if !isHiddenPath(".config/file") {
		t.Error("isHiddenPath(\".config/file\") = false; want true")
	}
}

func TestIsHiddenPath_Normal(t *testing.T) {
	if isHiddenPath("notes/file.md") {
		t.Error("isHiddenPath(\"notes/file.md\") = true; want false")
	}
}

func TestIsDailyNote_ValidDate(t *testing.T) {
	if !isDailyNote("2024-01-15.md") {
		t.Error("isDailyNote(\"2024-01-15.md\") = false; want true")
	}
}

func TestIsDailyNote_InvalidFormat(t *testing.T) {
	if isDailyNote("not-a-date.md") {
		t.Error("isDailyNote(\"not-a-date.md\") = true; want false")
	}
}

// ---------------------------------------------------------------------------
// Sidebar component tests
// ---------------------------------------------------------------------------

func newTestSidebar(files []string) Sidebar {
	s := NewSidebar(files)
	s.SetSize(40, 20)
	s.focused = true
	s.treeView = false // flat mode for easier testing
	return s
}

func sidebarKeyMsg(key string) tea.Msg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func sidebarSpecialKey(keyType tea.KeyType) tea.Msg {
	return tea.KeyMsg{Type: keyType}
}

func TestSidebar_CursorDown(t *testing.T) {
	s := newTestSidebar([]string{"a.md", "b.md", "c.md"})
	s, _ = s.Update(sidebarSpecialKey(tea.KeyDown))
	if s.cursor != 1 {
		t.Errorf("expected cursor=1 after down, got %d", s.cursor)
	}
	if s.Selected() != "b.md" {
		t.Errorf("expected b.md selected, got %q", s.Selected())
	}
}

func TestSidebar_CursorUp(t *testing.T) {
	s := newTestSidebar([]string{"a.md", "b.md", "c.md"})
	s.cursor = 2
	s, _ = s.Update(sidebarSpecialKey(tea.KeyUp))
	if s.cursor != 1 {
		t.Errorf("expected cursor=1 after up, got %d", s.cursor)
	}
}

func TestSidebar_CursorStaysAtTop(t *testing.T) {
	s := newTestSidebar([]string{"a.md", "b.md"})
	s.cursor = 0
	s, _ = s.Update(sidebarSpecialKey(tea.KeyUp))
	if s.cursor != 0 {
		t.Errorf("cursor should stay at 0 when already at top, got %d", s.cursor)
	}
}

func TestSidebar_CursorStaysAtBottom(t *testing.T) {
	s := newTestSidebar([]string{"a.md", "b.md"})
	s.cursor = 1
	s, _ = s.Update(sidebarSpecialKey(tea.KeyDown))
	if s.cursor != 1 {
		t.Errorf("cursor should stay at 1 when at bottom, got %d", s.cursor)
	}
}

func TestSidebar_HomeEnd(t *testing.T) {
	s := newTestSidebar([]string{"a.md", "b.md", "c.md", "d.md"})
	s.cursor = 2

	s, _ = s.Update(sidebarSpecialKey(tea.KeyHome))
	if s.cursor != 0 {
		t.Errorf("expected cursor=0 after Home, got %d", s.cursor)
	}

	s, _ = s.Update(sidebarSpecialKey(tea.KeyEnd))
	if s.cursor != 3 {
		t.Errorf("expected cursor=3 after End, got %d", s.cursor)
	}
}

func TestSidebar_SearchFilters(t *testing.T) {
	s := newTestSidebar([]string{"notes.md", "tasks.md", "habits.md"})

	// Enter search mode
	s, _ = s.Update(sidebarKeyMsg("/"))
	if !s.searching {
		t.Fatal("expected searching=true after /")
	}

	// Type "t" to filter — matches "notes", "tasks", "habits"
	s, _ = s.Update(sidebarKeyMsg("t"))

	if len(s.filtered) != 3 {
		t.Errorf("expected 3 matches for 't', got %d: %v", len(s.filtered), s.filtered)
	}

	// Type "a" to narrow — "ta" matches "tasks" (subsequence t->a)
	s, _ = s.Update(sidebarKeyMsg("a"))

	if len(s.filtered) != 1 {
		t.Errorf("expected 1 match for 'ta' (tasks), got %d: %v", len(s.filtered), s.filtered)
	}
}

func TestSidebar_EscClearsSearch(t *testing.T) {
	s := newTestSidebar([]string{"a.md", "b.md", "c.md"})
	s.searching = true
	s.search = "a"
	s.applyFilter()

	s, _ = s.Update(sidebarSpecialKey(tea.KeyEscape))
	if s.searching {
		t.Error("expected searching=false after Esc")
	}
	if s.search != "" {
		t.Errorf("expected search cleared, got %q", s.search)
	}
	if len(s.filtered) != 3 {
		t.Errorf("expected all 3 files after clearing search, got %d", len(s.filtered))
	}
}

func TestSidebar_SelectedEmpty(t *testing.T) {
	s := newTestSidebar([]string{})
	if s.Selected() != "" {
		t.Errorf("expected empty selected for empty sidebar, got %q", s.Selected())
	}
}

func TestSidebar_SetFilesRefilters(t *testing.T) {
	s := newTestSidebar([]string{"a.md", "b.md"})
	s.search = "c"
	s.applyFilter()

	if len(s.filtered) != 0 {
		t.Fatal("expected 0 filtered results for 'c'")
	}

	s.SetFiles([]string{"a.md", "b.md", "c.md"})
	// search is still "c" — should auto-refilter
	if len(s.filtered) != 1 {
		t.Errorf("expected 1 filtered result after adding c.md, got %d", len(s.filtered))
	}
}

func TestSidebar_HiddenFilesFiltered(t *testing.T) {
	s := newTestSidebar([]string{"notes.md", ".hidden.md", ".config/settings.md"})
	s.showHidden = false
	s.applyFilter()

	if len(s.filtered) != 1 {
		t.Errorf("expected 1 visible file, got %d", len(s.filtered))
	}
	if s.filtered[0] != "notes.md" {
		t.Errorf("expected notes.md, got %q", s.filtered[0])
	}
}

func TestSidebar_ShowHiddenFiles(t *testing.T) {
	s := newTestSidebar([]string{"notes.md", ".hidden.md"})
	s.SetShowHidden(true)

	if len(s.filtered) != 2 {
		t.Errorf("expected 2 files with showHidden=true, got %d", len(s.filtered))
	}
}

func TestSidebar_NotFocused(t *testing.T) {
	s := newTestSidebar([]string{"a.md", "b.md"})
	s.focused = false
	origCursor := s.cursor
	s, _ = s.Update(sidebarSpecialKey(tea.KeyDown))
	if s.cursor != origCursor {
		t.Error("unfocused sidebar should not respond to keys")
	}
}
