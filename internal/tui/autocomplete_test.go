package tui

import "testing"

// ---------------------------------------------------------------------------
// Constructor & Initial State
// ---------------------------------------------------------------------------

func TestAutocomplete_NewAutocomplete(t *testing.T) {
	ac := NewAutocomplete()
	if ac.active {
		t.Error("expected inactive after construction")
	}
	if ac.cursor != 0 {
		t.Error("expected cursor=0")
	}
	if ac.query != "" {
		t.Error("expected empty query")
	}
	if len(ac.suggestions) != 0 {
		t.Error("expected no suggestions")
	}
	if len(ac.allNotes) != 0 {
		t.Error("expected no notes set")
	}
}

// ---------------------------------------------------------------------------
// SetNotes
// ---------------------------------------------------------------------------

func TestAutocomplete_SetNotes(t *testing.T) {
	ac := NewAutocomplete()
	paths := []string{"notes/alpha.md", "notes/beta.md", "notes/gamma.md"}
	ac.SetNotes(paths)
	if len(ac.allNotes) != 3 {
		t.Errorf("expected 3 notes, got %d", len(ac.allNotes))
	}
}

// ---------------------------------------------------------------------------
// Open / Close / IsActive
// ---------------------------------------------------------------------------

func TestAutocomplete_OpenCloseIsActive(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetNotes([]string{"notes/foo.md"})

	if ac.IsActive() {
		t.Error("should be inactive before Open")
	}

	ac.Open("", 10, 5)
	if !ac.IsActive() {
		t.Error("should be active after Open")
	}
	if ac.x != 10 || ac.y != 5 {
		t.Errorf("expected position (10,5), got (%d,%d)", ac.x, ac.y)
	}

	ac.Close()
	if ac.IsActive() {
		t.Error("should be inactive after Close")
	}
	if ac.query != "" {
		t.Error("Close should reset query")
	}
	if len(ac.suggestions) != 0 {
		t.Error("Close should clear suggestions")
	}
	if ac.cursor != 0 {
		t.Error("Close should reset cursor")
	}
}

// ---------------------------------------------------------------------------
// Empty query — shows all notes
// ---------------------------------------------------------------------------

func TestAutocomplete_EmptyQueryShowsAll(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetNotes([]string{"notes/alpha.md", "notes/beta.md", "notes/gamma.md"})
	ac.Open("", 0, 0)

	if len(ac.suggestions) != 3 {
		t.Errorf("empty query should show all notes, got %d", len(ac.suggestions))
	}
}

// ---------------------------------------------------------------------------
// Exact match
// ---------------------------------------------------------------------------

func TestAutocomplete_ExactMatch(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetNotes([]string{"notes/my note.md", "notes/other.md"})
	ac.Open("my note", 0, 0)

	found := false
	for _, s := range ac.suggestions {
		if s == "my note" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'my note' in suggestions")
	}
}

// ---------------------------------------------------------------------------
// Partial match
// ---------------------------------------------------------------------------

func TestAutocomplete_PartialMatch(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetNotes([]string{"notes/my note.md", "notes/my other.md", "notes/unrelated.md"})
	ac.Open("my", 0, 0)

	if len(ac.suggestions) < 2 {
		t.Errorf("expected at least 2 matches for 'my', got %d", len(ac.suggestions))
	}
	// Verify both "my" notes are present
	has := map[string]bool{}
	for _, s := range ac.suggestions {
		has[s] = true
	}
	if !has["my note"] {
		t.Error("expected 'my note' in suggestions")
	}
	if !has["my other"] {
		t.Error("expected 'my other' in suggestions")
	}
}

// ---------------------------------------------------------------------------
// Case insensitive matching
// ---------------------------------------------------------------------------

func TestAutocomplete_CaseInsensitive(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetNotes([]string{"notes/My Note.md", "notes/another.md"})
	ac.Open("my", 0, 0)

	found := false
	for _, s := range ac.suggestions {
		if s == "My Note" {
			found = true
		}
	}
	if !found {
		t.Error("expected case-insensitive match for 'my' -> 'My Note'")
	}
}

func TestAutocomplete_CaseInsensitiveUpperQuery(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetNotes([]string{"notes/hello world.md"})
	ac.Open("HELLO", 0, 0)

	// fuzzyMatch works on lowercased strings, so this should match
	if len(ac.suggestions) != 1 {
		t.Errorf("expected 1 match for uppercase query, got %d", len(ac.suggestions))
	}
}

// ---------------------------------------------------------------------------
// Navigation — up/down
// ---------------------------------------------------------------------------

func TestAutocomplete_NavigationUpDown(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetNotes([]string{"notes/alpha.md", "notes/beta.md", "notes/gamma.md"})
	ac.Open("", 0, 0)

	if ac.cursor != 0 {
		t.Error("cursor should start at 0")
	}

	ac.MoveDown()
	if ac.cursor != 1 {
		t.Errorf("after MoveDown, expected cursor=1, got %d", ac.cursor)
	}

	ac.MoveDown()
	if ac.cursor != 2 {
		t.Errorf("after MoveDown, expected cursor=2, got %d", ac.cursor)
	}

	// Cannot go past last item
	ac.MoveDown()
	if ac.cursor != 2 {
		t.Errorf("MoveDown past last should stay at 2, got %d", ac.cursor)
	}

	ac.MoveUp()
	if ac.cursor != 1 {
		t.Errorf("after MoveUp, expected cursor=1, got %d", ac.cursor)
	}

	ac.MoveUp()
	if ac.cursor != 0 {
		t.Errorf("after MoveUp, expected cursor=0, got %d", ac.cursor)
	}

	// Cannot go above first item
	ac.MoveUp()
	if ac.cursor != 0 {
		t.Errorf("MoveUp past first should stay at 0, got %d", ac.cursor)
	}
}

