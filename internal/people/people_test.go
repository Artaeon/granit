package people

import (
	"reflect"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	in := []Person{{
		ID:              "01a",
		Name:            "Sebastian",
		Email:           "seb@example.com",
		Birthday:        "1990-04-15",
		Relationship:    "friend",
		Tags:            []string{"university", "berlin"},
		LastContactedAt: "2026-04-01",
		CadenceDays:     30,
		NotePath:        "People/Sebastian.md",
		CreatedAt:       now,
		UpdatedAt:       now,
	}}
	if err := SaveAll(dir, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	got := LoadAll(dir)
	if !reflect.DeepEqual(got, in) {
		t.Errorf("round-trip mismatch:\n got=%+v\n want=%+v", got, in)
	}
}

func TestIsStale(t *testing.T) {
	today := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		p    Person
		want bool
	}{
		{"no cadence → never stale", Person{LastContactedAt: "2025-01-01"}, false},
		{"archived → never stale", Person{Archived: true, CadenceDays: 7, LastContactedAt: "2020-01-01"}, false},
		{"never contacted but cadence set → stale", Person{CadenceDays: 30}, true},
		{"contacted 31d ago, cadence 30 → stale", Person{CadenceDays: 30, LastContactedAt: "2026-03-31"}, true},
		{"contacted 5d ago, cadence 30 → not stale", Person{CadenceDays: 30, LastContactedAt: "2026-04-26"}, false},
		{"contacted today, cadence 7 → not stale", Person{CadenceDays: 7, LastContactedAt: "2026-05-01"}, false},
		{"unparseable date → not stale (don't crash)", Person{CadenceDays: 30, LastContactedAt: "garbage"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.p.IsStale(today); got != c.want {
				t.Errorf("IsStale = %v, want %v", got, c.want)
			}
		})
	}
}

func TestUpcomingBirthdays(t *testing.T) {
	today := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	people := []Person{
		{ID: "1", Name: "Alice", Birthday: "1990-05-10"},   // in 9 days — IN
		{ID: "2", Name: "Bob", Birthday: "1985-04-30"},     // 1 day ago, next = 2027-04-30 — OUT
		{ID: "3", Name: "Carol", Birthday: "1992-05-15"},   // in 14 days — IN (window 14)
		{ID: "4", Name: "Dave", Birthday: "1995-08-01"},    // far away — OUT
		{ID: "5", Name: "Eve", Birthday: "06-01"},          // MM-DD only, in 31 days — OUT
		{ID: "6", Name: "Archived", Birthday: "1990-05-02", Archived: true}, // OUT (archived)
	}
	got := UpcomingBirthdays(people, today, 14)
	if len(got) != 2 || got[0].ID != "1" || got[1].ID != "3" {
		t.Errorf("got %d results: %+v", len(got), got)
	}
}

func TestSortForDisplay(t *testing.T) {
	today := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	people := []Person{
		{ID: "1", Name: "Bob", Archived: true},
		{ID: "2", Name: "Alice"},                                                     // active, not stale (no cadence)
		{ID: "3", Name: "Carol", CadenceDays: 30, LastContactedAt: "2026-03-15"},     // stale (47 days)
		{ID: "4", Name: "Dave", CadenceDays: 7, LastContactedAt: "2026-04-30"},       // not stale (1 day)
	}
	got := SortForDisplay(people, today)
	want := []string{"3", "2", "4", "1"} // stale (Carol), then active alpha (Alice, Dave), then archived (Bob)
	for i, p := range got {
		if p.ID != want[i] {
			t.Errorf("sort[%d]=%q want %q", i, p.ID, want[i])
		}
	}
}
