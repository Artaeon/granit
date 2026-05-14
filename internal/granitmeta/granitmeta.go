// Package granitmeta exposes lightweight readers/writers for the JSON
// sidecars granit maintains under <vault>/.granit/. Both the TUI and the
// web API target the same files; granitmeta is the single source of truth
// for the on-disk schema.
package granitmeta

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/artaeon/granit/internal/atomicio"
)

// Event mirrors the schema in <vault>/.granit/events.json.
type Event struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Date      string `json:"date"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Location  string `json:"location,omitempty"`
	Color     string `json:"color,omitempty"`
	// Kind is the optional event type — meeting / focus / personal /
	// travel / break / blocker / "" (generic). Drives a small glyph
	// prefix on calendar chips + a default colour band so users can
	// scan a packed day and see at a glance "two meetings, a focus
	// block, a workout" without reading every title. Empty string
	// is the default (no type), unrecognised strings round-trip
	// through but display as generic.
	Kind string `json:"kind,omitempty"`
	// RemindMinutesBefore — Web Push reminder offset. 0 means
	// "no reminder". Common values: 5, 10, 15, 30, 60. The push
	// scheduler reads this to know when to fire a notification.
	RemindMinutesBefore int `json:"remind_minutes_before,omitempty"`
	// LastReminderFired is the RFC3339 timestamp of the most
	// recent reminder we sent. Set by the scheduler so a second
	// tick within the fire window doesn't double-notify.
	LastReminderFired string `json:"last_reminder_fired,omitempty"`
	CreatedAt         string `json:"created_at,omitempty"`
	// RRule is the RFC 5545 recurrence rule for repeating events
	// (e.g. "FREQ=WEEKLY;BYDAY=MO,WE,FR;UNTIL=20261231T235959Z").
	// Empty for one-off events. Stored verbatim so a user who
	// hand-edits events.json with a more advanced rule (BYSETPOS,
	// BYMONTHDAY) round-trips cleanly even when our expander only
	// honors the common subset.
	RRule string `json:"rrule,omitempty"`
	// ExDates is the list of cancelled occurrences for a recurring
	// event (RFC 5545 EXDATE). Each entry is either YYYY-MM-DD (for
	// all-day events) or YYYY-MM-DDTHH:MM:SS in the user's local
	// time. The expander filters these from the emitted instances
	// so 'cancel just this week's meeting' doesn't disrupt the
	// whole series. Stored as a slice rather than a map so JSON
	// round-trips cleanly through events.json.
	ExDates []string `json:"ex_dates,omitempty"`
	// ProjectID is the optional project name this event is linked
	// to. Free-text by design (matches Project.Name) so renaming a
	// project doesn't transitively break event links — same trade-
	// off as Task.Project. Drives the calendar's project-filter,
	// project-color overlay, and the "events for project X" pivot
	// in the project detail surface. Empty for unlinked events.
	ProjectID string `json:"project_id,omitempty"`
	// Overrides is the per-occurrence patch table for a recurring
	// event. Keyed by the occurrence's UTC timestamp in the same
	// shape as ExDates: YYYY-MM-DD for all-day, YYYY-MM-DDTHH:MM:SS
	// for timed. Used by the expander to surface a single tweaked
	// occurrence (e.g. "this Tuesday is at 10:00 instead of 09:00")
	// without rewriting the series base. ExDates remains the
	// "cancel just this one" path — overrides handle "edit just
	// this one" instead. Both can be authored by hand in events.json
	// and round-trip through the API.
	Overrides map[string]EventOverride `json:"overrides,omitempty"`
}

// EventOverride patches a single occurrence of a recurring event.
// Every field is optional: a non-empty value replaces the inherited
// series value for THIS occurrence only. Empty fields fall through
// to the series value. The key into Event.Overrides is the original
// occurrence's UTC timestamp (matching the EXDATE shape) — that way
// we identify "the 9am occurrence on March 4" canonically even if
// the user moves it to noon on March 5.
type EventOverride struct {
	// StartTime / EndTime: HH:MM 24-hour. Empty inherits the series time.
	StartTime string `json:"start_time,omitempty"`
	EndTime   string `json:"end_time,omitempty"`
	// Date: YYYY-MM-DD. Set ONLY when the occurrence shifts to a
	// different calendar day from its series anchor (the common drag-
	// move case). Empty means the override sits on the original date.
	Date string `json:"date,omitempty"`
	// Title / Location / Color: optional per-occurrence textual
	// overrides — covers the "team standup → all-hands today" case
	// without disturbing the rest of the series.
	Title    string `json:"title,omitempty"`
	Location string `json:"location,omitempty"`
	Color    string `json:"color,omitempty"`
}

func ReadEvents(vaultRoot string) ([]Event, error) {
	return readJSON[[]Event](filepath.Join(vaultRoot, ".granit", "events.json"))
}

func WriteEvents(vaultRoot string, events []Event) error {
	return writeJSON(filepath.Join(vaultRoot, ".granit", "events.json"), events)
}

// ProjectMilestone is a sub-step inside a ProjectGoal.
type ProjectMilestone struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// ProjectGoal is a high-level goal within a project, with optional
// milestones tracked under it.
type ProjectGoal struct {
	Title      string             `json:"title"`
	Done       bool               `json:"done"`
	Milestones []ProjectMilestone `json:"milestones,omitempty"`
}

// Project mirrors a single entry in <vault>/.granit/projects.json. The
// full schema — keep all fields the TUI writes so we can round-trip
// without dropping data on PATCH.
//
// Kind/Venture/RepoURL were added later; older projects.json files
// pre-dating these fields round-trip correctly because every new field
// is `omitempty` and the JSON decoder leaves missing keys at the zero
// value.
type Project struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Folder      string        `json:"folder"`
	Tags        []string      `json:"tags"`
	Status      string        `json:"status"`
	Color       string        `json:"color"`
	CreatedAt   string        `json:"created_at"`
	Notes       []string      `json:"notes,omitempty"`
	TaskFilter  string        `json:"task_filter,omitempty"`
	Category    string        `json:"category,omitempty"`
	Goals       []ProjectGoal `json:"goals,omitempty"`
	NextAction  string        `json:"next_action,omitempty"`
	Priority    int           `json:"priority,omitempty"`
	DueDate     string        `json:"due_date,omitempty"`
	TimeSpent   int           `json:"time_spent,omitempty"`
	UpdatedAt   string        `json:"updated_at,omitempty"`
	// Kind is the project type — drives which extra fields the UI
	// surfaces (e.g. RepoURL only renders when Kind == "software").
	// Free-form so the UI can introduce new types without a server
	// migration; the canonical set today is software, content,
	// research, business, personal, creative, client, other.
	Kind string `json:"kind,omitempty"`
	// Venture groups projects under a parent organization, company,
	// or umbrella initiative. Free-text by design — projects can be
	// grouped without first creating a formal venture record.
	Venture string `json:"venture,omitempty"`
	// RepoURL is the source-control URL for software projects.
	// Persisted regardless of Kind so a project can be reclassified
	// without losing the link, but the UI hides the field unless
	// Kind == "software".
	RepoURL string `json:"repo_url,omitempty"`
}

func ReadProjects(vaultRoot string) ([]Project, error) {
	return readJSON[[]Project](filepath.Join(vaultRoot, ".granit", "projects.json"))
}

func WriteProjects(vaultRoot string, projects []Project) error {
	return writeJSON(filepath.Join(vaultRoot, ".granit", "projects.json"), projects)
}

// NOTE: the top-level Goal schema previously lived here as granitmeta.Goal.
// It was a stripped subset that silently dropped fields the TUI wrote
// (Notes, ReviewFrequency, LastReviewed, ReviewLog, CompletedAt, Color,
// per-milestone DueDate / CompletedAt). It was retired in favour of the
// internal/goals package, which is the single source of truth for the
// .granit/goals.json on-disk schema. Use internal/goals from now on.

func readJSON[T any](path string) (T, error) {
	var zero T
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return zero, nil
		}
		return zero, err
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		return zero, err
	}
	return out, nil
}

func writeJSON(path string, v interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(path, data)
}
