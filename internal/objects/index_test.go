package objects

import "testing"

// Add records typed notes and skips untyped ones. The skip is silent
// — regular markdown notes shouldn't pollute the typed gallery views.
func TestBuilder_AddSkipsUntyped(t *testing.T) {
	b := NewBuilder(NewRegistry())
	b.Add("Untyped.md", "Untyped", map[string]string{"foo": "bar"})
	b.Add("Person.md", "Sebastian", map[string]string{
		"type": "person", "email": "s@example.com",
	})
	idx := b.Finalize()
	if idx.Total() != 1 {
		t.Errorf("total: got %d, want 1", idx.Total())
	}
	if idx.ByPath("Untyped.md") != nil {
		t.Error("untyped note should not be in the index")
	}
	o := idx.ByPath("Person.md")
	if o == nil {
		t.Fatal("typed note missing from index")
	}
	if o.TypeID != "person" || o.Title != "Sebastian" {
		t.Errorf("object: %+v", o)
	}
	if o.PropertyValue("email") != "s@example.com" {
		t.Errorf("email property: got %q", o.PropertyValue("email"))
	}
}

// Properties exclude `type:` and `title:` since those are promoted to
// the dedicated TypeID and Title fields. Otherwise the Object Browser
// would render duplicate columns.
func TestBuilder_PromotesTypeAndTitle(t *testing.T) {
	b := NewBuilder(NewRegistry())
	b.Add("X.md", "X-Title", map[string]string{
		"type": "book", "title": "X-Title", "author": "Aldous Huxley",
	})
	idx := b.Finalize()
	o := idx.ByPath("X.md")
	if _, has := o.Properties["type"]; has {
		t.Error("type should not appear in Properties map")
	}
	if _, has := o.Properties["title"]; has {
		t.Error("title should not appear in Properties map")
	}
	if o.Properties["author"] != "Aldous Huxley" {
		t.Errorf("author lost: %v", o.Properties)
	}
}

// ByType returns each type's objects sorted alphabetically by title
// (case-insensitive) so the gallery stays stable across rebuilds.
func TestBuilder_ByTypeSortedByTitle(t *testing.T) {
	b := NewBuilder(NewRegistry())
	b.Add("a.md", "Charlie", map[string]string{"type": "person"})
	b.Add("b.md", "alice", map[string]string{"type": "person"})
	b.Add("c.md", "Bob", map[string]string{"type": "person"})
	idx := b.Finalize()
	people := idx.ByType("person")
	if len(people) != 3 {
		t.Fatalf("count: got %d, want 3", len(people))
	}
	want := []string{"alice", "Bob", "Charlie"}
	for i, w := range want {
		if people[i].Title != w {
			t.Errorf("position %d: got %q, want %q", i, people[i].Title, w)
		}
	}
}

// CountByType returns the per-type object count for the type-list
// badge. Empty types should not appear (saves an empty branch in
// the badge renderer).
func TestIndex_CountByType(t *testing.T) {
	b := NewBuilder(NewRegistry())
	b.Add("p1.md", "P1", map[string]string{"type": "person"})
	b.Add("p2.md", "P2", map[string]string{"type": "person"})
	b.Add("b1.md", "B1", map[string]string{"type": "book"})
	idx := b.Finalize()
	counts := idx.CountByType()
	if counts["person"] != 2 || counts["book"] != 1 {
		t.Errorf("counts: %+v", counts)
	}
	if _, has := counts["meeting"]; has {
		t.Error("empty types should not appear in counts")
	}
}

// UntypedCount tracks frontmatter `type:` values that don't match any
// registered type. Surfaces in the UI as a "register or fix typo"
// hint.
func TestIndex_UntypedCount(t *testing.T) {
	b := NewBuilder(NewRegistry())
	b.Add("ok.md", "OK", map[string]string{"type": "person"})
	b.Add("bad.md", "Bad", map[string]string{"type": "definitely-not-a-type"})
	idx := b.Finalize()
	if idx.UntypedCount() != 1 {
		t.Errorf("untyped: got %d, want 1", idx.UntypedCount())
	}
	// The unknown-type object IS still indexed under its TypeID so
	// users can find and fix it. We just count it for the warning.
	if len(idx.ByType("definitely-not-a-type")) != 1 {
		t.Error("unknown-type object should still be retrievable by its TypeID")
	}
}

// Search filters within a type by title OR any property value. Empty
// query returns the unfiltered list. Pass typeID="" to search globally.
func TestIndex_Search(t *testing.T) {
	b := NewBuilder(NewRegistry())
	b.Add("p1.md", "Alice Smith", map[string]string{"type": "person", "email": "alice@x.com"})
	b.Add("p2.md", "Bob Jones", map[string]string{"type": "person", "email": "b@example.com"})
	b.Add("p3.md", "Carol Reed", map[string]string{"type": "person", "email": "carol@x.com"})
	idx := b.Finalize()

	if got := idx.Search("person", ""); len(got) != 3 {
		t.Errorf("empty query: got %d, want 3", len(got))
	}
	if got := idx.Search("person", "alice"); len(got) != 1 || got[0].Title != "Alice Smith" {
		t.Errorf("title match: %+v", got)
	}
	// Property match: "x.com" appears in two emails (Alice, Carol).
	if got := idx.Search("person", "x.com"); len(got) != 2 {
		t.Errorf("property match: got %d, want 2", len(got))
	}
	// Case-insensitive.
	if got := idx.Search("person", "BOB"); len(got) != 1 {
		t.Errorf("case-insensitive: got %d", len(got))
	}
}

// Empty index returns sane zero values for every accessor — not nil
// panics. Important because the Object Browser renders against the
// index before vault loading completes.
func TestIndex_NilSafety(t *testing.T) {
	var i *Index
	if i.Total() != 0 {
		t.Error("Total on nil should be 0")
	}
	if i.UntypedCount() != 0 {
		t.Error("UntypedCount on nil should be 0")
	}
	if i.ByType("person") != nil {
		t.Error("ByType on nil should be nil slice")
	}
	if i.ByPath("x") != nil {
		t.Error("ByPath on nil should be nil")
	}
	if got := i.Search("", ""); got != nil {
		t.Errorf("Search on nil should be nil, got %v", got)
	}
}
