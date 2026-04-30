package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/objects"
)

func TestSavedViews_NQuickCreatesObject(t *testing.T) {
	reg := objects.NewRegistryEmpty()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(reg.Set(objects.Type{
		ID: "article", Name: "Article", Folder: "Articles",
		Properties: []objects.Property{
			{Name: "title", Kind: objects.KindText, Required: true},
			{Name: "url", Kind: objects.KindURL, Required: true},
		},
	}))

	cat := objects.NewViewCatalog([]objects.View{
		{ID: "a", Name: "Articles to Read", Description: "x", Type: "article"},
	})
	idx := objects.NewIndex()

	sv := NewSavedViews()
	sv.SetSize(80, 24)
	sv.SetRegistry(reg)
	if err := sv.Open(cat, idx, "a"); err != nil {
		t.Fatal(err)
	}
	// 'n' arms the create prompt.
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if !sv.creating {
		t.Fatal("expected creating=true after pressing n")
	}
	for _, r := range "My Article" {
		sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyEnter})

	path, content, ok := sv.GetCreateRequest()
	if !ok {
		t.Fatal("expected createReq to fire after Enter")
	}
	if !strings.Contains(path, "My Article") {
		t.Errorf("path doesn't contain title: %q", path)
	}
	if !strings.Contains(content, "type: article") {
		t.Errorf("content missing type: %q", content)
	}
	if !strings.Contains(content, "title: My Article") {
		t.Errorf("content missing title: %q", content)
	}
}

func TestSavedViews_NIsNoOpForAnyTypeView(t *testing.T) {
	reg := objects.NewRegistryEmpty()
	cat := objects.NewViewCatalog([]objects.View{
		{ID: "a", Name: "All", Description: "any type"},
	})
	sv := NewSavedViews()
	sv.SetRegistry(reg)
	if err := sv.Open(cat, objects.NewIndex(), "a"); err != nil {
		t.Fatal(err)
	}
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if sv.creating {
		t.Fatal("n on a type-less view should not arm create")
	}
}

func TestSavedViews_CreatePromptEscCancels(t *testing.T) {
	reg := objects.NewRegistryEmpty()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(reg.Set(objects.Type{ID: "article", Name: "Article",
		Properties: []objects.Property{
			{Name: "title", Kind: objects.KindText, Required: true},
		}}))
	cat := objects.NewViewCatalog([]objects.View{
		{ID: "a", Name: "Articles", Description: "x", Type: "article"},
	})
	sv := NewSavedViews()
	sv.SetRegistry(reg)
	_ = sv.Open(cat, objects.NewIndex(), "a")
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	sv, _ = sv.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if sv.creating {
		t.Fatal("Esc should close the prompt")
	}
	if _, _, ok := sv.GetCreateRequest(); ok {
		t.Fatal("Esc should not commit a create request")
	}
}
