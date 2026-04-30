package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/objects"
)

// fixtureBrowser builds a ready-to-use ObjectBrowser populated with a
// small set of typed objects covering both built-in types and an
// unknown type so tests can exercise every navigation branch.
//
// Uses NewRegistryEmpty + explicit Set so the type list has exactly
// two entries (book, person) — without this, every cursor-position
// assertion in this file would have to track the 12 built-in types'
// alphabetical ordering and recompute on every type added.
func fixtureBrowser(t *testing.T) ObjectBrowser {
	t.Helper()
	r := objects.NewRegistryEmpty()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(r.Set(objects.Type{
		ID: "book", Name: "Book", Folder: "Books",
		Properties: []objects.Property{
			{Name: "title", Kind: objects.KindText, Required: true},
			{Name: "author", Kind: objects.KindText},
			{Name: "rating", Kind: objects.KindNumber},
		},
	}))
	must(r.Set(objects.Type{
		ID: "person", Name: "Person", Folder: "People",
		Properties: []objects.Property{
			{Name: "title", Kind: objects.KindText, Required: true},
			{Name: "email", Kind: objects.KindText},
		},
	}))
	b := objects.NewBuilder(r)
	b.Add("People/Alice.md", "Alice", map[string]string{
		"type": "person", "email": "alice@x.com",
	})
	b.Add("People/Bob.md", "Bob", map[string]string{
		"type": "person", "email": "bob@x.com",
	})
	b.Add("Books/Atomic Habits.md", "Atomic Habits", map[string]string{
		"type": "book", "author": "James Clear", "rating": "5",
	})
	idx := b.Finalize()
	ob := NewObjectBrowser()
	ob.Open(r, idx)
	ob.SetSize(120, 30)
	return ob
}

// Open populates registry + index and starts in a sensible default
// state: focus on type list, no filter, cursor on first type.
func TestObjectBrowser_OpenDefaults(t *testing.T) {
	ob := fixtureBrowser(t)
	if !ob.IsActive() {
		t.Error("expected IsActive after Open")
	}
	if ob.focus != 0 {
		t.Errorf("focus: got %d, want 0", ob.focus)
	}
	if ob.typeCursor != 0 {
		t.Errorf("typeCursor: got %d, want 0", ob.typeCursor)
	}
	if ob.query != "" {
		t.Errorf("query: got %q, want empty", ob.query)
	}
}

// Tab swaps pane focus. Repeating returns to the original.
func TestObjectBrowser_TabSwapsFocus(t *testing.T) {
	ob := fixtureBrowser(t)
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyTab})
	if ob.focus != 1 {
		t.Errorf("after Tab: focus = %d, want 1", ob.focus)
	}
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyTab})
	if ob.focus != 0 {
		t.Errorf("after second Tab: focus = %d, want 0", ob.focus)
	}
}

// j/k navigates within the focused pane. In the type list, moving
// down past the last type clamps; in the grid, the same clamp
// applies to objects.
func TestObjectBrowser_JKMovesCursor(t *testing.T) {
	ob := fixtureBrowser(t)
	// Focus = type list. Two registered types-with-objects (book, person).
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if ob.typeCursor != 1 {
		t.Errorf("after j: typeCursor = %d, want 1", ob.typeCursor)
	}
	// Past end clamps.
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if ob.typeCursor != 1 {
		t.Errorf("clamp at end: typeCursor = %d, want 1", ob.typeCursor)
	}
	// Up returns to 0.
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if ob.typeCursor != 0 {
		t.Errorf("after k: typeCursor = %d, want 0", ob.typeCursor)
	}
}

