package profiles

// Widget is the manifest a registered widget presents. Pure data
// so a Lua-implemented widget satisfies it the same way a Go one
// does. Render and key handling live on a parallel interface in
// the tui package (WidgetRender) — this keeps the profiles
// package free of tea types.
type Widget interface {
	ID() string                 // stable, e.g. "today.jot"
	Title() string              // header rendered above the cell
	MinSize() (cols, rows int)  // grid downgrades cells smaller than this to an inline error stub
	DataNeeds() []DataKind      // which snapshots the Daily Hub controller pushes to this widget
}

// DataKind enumerates the snapshot kinds the Daily Hub controller
// can feed to widgets. Widgets declare what they need; the
// controller subscribes once per kind across all widgets and
// re-renders only widgets whose DataNeeds intersect with the
// dirty kinds. Strings (not iota ints) so Lua manifests can
// reference them by name without an enum table.
type DataKind string

const (
	DataTasks         DataKind = "tasks"
	DataCalendar      DataKind = "calendar_events"
	DataHabits        DataKind = "habits"
	DataGoals         DataKind = "goals"
	DataNotes         DataKind = "notes"
	DataPomodoro      DataKind = "pomodoro"
	DataPlanner       DataKind = "planner"
	DataScripture     DataKind = "scripture"
	DataBusinessPulse DataKind = "business_pulse"
	DataTriage        DataKind = "triage"
)
