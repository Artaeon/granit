// Package virtues is the canonical schema + IO for the character-
// formation tracker — the "kingdom in me" dimension that complements
// the project / venture / goal "kingdom through me" surface. A
// Virtue is a quality the user is intentionally cultivating
// (patience, humility, generosity, courage, presence, discipline,
// chastity, gratitude, ...). Each virtue carries a dated history of
// weekly self-checks: a 1–5 score plus a free-form reflection note
// captured during the Sunday review rhythm.
//
// Storage lives at <vault>/.granit/virtues.json so the TUI gets the
// same source of truth when its surface lands. The schema is
// purposefully minimal — virtue work is reflective, not metric — so
// the app stays a journal, not a scoreboard.
//
// Pure data + IO only. No HTTP, no rendering. Stdlib + atomicio.
package virtues

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// Status is the lifecycle state of a virtue.
//   - active: currently being practised
//   - paused: shelved without abandoning (the user may return)
//   - archived: completed a season — the virtue stays in history
//     but doesn't surface on the active list
type Status string

const (
	StatusActive   Status = "active"
	StatusPaused   Status = "paused"
	StatusArchived Status = "archived"
)

// Check is one weekly self-evaluation. Score is a 1–5 honest
// self-assessment; Note carries the user's reflection in their own
// words. WeekStart is the Monday (ISO-style) of the week being
// scored — using a week anchor rather than a free date keeps the
// chart axis clean and lets the UI render "missed weeks" gaps.
type Check struct {
	WeekStart string `json:"week_start"` // YYYY-MM-DD, Monday of the scored week
	Score     int    `json:"score"`      // 1–5; clamped on save
	Note      string `json:"note,omitempty"`
	LoggedAt  string `json:"logged_at"`  // RFC3339 — "when did the user actually write this"
}

