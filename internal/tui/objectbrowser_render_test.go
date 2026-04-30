package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/objects"
)

func obFixture() (objects.Type, *objects.Index) {
	reg := objects.NewRegistry()
	t := objects.Type{
		ID: "person", Name: "Person With An Absurdly Long Type Display Name", Icon: "👤",
		Properties: []objects.Property{
			{Name: "title", Kind: objects.KindText, Required: true},
			{Name: "email", Kind: objects.KindText},
			{Name: "company", Kind: objects.KindText},
		},
	}
	_ = reg.Set(t)

	b := objects.NewBuilder(reg)
	b.Add("People/Alice.md", "Alice Has An Extraordinarily Long Display Name That Will Wrap",
		map[string]string{"type": "person", "email": "alice@example.com",
			"company": "A Very Long Company Name Indeed Inc."})
	b.Add("People/Bob.md", "Bob",
		map[string]string{"type": "person", "email": "b@x"})
	idx := b.Finalize()
	return t, idx
}

// TestObjectBrowser_TypeListNeverWraps regression-tests the original
// complaint: long type names + counts overflowed into a second visual
// line inside the 28-col left pane.
func TestObjectBrowser_TypeListNeverWraps(t *testing.T) {
	_, idx := obFixture()
	reg := objects.NewRegistry()
	_ = reg.Set(objects.Type{
		ID: "person", Name: "Person With An Absurdly Long Type Display Name", Icon: "👤",
	})
	ob := NewObjectBrowser()
	ob.SetSize(120, 30)
	ob.Open(reg, idx)

	out := ob.renderTypeList(28, 20)
	for i, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 28 {
			t.Errorf("type-list line %d exceeded width 28 (got %d): %q", i, w, line)
		}
	}
}

// TestObjectBrowser_GridNeverWraps verifies grid rows fit the pane.
func TestObjectBrowser_GridNeverWraps(t *testing.T) {
	typ, idx := obFixture()
	reg := objects.NewRegistry()
	_ = reg.Set(typ)

	ob := NewObjectBrowser()
	ob.SetSize(120, 30)
	ob.Open(reg, idx)
	ob.focus = 1 // focus the grid

	out := ob.renderGrid(80, 20)
	for i, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 80 {
			t.Errorf("grid line %d exceeded width 80 (got %d): %q", i, w, line)
		}
	}
}

// TestObjectBrowser_HeaderHandlesUntypedHint exercises the second-line
// hint path so a wide untyped warning doesn't push the rule onto a
// third visual row.
func TestObjectBrowser_HeaderHandlesUntypedHint(t *testing.T) {
	typ, _ := obFixture()
	reg := objects.NewRegistry()
	_ = reg.Set(typ)

	bld := objects.NewBuilder(reg)
	bld.Add("Mystery/X.md", "X", map[string]string{"type": "totally-unknown-type"})
	idx := bld.Finalize()

	ob := NewObjectBrowser()
	ob.SetSize(80, 30)
	ob.Open(reg, idx)
	out := ob.renderHeader(80)

	// Hint should be present.
	if !strings.Contains(out, "unknown types") {
		t.Fatalf("expected untyped warning in header, got:\n%s", out)
	}
	for i, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 80 {
			t.Errorf("header line %d exceeded width 80 (got %d): %q", i, w, line)
		}
	}
}
