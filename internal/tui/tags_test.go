package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// NewTags — initial state
// ---------------------------------------------------------------------------

func TestTags_NewTagBrowser(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "# Hello",
	})
	tb := NewTagBrowser(v)

	if tb.active {
		t.Error("expected tag browser to be inactive on creation")
	}
	if tb.vault != v {
		t.Error("expected vault reference to be set")
	}
	if tb.mode != 0 {
		t.Errorf("expected mode=0, got %d", tb.mode)
	}
	if tb.cursor != 0 {
		t.Errorf("expected cursor=0, got %d", tb.cursor)
	}
}

// ---------------------------------------------------------------------------
// Open / Close / IsActive — state transitions
// ---------------------------------------------------------------------------

func TestTags_OpenCloseIsActive(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "# Hello",
	})
	tb := NewTagBrowser(v)

	if tb.IsActive() {
		t.Error("expected IsActive false before Open")
	}

	tb.Open()
	if !tb.IsActive() {
		t.Error("expected IsActive true after Open")
	}

	tb.Close()
	if tb.IsActive() {
		t.Error("expected IsActive false after Close")
	}
}

func TestTags_OpenResetsState(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "---\ntags: [go, test]\n---\n# Note\nContent with #inline",
	})
	tb := NewTagBrowser(v)
	tb.Open()

	// Mutate state
	tb.cursor = 5
	tb.scroll = 3
	tb.mode = 1
	tb.selected = "go"
	tb.result = "note.md"

	// Re-open should reset
	tb.Open()
	if tb.cursor != 0 {
		t.Errorf("expected cursor=0, got %d", tb.cursor)
	}
	if tb.scroll != 0 {
		t.Errorf("expected scroll=0, got %d", tb.scroll)
	}
	if tb.mode != 0 {
		t.Errorf("expected mode=0, got %d", tb.mode)
	}
	if tb.selected != "" {
		t.Errorf("expected selected empty, got %q", tb.selected)
	}
	if tb.result != "" {
		t.Errorf("expected result empty, got %q", tb.result)
	}
}

// ---------------------------------------------------------------------------
// SetSize
// ---------------------------------------------------------------------------

func TestTags_SetSize(t *testing.T) {
	v := testVault(t, map[string]string{"note.md": "# Hi"})
	tb := NewTagBrowser(v)
	tb.SetSize(120, 40)

	if tb.width != 120 {
		t.Errorf("expected width=120, got %d", tb.width)
	}
	if tb.height != 40 {
		t.Errorf("expected height=40, got %d", tb.height)
	}
}

// ---------------------------------------------------------------------------
// Tag collection — frontmatter and inline tags
// ---------------------------------------------------------------------------

func TestTags_CollectFromFrontmatter(t *testing.T) {
	v := testVault(t, map[string]string{
		"note1.md": "---\ntags: [golang, testing]\n---\n# Note 1",
		"note2.md": "---\ntags: [golang, rust]\n---\n# Note 2",
	})
	tb := NewTagBrowser(v)
	tb.Open()

	if len(tb.tags) == 0 {
		t.Fatal("expected tags to be collected")
	}

	tagMap := make(map[string]int)
	for _, tag := range tb.tags {
		tagMap[tag.name] = tag.count
	}

	if tagMap["golang"] != 2 {
		t.Errorf("expected 'golang' count=2, got %d", tagMap["golang"])
	}
	if tagMap["testing"] != 1 {
		t.Errorf("expected 'testing' count=1, got %d", tagMap["testing"])
	}
	if tagMap["rust"] != 1 {
		t.Errorf("expected 'rust' count=1, got %d", tagMap["rust"])
	}
}

func TestTags_CollectInlineTags(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "# Title\nSome text with #python and #go tags\nMore #python here",
	})
	tb := NewTagBrowser(v)
	tb.Open()

	tagMap := make(map[string]int)
	for _, tag := range tb.tags {
		tagMap[tag.name] = tag.count
	}

	if tagMap["python"] != 2 {
		t.Errorf("expected 'python' count=2, got %d", tagMap["python"])
	}
	if tagMap["go"] != 1 {
		t.Errorf("expected 'go' count=1, got %d", tagMap["go"])
	}
}

// ---------------------------------------------------------------------------
// Empty vault — no tags
// ---------------------------------------------------------------------------

func TestTags_EmptyVault(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "# No tags here\nJust some text.",
	})
	tb := NewTagBrowser(v)
	tb.Open()

	if len(tb.tags) != 0 {
		t.Errorf("expected 0 tags for vault without tags, got %d", len(tb.tags))
	}
}

// ---------------------------------------------------------------------------
// Tag sorting — by count descending, then alphabetical
// ---------------------------------------------------------------------------

