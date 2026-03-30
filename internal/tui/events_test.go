package tui

import "testing"

func TestNativeEventDuration(t *testing.T) {
	e := NativeEvent{StartTime: "09:00", EndTime: "10:30"}
	if e.Duration() != 90 {
		t.Errorf("Duration() = %d, want 90", e.Duration())
	}
}

func TestNativeEventDuration_NoTime(t *testing.T) {
	e := NativeEvent{}
	if e.Duration() != 60 {
		t.Errorf("Duration() with no times = %d, want 60", e.Duration())
	}
}

func TestEventStoreNextID(t *testing.T) {
	es := &EventStore{
		events: []NativeEvent{{ID: "E001"}, {ID: "E005"}},
	}
	got := es.nextID()
	if got != "E006" {
		t.Errorf("nextID() = %q, want %q", got, "E006")
	}
}

func TestEventStoreNextID_Empty(t *testing.T) {
	es := &EventStore{}
	got := es.nextID()
	if got != "E001" {
		t.Errorf("nextID() = %q, want %q", got, "E001")
	}
}

func TestEventsForDate_Simple(t *testing.T) {
	es := &EventStore{
		events: []NativeEvent{
			{ID: "E001", Title: "Meeting", Date: "2026-03-30", StartTime: "10:00"},
			{ID: "E002", Title: "Lunch", Date: "2026-03-31", StartTime: "12:00"},
		},
	}
	got := es.EventsForDate("2026-03-30")
	if len(got) != 1 || got[0].Title != "Meeting" {
		t.Errorf("EventsForDate = %v, want 1 event 'Meeting'", got)
	}
}

func TestEventsForDate_Weekly(t *testing.T) {
	es := &EventStore{
		events: []NativeEvent{
			{ID: "E001", Title: "Standup", Date: "2026-03-30", StartTime: "09:00", Recurrence: "weekly"},
		},
	}
	// 2026-03-30 is Monday, next Monday is 2026-04-06
	got := es.EventsForDate("2026-04-06")
	if len(got) != 1 {
		t.Errorf("Weekly recurrence should match next week, got %d events", len(got))
	}
}

func TestEventsForDate_Daily(t *testing.T) {
	es := &EventStore{
		events: []NativeEvent{
			{ID: "E001", Title: "Standup", Date: "2026-03-01", Recurrence: "daily"},
		},
	}
	got := es.EventsForDate("2026-03-30")
	if len(got) != 1 {
		t.Errorf("Daily recurrence should match any date, got %d events", len(got))
	}
}

func TestEventSearch(t *testing.T) {
	es := &EventStore{
		events: []NativeEvent{
			{ID: "E001", Title: "Team Meeting", Location: "Room 42"},
			{ID: "E002", Title: "Lunch Break"},
		},
	}
	got := es.Search("meeting")
	if len(got) != 1 || got[0].ID != "E001" {
		t.Errorf("Search('meeting') = %v, want E001", got)
	}
	got2 := es.Search("room")
	if len(got2) != 1 {
		t.Errorf("Search('room') should match location, got %d", len(got2))
	}
}
