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
	// Editable is false for ICS events whose source file lives in a
	// read-only location (vault root or <vault>/Calendars/ — only
	// <vault>/calendars/ is writable). The frontend hides edit/drag
	// affordances when this is false so the user doesn't waste a click
	// on something the server is going to bounce with 403. Native
	// events and tasks are always editable through their own endpoints,
	// so the field is only emitted for type="ics_event".
	Editable *bool `json:"editable,omitempty"`
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
	// OverrideKey is set on a recurring-event occurrence when the
	// user has authored a per-instance override (Event.Overrides
	// map hit). Carries the canonical key into Event.Overrides so
	// the frontend can offer a 'reset this occurrence' action that
	// posts an empty override at the same key. Empty for plain
	// occurrences (no override applied).
	OverrideKey string `json:"override_key,omitempty"`
}

// overrideKey is the canonical key shape into Event.Overrides. Mirrors
// the EXDATE format from ics.go's isExcluded so the override and skip
// paths agree on what "this Tuesday's 9am occurrence" identifies as:
// YYYY-MM-DD for all-day, YYYY-MM-DDTHH:MM:SS in UTC for timed.
func overrideKey(start time.Time, allDay bool) string {
	if allDay {
		return start.Format("2006-01-02")
	}
	return start.UTC().Format("2006-01-02T15:04:05")
}