func TestTags_SortingByCountDescending(t *testing.T) {
	v := testVault(t, map[string]string{
		"a.md": "# A\n#alpha\n#alpha\n#alpha",
		"b.md": "# B\n#beta",
		"c.md": "# C\n#gamma\n#gamma",
	})
	tb := NewTagBrowser(v)
	tb.Open()

	if len(tb.tags) < 3 {
		t.Fatalf("expected at least 3 tags, got %d", len(tb.tags))
	}

	// First tag should have the highest count
	if tb.tags[0].name != "alpha" {
		t.Errorf("expected first tag to be 'alpha' (highest count), got %q", tb.tags[0].name)
	}
	if tb.tags[0].count != 3 {
		t.Errorf("expected alpha count=3, got %d", tb.tags[0].count)
	}

	// Tags with equal counts should be sorted alphabetically
	// gamma (2) should come before beta (1)
	if tb.tags[1].name != "gamma" {
		t.Errorf("expected second tag to be 'gamma' (count=2), got %q (count=%d)", tb.tags[1].name, tb.tags[1].count)
	}
}

func TestTags_SortingAlphabeticalForEqualCounts(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "# Note\n#cherry #apple #banana",
	})
	tb := NewTagBrowser(v)
	tb.Open()

	if len(tb.tags) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(tb.tags))
	}

	// All have count 1, so sorted alphabetically
	if tb.tags[0].name != "apple" {
		t.Errorf("expected first tag 'apple', got %q", tb.tags[0].name)
	}
	if tb.tags[1].name != "banana" {
		t.Errorf("expected second tag 'banana', got %q", tb.tags[1].name)
	}
	if tb.tags[2].name != "cherry" {
		t.Errorf("expected third tag 'cherry', got %q", tb.tags[2].name)
	}
}

// ---------------------------------------------------------------------------
// Navigation — up/down in tag list
// ---------------------------------------------------------------------------

func TestTags_NavigateTagListDown(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "# Note\n#aaa #bbb #ccc",
	})
	tb := NewTagBrowser(v)
	tb.SetSize(120, 40)
	tb.Open()

	if tb.cursor != 0 {
		t.Fatalf("expected cursor at 0, got %d", tb.cursor)
	}

	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyDown})
	if tb.cursor != 1 {
		t.Errorf("expected cursor at 1 after down, got %d", tb.cursor)
	}

	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyDown})
	if tb.cursor != 2 {
		t.Errorf("expected cursor at 2 after second down, got %d", tb.cursor)
	}
}

func TestTags_NavigateTagListUp(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "# Note\n#aaa #bbb #ccc",
	})
	tb := NewTagBrowser(v)
	tb.SetSize(120, 40)
	tb.Open()

	// Move down then up
	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyDown})
	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyDown})
	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyUp})

	if tb.cursor != 1 {
		t.Errorf("expected cursor at 1 after up, got %d", tb.cursor)
	}
}

func TestTags_NavigateVimKeys(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "# Note\n#aaa #bbb",
	})
	tb := NewTagBrowser(v)
	tb.SetSize(120, 40)
	tb.Open()

	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if tb.cursor != 1 {
		t.Errorf("expected cursor at 1 after 'j', got %d", tb.cursor)
	}

	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if tb.cursor != 0 {
		t.Errorf("expected cursor at 0 after 'k', got %d", tb.cursor)
	}
}

func TestTags_NavigateBoundsTop(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "# Note\n#aaa #bbb",
	})
	tb := NewTagBrowser(v)
	tb.SetSize(120, 40)
	tb.Open()

	// Already at top, going up should stay
	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyUp})
	if tb.cursor != 0 {
		t.Errorf("expected cursor to stay at 0, got %d", tb.cursor)
	}
}

func TestTags_NavigateBoundsBottom(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "# Note\n#aaa #bbb",
	})
	tb := NewTagBrowser(v)
	tb.SetSize(120, 40)
	tb.Open()

	// Move to last tag
	for i := 0; i < 10; i++ {
		tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	// Should stop at last item
	if tb.cursor != len(tb.tags)-1 {
		t.Errorf("expected cursor at %d (last tag), got %d", len(tb.tags)-1, tb.cursor)
	}
}

// ---------------------------------------------------------------------------
// Mode switching — tag list (0) to notes (1) and back
// ---------------------------------------------------------------------------

func TestTags_EnterTagSwitchesToNotesList(t *testing.T) {
	v := testVault(t, map[string]string{
		"a.md": "# A\n#mytag content",
		"b.md": "# B\n#mytag more",
	})
	tb := NewTagBrowser(v)
	tb.SetSize(120, 40)
	tb.Open()

	if tb.mode != 0 {
		t.Fatal("expected mode=0 (tag list)")
	}

	// Press enter on selected tag
	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if tb.mode != 1 {
		t.Errorf("expected mode=1 (notes list), got %d", tb.mode)
	}
	if tb.selected == "" {
		t.Error("expected selected tag to be set")
	}
	if len(tb.notes) != 2 {
		t.Errorf("expected 2 notes for tag, got %d", len(tb.notes))
	}
}

