// Package people is the canonical schema + IO for granit's
// relationships / lightweight CRM. State lives at
// <vault>/.granit/people.json so the TUI gets the same source of
// truth when its surface lands.
//
// Deliberately NOT trying to be a full address book — no merging, no
// dedup, no per-person history of every interaction. The point is
// "remember to keep in touch": a person record carries a
// last-contacted date and an optional cadence (days), and the UI
// computes "stale" against that. The actual conversation history
// lives in regular notes that the user wikilinks back to the person
// (or doesn't — the cadence + stale view is useful by itself).
//
// Pure data + IO only.
package people

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// Person is one entry in the relationship list.
//
// Cadence: zero means "no reminder" (just a contact record). When >0,
// a person is "stale" iff today minus LastContactedAt > Cadence days.
// The handler / UI computes this — the schema only stores the inputs.
type Person struct {
	ID                string    `json:"id"`         // ULID, lowercase
	Name              string    `json:"name"`
	Email             string    `json:"email,omitempty"`
	Phone             string    `json:"phone,omitempty"`
	Birthday          string    `json:"birthday,omitempty"`            // YYYY-MM-DD or YYYY-??-?? if year unknown
	Relationship      string    `json:"relationship,omitempty"`        // family / friend / colleague / mentor / acquaintance — freeform
	Tags              []string  `json:"tags,omitempty"`
	LastContactedAt   string    `json:"last_contacted_at,omitempty"`   // YYYY-MM-DD
	CadenceDays       int       `json:"cadence_days,omitempty"`        // 0 = no reminder
	NotePath          string    `json:"note_path,omitempty"`           // wikilink to a /notes file with conversation history
	Notes             string    `json:"notes,omitempty"`               // inline freeform — small things that don't deserve a note
	Archived          bool      `json:"archived,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// StatePath: .granit/people.json (singular folder, plural file —
// same convention as goals.json / deadlines.json since people is one
// flat list, not a nested concept like prayer/intentions.json).
func StatePath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "people.json")
}

func LoadAll(vaultRoot string) []Person {
	data, err := os.ReadFile(StatePath(vaultRoot))
	if err != nil {
		return nil
	}
	var all []Person
	if err := json.Unmarshal(data, &all); err != nil {
		return nil
	}
	return all
}

func SaveAll(vaultRoot string, xs []Person) error {
	if vaultRoot == "" {
		return errors.New("people: empty vault root")
	}
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if xs == nil {
		xs = []Person{}
	}
	data, err := json.MarshalIndent(xs, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(StatePath(vaultRoot), data)
}

// IsStale reports whether a person should surface in the
// "ping these people" view. Returns false when cadence isn't set
// (the user opted out of reminders for this person), when the
// person is archived, or when last-contact is within the cadence
// window. Today is passed in (rather than computed) so tests are
// deterministic and the UI can render "stale relative to which day"
// for snapshot views.
func (p Person) IsStale(today time.Time) bool {
	if p.Archived || p.CadenceDays <= 0 {
		return false
	}
	if p.LastContactedAt == "" {
		// No contact ever recorded but a cadence is set → stale.
		return true
	}
	last, err := time.Parse("2006-01-02", p.LastContactedAt)
	if err != nil {
		return false
	}
	return today.Sub(last) > time.Duration(p.CadenceDays)*24*time.Hour
}

// SortForDisplay: stale-first (the "act on this" group), then
// non-stale active people alpha by Name, then archived at the bottom.
// ID stable tiebreak.
func SortForDisplay(xs []Person, today time.Time) []Person {
	out := make([]Person, len(xs))
	copy(out, xs)
	sort.SliceStable(out, func(i, j int) bool {
		ai, aj := out[i].Archived, out[j].Archived
		if ai != aj {
			return !ai
		}
		si, sj := out[i].IsStale(today), out[j].IsStale(today)
		if si != sj {
			return si
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// UpcomingBirthdays returns people whose birthday falls in the next
// `windowDays` days from `today`. Birthdays with year == 0000 (or
// missing year) match by month-day only. Used for the dashboard
// "upcoming birthdays" widget and the people page header.
func UpcomingBirthdays(xs []Person, today time.Time, windowDays int) []Person {
	if windowDays <= 0 {
		return nil
	}
	end := today.AddDate(0, 0, windowDays)
	out := []Person{}
	for _, p := range xs {
		if p.Archived || p.Birthday == "" {
			continue
		}
		if when, ok := nextBirthdayOccurrence(p.Birthday, today); ok {
			if !when.Before(today) && !when.After(end) {
				out = append(out, p)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		// Both birthdays already in the window — sort by next occurrence.
		ti, _ := nextBirthdayOccurrence(out[i].Birthday, today)
		tj, _ := nextBirthdayOccurrence(out[j].Birthday, today)
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// nextBirthdayOccurrence projects the next time a birthday lands on
// or after `today`. Accepts both "YYYY-MM-DD" (real birth year) and
// "MM-DD" / "0000-MM-DD" (year unknown). Returns false if the input
// can't be parsed.
func nextBirthdayOccurrence(birthday string, today time.Time) (time.Time, bool) {
	month, day, ok := parseBirthdayMonthDay(birthday)
	if !ok {
		return time.Time{}, false
	}
	year := today.Year()
	candidate := time.Date(year, time.Month(month), day, 0, 0, 0, 0, today.Location())
	if candidate.Before(today.Truncate(24 * time.Hour)) {
		candidate = candidate.AddDate(1, 0, 0)
	}
	return candidate, true
}

func parseBirthdayMonthDay(s string) (month, day int, ok bool) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return int(t.Month()), t.Day(), true
	}
	// Try MM-DD shorthand.
	if t, err := time.Parse("01-02", s); err == nil {
		return int(t.Month()), t.Day(), true
	}
	return 0, 0, false
}
