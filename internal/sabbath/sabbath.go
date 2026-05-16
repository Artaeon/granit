// Package sabbath persists the user's Sabbath-mode state to a
// vault-side JSON sidecar so server-side surfaces (push scheduler,
// background agents, AI handlers) can respect it. The frontend drives
// its own UI overlay from localStorage; this package mirrors the
// schedule + the daily flag to disk so the server gates work for the
// user's day of rest regardless of which device is open.
//
// Two pieces of state:
//
//   - ActiveOn (YYYY-MM-DD) — the manual "begin now" flag. Per-device
//     on the UI side, but mirrored here so server-side handlers gate
//     when ANY device is in sabbath. Auto-clears when the date no
//     longer matches today.
//
//   - Schedule {Enabled, DayOfWeek, StartHour, StartMinute,
//     DurationMinutes} — the recurring rule. Synced across devices
//     so a sabbath configured on one phone shows up on the laptop.
//     The window can span midnight (Friday 18:00 + 24h goes through
//     Saturday 18:00) so traditional sundown-to-sundown observance is
//     a first-class shape, not a special case.
//
// IsActiveNow returns true when either: (a) the manual flag matches
// today, or (b) the schedule window covers the current instant. Used
// by every server-side gate (push, AI, chat, agent runs).
//
// Stored at <vault>/.granit/sabbath.json with 0o600 perms via
// atomicio.
package sabbath

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// Schedule is the recurring auto-enable rule. DayOfWeek matches
// time.Weekday (0=Sunday … 6=Saturday). The window starts at
// StartHour:StartMinute on that day and runs DurationMinutes.
// Default 1440 minutes = 24 hours = traditional midnight-to-midnight
// when StartHour/StartMinute are both 0. Setting StartHour=18 with
// a Friday DayOfWeek and 1440 duration produces a sundown-to-sundown
// Jewish sabbath observance.
//
// Enabled=false means schedule never fires (manual flag is the only
// way into sabbath mode).
type Schedule struct {
	Enabled         bool `json:"enabled"`
	DayOfWeek       int  `json:"day_of_week"`
	StartHour       int  `json:"start_hour"`
	StartMinute     int  `json:"start_minute"`
	DurationMinutes int  `json:"duration_minutes"`
}

// DefaultSchedule is what an unconfigured State returns. Off, but with
// sane field values so a UI binding to the struct doesn't see zeros
// that look like "configured midnight Sunday".
func DefaultSchedule() Schedule {
	return Schedule{Enabled: false, DayOfWeek: 0, StartHour: 0, StartMinute: 0, DurationMinutes: 1440}
}

// State is the persisted Sabbath flag. ActiveOn is the YYYY-MM-DD
// date the user manually enabled the mode (empty → manual flag off).
// Schedule is the recurring auto-enable rule.
type State struct {
	ActiveOn string   `json:"active_on,omitempty"`
	Schedule Schedule `json:"schedule"`
}

// Path returns the absolute path of the sidecar.
func Path(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "sabbath.json")
}

// Load reads the sidecar. Missing file → State with DefaultSchedule
// (not an error). Missing schedule field → DefaultSchedule applied so
// older sidecars (pre-schedule) read cleanly without a migration.
func Load(vaultRoot string) (State, error) {
	data, err := os.ReadFile(Path(vaultRoot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{Schedule: DefaultSchedule()}, nil
		}
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, err
	}
	// Pre-schedule sidecars deserialize with a zero Schedule. The
	// zero value has DurationMinutes=0 which would make IsActiveNow
	// reject every instant — distinguishable from the explicit
	// "schedule off" intent by treating a zero-duration as a missing
	// field and substituting the default.
	if s.Schedule.DurationMinutes == 0 {
		s.Schedule = DefaultSchedule()
		s.Schedule.Enabled = false
	}
	return s, nil
}

// Save persists the state.
func Save(vaultRoot string, s State) error {
	dir := filepath.Dir(Path(vaultRoot))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(Path(vaultRoot), data)
}

