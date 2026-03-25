package tui

import "testing"

// ---------------------------------------------------------------------------
// LayoutHasCalendarPanel
// ---------------------------------------------------------------------------

func TestLayoutCalendar_HasCalendarPanel(t *testing.T) {
	if !LayoutHasCalendarPanel(LayoutCalendar) {
		t.Error("expected LayoutHasCalendarPanel to return true for 'calendar' layout")
	}

	// Other layouts should not have a calendar panel.
	for _, layout := range []string{LayoutDefault, LayoutWriter, LayoutMinimal, LayoutReading, LayoutDashboard, LayoutZen, LayoutTaskboard, LayoutResearch} {
		if LayoutHasCalendarPanel(layout) {
			t.Errorf("expected LayoutHasCalendarPanel to return false for %q", layout)
		}
	}
}

// ---------------------------------------------------------------------------
// LayoutHasBacklinks — calendar layout should not have backlinks
// ---------------------------------------------------------------------------

func TestLayoutCalendar_NoBacklinks(t *testing.T) {
	if LayoutHasBacklinks(LayoutCalendar) {
		t.Error("expected LayoutHasBacklinks to return false for 'calendar' layout")
	}
}

// ---------------------------------------------------------------------------
// AllLayouts — contains calendar
// ---------------------------------------------------------------------------

func TestAllLayouts_IncludesCalendar(t *testing.T) {
	layouts := AllLayouts()

	found := false
	for _, l := range layouts {
		if l == LayoutCalendar {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected AllLayouts to contain %q, got %v", LayoutCalendar, layouts)
	}
}
