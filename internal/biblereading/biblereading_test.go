package biblereading

import (
	"fmt"
	"sync"
	"testing"
)

func TestRecordRead_AddsNewDate(t *testing.T) {
	dir := t.TempDir()
	added, err := RecordRead(dir, "2026-05-11")
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Errorf("first RecordRead must return added=true")
	}
	snap, _ := Snapshot(dir)
	if len(snap) != 1 || snap[0] != "2026-05-11" {
		t.Errorf("snapshot wrong: %v", snap)
	}
}

func TestRecordRead_IsIdempotent(t *testing.T) {
	// Same calendar day, recorded twice — must not duplicate AND
	// must signal "nothing new" via added=false so the UI can skip
	// the toast on a routine re-page-load.
	dir := t.TempDir()
	_, _ = RecordRead(dir, "2026-05-11")
	added, err := RecordRead(dir, "2026-05-11")
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Errorf("second RecordRead on same date must return added=false")
	}
	snap, _ := Snapshot(dir)
	if len(snap) != 1 {
		t.Errorf("expected 1 date after dedupe, got %d", len(snap))
	}
}

func TestRecordRead_RejectsMalformedDate(t *testing.T) {
	dir := t.TempDir()
	for _, bad := range []string{"", "2026-5-11", "11/05/2026", "yesterday", "2026-13-01"} {
		if _, err := RecordRead(dir, bad); err == nil {
			t.Errorf("expected error for malformed date %q", bad)
		}
	}
}

func TestSnapshot_EmptyVault(t *testing.T) {
	// Fresh vault — Snapshot returns nil, not an error, so the
	// frontend can fold "no reading history" into a hidden badge
	// without special-casing the error path.
	dir := t.TempDir()
	snap, err := Snapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if snap != nil {
		t.Errorf("empty vault should return nil snapshot, got %v", snap)
	}
}

func TestSave_SortsAndDedupes(t *testing.T) {
	// Defensive: a future caller that constructs a Log directly
	// (skipping RecordRead) must still get a sorted + deduped file
	// on disk so the JSON stays stable across saves.
	dir := t.TempDir()
	if err := Save(dir, Log{
		Dates: []string{"2026-05-11", "2026-05-09", "", "2026-05-10", "2026-05-09"},
	}); err != nil {
		t.Fatal(err)
	}
	snap, _ := Snapshot(dir)
	if len(snap) != 3 {
		t.Fatalf("expected 3 dates after sort+dedupe, got %d (%v)", len(snap), snap)
	}
	if snap[0] != "2026-05-09" || snap[2] != "2026-05-11" {
		t.Errorf("expected ascending sort, got %v", snap)
	}
}

func TestRecordRead_PreservesPriorEntries(t *testing.T) {
	dir := t.TempDir()
	_, _ = RecordRead(dir, "2026-05-09")
	_, _ = RecordRead(dir, "2026-05-10")
	_, _ = RecordRead(dir, "2026-05-11")
	snap, _ := Snapshot(dir)
	if len(snap) != 3 {
		t.Errorf("expected 3 entries, got %d", len(snap))
	}
}

func TestConcurrentRecords_AllPersist(t *testing.T) {
	// Same read-modify-write race pattern as annotations + book
	// sidecars + AI memory. Without the mutex, two parallel
	// RecordRead calls for different dates would race the load-
	// mutate-save and lose one. 30 parallel records covers the
	// realistic burst (two tabs + a keyboard shortcut).
	dir := t.TempDir()
	const N = 30
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			// Spread across distinct dates so we test concurrent
			// add-different-dates (the dedupe path runs serially
			// by definition).
			date := fmt.Sprintf("2026-01-%02d", idx+1)
			if _, err := RecordRead(dir, date); err != nil {
				t.Errorf("concurrent RecordRead %d failed: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()
	snap, _ := Snapshot(dir)
	if len(snap) != N {
		t.Errorf("expected %d entries after concurrent records, got %d (lost writes)", N, len(snap))
	}
}