// scheduleWindow returns the [start, end) window of the schedule's
// CURRENT or MOST-RECENTLY-STARTED occurrence relative to `at`. The
// idea: find the most recent schedule-day occurrence whose start is
// in the past, and return its window. If `at` falls inside that
// window we're in sabbath. If not, we aren't.
//
// Why most-recent-past instead of "today's instance": a Friday 18:00
// → Saturday 18:00 window straddles Friday and Saturday calendar
// days. At Saturday 10:00 the "today's start" would be Saturday
// 00:00 which is wrong; we need to look back at Friday's start.
//
// Returns ok=false when schedule is disabled.
func scheduleWindow(s Schedule, at time.Time) (start, end time.Time, ok bool) {
	if !s.Enabled || s.DurationMinutes <= 0 {
		return time.Time{}, time.Time{}, false
	}
	// Find the most recent occurrence of (DayOfWeek, StartHour,
	// StartMinute) at or before `at`. Walk back at most 8 days from
	// today to cover the case where today's schedule-time hasn't hit
	// yet — we'd then use last week's window (already past, won't
	// match) but the function stays correct.
	for daysBack := 0; daysBack < 8; daysBack++ {
		cand := time.Date(at.Year(), at.Month(), at.Day()-daysBack,
			s.StartHour, s.StartMinute, 0, 0, at.Location())
		if int(cand.Weekday()) != s.DayOfWeek {
			continue
		}
		if cand.After(at) {
			continue
		}
		return cand, cand.Add(time.Duration(s.DurationMinutes) * time.Minute), true
	}
	return time.Time{}, time.Time{}, false
}

// IsActiveNow returns true when sabbath is in effect right now per
// either the manual ActiveOn flag (date matches today) or the
// schedule window (now ∈ [start, end)). Used by every server-side
// gate.
//
// Errors are treated as "not active" — failing-open is safer than
// failing-closed (a missing sidecar shouldn't suppress the user's
// expected notifications).
func IsActiveNow(vaultRoot string) bool {
	s, err := Load(vaultRoot)
	if err != nil {
		return false
	}
	return s.IsActiveAt(time.Now())
}

// IsActiveAt is the pure function behind IsActiveNow — useful for
// tests and for callers that already have a State and a specific
// instant in mind.
func (s State) IsActiveAt(at time.Time) bool {
	if s.ActiveOn != "" && s.ActiveOn == at.Format("2006-01-02") {
		return true
	}
	start, end, ok := scheduleWindow(s.Schedule, at)
	if !ok {
		return false
	}
	return !at.Before(start) && at.Before(end)
}

// RemainingMinutes returns how many minutes are left in the current
// sabbath window. For manual-only activations (ActiveOn matches
// today, no schedule window) it falls back to "until midnight" — the
// behavior the old client assumed. Returns 0 when not active.
func RemainingMinutes(s State, at time.Time) int {
	if !s.IsActiveAt(at) {
		return 0
	}
	if start, end, ok := scheduleWindow(s.Schedule, at); ok && !at.Before(start) && at.Before(end) {
		return int(end.Sub(at) / time.Minute)
	}
	// Manual-only path: midnight tomorrow.
	tomorrow := time.Date(at.Year(), at.Month(), at.Day()+1, 0, 0, 0, 0, at.Location())
	return int(tomorrow.Sub(at) / time.Minute)
}

// NormalizeSchedule clamps a Schedule's fields to valid ranges so a
// malformed PUT can't poison the on-disk state. Duration <=0 falls
// back to 1440 (24h) so a client sending an unset value still gets
// the default window.
func NormalizeSchedule(s Schedule) Schedule {
	if s.DayOfWeek < 0 || s.DayOfWeek > 6 {
		s.DayOfWeek = 0
	}
	if s.StartHour < 0 || s.StartHour > 23 {
		s.StartHour = 0
	}
	if s.StartMinute < 0 || s.StartMinute > 59 {
		s.StartMinute = 0
	}
	if s.DurationMinutes <= 0 {
		s.DurationMinutes = 1440
	}
	if s.DurationMinutes > 7*1440 {
		s.DurationMinutes = 7 * 1440 // cap at a week
	}
	return s
}

// IsActiveToday is kept as a thin alias for IsActiveNow. The old
// name suggested calendar-day semantics which the schedule windows
// break; new code should call IsActiveNow.
//
// Deprecated: use IsActiveNow.
func IsActiveToday(vaultRoot string) bool {
	return IsActiveNow(vaultRoot)
}
