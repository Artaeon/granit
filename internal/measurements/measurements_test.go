package measurements

import (
	"reflect"
	"testing"
	"time"
)

func TestSeriesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	target := 75.0
	in := []Series{{
		ID:        "01a",
		Name:      "Weight",
		Unit:      "kg",
		Target:    &target,
		Direction: "down",
		CreatedAt: now,
		UpdatedAt: now,
	}}
	if err := SaveSeries(dir, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	got := LoadSeries(dir)
	if !reflect.DeepEqual(got, in) {
		t.Errorf("round-trip mismatch:\n got=%+v\n want=%+v", got, in)
	}
}

func TestEntriesRoundTripAndSort(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	in := []Entry{
		{ID: "e1", SeriesID: "s1", Date: "2026-05-01", Value: 78.5, CreatedAt: now},
		{ID: "e2", SeriesID: "s1", Date: "2026-04-30", Value: 79.0, CreatedAt: now},
		{ID: "e3", SeriesID: "s2", Date: "2026-05-01", Value: 100, CreatedAt: now},
	}
	if err := SaveEntries(dir, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := LoadEntries(dir); !reflect.DeepEqual(got, in) {
		t.Errorf("round-trip mismatch")
	}
	got := EntriesForSeries(in, "s1")
	if len(got) != 2 || got[0].ID != "e1" || got[1].ID != "e2" {
		t.Errorf("EntriesForSeries(s1) = %+v", got)
	}
}

func TestLatestForSeries(t *testing.T) {
	now := time.Now().UTC()
	entries := []Entry{
		{ID: "old", SeriesID: "s1", Date: "2026-04-01", Value: 80, CreatedAt: now.Add(-24 * time.Hour)},
		{ID: "new", SeriesID: "s1", Date: "2026-05-01", Value: 78, CreatedAt: now},
	}
	got, ok := LatestForSeries(entries, "s1")
	if !ok || got.ID != "new" || got.Value != 78 {
		t.Errorf("got %+v ok=%v, want new entry", got, ok)
	}
	if _, ok := LatestForSeries(entries, "no-such-id"); ok {
		t.Error("LatestForSeries on unknown series should return ok=false")
	}
}

func TestSortSeries(t *testing.T) {
	in := []Series{
		{ID: "1", Name: "Zebra", Archived: false},
		{ID: "2", Name: "Apple", Archived: true},
		{ID: "3", Name: "Banana", Archived: false},
	}
	got := SortSeries(in)
	want := []string{"3", "1", "2"} // active alpha (Banana, Zebra), archived last (Apple)
	for i, s := range got {
		if s.ID != want[i] {
			t.Errorf("sort[%d] = %q, want %q", i, s.ID, want[i])
		}
	}
}

func TestSaveEmptyProducesArray(t *testing.T) {
	dir := t.TempDir()
	if err := SaveSeries(dir, nil); err != nil {
		t.Fatal(err)
	}
	if got := LoadSeries(dir); got == nil || len(got) != 0 {
		t.Errorf("empty save round-trip should be non-nil empty slice; got %v", got)
	}
}
