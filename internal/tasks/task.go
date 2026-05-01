// Package tasks owns granit's canonical task model: the Task struct
// that every overlay reads, the TaskStore that persists it, and the
// reconciliation logic that keeps stable IDs attached to lines as the
// user edits markdown by hand.
//
// Tasks live in two places:
//
//   - The user's vault — Tasks.md at vault root and any .md note that
//     contains a GFM checkbox line. This is the canonical *text* and
//     stays editable by vim, git, external tools.
//   - The sidecar at .granit/tasks-meta.json — stable IDs, triage
//     state, scheduled time, project link, timestamps. Anything
//     markdown can't express. Owned by granit, never edited by hand.
//
// The store reconciles the two on every read and keeps them in sync
// on every write. Stable IDs survive text edits, line moves, and
// cross-file moves so triage state and schedule attachments stay
// glued to the right task.
package tasks

import "time"

// TriageState is where a task sits in the planning loop.
//
// New tasks land in Inbox. The triage flow promotes them to Triaged
// (decided what to do with it) → Scheduled (placed on the calendar)
// → Done (executed) or Dropped (decided not to do). Snoozed is a
// detour for "not now, surface again later."
type TriageState string

const (
	TriageInbox     TriageState = "inbox"
	TriageTriaged   TriageState = "triaged"
	TriageScheduled TriageState = "scheduled"
	TriageDone      TriageState = "done"
	TriageDropped   TriageState = "dropped"
	TriageSnoozed   TriageState = "snoozed"
)

// Origin records how a task entered the system. Drives default
// triage state, surfaces in standup/digest summaries, and lets the
// user filter "show me everything I jotted today."
type Origin string

const (
	OriginManual         Origin = "manual"
	OriginJot            Origin = "jot"
	OriginRecurring      Origin = "recurring"
	OriginProjectImport  Origin = "project_import"
	OriginAICapture      Origin = "ai_capture"
)

// Task is the canonical task representation. A superset of the
// legacy tui.Task struct: every existing field is preserved with
// identical JSON tag and zero-value semantics so the reader
// migration is a drop-in. The new fields (ID, Triage, ScheduledStart
// + Duration, ProjectID, Origin, timestamps, Notes) live in the
// sidecar.
//
// ActualMinutes stays computed from the time tracker and is not
// persisted (json:"-"). The same for any other derived field.
type Task struct {
	// ── Stable identity (sidecar) ─────────────────────────────
	ID string `json:"id"`

	// ── Markdown-derived fields (parsed from the note) ────────
	Text             string   `json:"text"`
	Done             bool     `json:"done"`
	DueDate          string   `json:"due_date"`
	Priority         int      `json:"priority"`
	ScheduledTime    string   `json:"scheduled_time,omitempty"`
	Tags             []string `json:"tags,omitempty"`
	NotePath         string   `json:"note_path"`
	LineNum          int      `json:"line_num"`
	Indent           int      `json:"indent,omitempty"`
	ParentLine       int      `json:"parent_line,omitempty"`
	DependsOn        []string `json:"depends_on,omitempty"`
	EstimatedMinutes int      `json:"estimated_minutes,omitempty"`
	Recurrence       string   `json:"recurrence,omitempty"`
	Project          string   `json:"project,omitempty"`
	SnoozedUntil     string   `json:"snoozed_until,omitempty"`
	GoalID           string   `json:"goal_id,omitempty"`

	// ── Sidecar-only metadata (Phase 2 additions) ─────────────
	Triage         TriageState `json:"triage,omitempty"`
	ScheduledStart *time.Time  `json:"scheduled_start,omitempty"`
	Duration       time.Duration `json:"duration,omitempty"`
	ProjectID      string      `json:"project_id,omitempty"`
	Origin         Origin      `json:"origin,omitempty"`
	CreatedAt      time.Time   `json:"created_at,omitempty"`
	LastTriagedAt  *time.Time  `json:"last_triaged_at,omitempty"`
	CompletedAt    *time.Time  `json:"completed_at,omitempty"`
	Notes          string      `json:"notes,omitempty"`

	// ── Computed, never persisted ─────────────────────────────
	ActualMinutes int `json:"-"`
}

// CreateOpts customizes Create() — file destination, origin, and
// initial sidecar metadata. Zero value means "Tasks.md, manual
// origin, inbox triage."
type CreateOpts struct {
	File      string      // "" → vault-root Tasks.md
	Origin    Origin      // "" → OriginManual
	Triage    TriageState // "" → TriageInbox
	ProjectID string
	GoalID    string

	// Section, if non-empty, asks Create to insert the new task line
	// directly after the matching markdown heading (e.g. "## Tasks"
	// or "### Habits"). If the section isn't found, the task is
	// appended at the end of the file as a fallback. Empty (the
	// zero value) preserves the historical append-at-end behavior.
	Section string
}

// EventKind tags Subscribe() callbacks.
type EventKind string

const (
	EventCreated  EventKind = "created"
	EventUpdated  EventKind = "updated"
	EventDeleted  EventKind = "deleted"
	EventReloaded EventKind = "reloaded"
	EventDrifted  EventKind = "drifted" // ID survived a text/anchor change
)

// Event is delivered to Subscribe() callbacks after the store has
// applied a change and released its lock. Old is nil for Created;
// New is nil for Deleted.
type Event struct {
	Kind EventKind
	ID   string
	Old  *Task
	New  *Task
}
