package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/objects"
)

func savedViewsWithMatches(t *testing.T) (*objects.Registry, SavedViews) {
	t.Helper()
	reg := objects.NewRegistryEmpty()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(reg.Set(objects.Type{
		ID: "person", Name: "Person", Folder: "People",
		Properties: []objects.Property{
			{Name: "title", Kind: objects.KindText, Required: true},
		},
	}))
	cat := objects.NewViewCatalog([]objects.View{
		{ID: "v", Name: "All People", Description: "all", Type: "person"},
	})
	bld := objects.NewBuilder(reg)
	bld.Add("People/Alice.md", "Alice Chen", map[string]string{"type": "person"})
	bld.Add("People/Bob.md", "Bob Smith", map[string]string{"type": "person"})
	bld.Add("People/Charlie.md", "Charlie Diaz", map[string]string{"type": "person"})
	idx := bld.Finalize()

	sv := NewSavedViews()
	sv.SetSize(80, 24)
	sv.SetRegistry(reg)
	if err := sv.Open(cat, idx, "v"); err != nil {
		t.Fatal(err)
	}
	return reg, sv
}

func TestSavedViews_SlashEntersFilterMode(t *testing.T) {
	_, sv := savedViewsWithMatches(t)
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !sv.filterMode {
		t.Fatal("expected filterMode after pressing /")
	}
}

func TestSavedViews_FilterNarrowsByTitleSubstring(t *testing.T) {
	_, sv := savedViewsWithMatches(t)
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "char" {
		sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	visible := sv.visibleMatches()
	if len(visible) != 1 || visible[0].Title != "Charlie Diaz" {
		t.Fatalf("expected 1 match (Charlie), got %d (%v)", len(visible), titlesFromObjs(visible))
	}
}

func TestSavedViews_FilterEscClears(t *testing.T) {
	_, sv := savedViewsWithMatches(t)
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "ali" {
		sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if sv.filterMode {
		t.Error("Esc should exit filter mode")
	}
	if sv.filterBuf != "" {
		t.Errorf("Esc should clear filter buf, got %q", sv.filterBuf)
	}
	if len(sv.visibleMatches()) != 3 {
		t.Errorf("after clearing filter, all 3 results should show; got %d", len(sv.visibleMatches()))
	}
}

func TestSavedViews_DKeyArmsDelete(t *testing.T) {
	_, sv := savedViewsWithMatches(t)
	sv.cursor = 0
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	if !sv.confirmingDelete {
		t.Fatal("expected confirmingDelete after D")
	}
	if sv.deletePath != "People/Alice.md" {
		t.Errorf("wrong path: %q", sv.deletePath)
	}
}

func TestSavedViews_YConfirmsDelete(t *testing.T) {
	_, sv := savedViewsWithMatches(t)
	sv.cursor = 0
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	path, ok := sv.GetDeleteRequest()
	if !ok || path != "People/Alice.md" {
		t.Fatalf("expected delete fired for Alice; got (%q, %v)", path, ok)
	}
}

func TestSavedViews_NCancelsDelete(t *testing.T) {
	_, sv := savedViewsWithMatches(t)
	sv.cursor = 0
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if sv.confirmingDelete {
		t.Error("n should clear confirming-delete state")
	}
	if _, ok := sv.GetDeleteRequest(); ok {
		t.Error("delete should not fire after n")
	}
}

func TestSavedViews_FilterEnterCommitsAndKeepsResults(t *testing.T) {
	_, sv := savedViewsWithMatches(t)
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "bob" {
		sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sv.filterMode {
		t.Fatal("Enter should exit filter mode")
	}
	if sv.filterBuf != "bob" {
		t.Errorf("Enter should preserve filter, got %q", sv.filterBuf)
	}
	if len(sv.visibleMatches()) != 1 {
		t.Errorf("filter should still narrow to 1 (Bob); got %d", len(sv.visibleMatches()))
	}
}

func titlesFromObjs(objs []*objects.Object) []string {
	out := make([]string, len(objs))
	for i, o := range objs {
		out[i] = o.Title
	}
	return out
}