// Virtue is a single character-formation track.
type Virtue struct {
	ID          string  `json:"id"`          // ULID, lowercase
	Name        string  `json:"name"`        // "Patience", "Humility", ...
	Description string  `json:"description,omitempty"`
	// Anchor is the canonical text the virtue is rooted in — typically
	// a scripture reference (e.g. "1 Cor 13:4-7" for love), a quoted
	// definition, or the user's personal commitment. Free-form so it
	// can hold a verse, a teacher's quote, or a single sentence.
	Anchor      string  `json:"anchor,omitempty"`
	Status      string  `json:"status"`
	// Season is a free-text label for the bounded period the user is
	// cultivating this virtue ("Lent 2026", "Q3 deep work", "this
	// fatherhood season"). Optional — virtues without a season just
	// run continuously.
	Season      string  `json:"season,omitempty"`
	Color       string  `json:"color,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
	Checks      []Check `json:"checks,omitempty"`
}

// NormalizeStatus collapses a user-supplied status string to one of
// the three canonical values. Empty / unknown defaults to active so
// a fresh entry without an explicit status starts on the working list.
func NormalizeStatus(s string) string {
	switch Status(strings.ToLower(strings.TrimSpace(s))) {
	case StatusActive, "":
		return string(StatusActive)
	case StatusPaused:
		return string(StatusPaused)
	case StatusArchived:
		return string(StatusArchived)
	default:
		return string(StatusActive)
	}
}

// ClampScore enforces the 1–5 range. We use 0 to mean "not yet
// scored" elsewhere (e.g. an in-progress form), so a save call
// receiving 0 is rejected by Validate; this function exists for
// the case where a client sent 7 by mistake — round to 5 rather
// than refusing the save.
func ClampScore(n int) int {
	if n < 1 {
		return 1
	}
	if n > 5 {
		return 5
	}
	return n
}

// Validate reports problems with a Virtue before save. Empty Name
// is the only hard requirement — the rest is voluntary self-direction.
func (v Virtue) Validate() error {
	if strings.TrimSpace(v.Name) == "" {
		return errors.New("virtues: name is required")
	}
	return nil
}

// ValidateCheck reports problems with a Check before save. Score
// must be 1–5 (the API clamps but caller-supplied 0 is the canonical
// "didn't fill it in" sentinel and we reject explicitly so the
// frontend doesn't accidentally submit empty forms).
func ValidateCheck(c Check) error {
	if c.Score < 1 || c.Score > 5 {
		return fmt.Errorf("virtues: score must be 1..5, got %d", c.Score)
	}
	if strings.TrimSpace(c.WeekStart) == "" {
		return errors.New("virtues: week_start is required")
	}
	if _, err := time.Parse("2006-01-02", c.WeekStart); err != nil {
		return fmt.Errorf("virtues: week_start must be YYYY-MM-DD: %w", err)
	}
	return nil
}

// MondayOf returns the Monday (ISO week start) for the given time, in
// the same location as t. Used by the API to canonicalise a checkbox
// that the user clicked at any point in the week to a single
// week-start anchor — "I rated patience this week" lands on the same
// Monday no matter when in the week the click happened.
func MondayOf(t time.Time) string {
	// time.Weekday: Sunday=0, Monday=1, ... Saturday=6.
	// We want Monday=0 offset: subtract (weekday-1+7)%7 days.
	offset := (int(t.Weekday()) + 6) % 7
	monday := t.AddDate(0, 0, -offset)
	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, monday.Location()).Format("2006-01-02")
}

// StatePath returns the canonical file path. Centralised so a future
// relocation is a single edit.
func StatePath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "virtues.json")
}

// LoadAll reads virtues.json. Returns an empty slice (not an error)
// for both missing and corrupt files — same pattern as
// internal/goals and internal/ventures so a corrupt file doesn't
// crash callers.
func LoadAll(vaultRoot string) []Virtue {
	data, err := os.ReadFile(StatePath(vaultRoot))
	if err != nil {
		return nil
	}
	var all []Virtue
	if err := json.Unmarshal(data, &all); err != nil {
		return nil
	}
	return all
}

// SaveAll writes the full list via atomic tmp+rename so a crash
// mid-write cannot truncate history.
func SaveAll(vaultRoot string, list []Virtue) error {
	if vaultRoot == "" {
		return errors.New("virtues: empty vault root")
	}
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if list == nil {
		list = []Virtue{}
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(StatePath(vaultRoot), data)
}

// FindByID returns a copy + index, or (Virtue{}, -1).
func FindByID(list []Virtue, id string) (Virtue, int) {
	for i, v := range list {
		if v.ID == id {
			return v, i
		}
	}
	return Virtue{}, -1
}

// LatestCheck returns the most recent Check by WeekStart (lex sort
// works because dates are YYYY-MM-DD). Empty struct + false when no
// checks exist. Used by the UI to render the "this week's score" pill.
func (v Virtue) LatestCheck() (Check, bool) {
	if len(v.Checks) == 0 {
		return Check{}, false
	}
	latest := v.Checks[0]
	for _, c := range v.Checks[1:] {
		if c.WeekStart > latest.WeekStart {
			latest = c
		}
	}
	return latest, true
}

// SortChecksByWeek reorders the Checks slice newest-first. Mutates
// in place because the caller usually wants the sorted ordering
// from this point onward (rendering, JSON output).
func SortChecksByWeek(checks []Check) {
	sort.SliceStable(checks, func(i, j int) bool {
		return checks[i].WeekStart > checks[j].WeekStart
	})
}

// UpsertCheck adds a new Check or replaces an existing one for the
// same week. Returns the updated Checks slice — caller is responsible
// for assigning back to the Virtue and persisting.
func UpsertCheck(checks []Check, next Check) []Check {
	for i, c := range checks {
		if c.WeekStart == next.WeekStart {
			checks[i] = next
			return checks
		}
	}
	return append(checks, next)
}
