package tui

// Layout constants define the available panel arrangements.
//
// To wire these in app.go:
//   - "default"   — 3-panel: sidebar | editor | backlinks
//   - "writer"    — 2-panel: sidebar | editor (no backlinks)
//   - "minimal"   — 1-panel: editor only
//   - "reading"   — 2-panel: editor (wide) | backlinks (no sidebar)
//   - "dashboard" — 4-panel: sidebar | editor | outline | backlinks
//   - "zen"       — 1-panel: centered editor, no chrome (distraction-free)
const (
	LayoutDefault   = "default"
	LayoutWriter    = "writer"
	LayoutMinimal   = "minimal"
	LayoutReading   = "reading"
	LayoutDashboard = "dashboard"
	LayoutZen       = "zen"
	LayoutTaskboard = "taskboard"
	LayoutResearch  = "research"
	LayoutCalendar  = "calendar"
)

// AllLayouts returns every valid layout name in display order.
func AllLayouts() []string {
	return []string{
		LayoutDefault,
		LayoutWriter,
		LayoutMinimal,
		LayoutReading,
		LayoutDashboard,
		LayoutZen,
		LayoutTaskboard,
		LayoutResearch,
		LayoutCalendar,
	}
}

// LayoutDescription returns a human-readable description of a layout.
func LayoutDescription(layout string) string {
	switch layout {
	case LayoutDefault:
		return "Sidebar + Editor + Backlinks (3-panel)"
	case LayoutWriter:
		return "Sidebar + Editor (2-panel)"
	case LayoutMinimal:
		return "Editor only (1-panel)"
	case LayoutReading:
		return "Editor + Backlinks, no sidebar (2-panel)"
	case LayoutDashboard:
		return "Sidebar + Editor + Outline + Backlinks (4-panel)"
	case LayoutZen:
		return "Centered editor, no chrome (distraction-free)"
	case LayoutTaskboard:
		return "Sidebar + Editor + Task summary (3-panel)"
	case LayoutResearch:
		return "Sidebar + Editor + Notes panel (3-panel)"
	case LayoutCalendar:
		return "Sidebar + Editor + Calendar/Schedule (3-panel)"
	default:
		return "Unknown layout"
	}
}

// LayoutPanelCount returns how many panels a layout shows.
func LayoutPanelCount(layout string) int {
	switch layout {
	case LayoutDefault:
		return 3
	case LayoutWriter:
		return 2
	case LayoutMinimal:
		return 1
	case LayoutReading:
		return 2
	case LayoutDashboard:
		return 4
	case LayoutZen:
		return 1
	case LayoutTaskboard:
		return 3
	case LayoutResearch:
		return 3
	case LayoutCalendar:
		return 3
	default:
		return 3
	}
}

// LayoutHasSidebar reports whether the layout includes the file sidebar.
func LayoutHasSidebar(layout string) bool {
	switch layout {
	case LayoutMinimal, LayoutReading, LayoutZen:
		return false
	default:
		return true
	}
}

// LayoutHasBacklinks reports whether the layout includes the backlinks panel.
func LayoutHasBacklinks(layout string) bool {
	switch layout {
	case LayoutWriter, LayoutMinimal, LayoutZen, LayoutTaskboard, LayoutResearch, LayoutCalendar:
		return false
	default:
		return true
	}
}

// LayoutHasCalendarPanel reports whether the layout includes the calendar side panel.
func LayoutHasCalendarPanel(layout string) bool {
	return layout == LayoutCalendar
}

// LayoutHasOutline reports whether the layout includes a persistent outline panel.
func LayoutHasOutline(layout string) bool {
	return layout == LayoutDashboard
}
