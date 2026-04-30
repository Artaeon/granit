package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/objects"
)

// TestSidebar_TypesView_RowsNeverWrap is the regression test for the
// "many lines and cut-off names" report. Long titles previously wrapped
// inside lipgloss boxes when the row content exceeded contentWidth; now
// every row is hard-truncated before rendering, so no row should
// produce more than one visual line.
func TestSidebar_TypesView_RowsNeverWrap(t *testing.T) {
	reg := objects.NewRegistry()
	b := objects.NewBuilder(reg)
	// One person with a deliberately oversized title.
	b.Add("People/Long.md",
		"A Person With An Absurdly Long Display Title That Definitely Exceeds The Sidebar Width",
		map[string]string{"type": "person"})
	b.Add("People/Short.md", "Bob", map[string]string{"type": "person"})
	idx := b.Finalize()

	s := NewSidebar([]string{"People/Long.md", "People/Short.md"})
	s.SetSize(28, 20) // Narrow sidebar — typical default.
	s.focused = true
	s.SetTypedObjects(reg, idx)
	s.SetMode(ModeTypes)

	out := s.renderTypesView(s.contentWidth())

	// Every output line must fit within contentWidth (no wrapping).
	for i, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > s.contentWidth() {
			t.Errorf("line %d wrapped past contentWidth=%d (got %d): %q",
				i, s.contentWidth(), w, line)
		}
	}
}

// TestSidebar_TypesView_LongTitleTruncatedWithEllipsis verifies the
// truncation produces an ellipsis (so the user can tell the title
// was cut and isn't simply that short).
func TestSidebar_TypesView_LongTitleTruncatedWithEllipsis(t *testing.T) {
	reg := objects.NewRegistry()
	b := objects.NewBuilder(reg)
	b.Add("People/Long.md",
		"An Extremely Long Person Title Beyond Reasonable Width",
		map[string]string{"type": "person"})
	idx := b.Finalize()

	s := NewSidebar([]string{"People/Long.md"})
	s.SetSize(28, 20)
	s.focused = true
	s.SetTypedObjects(reg, idx)
	s.SetMode(ModeTypes)

	out := s.renderTypesView(s.contentWidth())
	if !strings.Contains(out, "…") {
		t.Fatalf("expected ellipsis in truncated row; got:\n%s", out)
	}
}

// contentWidth is exported via this small helper. Sidebar's renderer
// computes it internally; we recompute the same way for assertion
// purposes.
func (s *Sidebar) contentWidth() int {
	w := s.width - 2
	if w < 10 {
		w = 10
	}
	return w
}
