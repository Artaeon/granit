package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/objects"
)

// fixtureTypedSidebar builds a sidebar with three typed notes
// (book × 1, person × 2) wired into the registry/index. Used by
// every Types-mode test below.
func fixtureTypedSidebar(t *testing.T) Sidebar {
	t.Helper()
	reg := objects.NewRegistry()
	b := objects.NewBuilder(reg)
	b.Add("People/Alice.md", "Alice", map[string]string{"type": "person", "email": "a@x.com"})
	b.Add("People/Bob.md", "Bob", map[string]string{"type": "person", "email": "b@x.com"})
	b.Add("Books/Atomic.md", "Atomic Habits", map[string]string{"type": "book", "author": "James Clear"})
	idx := b.Finalize()

	s := NewSidebar([]string{"People/Alice.md", "People/Bob.md", "Books/Atomic.md"})
	s.SetSize(40, 20)
	s.focused = true
	s.SetTypedObjects(reg, idx)
	return s
}

// SetMode flips the render mode and resets cursor/scroll/search so
// row-index space changes (different per mode) don't leave the
// cursor pointing at a stale offset.
func TestSidebar_SetModeResetsState(t *testing.T) {
	s := fixtureTypedSidebar(t)
	s.cursor = 5
	s.scroll = 3
	s.search = "alice"
	s.SetMode(ModeTypes)
	if s.cursor != 0 || s.scroll != 0 || s.search != "" {
		t.Errorf("SetMode should reset state: cursor=%d scroll=%d search=%q",
			s.cursor, s.scroll, s.search)
	}
	if s.mode != ModeTypes {
		t.Errorf("mode: got %v, want ModeTypes", s.mode)
	}
}

// CycleMode advances Files → Types → Files. Status message
// confirms each step so the user sees feedback even on identical-
// looking content.
func TestSidebar_CycleMode(t *testing.T) {
	s := fixtureTypedSidebar(t)
	if s.mode != ModeFiles {
		t.Fatalf("default mode: got %v, want ModeFiles", s.mode)
	}
	s.CycleMode()
	if s.mode != ModeTypes {
		t.Errorf("after first cycle: got %v, want ModeTypes", s.mode)
	}
	if !strings.Contains(s.statusMsg, "types") {
		t.Errorf("status should mention 'types': %q", s.statusMsg)
	}
	s.CycleMode()
	if s.mode != ModeFiles {
		t.Errorf("after second cycle: got %v, want ModeFiles", s.mode)
	}
}

// rebuildTypeRows produces headers + objects in registry order;
// empty types are skipped (a "Meeting (0)" row would be visual
// dead weight).
func TestSidebar_RebuildTypeRows(t *testing.T) {
	s := fixtureTypedSidebar(t)
	// Index has book × 1 and person × 2 — expect 2 headers + 3 objects = 5 rows.
	if len(s.typeRows) != 5 {
		t.Fatalf("typeRows count: got %d, want 5\n%+v", len(s.typeRows), s.typeRows)
	}
	// Registry order is alphabetical by ID, so book comes first.
	if !s.typeRows[0].Header || s.typeRows[0].TypeID != "book" {
		t.Errorf("first row should be book header, got %+v", s.typeRows[0])
	}
	if s.typeRows[1].Header || s.typeRows[1].TypeID != "book" {
		t.Errorf("second row should be book object, got %+v", s.typeRows[1])
	}
	if !s.typeRows[2].Header || s.typeRows[2].TypeID != "person" {
		t.Errorf("third row should be person header, got %+v", s.typeRows[2])
	}
	// Object titles within a type are alphabetical.
	if s.typeRows[3].Title != "Alice" || s.typeRows[4].Title != "Bob" {
		t.Errorf("person objects should be sorted: got %q %q",
			s.typeRows[3].Title, s.typeRows[4].Title)
	}
}

