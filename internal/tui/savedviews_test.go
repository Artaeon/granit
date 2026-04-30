package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/objects"
)

func TestSavedViews_PickerThenLoad(t *testing.T) {
	cat := objects.NewViewCatalog([]objects.View{
		{ID: "a", Name: "View A", Description: "first"},
		{ID: "b", Name: "View B", Description: "second"},
	})
	idx := objects.NewIndex()

	sv := NewSavedViews()
	sv.SetSize(80, 24)
	sv.OpenPicker(cat, idx)

	if !sv.IsActive() {
		t.Fatal("picker should be active")
	}
	if !sv.pickerMode {
		t.Fatal("expected pickerMode=true after OpenPicker")
	}

	// Move cursor down then Enter — should load the second view.
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sv.pickerMode {
		t.Fatal("picker should be exited after Enter")
	}
	if sv.CurrentViewID() != "b" {
		t.Fatalf("expected view b, got %q", sv.CurrentViewID())
	}
}

func TestSavedViews_OpenSpecificViewBypassesPicker(t *testing.T) {
	cat := objects.NewViewCatalog([]objects.View{
		{ID: "a", Name: "View A", Description: "first"},
	})
	idx := objects.NewIndex()
	sv := NewSavedViews()
	if err := sv.Open(cat, idx, "a"); err != nil {
		t.Fatal(err)
	}
	if sv.pickerMode {
		t.Fatal("Open() should land in view mode, not picker")
	}
	if sv.CurrentViewID() != "a" {
		t.Fatalf("expected a, got %q", sv.CurrentViewID())
	}
}

func TestSavedViews_OpenUnknownViewErrors(t *testing.T) {
	cat := objects.NewViewCatalog(nil)
	sv := NewSavedViews()
	if err := sv.Open(cat, objects.NewIndex(), "missing"); err == nil {
		t.Fatal("expected error for missing view ID")
	}
}

func TestSavedViews_PReturnsToPickerFromView(t *testing.T) {
	cat := objects.NewViewCatalog([]objects.View{
		{ID: "a", Name: "View A", Description: "first"},
	})
	idx := objects.NewIndex()
	sv := NewSavedViews()
	sv.SetSize(80, 24)
	sv.OpenPicker(cat, idx)
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// Now in view mode. Press p → back to picker.
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if !sv.pickerMode {
		t.Fatal("p should return to picker mode")
	}
}

func TestSavedViews_EnterOnObjectRowSetsJumpRequest(t *testing.T) {
	cat := objects.NewViewCatalog([]objects.View{
		{ID: "a", Name: "View A", Description: "first", Type: "person"},
	})
	idx := objects.NewIndex()
	// Stuff a fake object in.
	obj := &objects.Object{TypeID: "person", NotePath: "People/Alice.md", Title: "Alice"}
	// Use the public Builder to seed properly.
	reg := objects.NewRegistry()
	b := objects.NewBuilder(reg)
	b.Add(obj.NotePath, obj.Title, map[string]string{"type": "person"})
	idx = b.Finalize()

	sv := NewSavedViews()
	sv.SetSize(80, 24)
	if err := sv.Open(cat, idx, "a"); err != nil {
		t.Fatal(err)
	}
	if len(sv.matches) == 0 {
		t.Fatal("expected the seeded object to match")
	}
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := sv.GetJumpRequest()
	if !ok || got != "People/Alice.md" {
		t.Fatalf("expected jump to People/Alice.md, got (%q, %v)", got, ok)
	}
	// Should be consumed-once.
	if _, ok := sv.GetJumpRequest(); ok {
		t.Fatal("jump request should be consumed-once")
	}
}

func TestSavedViews_RefreshUpdatesMatches(t *testing.T) {
	cat := objects.NewViewCatalog([]objects.View{
		{ID: "a", Name: "View A", Description: "first", Type: "person"},
	})
	idx := objects.NewIndex()

	sv := NewSavedViews()
	if err := sv.Open(cat, idx, "a"); err != nil {
		t.Fatal(err)
	}
	if len(sv.matches) != 0 {
		t.Fatalf("expected 0 matches initially, got %d", len(sv.matches))
	}

	// Now seed an index and Refresh.
	reg := objects.NewRegistry()
	b := objects.NewBuilder(reg)
	b.Add("p.md", "P", map[string]string{"type": "person"})
	newIdx := b.Finalize()
	sv.Refresh(newIdx)
	if len(sv.matches) != 1 {
		t.Fatalf("expected 1 match after refresh, got %d", len(sv.matches))
	}
}
