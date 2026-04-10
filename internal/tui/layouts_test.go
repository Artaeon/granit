package tui

import "testing"

// ---------------------------------------------------------------------------
// LayoutHasCalendarPanel — cockpit renders its own calendar, no standalone
// ---------------------------------------------------------------------------

func TestLayoutCalendar_HasCalendarPanel(t *testing.T) {
	// Cockpit and widescreen have calendar panels.
	if !LayoutHasCalendarPanel("cockpit") {
		t.Error("expected LayoutHasCalendarPanel to return true for cockpit")
	}
	if !LayoutHasCalendarPanel("widescreen") {
		t.Error("expected LayoutHasCalendarPanel to return true for widescreen")
	}
	// Other layouts should not.
	for _, layout := range []string{"default", "writer", "zen", "reading", "dashboard"} {
		if LayoutHasCalendarPanel(layout) {
			t.Errorf("expected LayoutHasCalendarPanel to return false for %q", layout)
		}
	}
}

// ---------------------------------------------------------------------------
// LayoutHasBacklinks — cockpit should not have a backlinks panel
// ---------------------------------------------------------------------------

func TestLayoutCockpit_NoBacklinks(t *testing.T) {
	if LayoutHasBacklinks(LayoutCockpit) {
		t.Error("expected LayoutHasBacklinks to return false for 'cockpit' layout")
	}
}

// ---------------------------------------------------------------------------
// AllLayouts — contains cockpit (replacement for calendar/taskboard)
// ---------------------------------------------------------------------------

func TestAllLayouts_IncludesCockpit(t *testing.T) {
	layouts := AllLayouts()

	found := false
	for _, l := range layouts {
		if l == LayoutCockpit {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected AllLayouts to contain %q, got %v", LayoutCockpit, layouts)
	}
}

// ---------------------------------------------------------------------------
// Legacy 'calendar' and 'taskboard' layouts must not appear in AllLayouts.
// They were merged into cockpit and a runtime migration in NewModel rewrites
// stored config values, so the constants no longer exist.
// ---------------------------------------------------------------------------

func TestAllLayouts_DropsRetiredLayouts(t *testing.T) {
	for _, retired := range []string{"calendar", "taskboard"} {
		for _, l := range AllLayouts() {
			if l == retired {
				t.Errorf("expected AllLayouts to NOT contain retired %q", retired)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// AllLayouts — verify count and no duplicates
// ---------------------------------------------------------------------------

func TestAllLayouts_NoDuplicates(t *testing.T) {
	layouts := AllLayouts()
	seen := make(map[string]bool)
	for _, l := range layouts {
		if seen[l] {
			t.Errorf("duplicate layout in AllLayouts: %q", l)
		}
		seen[l] = true
	}
}

// ---------------------------------------------------------------------------
// Every layout in AllLayouts has a description
// ---------------------------------------------------------------------------

func TestAllLayouts_HaveDescriptions(t *testing.T) {
	for _, l := range AllLayouts() {
		desc := LayoutDescription(l)
		if desc == "" || desc == "Unknown layout" {
			t.Errorf("layout %q has no description", l)
		}
	}
}