// Enter on a type focuses the right pane (does NOT close). Enter on
// an object emits a jump request and closes.
func TestObjectBrowser_EnterFlow(t *testing.T) {
	ob := fixtureBrowser(t)
	// Cursor is on "book" (first alphabetically in fixture). Enter focuses grid.
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if ob.focus != 1 {
		t.Errorf("Enter on type: focus = %d, want 1", ob.focus)
	}
	if !ob.IsActive() {
		t.Error("Enter on type should NOT close the browser")
	}
	// Enter on object emits a jump request and closes.
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyEnter})
	path, ok := ob.GetJumpRequest()
	if !ok {
		t.Fatal("expected jump request after Enter on object")
	}
	if path != "Books/Atomic Habits.md" {
		t.Errorf("jump path: got %q", path)
	}
	if ob.IsActive() {
		t.Error("Enter on object should close the browser")
	}
	// Consumed-once: next call returns false.
	if _, ok := ob.GetJumpRequest(); ok {
		t.Error("GetJumpRequest should be consumed-once")
	}
}

// `/` enters search mode; subsequent runes append; Enter commits;
// Esc clears + exits.
func TestObjectBrowser_SearchFlow(t *testing.T) {
	ob := fixtureBrowser(t)
	// Switch to "person" type (cursor 1 alphabetically).
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyEnter}) // focus grid

	// /  → search mode
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !ob.searching {
		t.Error("expected searching=true after /")
	}
	// Type "ali"
	for _, r := range "ali" {
		ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if ob.query != "ali" {
		t.Errorf("query: got %q, want %q", ob.query, "ali")
	}
	// Filter applies to currentObjects.
	if got := ob.currentObjects(); len(got) != 1 || got[0].Title != "Alice" {
		t.Errorf("filtered objects: got %d, want 1 (Alice)", len(got))
	}
	// Backspace → "al"
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if ob.query != "al" {
		t.Errorf("after backspace: query = %q, want %q", ob.query, "al")
	}
	// Enter commits, leaves filter in place.
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if ob.searching {
		t.Error("Enter should exit search mode")
	}
	if ob.query != "al" {
		t.Errorf("Enter should preserve query, got %q", ob.query)
	}
	// Esc clears query (first press), then closes (second press).
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if ob.query != "" {
		t.Errorf("Esc should clear query, got %q", ob.query)
	}
	if !ob.IsActive() {
		t.Error("first Esc should not close")
	}
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if ob.IsActive() {
		t.Error("second Esc should close")
	}
}

// typesWithObjects returns every registered type — populated AND
// empty — so the user can press 'n' on an empty type to create the
// first instance. The renderer dims empty rows visually; this test
// just confirms the data layer surfaces them all.
func TestObjectBrowser_TypeListIncludesAll(t *testing.T) {
	ob := fixtureBrowser(t)
	types := ob.typesWithObjects()
	if len(types) != 2 {
		t.Fatalf("typesWithObjects: got %d, want 2", len(types))
	}
	got := []string{types[0].ID, types[1].ID}
	if got[0] != "book" || got[1] != "person" {
		t.Errorf("typesWithObjects ordering: got %v, want [book person]", got)
	}
}

// Refresh clamps cursors against the new dataset so a deletion
// doesn't leave the cursor pointing past the end of the new slice.
func TestObjectBrowser_RefreshClampsCursors(t *testing.T) {
	ob := fixtureBrowser(t)
	// Move object cursor to position 1 (Bob).
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // type cursor → person
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyEnter})                    // focus grid
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // obj cursor → 1
	if ob.objCursor != 1 {
		t.Fatalf("setup: objCursor = %d, want 1", ob.objCursor)
	}
	// Rebuild index without Bob — only Alice remains.
	r := objects.NewRegistry()
	b := objects.NewBuilder(r)
	b.Add("People/Alice.md", "Alice", map[string]string{"type": "person"})
	idx := b.Finalize()
	ob.Refresh(r, idx)
	if ob.objCursor != 0 {
		t.Errorf("after refresh, objCursor = %d, want 0 (clamped)", ob.objCursor)
	}
}

