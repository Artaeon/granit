package serveapi

import (
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/deadlines"
	"github.com/artaeon/granit/internal/granitmeta"
)

type calendarEvent struct {
	Type            string  `json:"type"`
	Date            string  `json:"date,omitempty"`
	Start           *string `json:"start,omitempty"`
	End             *string `json:"end,omitempty"`
	Title           string  `json:"title"`
	NotePath        string  `json:"notePath,omitempty"`
	TaskID          string  `json:"taskId,omitempty"`
	EventID         string  `json:"eventId,omitempty"`
	Done            bool    `json:"done,omitempty"`
	Priority        int     `json:"priority,omitempty"`
	DurationMinutes int     `json:"durationMinutes,omitempty"`
	Color           string  `json:"color,omitempty"`
	Location        string  `json:"location,omitempty"`
	// Source is the .ics filename for ICS-derived events (e.g.
	// "faith.ics") — the web uses it to color-by-source so different
	// calendars are visually distinct on the grid.
	Source string `json:"source,omitempty"`
	// Importance is set ONLY on type=="deadline" entries — drives the
	// per-deadline color in the web's calendar overlay
	// (critical→error / high→warning / normal→secondary). Empty for
	// every other event type.
	Importance string `json:"importance,omitempty"`
	// RRule is the source recurrence rule when this event was
	// expanded from one. Repeated for every occurrence so the chip
	// can show a ↻ indicator and the edit modal can offer to edit
	// the series. Empty for one-off events.
	RRule string `json:"rrule,omitempty"`
	// ProjectID surfaces the optional project link from
	// granitmeta.Event.ProjectID. Used by the calendar's project
	// filter + colour-by-project overlay; also surfaced on
	// task_scheduled / task_due rows when the underlying task is
	// scoped to a project (via tasks.Project), so 'show only events
	// for project X' filters tasks AND events together.
	ProjectID string `json:"project_id,omitempty"`
}

// expandAllDayDates walks every day in an all-day event's [start, end)
// span (ICS DTEND is exclusive) and returns ISO dates clamped to the
// requested [from, rangeEnd) window. Used by the calendar handler so
// a multi-day vacation renders on every day of the trip rather than
// only on day 1. Pure helper, no I/O — tested separately to lock the
// inclusive-start / exclusive-end / window-clamp semantics.
func expandAllDayDates(start, end, from, rangeEnd time.Time) []string {
	if end.IsZero() || !end.After(start) {
		end = start.Add(24 * time.Hour)
	}
	var out []string
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		if d.Before(from) || !d.Before(rangeEnd) {
			continue
		}
		out = append(out, d.Format("2006-01-02"))
	}
	return out
}

func parseDateQuery(s string) (time.Time, error) {
	if s == "" {
		return time.Now(), nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local), nil
}

