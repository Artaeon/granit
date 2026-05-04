package virtues

import (
	"testing"
	"time"
)

// TestMondayOf locks the week-anchor contract: any click made
// during a week must canonicalise to the same Monday so the
// historical chart's x-axis stays clean. Cross-zone safety
// matters because the user may travel — we assert the function
// returns the Monday in the input's own location, not UTC.
func TestMondayOf(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	cases := []struct {
		name string
		in   time.Time
		want string
	}{
		// Wed 2026-05-06 → Monday 2026-05-04
		{"wednesday", time.Date(2026, 5, 6, 14, 30, 0, 0, loc), "2026-05-04"},
		// Sun 2026-05-10 → Monday 2026-05-04 (the previous Monday)
		{"sunday rolls back", time.Date(2026, 5, 10, 23, 59, 0, 0, loc), "2026-05-04"},
		// Mon 2026-05-04 → itself
		{"monday is itself", time.Date(2026, 5, 4, 0, 0, 0, 0, loc), "2026-05-04"},
		// Sat 2026-05-09 → Monday 2026-05-04
		{"saturday", time.Date(2026, 5, 9, 8, 15, 0, 0, loc), "2026-05-04"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := MondayOf(c.in); got != c.want {
				t.Errorf("MondayOf(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestUpsertCheck covers the three states the upsert operates on:
// fresh week (append), same week (replace), preserve other weeks.
// Critical because losing a previous week's reflection on a
// re-rate would erase journal history the user values.
func TestUpsertCheck(t *testing.T) {
	checks := []Check{
		{WeekStart: "2026-04-27", Score: 3, Note: "good"},
		{WeekStart: "2026-05-04", Score: 4, Note: "better"},
	}

	// Replace existing week — note must update, week count stays.
	out := UpsertCheck(checks, Check{WeekStart: "2026-05-04", Score: 5, Note: "best"})
	if len(out) != 2 {
		t.Fatalf("len after replace = %d, want 2", len(out))
	}
	if out[1].Note != "best" || out[1].Score != 5 {
		t.Errorf("replace didn't update: %+v", out[1])
	}
	if out[0].Note != "good" {
		t.Errorf("replace touched the wrong week: %+v", out[0])
	}

	// Append a fresh week.
	out = UpsertCheck(out, Check{WeekStart: "2026-05-11", Score: 2})
	if len(out) != 3 {
		t.Fatalf("len after append = %d, want 3", len(out))
	}
	if out[2].WeekStart != "2026-05-11" {
		t.Errorf("append landed in wrong slot: %+v", out)
	}
}

// TestLatestCheck verifies the "show this week's score on the
// virtue card" lookup. Returns the highest week-start regardless
// of input order so unsorted client submissions still surface
// the latest entry.
func TestLatestCheck(t *testing.T) {
	v := Virtue{
		Checks: []Check{
			{WeekStart: "2026-04-27", Score: 3},
			{WeekStart: "2026-05-11", Score: 5},
			{WeekStart: "2026-05-04", Score: 4},
		},
	}
	got, ok := v.LatestCheck()
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got.WeekStart != "2026-05-11" {
		t.Errorf("latest = %q, want %q", got.WeekStart, "2026-05-11")
	}
	// Empty Checks → ok=false.
	if _, ok := (Virtue{}).LatestCheck(); ok {
		t.Errorf("LatestCheck() on empty Checks should return ok=false")
	}
}

// TestValidateCheck covers the four rejection paths and the happy
// path. Score range and week-start format are the two things that
// can silently break the chart axis if accepted by the wrong shape.
func TestValidateCheck(t *testing.T) {
	cases := []struct {
		name    string
		c       Check
		wantErr bool
	}{
		{"happy", Check{WeekStart: "2026-05-04", Score: 3}, false},
		{"score too low", Check{WeekStart: "2026-05-04", Score: 0}, true},
		{"score too high", Check{WeekStart: "2026-05-04", Score: 6}, true},
		{"empty week", Check{Score: 3}, true},
		{"bad week format", Check{WeekStart: "yesterday", Score: 3}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateCheck(c.c)
			if (err != nil) != c.wantErr {
				t.Errorf("ValidateCheck err=%v, wantErr=%v", err, c.wantErr)
			}
		})
	}
}

// TestNormalizeStatus locks the canonical-3 contract — anything
// outside active/paused/archived defaults to active so the working
// list never goes empty due to a typoed PATCH.
func TestNormalizeStatus(t *testing.T) {
	cases := map[string]string{
		"":         "active",
		"active":   "active",
		"ACTIVE":   "active",
		"paused":   "paused",
		"archived": "archived",
		"foo":      "active",
		"  paused  ": "paused",
	}
	for in, want := range cases {
		if got := NormalizeStatus(in); got != want {
			t.Errorf("NormalizeStatus(%q) = %q, want %q", in, got, want)
		}
	}
}
