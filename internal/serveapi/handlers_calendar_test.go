package serveapi

import (
	"reflect"
	"testing"
	"time"

	"github.com/artaeon/granit/internal/granitmeta"
)

// TestExpandAllDayDates pins the multi-day all-day expansion contract.
// Locks: inclusive start, exclusive end (ICS DTEND semantics), the
// no-end fallback to a single day, and the [from, rangeEnd) window
// clamp so events extending outside the requested calendar window are
// trimmed correctly.
func TestExpandAllDayDates(t *testing.T) {
	d := func(year int, month time.Month, day int) time.Time {
		return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	}
	cases := []struct {
		name        string
		start, end  time.Time
		from, until time.Time
		want        []string
	}{
		{
			name:  "ten-day vacation fully inside window",
			start: d(2026, 8, 1), end: d(2026, 8, 11), // ICS DTEND exclusive → 10 days
			from: d(2026, 7, 1), until: d(2026, 9, 1),
			want: []string{
				"2026-08-01", "2026-08-02", "2026-08-03", "2026-08-04", "2026-08-05",
				"2026-08-06", "2026-08-07", "2026-08-08", "2026-08-09", "2026-08-10",
			},
		},
		{
			name:  "single-day event with explicit DTEND = start + 24h",
			start: d(2026, 8, 1), end: d(2026, 8, 2),
			from: d(2026, 7, 1), until: d(2026, 9, 1),
			want: []string{"2026-08-01"},
		},
		{
			name:  "no DTEND falls back to one day",
			start: d(2026, 8, 1), end: time.Time{},
			from: d(2026, 7, 1), until: d(2026, 9, 1),
			want: []string{"2026-08-01"},
		},
		{
			name:  "DTEND <= start defends against malformed ICS",
			start: d(2026, 8, 1), end: d(2026, 7, 31),
			from: d(2026, 7, 1), until: d(2026, 9, 1),
			want: []string{"2026-08-01"},
		},
		{
			name:  "trip spans the request window — left clamp",
			start: d(2026, 7, 28), end: d(2026, 8, 5),
			from: d(2026, 8, 1), until: d(2026, 9, 1),
			want: []string{"2026-08-01", "2026-08-02", "2026-08-03", "2026-08-04"},
		},
		{
			name:  "trip spans the request window — right clamp",
			start: d(2026, 7, 28), end: d(2026, 8, 5),
			from: d(2026, 7, 1), until: d(2026, 8, 1),
			want: []string{"2026-07-28", "2026-07-29", "2026-07-30", "2026-07-31"},
		},
		{
			name:  "trip entirely outside window",
			start: d(2026, 1, 1), end: d(2026, 1, 5),
			from: d(2026, 8, 1), until: d(2026, 9, 1),
			want: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := expandAllDayDates(c.start, c.end, c.from, c.until)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v\nwant %v", got, c.want)
			}
		})
	}
}

// TestOverrideKey pins the canonical key shape for Event.Overrides
// against the EXDATE format used by isExcluded — both are populated
// from the same UTC-stamp shape so the override and skip paths can
// safely point at the same recurrence anchor without translation.
func TestOverrideKey(t *testing.T) {
	timed := time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)
	if got := overrideKey(timed, false); got != "2026-03-04T09:00:00" {
		t.Errorf("timed key: got %q want 2026-03-04T09:00:00", got)
	}
	allDay := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	if got := overrideKey(allDay, true); got != "2026-03-04" {
		t.Errorf("all-day key: got %q want 2026-03-04", got)
	}
	// Non-UTC input must still produce a UTC-flavoured key — the
	// expander emits times in time.Local on systems where DTSTART
	// carries a TZID, but the key contract stays UTC for parity
	// with how parseICSFile stores EXDATE.
	loc, _ := time.LoadLocation("Europe/Vienna")
	if loc != nil {
		eu := time.Date(2026, 3, 4, 11, 0, 0, 0, loc) // 10:00 UTC in winter, 09:00 in summer
		got := overrideKey(eu, false)
		want := eu.UTC().Format("2006-01-02T15:04:05")
		if got != want {
			t.Errorf("non-UTC input: got %q want %q", got, want)
		}
	}
}

// TestApplyTimedOverride pins the (start, end) transform contract:
//   - StartTime alone preserves duration (drag-move UX)
//   - StartTime + EndTime sets both wall-clock independently (drag-resize)
//   - Date alone shifts the day, time stays put
//   - Empty override is a no-op
//
// Carrier zone is UTC because events.json stores HH:MM as floating
// wall-clock and the calendar pipeline now treats UTC as the zone-free
// frame end-to-end (see comment on applyTimedOverride). Switching from
// time.Local in 2026-05 fixed a +offset drift on non-UTC servers.
func TestApplyTimedOverride(t *testing.T) {
	loc := time.UTC
	start := time.Date(2026, 3, 4, 9, 0, 0, 0, loc)
	end := time.Date(2026, 3, 4, 10, 0, 0, 0, loc) // 1h duration

	cases := []struct {
		name              string
		ovr               granitmeta.EventOverride
		wantStart, wantEnd time.Time
	}{
		{
			name:      "empty override is no-op",
			ovr:       granitmeta.EventOverride{},
			wantStart: start,
			wantEnd:   end,
		},
		{
			name:      "start only — duration preserved",
			ovr:       granitmeta.EventOverride{StartTime: "11:00"},
			wantStart: time.Date(2026, 3, 4, 11, 0, 0, 0, loc),
			wantEnd:   time.Date(2026, 3, 4, 12, 0, 0, 0, loc),
		},
		{
			name:      "start + end — explicit duration",
			ovr:       granitmeta.EventOverride{StartTime: "14:00", EndTime: "16:30"},
			wantStart: time.Date(2026, 3, 4, 14, 0, 0, 0, loc),
			wantEnd:   time.Date(2026, 3, 4, 16, 30, 0, 0, loc),
		},
		{
			name:      "date only — same time, different day",
			ovr:       granitmeta.EventOverride{Date: "2026-03-05"},
			wantStart: time.Date(2026, 3, 5, 9, 0, 0, 0, loc),
			wantEnd:   time.Date(2026, 3, 5, 10, 0, 0, 0, loc),
		},
		{
			name:      "date + start — drag-move to new day & time",
			ovr:       granitmeta.EventOverride{Date: "2026-03-05", StartTime: "13:30"},
			wantStart: time.Date(2026, 3, 5, 13, 30, 0, 0, loc),
			wantEnd:   time.Date(2026, 3, 5, 14, 30, 0, 0, loc),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotS, gotE := applyTimedOverride(start, end, c.ovr)
			if !gotS.Equal(c.wantStart) {
				t.Errorf("start: got %v, want %v", gotS, c.wantStart)
			}
			if !gotE.Equal(c.wantEnd) {
				t.Errorf("end: got %v, want %v", gotE, c.wantEnd)
			}
		})
	}
}
