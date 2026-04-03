package tui

// Layout constants define the available panel arrangements.
//
// To wire these in app.go:
//   - "default"   — 3-panel: sidebar | editor | backlinks
//   - "writer"    — 2-panel: sidebar | editor (no backlinks)
//   - "minimal"   — alias for zen (backward compat)
//   - "reading"   — 2-panel: editor (wide) | backlinks (no sidebar)
//   - "dashboard" — 4-panel: sidebar | editor | outline | backlinks
//   - "zen"       — 1-panel: centered editor, no chrome (distraction-free)
//   - "cockpit"   — 4-panel: sidebar | editor | calendar & tasks
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
	LayoutCornell   = "cornell"
	LayoutFocus     = "focus"
	LayoutCockpit   = "cockpit"
	LayoutStacked   = "stacked"
	LayoutPreview   = "preview"
	LayoutPresenter = "presenter"
	LayoutKanban     = "kanban"
	LayoutWidescreen = "widescreen"
)

// AllLayouts returns every valid layout name in display order.
func AllLayouts() []string {
	return []string{
		LayoutDefault,
		LayoutWriter,
		LayoutReading,
		LayoutDashboard,
		LayoutZen,
		LayoutCockpit,
		LayoutStacked,
		LayoutCornell,
		LayoutFocus,
		LayoutPreview,
		LayoutPresenter,
		LayoutKanban,
		LayoutWidescreen,
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
		return "Centered editor, no chrome (distraction-free)"
	case LayoutReading:
		return "Editor + Backlinks, no sidebar (2-panel)"
	case LayoutDashboard:
		return "Sidebar + Editor + Outline + Backlinks (4-panel)"
	case LayoutZen:
		return "Centered editor, no chrome (distraction-free)"
	case LayoutTaskboard, LayoutCalendar, LayoutCockpit:
		return "Sidebar + Editor + Calendar & Tasks (command center)"
	case LayoutResearch:
		return "Sidebar + Editor + Backlinks (3-panel)"
	case LayoutStacked:
		return "Sidebar + Editor + bottom panels: outline & backlinks (IDE-like)"
	case LayoutCornell:
		return "Editor + Notes panel (vertical study layout)"
	case LayoutFocus:
		return "Sidebar + wide centered editor (focused writing)"
	case LayoutPreview:
		return "Editor + live markdown preview side by side"
	case LayoutPresenter:
		return "Full-screen rendered markdown — presentation mode"
	case LayoutKanban:
		return "Sidebar + Editor + mini Kanban board"
	case LayoutWidescreen:
		return "Sidebar + Outline + Editor + Backlinks + Calendar (ultra-wide)"
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
	case LayoutTaskboard, LayoutCalendar, LayoutCockpit:
		return 4
	case LayoutStacked:
		return 4
	case LayoutResearch:
		return 3
	case LayoutCornell:
		return 2
	case LayoutFocus:
		return 2
	case LayoutPreview:
		return 2
	case LayoutPresenter:
		return 1
	case LayoutKanban:
		return 3
	case LayoutWidescreen:
		return 5
	default:
		return 3
	}
}

// LayoutHasSidebar reports whether the layout includes the file sidebar.
func LayoutHasSidebar(layout string) bool {
	switch layout {
	case LayoutMinimal, LayoutReading, LayoutZen, LayoutPreview, LayoutPresenter:
		return false
	default:
		return true
	}
}

// LayoutHasBacklinks reports whether the layout includes the backlinks panel.
func LayoutHasBacklinks(layout string) bool {
	switch layout {
	case LayoutWriter, LayoutMinimal, LayoutZen, LayoutTaskboard, LayoutCalendar, LayoutCockpit, LayoutStacked, LayoutCornell, LayoutFocus, LayoutPreview, LayoutPresenter, LayoutKanban, LayoutWidescreen:
		return false
	default:
		return true
	}
}

// LayoutHasCalendarPanel reports whether the layout includes the calendar side panel.
func LayoutHasCalendarPanel(layout string) bool {
	return false // cockpit renders its own calendar; standalone calendar layout is retired
}

// LayoutHasOutline reports whether the layout includes a persistent outline panel.
func LayoutHasOutline(layout string) bool {
	return layout == LayoutDashboard
}
