// Package widgets implements granit's Daily Hub widget runtime.
// Widgets are small UI cells that render onto the dashboard grid
// declared by the active profile (internal/profiles).
//
// Each built-in widget is a struct that satisfies both
// profiles.Widget (the data manifest — Lua-implementable) and
// the local Render/HandleKey contract (Go-only since it uses
// tea.Cmd and lipgloss). The WidgetRegistry stores both halves
// keyed by ID.
//
// Widgets read all their data from WidgetCtx — they never reach
// into the Model. The Daily Hub controller (in package tui)
// populates WidgetCtx from the canonical sources (TaskStore,
// vault, .granit/* state files) once per render pass and feeds
// each widget the slice of context its DataNeeds() declared.
package widgets

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/profiles"
	"github.com/artaeon/granit/internal/tasks"
)

// Widget is what every Daily Hub cell implements. The manifest
// half (ID/Title/MinSize/DataNeeds) comes from profiles.Widget;
// Render and HandleKey are local to this package because they
// touch tea.Cmd. A Lua wrapper would satisfy profiles.Widget
// directly and Render/HandleKey via a generated adapter.
type Widget interface {
	profiles.Widget

	// Render returns the lipgloss-rendered cell content sized to
	// (width, height). The runtime guarantees those dimensions
	// meet MinSize before calling. Pure function — must not
	// mutate ctx and must not perform I/O.
	Render(ctx WidgetCtx, width, height int) string

	// HandleKey gets keyboard events when this widget has focus.
	// Return handled=true to stop propagation; cmd is optional
	// for widgets that need to issue follow-up actions (e.g.
	// open a note). Widgets that don't take input return false
	// for every key.
	HandleKey(ctx WidgetCtx, key string) (handled bool, cmd tea.Cmd)
}

// WidgetCtx carries the per-render snapshot a widget needs. The
// controller populates only the fields a widget's DataNeeds()
// declared, so widgets shouldn't assume every slice is non-nil.
//
// Action callbacks let widgets request side effects without
// importing the Model — the controller wires them to real
// behavior. Nil callbacks are no-ops.
type WidgetCtx struct {
	// Snapshots — populated based on widget's DataNeeds().
	Tasks         []tasks.Task
	RecentNotes   []NoteRef
	Scripture     ScriptureVerse
	BusinessPulse []BusinessSample
	TriageInbox   int
	TodayEvents   []CalendarEvent
	Goals         []GoalSummary
	Habits        []HabitEntry

	// Per-widget config from the DashboardCell in the profile.
	Config map[string]any

	// Action callbacks the widget can fire. Nil-safe.
	OpenNote      func(path string)
	CompleteTask  func(id string)
	StartPomodoro func(taskText string)
	OpenTriage    func()
	OpenJotEditor func()

	// CreateTask is for the today.jot widget. Returns the new
	// task or an error so the widget can show feedback.
	CreateTask func(text string) error
}

// NoteRef is the minimal note reference recent.notes needs.
// Defined here (not imported from vault) to keep the widgets
// package self-contained.
type NoteRef struct {
	Path     string
	Title    string // basename without .md
	Modified string // pre-formatted for the cell, e.g. "2h ago"
}

// ScriptureVerse mirrors the existing scripture data.
type ScriptureVerse struct {
	Reference string
	Text      string
}

// BusinessSample is one tick of the business-pulse trend.
type BusinessSample struct {
	Label string
	Value float64
}

// CalendarEvent is one item on today's calendar (ICS event,
// planner block, anything time-bounded).
type CalendarEvent struct {
	Time  string // pre-formatted "HH:MM"
	Title string
	Kind  string // "event", "block", "deadline"
}

// GoalSummary is one row in the goal-progress widget.
type GoalSummary struct {
	ID       string
	Name     string
	Progress float64 // 0.0–1.0
}

// HabitEntry is one row in the habit-streak widget.
type HabitEntry struct {
	Name      string
	DoneToday bool
	Streak    int
}
