package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/objects"
)

// fixtureTypedMentionPicker returns a picker pre-loaded with three
// typed objects across two types so every filter branch has data.
func fixtureTypedMentionPicker(t *testing.T) TypedMentionPicker {
	t.Helper()
	reg := objects.NewRegistry()
	b := objects.NewBuilder(reg)
	b.Add("People/Alice.md", "Alice Smith", map[string]string{"type": "person"})
	b.Add("People/Bob.md", "Bob Jones", map[string]string{"type": "person"})
	b.Add("Books/AH.md", "Atomic Habits", map[string]string{"type": "book"})
	idx := b.Finalize()

	p := NewTypedMentionPicker()
	p.SetSize(80, 30)
	p.Open(reg, idx)
	return p
}

// Empty query lists every typed object across all types.
func TestTypedMentionPicker_OpenShowsAll(t *testing.T) {
	p := fixtureTypedMentionPicker(t)
	if len(p.matches) != 3 {
		t.Errorf("expected 3 matches, got %d: %+v", len(p.matches), p.matches)
	}
}

// Bare-string filter narrows by title across every type.
func TestTypedMentionPicker_TitleFilter(t *testing.T) {
	p := fixtureTypedMentionPicker(t)
	for _, r := range "Alice" {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if len(p.matches) != 1 {
		t.Fatalf("expected 1 match for 'Alice', got %d: %+v", len(p.matches), p.matches)
	}
	if p.matches[0].Title != "Alice Smith" {
		t.Errorf("expected Alice Smith, got %q", p.matches[0].Title)
	}
}

// `typeID:` prefix scopes matching to that type — `book:` filters
// out the people. Empty query-after-colon shows every object of
// the type.
func TestTypedMentionPicker_TypePrefixScope(t *testing.T) {
	p := fixtureTypedMentionPicker(t)
	// Type 'book:' (5 keystrokes including the colon).
	for _, r := range "book:" {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if len(p.matches) != 1 {
		t.Fatalf("'book:' should match only books, got %d: %+v", len(p.matches), p.matches)
	}
	if p.matches[0].TypeID != "book" {
		t.Errorf("expected book result, got %q", p.matches[0].TypeID)
	}
}

// `typeID:title-frag` combines: type AND title substring.
func TestTypedMentionPicker_TypeAndTitle(t *testing.T) {
	p := fixtureTypedMentionPicker(t)
	for _, r := range "person:bob" {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if len(p.matches) != 1 || p.matches[0].Title != "Bob Jones" {
		t.Errorf("expected 1 match (Bob Jones), got %+v", p.matches)
	}
}

// Enter on a result emits a wikilink-style insert request, which
// the host model splices into the editor at cursor.
func TestTypedMentionPicker_EnterInsertsWikilink(t *testing.T) {
	p := fixtureTypedMentionPicker(t)
	for _, r := range "Alice" {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	insert, ok := p.ConsumeInsert()
	if !ok {
		t.Fatal("expected ConsumeInsert to return ok=true after Enter")
	}
	if insert != "[[Alice Smith]]" {
		t.Errorf("expected [[Alice Smith]], got %q", insert)
	}
	// Consumed-once: a second call returns ok=false.
	if _, ok := p.ConsumeInsert(); ok {
		t.Error("ConsumeInsert should be consumed-once")
	}
	// Picker closed itself on Enter.
	if p.IsActive() {
		t.Error("picker should close after Enter")
	}
}

// Esc closes the picker without inserting anything.
func TestTypedMentionPicker_EscCloses(t *testing.T) {
	p := fixtureTypedMentionPicker(t)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if p.IsActive() {
		t.Error("picker should close on Esc")
	}
	if _, ok := p.ConsumeInsert(); ok {
		t.Error("Esc should not produce an insert")
	}
}

// Up/Down navigation respects bounds. Repeated Down clamps at the
// last entry; Up at the first.
func TestTypedMentionPicker_NavigationClamps(t *testing.T) {
	p := fixtureTypedMentionPicker(t)
	if p.cursor != 0 {
		t.Fatalf("initial cursor: %d, want 0", p.cursor)
	}
	// Down × 10 should clamp at len-1.
	for i := 0; i < 10; i++ {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if p.cursor != len(p.matches)-1 {
		t.Errorf("cursor should clamp at last (%d), got %d", len(p.matches)-1, p.cursor)
	}
	// Up × 10 should clamp at 0.
	for i := 0; i < 10; i++ {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyUp})
	}
	if p.cursor != 0 {
		t.Errorf("cursor should clamp at first (0), got %d", p.cursor)
	}
}

// Backspace removes a query character + refreshes matches.
func TestTypedMentionPicker_Backspace(t *testing.T) {
	p := fixtureTypedMentionPicker(t)
	for _, r := range "Alic" {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	before := len(p.matches)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if len(p.query) != 3 || p.query != "Ali" {
		t.Errorf("backspace query: got %q, want Ali", p.query)
	}
	if len(p.matches) < before {
		t.Errorf("backspace should widen the result set or keep it the same: before=%d after=%d", before, len(p.matches))
	}
}

// Prefix matches sort before substring matches so a search for
// "ali" puts "Alice" above "Atomic Habits has alibis" if both match.
func TestTypedMentionPicker_PrefixMatchesFirst(t *testing.T) {
	reg := objects.NewRegistry()
	b := objects.NewBuilder(reg)
	b.Add("a.md", "Alice", map[string]string{"type": "person"})
	b.Add("b.md", "Calibrate Alice", map[string]string{"type": "book"})
	idx := b.Finalize()
	p := NewTypedMentionPicker()
	p.Open(reg, idx)
	for _, r := range "ali" {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if len(p.matches) != 2 {
		t.Fatalf("got %d matches: %+v", len(p.matches), p.matches)
	}
	// Prefix match (Alice) should come first.
	if !strings.HasPrefix(strings.ToLower(p.matches[0].Title), "ali") {
		t.Errorf("first result should be the prefix match, got %q", p.matches[0].Title)
	}
}
