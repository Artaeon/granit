package push

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// Preferences controls what the reminder scheduler will and won't
// fire. Stored at <vault>/.granit/notifications.json. Sensible
// defaults — calendar events on, tasks on with a 09:00 morning
// reminder, deadlines on with 7/3/1/0 day-before steps, quiet
// hours off.
//
// First load (file missing) returns DefaultPreferences. The
// scheduler reads this file on every tick so changes take effect
// within ~30 seconds without a server restart.
type Preferences struct {
	// Master toggles per category. When a category is disabled
	// the scheduler skips it entirely — no scan, no fire.
	Calendar  CategoryPrefs `json:"calendar"`
	Tasks     TaskPrefs     `json:"tasks"`
	Deadlines DeadlinePrefs `json:"deadlines"`
	// QuietHours suppresses ALL notifications during the window,
	// regardless of category. Implemented as a single window
	// (e.g. 22:00 → 07:00) — sufficient for most users; a
	// per-day-of-week schedule can come later.
	QuietHours QuietHours `json:"quiet_hours"`
	// DefaultEventReminder is the minutes-before value pre-filled
	// into the calendar create form so a power user doesn't have
	// to click the chip every time.
	DefaultEventReminder int `json:"default_event_reminder"`
}

type CategoryPrefs struct {
	Enabled bool `json:"enabled"`
}

type TaskPrefs struct {
	Enabled bool `json:"enabled"`
	// DueTodayTime — HH:MM string, the time-of-day at which a
	// "task due today" reminder fires. 09:00 default. One push
	// per day per task with `dueDate` matching today.
	DueTodayTime string `json:"due_today_time"`
}

type DeadlinePrefs struct {
	Enabled bool `json:"enabled"`
	// DaysBefore — list of offsets at which to fire reminders for
	// upcoming deadlines (e.g. [7,3,1,0] = a week, 3 days, 1 day,
	// and the day of). Each offset fires one notification.
	DaysBefore []int `json:"days_before"`
	// AtTime — HH:MM string. All deadline reminders fire at this
	// time-of-day (otherwise scheduler timing would scatter them
	// throughout the day).
	AtTime string `json:"at_time"`
}

type QuietHours struct {
	Enabled bool   `json:"enabled"`
	Start   string `json:"start"` // HH:MM, e.g. "22:00"
	End     string `json:"end"`   // HH:MM, e.g. "07:00"
}

// DefaultPreferences returns the out-of-the-box config. Used on
// first load and as the schema reference for the JSON file.
func DefaultPreferences() Preferences {
	return Preferences{
		Calendar:             CategoryPrefs{Enabled: true},
		Tasks:                TaskPrefs{Enabled: true, DueTodayTime: "09:00"},
		Deadlines:            DeadlinePrefs{Enabled: true, DaysBefore: []int{7, 3, 1, 0}, AtTime: "09:00"},
		QuietHours:           QuietHours{Enabled: false, Start: "22:00", End: "07:00"},
		DefaultEventReminder: 15,
	}
}

// IsQuiet reports whether `now` falls inside the quiet-hours
// window. False when the window is disabled. Handles wrap-around
// (a window like 22:00-07:00 spans midnight).
func (p Preferences) IsQuiet(now time.Time) bool {
	if !p.QuietHours.Enabled {
		return false
	}
	curMins := now.Hour()*60 + now.Minute()
	startMins, ok1 := parseHHMM(p.QuietHours.Start)
	endMins, ok2 := parseHHMM(p.QuietHours.End)
	if !ok1 || !ok2 {
		return false
	}
	if startMins == endMins {
		return false // zero-length window — treat as off
	}
	if startMins < endMins {
		// Same-day window (e.g. 12:00-14:00 lunch quiet).
		return curMins >= startMins && curMins < endMins
	}
	// Wrap-around (e.g. 22:00-07:00 — quiet from 22:00 through
	// midnight to 07:00 the next morning).
	return curMins >= startMins || curMins < endMins
}

// MatchesAtTime returns true when `now` is within ~30 seconds
// (one scheduler tick) of the configured at-time. Used by the
// scheduler to decide whether THIS tick should fire a daily
// reminder.
func MatchesAtTime(now time.Time, hhmm string) bool {
	t, ok := parseHHMM(hhmm)
	if !ok {
		return false
	}
	curMins := now.Hour()*60 + now.Minute()
	// Hit the window when now is in [t, t+1) minute. Scheduler
	// runs every 30s so this fires once or twice within the
	// same minute; LastReminderFired dedup catches the second.
	return curMins == t
}

func parseHHMM(s string) (int, bool) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, false
	}
	var h, m int
	for _, c := range parts[0] {
		if c < '0' || c > '9' {
			return 0, false
		}
		h = h*10 + int(c-'0')
	}
	for _, c := range parts[1] {
		if c < '0' || c > '9' {
			return 0, false
		}
		m = m*10 + int(c-'0')
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

func prefsPath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "notifications.json")
}

// LoadPrefs reads the prefs sidecar. Missing file → DefaultPreferences.
// Malformed file → DefaultPreferences with the error returned so the
// caller can surface it (don't lose pushes because the JSON went
// bad).
func LoadPrefs(vaultRoot string) (Preferences, error) {
	data, err := os.ReadFile(prefsPath(vaultRoot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultPreferences(), nil
		}
		return DefaultPreferences(), err
	}
	var p Preferences
	if err := json.Unmarshal(data, &p); err != nil {
		return DefaultPreferences(), err
	}
	// Fill defaults for any unset substructure — handles a JSON
	// upgrade where a new field was added.
	def := DefaultPreferences()
	if p.Tasks.DueTodayTime == "" {
		p.Tasks.DueTodayTime = def.Tasks.DueTodayTime
	}
	if p.Deadlines.AtTime == "" {
		p.Deadlines.AtTime = def.Deadlines.AtTime
	}
	if p.Deadlines.DaysBefore == nil {
		p.Deadlines.DaysBefore = def.Deadlines.DaysBefore
	}
	if p.QuietHours.Start == "" {
		p.QuietHours.Start = def.QuietHours.Start
	}
	if p.QuietHours.End == "" {
		p.QuietHours.End = def.QuietHours.End
	}
	if p.DefaultEventReminder == 0 {
		p.DefaultEventReminder = def.DefaultEventReminder
	}
	return p, nil
}

func SavePrefs(vaultRoot string, p Preferences) error {
	dir := filepath.Dir(prefsPath(vaultRoot))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(prefsPath(vaultRoot), data)
}
