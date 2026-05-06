package serveapi

import "testing"

// TestValidateEventTimes pins the boundary contract for the events
// API: only valid 24-hour HH:MM times pass through, only YYYY-MM-DD
// dates pass through, and the end-after-start rule is enforced when
// both bounds are set. Empty start/end is allowed (an "all-day-ish"
// event without explicit times) and end-only without start is
// silently accepted because granitmeta tolerates it on read.
func TestValidateEventTimes(t *testing.T) {
	cases := []struct {
		name           string
		date           string
		start, end     string
		wantErr        bool
		wantErrSubstr  string
	}{
		{name: "happy path with times", date: "2026-05-06", start: "09:00", end: "10:30"},
		{name: "happy path date only", date: "2026-05-06"},
		{name: "happy path edge midnight to 23:59", date: "2026-05-06", start: "00:00", end: "23:59"},
		{name: "bad date — month name", date: "May 6 2026", wantErr: true, wantErrSubstr: "YYYY-MM-DD"},
		{name: "bad date — slashes", date: "2026/05/06", wantErr: true, wantErrSubstr: "YYYY-MM-DD"},
		{name: "bad date — short", date: "26-5-6", wantErr: true, wantErrSubstr: "YYYY-MM-DD"},
		{name: "12-hour start with PM", date: "2026-05-06", start: "9:00 PM", end: "10:30", wantErr: true, wantErrSubstr: "HH:MM"},
		{name: "out-of-range hour", date: "2026-05-06", start: "25:00", end: "26:00", wantErr: true, wantErrSubstr: "HH:MM"},
		{name: "out-of-range minute", date: "2026-05-06", start: "10:60", end: "11:00", wantErr: true, wantErrSubstr: "HH:MM"},
		{name: "single-digit hour without leading zero", date: "2026-05-06", start: "9:00", end: "10:00", wantErr: true, wantErrSubstr: "HH:MM"},
		{name: "end before start", date: "2026-05-06", start: "14:00", end: "09:00", wantErr: true, wantErrSubstr: "after"},
		{name: "end equals start", date: "2026-05-06", start: "14:00", end: "14:00", wantErr: true, wantErrSubstr: "after"},
		{name: "end-only is allowed (paired with empty start)", date: "2026-05-06", end: "10:00"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateEventTimes(c.date, c.start, c.end)
			if c.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.wantErrSubstr != "" && err != nil {
				if !contains(err.Error(), c.wantErrSubstr) {
					t.Errorf("error %q missing substring %q", err.Error(), c.wantErrSubstr)
				}
			}
		})
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