func TestTags_EscInNotesGoesBackToTagList(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "# Note\n#sometag here",
	})
	tb := NewTagBrowser(v)
	tb.SetSize(120, 40)
	tb.Open()

	// Enter notes mode
	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if tb.mode != 1 {
		t.Fatal("expected mode=1")
	}

	// Esc should go back to tag list, not close
	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if tb.mode != 0 {
		t.Errorf("expected mode=0 after Esc in notes view, got %d", tb.mode)
	}
	if !tb.IsActive() {
		t.Error("expected tag browser to still be active")
	}
}

func TestTags_EscInTagListCloses(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "# Note\n#tag",
	})
	tb := NewTagBrowser(v)
	tb.SetSize(120, 40)
	tb.Open()

	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if tb.IsActive() {
		t.Error("expected tag browser to close on Esc in tag list mode")
	}
}

// ---------------------------------------------------------------------------
// Navigation in notes list (mode 1)
// ---------------------------------------------------------------------------

func TestTags_NavigateNotesList(t *testing.T) {
	v := testVault(t, map[string]string{
		"a.md": "# A\n#tag content",
		"b.md": "# B\n#tag more",
		"c.md": "# C\n#tag also",
	})
	tb := NewTagBrowser(v)
	tb.SetSize(120, 40)
	tb.Open()

	// Enter notes mode
	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if tb.noteCursor != 0 {
		t.Fatalf("expected noteCursor=0, got %d", tb.noteCursor)
	}

	// Navigate down in notes list
	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyDown})
	if tb.noteCursor != 1 {
		t.Errorf("expected noteCursor=1 after down, got %d", tb.noteCursor)
	}

	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyUp})
	if tb.noteCursor != 0 {
		t.Errorf("expected noteCursor=0 after up, got %d", tb.noteCursor)
	}
}

// ---------------------------------------------------------------------------
// SelectedNote — consumed-once pattern
// ---------------------------------------------------------------------------

func TestTags_SelectedNoteConsumedOnce(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "# Note\n#sometag content",
	})
	tb := NewTagBrowser(v)
	tb.SetSize(120, 40)
	tb.Open()

	// Enter notes mode
	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Select a note
	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyEnter})

	note := tb.SelectedNote()
	if note == "" {
		t.Error("expected a selected note")
	}

	// Second call should return empty (consumed)
	note2 := tb.SelectedNote()
	if note2 != "" {
		t.Errorf("expected empty on second SelectedNote call, got %q", note2)
	}
}

func TestTags_SelectNoteClosesTagBrowser(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "# Note\n#sometag content",
	})
	tb := NewTagBrowser(v)
	tb.SetSize(120, 40)
	tb.Open()

	// Enter notes mode
	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Select note
	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if tb.IsActive() {
		t.Error("expected tag browser to close after selecting a note")
	}
}

// ---------------------------------------------------------------------------
// Inactive tag browser ignores input
// ---------------------------------------------------------------------------

func TestTags_InactiveIgnoresInput(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "# Note",
	})
	tb := NewTagBrowser(v)

	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyDown})
	if tb.cursor != 0 {
		t.Error("inactive tag browser should not respond to key messages")
	}
}

// ---------------------------------------------------------------------------
// Enter on empty tag list does nothing
// ---------------------------------------------------------------------------

func TestTags_EnterEmptyTagList(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "# No tags here",
	})
	tb := NewTagBrowser(v)
	tb.SetSize(120, 40)
	tb.Open()

	// Should be in tag list mode with no tags
	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should remain in tag list mode (no switch to notes)
	if tb.mode != 0 {
		t.Errorf("expected mode=0 (no switch on empty tags), got %d", tb.mode)
	}
}

// ---------------------------------------------------------------------------
// Ctrl+T closes tag browser
// ---------------------------------------------------------------------------

func TestTags_CtrlTCloses(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "# Note\n#tag",
	})
	tb := NewTagBrowser(v)
	tb.SetSize(120, 40)
	tb.Open()

	// ctrl+t is handled by msg.String() == "ctrl+t"
	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	if tb.IsActive() {
		t.Error("expected tag browser to close on Ctrl+T")
	}
}

// ---------------------------------------------------------------------------
// Frontmatter comma-separated tags
// ---------------------------------------------------------------------------

func TestTags_CommaSeparatedFrontmatterTags(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "---\ntags: go, rust, python\n---\n# Note",
	})
	tb := NewTagBrowser(v)
	tb.Open()

	tagMap := make(map[string]int)
	for _, tag := range tb.tags {
		tagMap[tag.name] = tag.count
	}

	if tagMap["go"] != 1 {
		t.Errorf("expected 'go' count=1, got %d", tagMap["go"])
	}
	if tagMap["rust"] != 1 {
		t.Errorf("expected 'rust' count=1, got %d", tagMap["rust"])
	}
	if tagMap["python"] != 1 {
		t.Errorf("expected 'python' count=1, got %d", tagMap["python"])
	}
}