// applyTimedOverride mutates the (start, end) pair of one occurrence
// according to the override. The contract:
//   - If ovr.Date is set, the occurrence moves to that calendar day.
//     start_time / end_time stay at the same wall-clock-on-the-new-date
//     unless overridden by ovr.StartTime / ovr.EndTime.
//   - If ovr.StartTime / ovr.EndTime are set, the wall-clock time on
//     the (possibly shifted) day is replaced. When only StartTime is
//     set, the duration is preserved (end shifts by the same delta) —
//     matches the drag-move UX where the user picks a new time and
//     expects the event to keep its length.
//
// Times are interpreted in time.UTC, but conceptually they are FLOATING
// wall-clock numbers — the events.json schema stores HH:MM with no
// zone, so we treat the digits as zone-free. UTC is just the carrier
// frame: it has no DST and a stable offset, so the round-trip
// digits→time.Time→digits is lossless across server reboots, server
// timezone changes, and client timezones. The previous code used
// time.Local, which silently re-anchored those wall-clock numbers to
// the SERVER's zone — on a UTC server with a UTC+2 client, an event
// the user typed as 08:00 ended up rendering at 10:00 because the
// server emitted "08:00Z" and the browser added the +2hr offset.
func applyTimedOverride(start, end time.Time, ovr granitmeta.EventOverride) (time.Time, time.Time) {
	dur := end.Sub(start)
	loc := time.UTC
	// Anchor day: the override's Date wins; otherwise the original
	// occurrence's UTC date. Time-of-day is derived next.
	yyyy, mm, dd := start.In(loc).Date()
	if ovr.Date != "" {
		if d, err := time.ParseInLocation("2006-01-02", ovr.Date, loc); err == nil {
			yyyy, mm, dd = d.Year(), d.Month(), d.Day()
		}
	}
	// Wall-clock start: ovr.StartTime wins, else original wall clock.
	hh, mi := start.In(loc).Hour(), start.In(loc).Minute()
	if ovr.StartTime != "" {
		if t, err := time.ParseInLocation("15:04", ovr.StartTime, loc); err == nil {
			hh, mi = t.Hour(), t.Minute()
		}
	}
	newStart := time.Date(yyyy, mm, dd, hh, mi, 0, 0, loc)
	newEnd := newStart.Add(dur)
	if ovr.EndTime != "" {
		if t, err := time.ParseInLocation("15:04", ovr.EndTime, loc); err == nil {
			newEnd = time.Date(yyyy, mm, dd, t.Hour(), t.Minute(), 0, 0, loc)
		}
	}
	return newStart, newEnd
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
		// Default to "today" in UTC so the empty-query window aligns
		// with the UTC parse used for the events.json wall-clock
		// numbers below — keeping the comparison frame consistent
		// avoids edge-of-day drift on non-UTC servers.
		now := time.Now().UTC()
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, err
	}
	// Anchor the window in UTC. events.json stores HH:MM as floating
	// wall-clock and we parse those numbers in UTC (see
	// applyTimedOverride / handleCalendar) — the from/to bounds need
	// the same frame so a server in a non-UTC zone doesn't shift the
	// requested calendar day under the client's feet.
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
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
				// Parse-in-UTC: events.json stores HH:MM as zone-free
				// wall-clock numbers. We carry them in UTC time.Time
				// values (UTC has no DST, stable offset → lossless
				// digit round-trip), then emit a floating ISO string
				// so the browser anchors them to the CLIENT's zone.
				// The previous time.Local parse silently re-zoned the
				// digits to the server's TZ, which on a UTC server with
				// a UTC+2 client materialised as a clean +2hr drift
				// (08:00 typed → "08:00Z" emitted → 10:00 displayed).
				if startT, err := time.ParseInLocation("2006-01-02 15:04", ev.Date+" "+ev.StartTime, time.UTC); err == nil {
					seed.Start = startT
					if ev.EndTime != "" {
						if endT, err := time.ParseInLocation("2006-01-02 15:04", ev.Date+" "+ev.EndTime, time.UTC); err == nil {
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
				// Per-occurrence override lookup. Key by the
				// ORIGINAL (untransformed) occurrence's UTC stamp
				// so the override survives even after the user
				// shifts the day/time — the override moves with
				// the series anchor, not the displayed cell. This
				// matches the EXDATE key shape exactly (see
				// isExcluded in ics.go).
				ovrKey := overrideKey(occ.Start, occ.AllDay)
				ovr, hasOvr := ev.Overrides[ovrKey]
				occDate := occ.Start.Format("2006-01-02")
				if occ.AllDay {
					if occ.Start.Before(from) || !occ.Start.Before(rangeEndForNative) {
						continue
					}
					title := ev.Title
					color := ev.Color
					location := ev.Location
					if hasOvr {
						if ovr.Title != "" {
							title = ovr.Title
						}
						if ovr.Color != "" {
							color = ovr.Color
						}
						if ovr.Location != "" {
							location = ovr.Location
						}
						if ovr.Date != "" {
							occDate = ovr.Date
						}
					}
					ce := calendarEvent{
						Type:      "event",
						Title:     title,
						Date:      occDate,
						EventID:   ev.ID,
						Color:     color,
						Location:  location,
						RRule:     ev.RRule,
						ProjectID: ev.ProjectID,
					}
					if hasOvr {
						ce.OverrideKey = ovrKey
					}
					events = append(events, ce)
					continue
				}
				if occ.Start.Before(from) || !occ.Start.Before(rangeEndForNative) {
					continue
				}
				// Apply timed override: shift start/end to the
				// override's wall-clock values, optionally on the
				// override's date. Falls back to the expanded
				// occurrence values when fields are empty.
				occStart := occ.Start
				occEnd := occ.End
				title := ev.Title
				color := ev.Color
				location := ev.Location
				if hasOvr {
					if ovr.Title != "" {
						title = ovr.Title
					}
					if ovr.Color != "" {
						color = ovr.Color
					}
					if ovr.Location != "" {
						location = ovr.Location
					}
					occStart, occEnd = applyTimedOverride(occStart, occEnd, ovr)
					// Re-window check after override shift: a
					// moved occurrence might now fall outside the
					// requested window even if its anchor was in.
					if occStart.Before(from) || !occStart.Before(rangeEndForNative) {
						continue
					}
				}
				ce := calendarEvent{
					Type:      "event",
					Title:     title,
					EventID:   ev.ID,
					Color:     color,
					Location:  location,
					RRule:     ev.RRule,
					ProjectID: ev.ProjectID,
				}
				if hasOvr {
					ce.OverrideKey = ovrKey
				}
				// Floating ISO emit (no Z, no offset). The browser
				// parses "2006-01-02T15:04:05" as the CLIENT's local
				// zone, so the wall-clock numbers the user typed
				// (events.json HH:MM) round-trip cleanly to the grid
				// regardless of server or client timezone.
				// time.RFC3339 would attach the parser's zone offset
				// to the string — on a UTC server that's "Z", which
				// the browser then converts back into local time and
				// adds the offset, producing the +2hr drift the user
				// reported. ICS events keep their RFC3339 emit (see
				// the ICS branch below) because they carry real
				// instants with their own TZID semantics.
				sStr := occStart.Format("2006-01-02T15:04:05")
				ce.Start = &sStr
				if !occEnd.IsZero() {
					eStr := occEnd.Format("2006-01-02T15:04:05")
					ce.End = &eStr
					ce.DurationMinutes = int(occEnd.Sub(occStart).Minutes())
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
					editable := ev.Writable
					events = append(events, calendarEvent{
						Type:     "ics_event",
						Title:    ev.Title,
						Location: ev.Location,
						EventID:  ev.UID,
						Color:    "cyan",
						Source:   ev.Source,
						Date:     dayISO,
						RRule:    ev.RRule,
						Editable: &editable,
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
			editable := ev.Writable
			ce := calendarEvent{
				Type:     "ics_event",
				Title:    ev.Title,
				Location: ev.Location,
				EventID:  ev.UID,
				Color:    "cyan",
				Source:   ev.Source,
				RRule:    ev.RRule,
				Editable: &editable,
			}
			// Floating ICS times (no Z, no TZID) emit as RFC3339-style
			// WITHOUT an offset so the browser's Date parser uses its
			// own local zone for display. RFC3339 attaches the parser's
			// zone (server-local), which the browser then converts again
			// — producing a {server-tz - client-tz} drift on every
			// occurrence. Zoned/UTC times keep their RFC3339 emit
			// because they carry real instants and the offset is correct.
			var startStr string
			if ev.Floating {
				startStr = ev.Start.Format("2006-01-02T15:04:05")
			} else {
				startStr = ev.Start.Format(time.RFC3339)
			}
			ce.Start = &startStr
			if !ev.End.IsZero() {
				var endStr string
				if ev.Floating {
					endStr = ev.End.Format("2006-01-02T15:04:05")
				} else {
					endStr = ev.End.Format(time.RFC3339)
				}
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