// ---------------------------------------------------------------------------
// Selected returns correct note name
// ---------------------------------------------------------------------------

func TestAutocomplete_Selected(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetNotes([]string{"notes/alpha.md", "notes/beta.md", "notes/gamma.md"})
	ac.Open("", 0, 0)

	sel := ac.Selected()
	if sel != "alpha" {
		t.Errorf("expected 'alpha', got '%s'", sel)
	}

	ac.MoveDown()
	sel = ac.Selected()
	if sel != "beta" {
		t.Errorf("expected 'beta', got '%s'", sel)
	}

	ac.MoveDown()
	sel = ac.Selected()
	if sel != "gamma" {
		t.Errorf("expected 'gamma', got '%s'", sel)
	}
}

func TestAutocomplete_SelectedEmpty(t *testing.T) {
	ac := NewAutocomplete()
	// No notes set
	ac.Open("zzz", 0, 0)

	sel := ac.Selected()
	if sel != "" {
		t.Errorf("expected empty string when no suggestions, got '%s'", sel)
	}
}

// ---------------------------------------------------------------------------
// No matches
// ---------------------------------------------------------------------------

func TestAutocomplete_NoMatches(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetNotes([]string{"notes/alpha.md", "notes/beta.md"})
	ac.Open("zzzzz", 0, 0)

	if len(ac.suggestions) != 0 {
		t.Errorf("expected 0 suggestions for non-matching query, got %d", len(ac.suggestions))
	}
	if ac.Selected() != "" {
		t.Error("Selected should be empty when no suggestions")
	}
}

// ---------------------------------------------------------------------------
// Special characters — spaces, hyphens
// ---------------------------------------------------------------------------

func TestAutocomplete_SpecialCharsSpacesHyphens(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetNotes([]string{
		"notes/my-note.md",
		"notes/my note.md",
		"notes/another_note.md",
	})

	// Query with hyphen
	ac.Open("my-", 0, 0)
	found := false
	for _, s := range ac.suggestions {
		if s == "my-note" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'my-note' in suggestions for 'my-' query")
	}
}

func TestAutocomplete_NoteWithSpaceInName(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetNotes([]string{"vault/meeting notes.md", "vault/daily log.md"})
	ac.Open("meeting", 0, 0)

	if len(ac.suggestions) != 1 {
		t.Errorf("expected 1 suggestion, got %d", len(ac.suggestions))
	}
	if ac.Selected() != "meeting notes" {
		t.Errorf("expected 'meeting notes', got '%s'", ac.Selected())
	}
}

// ---------------------------------------------------------------------------
// UpdateQuery filters dynamically
// ---------------------------------------------------------------------------

func TestAutocomplete_UpdateQueryFilters(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetNotes([]string{"notes/apple.md", "notes/banana.md", "notes/avocado.md"})
	ac.Open("", 0, 0)

	if len(ac.suggestions) != 3 {
		t.Errorf("expected 3 with empty query, got %d", len(ac.suggestions))
	}

	ac.UpdateQuery("a")
	// "a" fuzzy-matches: "apple" (has a), "banana" (has a), "avocado" (has a) = all 3
	if len(ac.suggestions) != 3 {
		t.Errorf("expected 3 for 'a', got %d", len(ac.suggestions))
	}

	ac.UpdateQuery("ap")
	// "ap" fuzzy: "apple" (a then p), "avocado" does not (no p after a... wait, no p at all)
	// Actually avocado has no 'p'. banana has no 'p' after the 'a'.
	// Only "apple" matches "ap"
	if len(ac.suggestions) != 1 {
		t.Errorf("expected 1 for 'ap', got %d", len(ac.suggestions))
	}
}

// ---------------------------------------------------------------------------
// Cursor clamped when suggestions shrink
// ---------------------------------------------------------------------------

func TestAutocomplete_CursorClampedOnShrink(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetNotes([]string{"notes/alpha.md", "notes/beta.md", "notes/gamma.md"})
	ac.Open("", 0, 0)

	// Move to last item
	ac.MoveDown()
	ac.MoveDown()
	if ac.cursor != 2 {
		t.Fatalf("expected cursor=2, got %d", ac.cursor)
	}

	// Update query to have fewer results
	ac.UpdateQuery("alpha")
	if ac.cursor >= len(ac.suggestions) {
		t.Errorf("cursor should be clamped, got %d with %d suggestions", ac.cursor, len(ac.suggestions))
	}
}

// ---------------------------------------------------------------------------
// View does not panic
// ---------------------------------------------------------------------------

func TestAutocomplete_ViewNoPanic(t *testing.T) {
	ac := NewAutocomplete()

	// Inactive view
	v := ac.View()
	if v != "" {
		t.Error("inactive view should be empty")
	}

	// Active with no suggestions
	ac.SetNotes([]string{})
	ac.Open("xyz", 0, 0)
	v = ac.View()
	if v != "" {
		t.Error("active view with no suggestions should be empty")
	}

	// Active with suggestions
	ac.SetNotes([]string{"notes/hello.md", "notes/world.md"})
	ac.Open("", 0, 0)
	v = ac.View()
	if v == "" {
		t.Error("expected non-empty view with suggestions")
	}
}

// ---------------------------------------------------------------------------
// Fuzzy matching — non-contiguous characters
// ---------------------------------------------------------------------------

func TestAutocomplete_FuzzyNonContiguous(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetNotes([]string{"notes/project-notes.md", "notes/daily.md"})
	ac.Open("pn", 0, 0)

	// "pn" should fuzzy-match "project-notes" (p...n)
	found := false
	for _, s := range ac.suggestions {
		if s == "project-notes" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'project-notes' to fuzzy-match 'pn'")
	}
}