func (s *Server) handleCalendar(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, err := parseDateQuery(q.Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid from")
		return
	}
	to, err := parseDateQuery(q.Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid to")
		return
	}
	if to.Before(from) {
		writeError(w, http.StatusBadRequest, "to must be >= from")
		return
	}

	events := []calendarEvent{}
	cfg := s.dailyConfigFor()

	// Daily notes (existing only)
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		filename := d.Format("2006-01-02") + ".md"
		rel := filename
		if cfg.Folder != "" {
			rel = filepath.ToSlash(filepath.Join(cfg.Folder, filename))
		}
		if n := s.cfg.Vault.GetNote(rel); n != nil {
			events = append(events, calendarEvent{
				Type:     "daily",
				Date:     d.Format("2006-01-02"),
				Title:    d.Format("2006-01-02"),
				NotePath: n.RelPath,
			})
		}
	}

	// Granit events.json (calendar events). Events with an RRule
	// expand into multiple occurrences across the requested window
	// using the same expander as the ICS pipeline — single source of
	// truth for recurrence semantics so a native weekly event behaves
	// identically to a weekly event imported from a .ics file.
	rangeEndForNative := to.Add(24 * time.Hour)
	if granitEvents, err := granitmeta.ReadEvents(s.cfg.Vault.Root); err == nil {
		for _, ev := range granitEvents {
			d, err := time.Parse("2006-01-02", ev.Date)
			if err != nil {
				continue
			}
			// Build the icsEvent shape so we can run expandRRULE.
			// AllDay = no start time set; Start/End get parsed in
			// local time the same way the original branch did.
			seed := icsEvent{
				Title:    ev.Title,
				Location: ev.Location,
				UID:      ev.ID,
				RRule:    ev.RRule,
			}
			// Seed EXDATE filter from the native event's cancelled-
			// occurrence list. The expander filters these out the
			// same way it does for ICS-imported events, so 'skip
			// this week' on a recurring meeting drops just that
			// instance without disrupting the series.
			if len(ev.ExDates) > 0 {
				seed.ExDates = make(map[string]struct{}, len(ev.ExDates))
				for _, x := range ev.ExDates {
					seed.ExDates[x] = struct{}{}
				}
			}
			if ev.StartTime == "" {
				seed.AllDay = true
				seed.Start = d
				seed.End = d.Add(24 * time.Hour) // exclusive end day
			} else {
				if startT, err := time.ParseInLocation("2006-01-02 15:04", ev.Date+" "+ev.StartTime, time.Local); err == nil {
					seed.Start = startT
					if ev.EndTime != "" {
						if endT, err := time.ParseInLocation("2006-01-02 15:04", ev.Date+" "+ev.EndTime, time.Local); err == nil {
							seed.End = endT
						}
					}
				} else {
					continue
				}
			}
			// Non-recurring fast path: single window check, no
			// expand call. Keeps the common case cheap on large
			// events.json files.
			occurrences := []icsEvent{seed}
			if ev.RRule != "" {
				occurrences = expandRRULE(seed, from, rangeEndForNative)
			}
			for _, occ := range occurrences {
				occDate := occ.Start.Format("2006-01-02")
				if occ.AllDay {
					if occ.Start.Before(from) || !occ.Start.Before(rangeEndForNative) {
						continue
					}
					events = append(events, calendarEvent{
						Type:      "event",
						Title:     ev.Title,
						Date:      occDate,
						EventID:   ev.ID,
						Color:     ev.Color,
						Location:  ev.Location,
						RRule:     ev.RRule,
						ProjectID: ev.ProjectID,
					})
					continue
				}
				if occ.Start.Before(from) || !occ.Start.Before(rangeEndForNative) {
					continue
				}
				ce := calendarEvent{
					Type:      "event",
					Title:     ev.Title,
					EventID:   ev.ID,
					Color:     ev.Color,
					Location:  ev.Location,
					RRule:     ev.RRule,
					ProjectID: ev.ProjectID,
				}
				sStr := occ.Start.Format(time.RFC3339)
				ce.Start = &sStr
				if !occ.End.IsZero() {
					eStr := occ.End.Format(time.RFC3339)
					ce.End = &eStr
					ce.DurationMinutes = int(occ.End.Sub(occ.Start).Minutes())
				}
				events = append(events, ce)
			}
		}
	}

	// Tasks: due dates (all-day) + scheduled (timed)
	all := s.cfg.TaskStore.All()
	fromDate := from.Format("2006-01-02")
	toDate := to.Format("2006-01-02")
	for _, t := range all {
		// Project link: ProjectID wins (canonical sidecar field), Project
		// (markdown-extracted) is the fallback. Either drives the
		// project-filter / colour-by-project overlay on the grid.
		proj := t.ProjectID
		if proj == "" {
			proj = t.Project
		}
		if t.DueDate != "" && t.DueDate >= fromDate && t.DueDate <= toDate {
			events = append(events, calendarEvent{
				Type:      "task_due",
				Date:      t.DueDate,
				Title:     t.Text,
				NotePath:  t.NotePath,
				TaskID:    t.ID,
				Done:      t.Done,
				Priority:  t.Priority,
				ProjectID: proj,
			})
		}
		if t.ScheduledStart != nil {
			st := *t.ScheduledStart
			if st.Before(from) || st.After(to.Add(24*time.Hour)) {
				continue
			}
			startStr := st.Format(time.RFC3339)
			ce := calendarEvent{
				Type:      "task_scheduled",
				Start:     &startStr,
				Title:     t.Text,
				NotePath:  t.NotePath,
				TaskID:    t.ID,
				Done:      t.Done,
				Priority:  t.Priority,
				ProjectID: proj,
			}
			if t.Duration > 0 {
				ce.DurationMinutes = int(t.Duration / time.Minute)
				e := st.Add(t.Duration).Format(time.RFC3339)
				ce.End = &e
			}
			events = append(events, ce)
		}
	}

	// ICS calendar files (vault root + calendars/). Honor granit's
	// `disabled_calendars` list (config.json + .granit.json) so the web
	// view stays in sync with the TUI's source toggles.
	//
	// Dedup mirrors the TUI: title|start|end. UID-based dedup misses the
	// common case where a vault has a per-source file *and* a merged.ics —
	// the merged version assigns its own UIDs, so the same event ends up
	// with two different UIDs. Title+start+end is the right granularity
	// for "same occurrence shown twice".
	gcfg := config.LoadForVault(s.cfg.Vault.Root)
	icsSeen := map[string]struct{}{}
	// Window the ICS expansion will render into. Used both as the
	// per-day expansion clamp for multi-day events and as the
	// time-window filter for timed events.
	rangeEnd := to.Add(24 * time.Hour)
	for _, base := range icsScan(s.cfg.Vault.Root, gcfg.DisabledCalendars) {
		for _, ev := range expandRRULE(base, from, rangeEnd) {
			if ev.AllDay {
				// Multi-day all-day events: ICS DTEND is exclusive
				// (DTSTART=Aug1 DTEND=Aug11 → 10 days, Aug 1 through 10).
				// Without per-day expansion, a 10-day vacation only
				// rendered on day 1 because calendarEvent carries a
				// single Date field. expandAllDayDates clamps to the
				// requested window and returns one ISO date per day.
				// Per-day dedup key folds duplicates from a per-source
				// merged.ics into a single entry per day.
				for _, dayISO := range expandAllDayDates(ev.Start, ev.End, from, rangeEnd) {
					key := ev.Title + "|allday|" + dayISO
					if _, dup := icsSeen[key]; dup {
						continue
					}
					icsSeen[key] = struct{}{}
					events = append(events, calendarEvent{
						Type:     "ics_event",
						Title:    ev.Title,
						Location: ev.Location,
						EventID:  ev.UID,
						Color:    "cyan",
						Source:   ev.Source,
						Date:     dayISO,
						RRule:    ev.RRule,
					})
				}
				continue
			}
			// Timed event branch — keep the existing dedup shape.
			endKey := ""
			if !ev.End.IsZero() {
				endKey = ev.End.Format("15:04")
			}
			key := ev.Title + "|" + ev.Start.Format("2006-01-02T15:04") + "|" + endKey
			if _, dup := icsSeen[key]; dup {
				continue
			}
			icsSeen[key] = struct{}{}
			ce := calendarEvent{
				Type:     "ics_event",
				Title:    ev.Title,
				Location: ev.Location,
				EventID:  ev.UID,
				Color:    "cyan",
				Source:   ev.Source,
				RRule:    ev.RRule,
			}
			startStr := ev.Start.Format(time.RFC3339)
			ce.Start = &startStr
			if !ev.End.IsZero() {
				endStr := ev.End.Format(time.RFC3339)
				ce.End = &endStr
				ce.DurationMinutes = int(ev.End.Sub(ev.Start) / time.Minute)
			}
			events = append(events, ce)
		}
	}

	// Deadlines (.granit/deadlines.json). Always all-day. Importance
	// is the only non-default field — the web overlay maps it to a
	// distinct red/yellow/purple tone so a critical deadline stands
	// out on the grid even at week-zoom. Status filter excludes
	// "cancelled" (the user explicitly dismissed the date) but keeps
	// "missed" / "met" so a calendar of the past tells the truth.
	for _, d := range deadlines.LoadAll(s.cfg.Vault.Root) {
		if d.Status == string(deadlines.StatusCancelled) {
			continue
		}
		dt, err := time.Parse("2006-01-02", d.Date)
		if err != nil {
			continue
		}
		if dt.Before(from) || dt.After(to) {
			continue
		}
		events = append(events, calendarEvent{
			Type:       "deadline",
			Date:       d.Date,
			Title:      d.Title,
			EventID:    d.ID,
			Importance: deadlines.NormalizeImportance(d.Importance),
		})
	}

	_ = strings.TrimSpace
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"from":   from.Format("2006-01-02"),
		"to":     to.Format("2006-01-02"),
		"events": events,
	})
}
