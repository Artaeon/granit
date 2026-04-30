package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/objects"
)

func obDeleteFixture(t *testing.T) ObjectBrowser {
	t.Helper()
	r := objects.NewRegistryEmpty()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(r.Set(objects.Type{ID: "person", Name: "Person", Folder: "People",
		Properties: []objects.Property{
			{Name: "title", Kind: objects.KindText, Required: true},
		}}))
	bld := objects.NewBuilder(r)
	bld.Add("People/Alice.md", "Alice", map[string]string{"type": "person"})
	bld.Add("People/Bob.md", "Bob", map[string]string{"type": "person"})
	idx := bld.Finalize()
	ob := NewObjectBrowser()
	ob.SetSize(120, 30)
	ob.Open(r, idx)
	// Focus the grid and put cursor on Alice (alphabetically first).
	ob.focus = 1
	ob.objCursor = 0
	return ob
}

func TestObjectBrowser_DKeyArmsDeleteConfirmation(t *testing.T) {
	ob := obDeleteFixture(t)
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	if !ob.confirmingDelete {
		t.Fatal("expected confirmingDelete after pressing D")
	}
	if ob.deletePath != "People/Alice.md" {
		t.Errorf("wrong path queued: %q", ob.deletePath)
	}
	// No request should be emitted yet — must wait for y.
	if _, ok := ob.GetDeleteRequest(); ok {
		t.Fatal("delete should not fire before y confirmation")
	}
}

func TestObjectBrowser_YConfirmsDelete(t *testing.T) {
	ob := obDeleteFixture(t)
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	path, ok := ob.GetDeleteRequest()
	if !ok || path != "People/Alice.md" {
		t.Fatalf("expected delete fired for Alice, got (%q, %v)", path, ok)
	}
	// Consumed-once.
	if _, ok := ob.GetDeleteRequest(); ok {
		t.Fatal("delete request should be consumed-once")
	}
}

func TestObjectBrowser_NCancelsDelete(t *testing.T) {
	ob := obDeleteFixture(t)
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if ob.confirmingDelete {
		t.Fatal("n should clear the confirming-delete state")
	}
	if _, ok := ob.GetDeleteRequest(); ok {
		t.Fatal("delete should not fire after n")
	}
}

func TestObjectBrowser_DOnTypeListNoOp(t *testing.T) {
	ob := obDeleteFixture(t)
	ob.focus = 0 // type list
	ob, _ = ob.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	if ob.confirmingDelete {
		t.Fatal("D on type-list pane should not arm delete")
	}
}
