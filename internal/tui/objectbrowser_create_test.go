package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/objects"
)

// TestObjectBrowser_ShowsAllTypesIncludingEmpty regression-tests the
// "only shows two types" complaint. Empty types must still appear in
// the type list — otherwise users with a fresh vault have no way to
// see what types are available for creation.
func TestObjectBrowser_ShowsAllTypesIncludingEmpty(t *testing.T) {
	reg := objects.NewRegistry() // ships with 12 built-ins
	idx := objects.NewIndex()    // empty — no objects yet

	ob := NewObjectBrowser()
	ob.SetSize(120, 30)
	ob.Open(reg, idx)

	types := ob.typesWithObjects()
	if len(types) < 5 {
		t.Fatalf("expected built-in types to appear even with empty index, got %d", len(types))
	}
}

func TestObjectBrowser_NKeyOpensCreatePrompt(t *testing.T) {
	reg := objects.NewRegistry()
	idx := objects.NewIndex()
	ob := NewObjectBrowser()
	ob.SetSize(120, 30)
	ob.Open(reg, idx)

	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if !ob.creating {
		t.Fatal("expected create prompt to be active after pressing n")
	}
}

func TestObjectBrowser_CreatePromptCommitsOnEnter(t *testing.T) {
	reg := objects.NewRegistry()
	idx := objects.NewIndex()
	ob := NewObjectBrowser()
	ob.SetSize(120, 30)
	ob.Open(reg, idx)

	// Use the first registered type — built-in 'person'.
	first := reg.All()[0]
	// Position cursor on it (already at 0, but explicit).
	ob.typeCursor = 0

	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	for _, r := range "Alice Chen" {
		ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyEnter})

	path, content, ok := ob.GetCreateRequest()
	if !ok {
		t.Fatal("expected createReq to be set after Enter")
	}
	if !strings.Contains(path, "Alice Chen") {
		t.Fatalf("path doesn't contain title: %q", path)
	}
	if !strings.Contains(content, "type: "+first.ID) {
		t.Fatalf("content missing type: %s; got:\n%s", first.ID, content)
	}
	if !strings.Contains(content, "title: Alice Chen") {
		t.Fatalf("content missing title; got:\n%s", content)
	}
	// Consumed-once.
	if _, _, ok := ob.GetCreateRequest(); ok {
		t.Fatal("create request should be consumed-once")
	}
}

func TestObjectBrowser_CreatePromptEscCancels(t *testing.T) {
	reg := objects.NewRegistry()
	idx := objects.NewIndex()
	ob := NewObjectBrowser()
	ob.SetSize(120, 30)
	ob.Open(reg, idx)

	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if ob.creating {
		t.Fatal("create prompt should be closed after Esc")
	}
	if _, _, ok := ob.GetCreateRequest(); ok {
		t.Fatal("Esc should not commit a create request")
	}
}

func TestObjectBrowser_CreatePromptEmptyTitleRejected(t *testing.T) {
	reg := objects.NewRegistry()
	idx := objects.NewIndex()
	ob := NewObjectBrowser()
	ob.SetSize(120, 30)
	ob.Open(reg, idx)

	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyEnter}) // empty title
	if _, _, ok := ob.GetCreateRequest(); ok {
		t.Fatal("empty title should not produce a create request")
	}
	if ob.creating {
		t.Fatal("prompt should still close even with empty input")
	}
}
