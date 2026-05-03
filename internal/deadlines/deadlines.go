// Package deadlines is the canonical schema + IO for top-level
// deadlines stored in <vault>/.granit/deadlines.json. A deadline is a
// "this matters by date X" marker — semantically distinct from a task
// (a deadline isn't checkboxed and never appears in someone's todo
// list) and from a goal (a goal has milestones / progress; a deadline
// is a hard moment in time, possibly linked to a goal). Lifted into
// its own package so the TUI, web server, and any future agents share
// one source of truth on disk — round-tripping through the web cannot
// drop fields the TUI wrote.
//
// Pure data + IO only. No HTTP, no rendering.
package deadlines

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// Importance ranks how badly a deadline being missed matters. Three
// levels covers the realistic spread without forcing the user to pick
// from a long list (and the calendar overlay only has three colours
// to spend on it).
type Importance string

const (
	ImportanceCritical Importance = "critical"
	ImportanceHigh     Importance = "high"
	ImportanceNormal   Importance = "normal"
)

// Status is the lifecycle state of a deadline. "missed" is computed at
// load-time when an active deadline's date has passed, but is also
// persisted so a user-driven mark stays put even if today's clock
// moves backwards (NTP jitter, timezone change).
type Status string

const (
	StatusActive    Status = "active"
	StatusMissed    Status = "missed"
	StatusMet       Status = "met"
	StatusCancelled Status = "cancelled"
)

// Deadline is a top-level "this matters by date X" marker. Linkable to
// at most one goal, one project, and any number of tasks — the links
// are loose foreign keys (no FK enforcement on disk; the web/TUI just
// renders chips when the IDs resolve). Every JSON field below MUST be
// preserved by writers — a web PATCH cannot silently drop a field the
// TUI wrote.
type Deadline struct {
	ID          string    `json:"id"`             // ULID (lowercase, matches Event)
	Title       string    `json:"title"`          // required
	Date        string    `json:"date"`           // YYYY-MM-DD; required
	Description string    `json:"description,omitempty"`
	GoalID      string    `json:"goal_id,omitempty"`     // matches Goal.ID
	ProjectName string    `json:"project,omitempty"`     // matches Project.Name
	TaskIDs     []string  `json:"task_ids,omitempty"`    // matches Task.ID
	Importance  string    `json:"importance"`            // critical | high | normal
	Status      string    `json:"status"`                // active | missed | met | cancelled
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ValidateDate reports whether s is a YYYY-MM-DD date the package will
// accept. Exposed so handlers can reject bad input with a 400 before
// touching disk.
func ValidateDate(s string) bool {
	if s == "" {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// NormalizeImportance returns one of the three canonical importance
// values, defaulting to "normal" for empty / unknown input. Centralised
// so a typo in a TUI patch ("hi") doesn't escape into the web's UI as
// an unrecognised pill.
func NormalizeImportance(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(ImportanceCritical):
		return string(ImportanceCritical)
	case string(ImportanceHigh):
		return string(ImportanceHigh)
	case string(ImportanceNormal), "":
		return string(ImportanceNormal)
	default:
		return string(ImportanceNormal)
	}
}

// NormalizeStatus returns one of the four canonical status values,
// defaulting to "active" for empty / unknown input.
func NormalizeStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(StatusActive), "":
		return string(StatusActive)
	case string(StatusMissed):
		return string(StatusMissed)
	case string(StatusMet):
		return string(StatusMet)
	case string(StatusCancelled):
		return string(StatusCancelled)
	default:
		return string(StatusActive)
	}
}