// Empty types are omitted. A registry with 5 built-ins but only 2
// populated should produce 2 headers, not 5.
func TestSidebar_EmptyTypesSkipped(t *testing.T) {
	s := fixtureTypedSidebar(t)
	headerCount := 0
	for _, r := range s.typeRows {
		if r.Header {
			headerCount++
		}
	}
	// Built-in registry has 5 types (person, book, project,
	// meeting, idea); fixture only populates 2.
	if headerCount != 2 {
		t.Errorf("header count: got %d, want 2 (only populated types)", headerCount)
	}
}

// filteredTypeRows narrows objects by case-insensitive title
// match, keeping the parent type's header visible only when at
// least one of its objects matches.
func TestSidebar_FilteredTypeRows(t *testing.T) {
	s := fixtureTypedSidebar(t)
	s.mode = ModeTypes
	s.search = "ali"
	rows := s.filteredTypeRows()
	// Expect: person header + Alice. Bob and the book are filtered out.
	if len(rows) != 2 {
		t.Fatalf("filtered rows: got %d, want 2 (header + Alice)\n%+v", len(rows), rows)
	}
	if !rows[0].Header || rows[0].TypeID != "person" {
		t.Errorf("first row should be person header, got %+v", rows[0])
	}
	if rows[1].Title != "Alice" {
		t.Errorf("second row should be Alice, got %+v", rows[1])
	}
	// Header count badge reflects the filter result, not the full count.
	if rows[0].Count != 1 {
		t.Errorf("header count after filter: got %d, want 1", rows[0].Count)
	}
}

// Selected() returns the object's path on object rows; empty on
// header rows so the host's loadNote(Selected()) is a no-op when
// the cursor is on a Type heading.
func TestSidebar_TypesModeSelected(t *testing.T) {
	s := fixtureTypedSidebar(t)
	s.mode = ModeTypes
	s.cursor = 0 // book header
	if s.Selected() != "" {
		t.Errorf("Selected on header should be empty, got %q", s.Selected())
	}
	s.cursor = 1 // book object (Atomic Habits)
	if s.Selected() != "Books/Atomic.md" {
		t.Errorf("Selected on object: got %q, want Books/Atomic.md", s.Selected())
	}
}

// 'm' cycles modes when sidebar is focused. This is the user's
// primary entry point to the Types view; a regression here breaks
// the feature.
func TestSidebar_MKeyCyclesMode(t *testing.T) {
	s := fixtureTypedSidebar(t)
	if s.mode != ModeFiles {
		t.Fatal("setup: should start in Files mode")
	}
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	if s.mode != ModeTypes {
		t.Errorf("after 'm': mode=%v, want ModeTypes", s.mode)
	}
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	if s.mode != ModeFiles {
		t.Errorf("after second 'm': mode=%v, want ModeFiles", s.mode)
	}
}

// 'm' is suppressed during search-typing so the user can include
// the letter 'm' in their query.
func TestSidebar_MKeyIgnoredWhileSearching(t *testing.T) {
	s := fixtureTypedSidebar(t)
	s.mode = ModeTypes
	s.searching = true
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	if s.mode != ModeTypes {
		t.Error("'m' while searching should NOT cycle mode")
	}
	if !strings.Contains(s.search, "m") {
		t.Errorf("'m' should append to search query: got %q", s.search)
	}
}

// j/k navigation in Types mode auto-skips header rows so the user
// doesn't have to dance "j to header, j again to next object".
// Header rows ARE positions in the row list (so the cursor lands
// on them when changing types), but j/k advance through them.
func TestSidebar_TypesModeNavSkipsHeaders(t *testing.T) {
	s := fixtureTypedSidebar(t)
	s.mode = ModeTypes
	// Cursor starts at row 0 (book header).
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	// j from header should land on the first object (row 1).
	if s.cursor != 1 {
		t.Errorf("after j on header: cursor=%d, want 1 (first book)", s.cursor)
	}
	// j again should skip the next header (person header is row 2)
	// and land on the first person (row 3).
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if s.cursor != 3 {
		t.Errorf("after second j: cursor=%d, want 3 (skipped header, landed on Alice)", s.cursor)
	}
}
