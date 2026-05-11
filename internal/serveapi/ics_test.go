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

// TestExpandRRULE_EXDATE pins the EXDATE filter: a recurring event
// with a single excluded occurrence drops that occurrence from the
// emitted instances. Models the common case where a user cancels a
// single instance of a weekly meeting in their calendar app.
func TestExpandRRULE_EXDATE(t *testing.T) {
	// Daily 9am UTC, 5 occurrences, with the third one excluded.
	excluded := time.Date(2026, 3, 3, 9, 0, 0, 0, time.UTC)
	base := icsEvent{
		Title: "Daily standup",
		Start: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC),
		RRule: "FREQ=DAILY;COUNT=5",
		ExDates: map[string]struct{}{
			excluded.UTC().Format("2006-01-02T15:04:05"): {},
		},
	}
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	got := expandRRULE(base, from, to)
	// COUNT=5 minus 1 excluded → 4 emitted.
	if len(got) != 4 {
		t.Fatalf("expected 4 instances after EXDATE filter, got %d", len(got))
	}
	// The excluded date must NOT appear in the output.
	for _, inst := range got {
		if inst.Start.Equal(excluded) {
			t.Errorf("excluded occurrence leaked through: %v", inst.Start)
		}
	}
	// Instances should be cleared of the EXDATE map so downstream
	// JSON serialization stays clean.
	for i, inst := range got {
		if inst.ExDates != nil {
			t.Errorf("instance %d still carries ExDates", i)
		}
	}
}

// TestExpandRRULE_EXDATE_AllDay covers the all-day form: EXDATEs for
// VALUE=DATE events are stored as bare YYYY-MM-DD and matched against
// the occurrence's date format directly (no time component).
func TestExpandRRULE_EXDATE_AllDay(t *testing.T) {
	base := icsEvent{
		Title:  "Holiday",
		Start:  time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		End:    time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC),
		AllDay: true,
		RRule:  "FREQ=DAILY;COUNT=5",
		ExDates: map[string]struct{}{
			"2026-03-04": {},
		},
	}
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	got := expandRRULE(base, from, to)
	if len(got) != 4 {
		t.Fatalf("expected 4 instances, got %d", len(got))
	}
	for _, inst := range got {
		if inst.Start.Format("2006-01-02") == "2026-03-04" {
			t.Errorf("excluded all-day occurrence leaked through")
		}
	}
}

