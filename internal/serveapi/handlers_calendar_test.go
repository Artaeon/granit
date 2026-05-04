package serveapi

import (
	"reflect"
	"testing"
	"time"
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
