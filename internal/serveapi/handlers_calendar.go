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

	// Granit events.json (calendar events)
	if granitEvents, err := granitmeta.ReadEvents(s.cfg.Vault.Root); err == nil {
		for _, ev := range granitEvents {
			d, err := time.Parse("2006-01-02", ev.Date)
			if err != nil {
				continue
			}
			if d.Before(from) || d.After(to) {
				continue
			}
			ce := calendarEvent{
				Type:     "event",
				Title:    ev.Title,
				Date:     ev.Date,
				EventID:  ev.ID,
				Color:    ev.Color,
				Location: ev.Location,
			}
			if ev.StartTime != "" {
				if startT, err := time.ParseInLocation("2006-01-02 15:04", ev.Date+" "+ev.StartTime, time.Local); err == nil {
					sStr := startT.Format(time.RFC3339)
					ce.Start = &sStr
					ce.Date = ""
					if ev.EndTime != "" {
						if endT, err := time.ParseInLocation("2006-01-02 15:04", ev.Date+" "+ev.EndTime, time.Local); err == nil {
							eStr := endT.Format(time.RFC3339)
							ce.End = &eStr
							ce.DurationMinutes = int(endT.Sub(startT).Minutes())
						}
					}
				}
			}
			events = append(events, ce)
		}
	}

	// Tasks: due dates (all-day) + scheduled (timed)
	all := s.cfg.TaskStore.All()
	fromDate := from.Format("2006-01-02")
	toDate := to.Format("2006-01-02")
	for _, t := range all {
		if t.DueDate != "" && t.DueDate >= fromDate && t.DueDate <= toDate {
			events = append(events, calendarEvent{
				Type:     "task_due",
				Date:     t.DueDate,
				Title:    t.Text,
				NotePath: t.NotePath,
				TaskID:   t.ID,
				Done:     t.Done,
				Priority: t.Priority,
			})
		}
		if t.ScheduledStart != nil {
			st := *t.ScheduledStart
			if st.Before(from) || st.After(to.Add(24*time.Hour)) {
				continue
			}
			startStr := st.Format(time.RFC3339)
			ce := calendarEvent{
				Type:     "task_scheduled",
				Start:    &startStr,
				Title:    t.Text,
				NotePath: t.NotePath,
				TaskID:   t.ID,
				Done:     t.Done,
				Priority: t.Priority,
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
	for _, base := range icsScan(s.cfg.Vault.Root, gcfg.DisabledCalendars) {
		for _, ev := range expandRRULE(base, from, to.Add(24*time.Hour)) {
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
			}
			if ev.AllDay {
				ce.Date = ev.Start.Format("2006-01-02")
			} else {
				startStr := ev.Start.Format(time.RFC3339)
				ce.Start = &startStr
				if !ev.End.IsZero() {
					endStr := ev.End.Format(time.RFC3339)
					ce.End = &endStr
					ce.DurationMinutes = int(ev.End.Sub(ev.Start) / time.Minute)
				}
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