// DaysRemaining returns the integer days from today (local midnight) to
// the deadline date. Negative when overdue, 0 when today, positive when
// future. Returns 0 (not -1) for unparseable dates — callers usually
// want to short-circuit on the err path before this anyway, but a
// silent zero is the least surprising fallback for label rendering.
//
// We compute date-difference, not hour-difference: the deadline value
// is YYYY-MM-DD (no time component), so going from "today midnight" to
// "yesterday midnight" via Sub().Hours()/24 truncates to 0 instead of
// -1 when the local timezone is anything but UTC. Building both ends as
// midnight-local and dividing the wall-clock minutes / 1440 (rounded
// to the nearest int via math.Round) lands on the right integer day in
// every zone.
func (d Deadline) DaysRemaining() int {
	t, err := time.Parse("2006-01-02", d.Date)
	if err != nil {
		return 0
	}
	target := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	diff := target.Sub(today)
	// math.Round to the nearest whole day — int(-0.5) truncates to 0
	// in Go, which would map yesterday → "today". DST 23/25-hour skew
	// is also absorbed by rounding rather than floor.
	return int(math.Round(diff.Hours() / 24))
}

// Countdown returns a short human-readable label like "in 12d", "today",
// "3d ago" — a tiny formatter the web uses on row chips, lifted here
// so the TUI can render the same string.
func (d Deadline) Countdown() string {
	n := d.DaysRemaining()
	switch {
	case n == 0:
		return "today"
	case n == 1:
		return "tomorrow"
	case n == -1:
		return "yesterday"
	case n > 1:
		return fmt.Sprintf("in %dd", n)
	default:
		return fmt.Sprintf("%dd ago", -n)
	}
}

// IsOverdue is true when the deadline date is in the past AND the
// status is still active (a met / cancelled deadline is never overdue).
func (d Deadline) IsOverdue() bool {
	if d.Status != string(StatusActive) {
		return false
	}
	return d.DaysRemaining() < 0
}

// StatePath returns the canonical path to the deadlines.json file.
// Centralised so a future relocation is a single edit.
func StatePath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "deadlines.json")
}

// LoadAll reads every deadline from disk. Returns nil for both
// missing and corrupt files — callers handle the nil slice as the
// empty state (a corrupt file would otherwise crash the TUI on load).
func LoadAll(vaultRoot string) []Deadline {
	data, err := os.ReadFile(StatePath(vaultRoot))
	if err != nil {
		return nil
	}
	var all []Deadline
	if err := json.Unmarshal(data, &all); err != nil {
		return nil
	}
	return all
}

// SaveAll writes every deadline to disk via an atomic tmp+rename so a
// crash mid-write cannot truncate the user's history. Returns nil on
// success.
func SaveAll(vaultRoot string, ds []Deadline) error {
	if vaultRoot == "" {
		return errors.New("deadlines: empty vault root")
	}
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// Marshal an empty slice as `[]`, not `null` — the TUI reads
	// either fine, but the web's JSON parser unwraps `[]` directly into
	// an empty array (Goal does the same in its handler).
	if ds == nil {
		ds = []Deadline{}
	}
	data, err := json.MarshalIndent(ds, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(StatePath(vaultRoot), data)
}

// SortForDisplay orders deadlines for the list view: active + missed
// (the ones that still need attention) come first, sorted by date
// ascending; met + cancelled come last, also by date. Within the same
// status bucket, ties are broken by ID so the order is stable across
// reloads. Returns a sorted COPY — callers iterating the original
// slice via &ds[i] don't get their indexes scrambled out from under
// them.
func SortForDisplay(ds []Deadline) []Deadline {
	out := make([]Deadline, len(ds))
	copy(out, ds)
	rank := func(s string) int {
		switch s {
		case string(StatusActive):
			return 0
		case string(StatusMissed):
			return 1
		case string(StatusMet):
			return 2
		case string(StatusCancelled):
			return 3
		default:
			return 4
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := rank(out[i].Status), rank(out[j].Status)
		if ri != rj {
			return ri < rj
		}
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// FindByID returns a pointer-to-copy and the index of the deadline
// with the given ID, or (Deadline{}, -1) if not found. Pointer-to-copy
// pattern matches granitmeta — callers mutate then re-save the whole
// slice rather than mutating in place (atomicio.WriteState gives us
// crash-safety only on the rewrite).
func FindByID(ds []Deadline, id string) (Deadline, int) {
	for i, d := range ds {
		if d.ID == id {
			return d, i
		}
	}
	return Deadline{}, -1
}
