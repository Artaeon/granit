package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Native event data model
// ---------------------------------------------------------------------------

// NativeEvent is a calendar event stored in .granit/events.json.
type NativeEvent struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Date        string `json:"date"`       // YYYY-MM-DD
	StartTime   string `json:"start_time"` // HH:MM (empty for all-day)
	EndTime     string `json:"end_time"`   // HH:MM
	Location    string `json:"location,omitempty"`
	Color       string `json:"color,omitempty"`      // red, blue, green, yellow, mauve, teal
	Recurrence  string `json:"recurrence,omitempty"` // daily, weekly, monthly, yearly
	AllDay      bool   `json:"all_day,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// Duration returns the event duration in minutes.
func (e NativeEvent) Duration() int {
	if e.StartTime == "" || e.EndTime == "" {
		return 60
	}
	sh, sm, eh, em := 0, 0, 0, 0
	_, _ = fmt.Sscanf(e.StartTime, "%d:%d", &sh, &sm)
	_, _ = fmt.Sscanf(e.EndTime, "%d:%d", &eh, &em)
	return (eh*60 + em) - (sh*60 + sm)
}

// ToCalendarEvent converts to the display format used by the calendar.
func (e NativeEvent) ToCalendarEvent() CalendarEvent {
	t, _ := time.Parse("2006-01-02", e.Date)
	if e.StartTime != "" {
		h, m := 0, 0
		_, _ = fmt.Sscanf(e.StartTime, "%d:%d", &h, &m)
		t = time.Date(t.Year(), t.Month(), t.Day(), h, m, 0, 0, time.Local)
	}
	endT := t.Add(time.Duration(e.Duration()) * time.Minute)
	return CalendarEvent{
		Title:       e.Title,
		Date:        t,
		EndDate:     endT,
		Location:    e.Location,
		Description: e.Description,
		AllDay:      e.AllDay,
		ID:          e.ID,
		Color:       e.Color,
		Recurrence:  e.Recurrence,
	}
}

// ---------------------------------------------------------------------------
// Event store
// ---------------------------------------------------------------------------

// EventStore manages native events persisted in .granit/events.json.
type EventStore struct {
	vaultRoot string
	events    []NativeEvent
}

func NewEventStore(vaultRoot string) *EventStore {
	es := &EventStore{vaultRoot: vaultRoot}
	es.load()
	return es
}

func (es *EventStore) path() string {
	return filepath.Join(es.vaultRoot, ".granit", "events.json")
}

func (es *EventStore) load() {
	es.events = nil
	data, err := os.ReadFile(es.path())
	if err != nil {
		return
	}
	if err := json.Unmarshal(data, &es.events); err != nil {
		es.events = []NativeEvent{}
	}
}

func (es *EventStore) save() {
	dir := filepath.Join(es.vaultRoot, ".granit")
	_ = os.MkdirAll(dir, 0755)
	data, err := json.MarshalIndent(es.events, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(es.path(), data, 0o600)
}

func (es *EventStore) nextID() string {
	max := 0
	for _, e := range es.events {
		if len(e.ID) > 1 && e.ID[0] == 'E' {
			n := 0
			_, _ = fmt.Sscanf(e.ID[1:], "%d", &n)
			if n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("E%03d", max+1)
}

// Add creates a new event and returns its ID.
func (es *EventStore) Add(title, date, startTime, endTime, location, description, color, recurrence string, allDay bool) string {
	id := es.nextID()
	es.events = append(es.events, NativeEvent{
		ID:          id,
		Title:       title,
		Date:        date,
		StartTime:   startTime,
		EndTime:     endTime,
		Location:    location,
		Description: description,
		Color:       color,
		Recurrence:  recurrence,
		AllDay:      allDay,
		CreatedAt:   time.Now().Format("2006-01-02"),
	})
	es.save()
	return id
}

// Update modifies an existing event.
func (es *EventStore) Update(id string, title, date, startTime, endTime, location, description, color, recurrence string, allDay bool) bool {
	for i, e := range es.events {
		if e.ID == id {
			es.events[i].Title = title
			es.events[i].Date = date
			es.events[i].StartTime = startTime
			es.events[i].EndTime = endTime
			es.events[i].Location = location
			es.events[i].Description = description
			es.events[i].Color = color
			es.events[i].Recurrence = recurrence
			es.events[i].AllDay = allDay
			es.save()
			return true
		}
	}
	return false
}

// Delete removes an event by ID.
func (es *EventStore) Delete(id string) bool {
	for i, e := range es.events {
		if e.ID == id {
			es.events = append(es.events[:i], es.events[i+1:]...)
			es.save()
			return true
		}
	}
	return false
}

// Get returns an event by ID.
func (es *EventStore) Get(id string) *NativeEvent {
	for i, e := range es.events {
		if e.ID == id {
			return &es.events[i]
		}
	}
	return nil
}

// EventsForDate returns all events on a given date (including recurring).
func (es *EventStore) EventsForDate(dateStr string) []NativeEvent {
	target, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return nil
	}

	var result []NativeEvent
	for _, e := range es.events {
		eDate, err := time.Parse("2006-01-02", e.Date)
		if err != nil {
			continue
		}

		match := false
		if e.Date == dateStr {
			match = true
		} else if e.Recurrence != "" && !target.Before(eDate) {
			switch e.Recurrence {
			case "daily":
				match = true
			case "weekly":
				match = eDate.Weekday() == target.Weekday()
			case "monthly":
				match = eDate.Day() == target.Day()
			case "yearly":
				match = eDate.Month() == target.Month() && eDate.Day() == target.Day()
			}
		}

		if match {
			// Return a copy with the target date
			ev := e
			ev.Date = dateStr
			result = append(result, ev)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].StartTime < result[j].StartTime
	})
	return result
}

// AllEvents returns all events sorted by date.
func (es *EventStore) AllEvents() []NativeEvent {
	sorted := make([]NativeEvent, len(es.events))
	copy(sorted, es.events)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Date == sorted[j].Date {
			return sorted[i].StartTime < sorted[j].StartTime
		}
		return sorted[i].Date < sorted[j].Date
	})
	return sorted
}

// ToCalendarEvents converts all native events to CalendarEvent format
// for the given date range, expanding recurring events.
func (es *EventStore) ToCalendarEvents(startDate, endDate string) []CalendarEvent {
	start, _ := time.Parse("2006-01-02", startDate)
	end, _ := time.Parse("2006-01-02", endDate)
	if start.IsZero() || end.IsZero() {
		return nil
	}

	var result []CalendarEvent
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		for _, e := range es.EventsForDate(dateStr) {
			result = append(result, e.ToCalendarEvent())
		}
	}
	return result
}

// Search finds events matching a query string.
func (es *EventStore) Search(query string) []NativeEvent {
	q := strings.ToLower(query)
	var result []NativeEvent
	for _, e := range es.events {
		if strings.Contains(strings.ToLower(e.Title), q) ||
			strings.Contains(strings.ToLower(e.Description), q) ||
			strings.Contains(strings.ToLower(e.Location), q) {
			result = append(result, e)
		}
	}
	return result
}
