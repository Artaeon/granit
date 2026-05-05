package serveapi

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const sampleICS = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
UID:1@test
SUMMARY:All-day event
DTSTART;VALUE=DATE:20260315
END:VEVENT
BEGIN:VEVENT
UID:2@test
SUMMARY:Timed event UTC
DTSTART:20260315T140000Z
DTEND:20260315T150000Z
LOCATION:Conference Room
END:VEVENT
BEGIN:VEVENT
UID:3@test
SUMMARY:Recurring daily
DTSTART:20260301T090000Z
DTEND:20260301T093000Z
RRULE:FREQ=DAILY;COUNT=5
END:VEVENT
BEGIN:VEVENT
UID:4@test
SUMMARY:Folded multi-line
DTSTART:20260320T120000Z
DTEND:20260320T130000Z
DESCRIPTION:line one
 continued here
END:VEVENT
END:VCALENDAR
`

func writeTempICS(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.ics")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseICSFile_Basic(t *testing.T) {
	path := writeTempICS(t, sampleICS)
	events, err := parseICSFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}

	// All-day
	if !events[0].AllDay {
		t.Errorf("event 0 should be all-day")
	}
	if events[0].Title != "All-day event" {
		t.Errorf("event 0 title: %s", events[0].Title)
	}

	// Timed UTC
	if events[1].AllDay {
		t.Errorf("event 1 should NOT be all-day")
	}
	if events[1].Location != "Conference Room" {
		t.Errorf("event 1 location: %s", events[1].Location)
	}
	if events[1].End.Sub(events[1].Start) != time.Hour {
		t.Errorf("event 1 duration: %v", events[1].End.Sub(events[1].Start))
	}

	// RRULE preserved on the base
	if events[2].RRule != "FREQ=DAILY;COUNT=5" {
		t.Errorf("event 2 RRULE: %q", events[2].RRule)
	}
}

func TestExpandRRULE_DailyCount(t *testing.T) {
	base := icsEvent{
		Title: "Daily standup",
		Start: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC),
		RRule: "FREQ=DAILY;COUNT=5",
	}
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	got := expandRRULE(base, from, to)
	if len(got) != 5 {
		t.Fatalf("expected 5 instances, got %d", len(got))
	}
	// All instances 30 minutes long
	for i, ev := range got {
		if ev.End.Sub(ev.Start) != 30*time.Minute {
			t.Errorf("instance %d duration wrong: %v", i, ev.End.Sub(ev.Start))
		}
		// RRule cleared on instances
		if ev.RRule != "" {
			t.Errorf("instance %d should have empty RRule, got %q", i, ev.RRule)
		}
	}
}

func TestExpandRRULE_WeeklyInterval(t *testing.T) {
	base := icsEvent{
		Title: "Biweekly meeting",
		Start: time.Date(2026, 3, 2, 14, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 3, 2, 15, 0, 0, 0, time.UTC),
		RRule: "FREQ=WEEKLY;INTERVAL=2;COUNT=4",
	}
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	got := expandRRULE(base, from, to)
	if len(got) != 4 {
		t.Fatalf("expected 4 instances, got %d", len(got))
	}
	// Each instance 14 days apart
	for i := 1; i < len(got); i++ {
		gap := got[i].Start.Sub(got[i-1].Start)
		if gap != 14*24*time.Hour {
			t.Errorf("instance gap between %d and %d wrong: %v", i-1, i, gap)
		}
	}
}

func TestExpandRRULE_WindowFiltering(t *testing.T) {
	// Event that starts before the window and recurs into it
	base := icsEvent{
		Title: "Old recurring",
		Start: time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		RRule: "FREQ=DAILY",
	}
	from := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 7, 23, 59, 59, 0, time.UTC)
	got := expandRRULE(base, from, to)
	// Should yield ~7 instances (one per day in the window)
	if len(got) < 7 || len(got) > 8 {
		t.Errorf("expected ~7 instances in window, got %d", len(got))
	}
	// All inside the window (allowing for the [from-dur, to] interpretation)
	for _, ev := range got {
		if ev.Start.After(to) {
			t.Errorf("instance after window: %v", ev.Start)
		}
	}
}

func TestExpandRRULE_NoRule(t *testing.T) {
	base := icsEvent{
		Title: "Single",
		Start: time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC),
	}
	got := expandRRULE(base, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC))
	if len(got) != 1 {
		t.Errorf("expected 1 instance for no-rule event, got %d", len(got))
	}
}

// TestExpandRRULE_WeeklyBYDAYWorkdays pins the bug fix: a workday
// recurring event (FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR) used to fire only
// on Mondays because BYDAY was silently ignored. Locks: 5 instances per
// ISO week, time-of-day preserved, weekdays in Mon→Fri order.
func TestExpandRRULE_WeeklyBYDAYWorkdays(t *testing.T) {
	// 2026-03-02 is a Monday.
	base := icsEvent{
		Title: "Morning Walk",
		Start: time.Date(2026, 3, 2, 6, 15, 0, 0, time.UTC),
		End:   time.Date(2026, 3, 2, 6, 40, 0, 0, time.UTC),
		RRule: "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR",
	}
	// One full ISO week window (Mon 2026-05-04 → Sun 2026-05-10).
	from := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 10, 23, 59, 59, 0, time.UTC)
	got := expandRRULE(base, from, to)
	if len(got) != 5 {
		t.Fatalf("expected 5 weekday instances, got %d", len(got))
	}
	wantWeekdays := []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}
	for i, ev := range got {
		if ev.Start.Weekday() != wantWeekdays[i] {
			t.Errorf("instance %d weekday = %v, want %v", i, ev.Start.Weekday(), wantWeekdays[i])
		}
		if ev.Start.Hour() != 6 || ev.Start.Minute() != 15 {
			t.Errorf("instance %d time-of-day drifted: %v", i, ev.Start)
		}
	}
}

// TestExpandRRULE_WeeklyBYDAYSingleDay covers BYDAY=SA where DTSTART is
// already on Saturday — the Saturday-only series should fire weekly.
func TestExpandRRULE_WeeklyBYDAYSingleDay(t *testing.T) {
	// 2026-03-07 is a Saturday.
	base := icsEvent{
		Title: "Long Walk",
		Start: time.Date(2026, 3, 7, 6, 15, 0, 0, time.UTC),
		End:   time.Date(2026, 3, 7, 9, 30, 0, 0, time.UTC),
		RRule: "FREQ=WEEKLY;BYDAY=SA",
	}
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 31, 23, 59, 59, 0, time.UTC)
	got := expandRRULE(base, from, to)
	// May 2026 Saturdays: 2, 9, 16, 23, 30 → 5 instances.
	if len(got) != 5 {
		t.Fatalf("expected 5 Saturday instances, got %d", len(got))
	}
	for i, ev := range got {
		if ev.Start.Weekday() != time.Saturday {
			t.Errorf("instance %d weekday = %v, want Saturday", i, ev.Start.Weekday())
		}
	}
}

// TestExpandRRULE_WeeklyBYDAYBeforeStart verifies that BYDAY weekdays
// occurring earlier in DTSTART's own week are NOT emitted. RFC 5545:
// the recurrence set excludes occurrences before DTSTART.
func TestExpandRRULE_WeeklyBYDAYBeforeStart(t *testing.T) {
	// DTSTART is Wednesday 2026-03-04. BYDAY=MO,WE,FR.
	// In the DTSTART week, only WE and FR should fire — Monday is
	// before DTSTART and must be skipped.
	base := icsEvent{
		Title: "T/R/F",
		Start: time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC),
		RRule: "FREQ=WEEKLY;BYDAY=MO,WE,FR;COUNT=5",
	}
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	got := expandRRULE(base, from, to)
	if len(got) != 5 {
		t.Fatalf("expected 5 instances by COUNT, got %d", len(got))
	}
	// Order: Wed 3/4, Fri 3/6, Mon 3/9, Wed 3/11, Fri 3/13.
	want := []struct {
		date time.Time
		wd   time.Weekday
	}{
		{time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC), time.Wednesday},
		{time.Date(2026, 3, 6, 9, 0, 0, 0, time.UTC), time.Friday},
		{time.Date(2026, 3, 9, 9, 0, 0, 0, time.UTC), time.Monday},
		{time.Date(2026, 3, 11, 9, 0, 0, 0, time.UTC), time.Wednesday},
		{time.Date(2026, 3, 13, 9, 0, 0, 0, time.UTC), time.Friday},
	}
	for i, w := range want {
		if !got[i].Start.Equal(w.date) {
			t.Errorf("instance %d start = %v, want %v", i, got[i].Start, w.date)
		}
		if got[i].Start.Weekday() != w.wd {
			t.Errorf("instance %d weekday = %v, want %v", i, got[i].Start.Weekday(), w.wd)
		}
	}
}

// TestParseBYDAY locks the parser shape: numeric prefixes are stripped
// (so "-1SU" still resolves to Sunday) and unknown tokens are dropped
// silently rather than crashing the calendar feed.
func TestParseBYDAY(t *testing.T) {
	cases := []struct {
		in   string
		want []time.Weekday
	}{
		{"", nil},
		{"MO", []time.Weekday{time.Monday}},
		{"MO,TU,WE,TH,FR", []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}},
		{"SA,SU", []time.Weekday{time.Saturday, time.Sunday}},
		{"-1SU", []time.Weekday{time.Sunday}},
		{"2WE", []time.Weekday{time.Wednesday}},
		{"BOGUS,MO", []time.Weekday{time.Monday}},
	}
	for _, c := range cases {
		got := parseBYDAY(c.in)
		if len(got) != len(c.want) {
			t.Errorf("parseBYDAY(%q) len = %d, want %d", c.in, len(got), len(c.want))
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("parseBYDAY(%q)[%d] = %v, want %v", c.in, i, got[i], c.want[i])
			}
		}
	}
}