// TestParseICSFile_EXDATE verifies that an EXDATE line in the ICS
// source parses into the icsEvent's ExDates map in the expected
// canonical form.
func TestParseICSFile_EXDATE(t *testing.T) {
	const ics = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
UID:r@test
SUMMARY:Recurring with skip
DTSTART:20260301T090000Z
DTEND:20260301T093000Z
RRULE:FREQ=DAILY;COUNT=5
EXDATE:20260303T090000Z
END:VEVENT
END:VCALENDAR
`
	path := writeTempICS(t, ics)
	events, err := parseICSFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if len(events[0].ExDates) != 1 {
		t.Fatalf("expected 1 EXDATE, got %d", len(events[0].ExDates))
	}
	if _, ok := events[0].ExDates["2026-03-03T09:00:00"]; !ok {
		t.Errorf("EXDATE not stored in expected key shape; got %v", events[0].ExDates)
	}
}

// TestExpandRRULE_BoundaryDay pins the timed-vs-all-day boundary
// symmetry. Both branches use exclusive upper-bound semantics: an
// instance whose Start equals `to` (the rangeEnd, typically next-day
// midnight) must NOT be emitted, mirroring expandAllDayDates'
// `d.Before(end)` clamp. Without symmetry the timed branch could leak
// a next-day-midnight occurrence into the prior day's render and the
// all-day branch wouldn't.
func TestExpandRRULE_BoundaryDay(t *testing.T) {
	// Daily 00:00 UTC starting 2026-05-01. Window: 2026-05-01 to
	// 2026-05-03 (rangeEnd = 2026-05-04 00:00 UTC, exclusive).
	base := icsEvent{
		Title: "Midnight tick",
		Start: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 5, 1, 0, 30, 0, 0, time.UTC),
		RRule: "FREQ=DAILY",
	}
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	rangeEnd := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	got := expandRRULE(base, from, rangeEnd)
	// Expected: 2026-05-01, 02, 03 — three instances. NOT 05-04 (==to).
	if len(got) != 3 {
		t.Fatalf("expected 3 instances (May 1/2/3), got %d:\n%v", len(got), starts(got))
	}
	for _, ev := range got {
		if !ev.Start.Before(rangeEnd) {
			t.Errorf("instance at %v leaked past rangeEnd %v", ev.Start, rangeEnd)
		}
	}

	// All-day mirror: same window, daily all-day rule. Counterpart
	// branch (expandAllDayDates) is what handlers_calendar.go uses
	// to expand multi-day all-day events; its `d.Before(rangeEnd)`
	// is the contract this test pins to the timed side.
	dates := expandAllDayDates(
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC),
		from, rangeEnd,
	)
	if len(dates) != 3 || dates[len(dates)-1] != "2026-05-03" {
		t.Errorf("all-day boundary: got %v, want 3 days ending 2026-05-03", dates)
	}
}

// TestExpandRRULE_BoundaryWeeklyBYDAY: the WEEKLY+BYDAY branch has its
// own outer-loop break — verify it also stops short of `to` rather
// than leaking a boundary-day instance.
func TestExpandRRULE_BoundaryWeeklyBYDAY(t *testing.T) {
	// 2026-05-04 is a Monday. Weekly Mon/Wed/Fri rule.
	base := icsEvent{
		Title: "MWF",
		Start: time.Date(2026, 5, 4, 8, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 5, 4, 9, 0, 0, 0, time.UTC),
		RRule: "FREQ=WEEKLY;BYDAY=MO,WE,FR",
	}
	// rangeEnd EXACTLY on a Monday — that Monday must not appear.
	from := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	rangeEnd := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC) // next Monday 00:00
	got := expandRRULE(base, from, rangeEnd)
	// Expected: Mon 5/4, Wed 5/6, Fri 5/8 — three. NOT 5/11.
	if len(got) != 3 {
		t.Fatalf("expected 3 (Mon/Wed/Fri of week 1), got %d:\n%v", len(got), starts(got))
	}
	for _, ev := range got {
		if !ev.Start.Before(rangeEnd) {
			t.Errorf("instance at %v leaked past rangeEnd %v", ev.Start, rangeEnd)
		}
	}
}

// starts is a tiny helper for failure-message readability — formats
// the start times of the matched occurrences as a slice of strings.
func starts(evs []icsEvent) []string {
	out := make([]string, len(evs))
	for i, ev := range evs {
		out[i] = ev.Start.Format(time.RFC3339)
	}
	return out
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

// parseICSTime + the icsTimeIsFloating helper are the load-bearing
// pieces of the "calendar times are sometimes wrong" fix. The bug
// repro: a server running in UTC parses a floating ICS time
// "20260315T140000" as time.Local (= UTC since server tz is UTC),
// then emits as RFC3339 → "2026-03-15T14:00:00Z". A client in
// Europe/Berlin (UTC+1 in winter) renders that as 15:00 — drifted
// by the client's offset. Floating times must round-trip wall-clock
// numbers, NOT instants.

func TestICSTimeIsFloating(t *testing.T) {
	cases := []struct {
		value, tzid string
		want        bool
	}{
		{"20260315T140000", "", true},               // no Z + no TZID → floating
		{"20260315T140000Z", "", false},             // UTC instant
		{"20260315T140000", "Europe/Berlin", false}, // TZID-qualified
		{"20260315", "", false},                     // all-day (date) — not floating in the time sense
		{"", "", false},
	}
	for _, c := range cases {
		got := icsTimeIsFloating(c.value, c.tzid)
		if got != c.want {
			t.Errorf("icsTimeIsFloating(%q, %q) = %v, want %v", c.value, c.tzid, got, c.want)
		}
	}
}

func TestParseICSTime_FloatingPreservesWallClock(t *testing.T) {
	// Floating times must parse to the exact wall-clock numbers
	// regardless of the server's local zone. We assert the Y/M/D/h/m
	// fields rather than the absolute instant — the absolute is
	// meaningless for floating times by definition.
	t0, _, ok := parseICSTime("20260315T140000", "")
	if !ok {
		t.Fatal("parseICSTime returned !ok")
	}
	if t0.Year() != 2026 || t0.Month() != 3 || t0.Day() != 15 || t0.Hour() != 14 || t0.Minute() != 0 {
		t.Errorf("floating time mis-parsed: %v", t0)
	}
	// The instant should be in UTC for stability. A previous bug
	// used time.Local here and the emitted instant drifted with the
	// server timezone — pin it.
	if t0.Location() != time.UTC {
		t.Errorf("floating time should land in UTC, got %v", t0.Location())
	}
}

func TestParseICSTime_TZIDKnown(t *testing.T) {
	// IANA TZID resolves correctly. Use Europe/Berlin since Granit's
	// primary user is in CET; 14:00 Berlin in March (CET, before DST)
	// is 13:00 UTC.
	t0, _, ok := parseICSTime("20260315T140000", "Europe/Berlin")
	if !ok {
		t.Fatal("parseICSTime returned !ok")
	}
	utc := t0.UTC()
	if utc.Hour() != 13 || utc.Minute() != 0 {
		t.Errorf("Europe/Berlin 14:00 → UTC %02d:%02d, want 13:00", utc.Hour(), utc.Minute())
	}
}

func TestParseICSTime_TZIDWindowsAlias(t *testing.T) {
	// Outlook/Exchange exports use Windows-style TZIDs; the fallback
	// map should resolve them. "Central European Standard Time" →
	// Europe/Warsaw (CET, same offset as Berlin for our purposes).
	t0, _, ok := parseICSTime("20260315T140000", "Central European Standard Time")
	if !ok {
		t.Fatal("parseICSTime returned !ok")
	}
	utc := t0.UTC()
	if utc.Hour() != 13 || utc.Minute() != 0 {
		t.Errorf("Windows CET 14:00 → UTC %02d:%02d, want 13:00", utc.Hour(), utc.Minute())
	}
}

func TestParseICSTime_TZIDUnknown_FallsBackToUTC(t *testing.T) {
	// An unrecognised TZID (typo / obscure zone) must not silently
	// drift to server-local. Falling back to UTC preserves the wall-
	// clock numbers, which the user can spot and correct rather than
	// having an invisible shift creep in.
	t0, _, ok := parseICSTime("20260315T140000", "Not/A/Zone")
	if !ok {
		t.Fatal("parseICSTime returned !ok")
	}
	if t0.Hour() != 14 || t0.Location() != time.UTC {
		t.Errorf("unknown TZID should preserve 14:00 in UTC, got %v in %v", t0, t0.Location())
	}
}

func TestExpandRRULE_PreservesSourceWritableFloating(t *testing.T) {
	// These three icsEvent fields are quietly load-bearing for the
	// calendar UX:
	//   - Source drives the per-calendar colouring on the grid.
	//   - Writable drives the editable-flag the frontend uses to
	//     show/hide drag handles. A regression that drops it leaks
	//     "this event is editable" on read-only sources, leading
	//     straight back to the "event not found" UX bug.
	//   - Floating tells the feed which times to emit without an
	//     offset so wall-clock display works in any timezone. A
	//     regression here re-introduces the {server-tz - client-tz}
	//     drift on every recurring floating event.
	// The expansion is a value-copy today (`inst := ev`), but pinning
	// it explicitly catches any future refactor that builds the
	// instance struct field-by-field and forgets one of these.
	base := icsEvent{
		Title:    "Weekly meet",
		Start:    time.Date(2026, 5, 11, 14, 0, 0, 0, time.UTC),
		End:      time.Date(2026, 5, 11, 15, 0, 0, 0, time.UTC),
		UID:      "weekly@cal",
		RRule:    "FREQ=WEEKLY;COUNT=4",
		Source:   "work.ics",
		Writable: true,
		Floating: true,
	}
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	occs := expandRRULE(base, from, to)
	if len(occs) == 0 {
		t.Fatal("expected at least one occurrence")
	}
	for i, occ := range occs {
		if occ.Source != "work.ics" {
			t.Errorf("occ[%d].Source = %q, want work.ics", i, occ.Source)
		}
		if !occ.Writable {
			t.Errorf("occ[%d].Writable lost — frontend would mis-classify as read-only", i)
		}
		if !occ.Floating {
			t.Errorf("occ[%d].Floating lost — tz drift would re-appear on this occurrence", i)
		}
		if occ.UID != "weekly@cal" {
			t.Errorf("occ[%d].UID = %q, want weekly@cal", i, occ.UID)
		}
	}
}

func TestParseICSFile_TagsFloatingFlag(t *testing.T) {
	// End-to-end: a file with a mix of UTC/zoned/floating timestamps
	// gets correctly tagged so the calendar feed knows which to emit
	// with an offset vs as floating-ISO.
	const mixed = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
UID:utc@test
SUMMARY:UTC event
DTSTART:20260315T140000Z
DTEND:20260315T150000Z
END:VEVENT
BEGIN:VEVENT
UID:zoned@test
SUMMARY:Berlin event
DTSTART;TZID=Europe/Berlin:20260315T160000
DTEND;TZID=Europe/Berlin:20260315T170000
END:VEVENT
BEGIN:VEVENT
UID:floating@test
SUMMARY:Floating event
DTSTART:20260315T180000
DTEND:20260315T190000
END:VEVENT
END:VCALENDAR
`
	path := writeTempICS(t, mixed)
	events, err := parseICSFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	flagByUID := map[string]bool{}
	for _, e := range events {
		flagByUID[e.UID] = e.Floating
	}
	if flagByUID["utc@test"] {
		t.Errorf("UTC event should NOT be flagged floating")
	}
	if flagByUID["zoned@test"] {
		t.Errorf("zoned (TZID) event should NOT be flagged floating")
	}
	if !flagByUID["floating@test"] {
		t.Errorf("no-Z no-TZID event MUST be flagged floating")
	}
}