// formatPropertyValue handles the kinds we use in the gallery: empty
// renders as "—", checkboxes flip to ✓/✗, anything else passes
// through trimmed.
func TestFormatPropertyValue(t *testing.T) {
	cases := []struct {
		v, want string
		kind    objects.PropertyKind
	}{
		{"", "—", objects.KindText},
		{"  ", "—", objects.KindText},
		{"hello", "hello", objects.KindText},
		{"  hello  ", "hello", objects.KindText},
		{"true", "✓", objects.KindCheckbox},
		{"yes", "✓", objects.KindCheckbox},
		{"no", "✗", objects.KindCheckbox},
		{"weird", "weird", objects.KindCheckbox},
	}
	for _, c := range cases {
		if got := formatPropertyValue(c.v, c.kind); got != c.want {
			t.Errorf("formatPropertyValue(%q, %s) = %q, want %q",
				c.v, c.kind, got, c.want)
		}
	}
}

// renderPreview shows the focused object's full property bag plus
// a path footer. Always rendered when the type-list cursor lands on
// a populated type — even when the user hasn't yet pressed Tab to
// move focus into the gallery. That doubles as a "what does a Book
// look like?" view while browsing types.
func TestObjectBrowser_RenderPreview(t *testing.T) {
	ob := fixtureBrowser(t)
	ob.objCursor = 0
	ob.typeCursor = 0 // first type with objects (book in fixture)
	out := ob.renderPreview(40, 20)
	if !strings.Contains(out, "Atomic Habits") {
		t.Errorf("preview missing object title: %s", out)
	}
	if !strings.Contains(out, "Author") {
		t.Errorf("preview missing Author property label: %s", out)
	}
	if !strings.Contains(out, "James Clear") {
		t.Errorf("preview missing Author value: %s", out)
	}
	if !strings.Contains(out, "Books/Atomic Habits.md") {
		t.Errorf("preview missing note path: %s", out)
	}
	// Empty properties render as "—" so the user sees schema
	// completeness at a glance.
	if !strings.Contains(out, "—") {
		t.Errorf("expected em-dash placeholder for empty properties: %s", out)
	}
}

// When the cursor points past the available object range — for
// example a freshly-built type with no objects, or before the
// gallery has been populated — the preview shows a friendly hint
// instead of a render error.
func TestObjectBrowser_RenderPreviewEmpty(t *testing.T) {
	ob := fixtureBrowser(t)
	ob.objCursor = 99 // past end
	out := ob.renderPreview(40, 20)
	if !strings.Contains(out, "Select an object") {
		t.Errorf("expected fallback hint when cursor out of range, got: %s", out)
	}
}

// renderPreview at narrow widths still renders without panicking
// and truncates content with ellipsis rather than overflowing.
func TestObjectBrowser_RenderPreviewNarrow(t *testing.T) {
	ob := fixtureBrowser(t)
	ob.focus = 1
	ob.objCursor = 0
	out := ob.renderPreview(15, 8)
	if out == "" {
		t.Error("narrow preview should still render content")
	}
}

// buildColumnSpec falls back to title-only when the pane is too
// narrow for additional columns. Otherwise it picks at most 3
// property columns prioritising required fields.
func TestBuildColumnSpec_TitleOnlyOnNarrow(t *testing.T) {
	tt := objects.Type{ID: "person", Name: "Person", Properties: []objects.Property{
		{Name: "email", Kind: objects.KindURL},
		{Name: "phone", Kind: objects.KindText},
	}}
	cols := buildColumnSpec(tt, 20)
	if len(cols) != 1 || cols[0].name != "Title" {
		t.Errorf("narrow: got %+v, want Title-only", cols)
	}
}

func TestBuildColumnSpec_PrioritisesRequired(t *testing.T) {
	tt := objects.Type{ID: "x", Name: "X", Properties: []objects.Property{
		{Name: "alpha", Kind: objects.KindText},
		{Name: "beta", Kind: objects.KindText, Required: true},
		{Name: "gamma", Kind: objects.KindText},
	}}
	cols := buildColumnSpec(tt, 100)
	// Title + at most 3 prop cols. First prop col must be the
	// Required one (beta), then non-required in declaration order.
	if len(cols) < 2 {
		t.Fatalf("got %d cols, want >=2", len(cols))
	}
	if cols[1].name != "Beta" {
		t.Errorf("first prop col: got %q, want Beta (required first)", cols[1].name)
	}
}
