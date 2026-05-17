package lectionary

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Tests cover the public surface the handlers depend on:
//   Plans / Get        catalogue lookup
//   LoadState / SaveState / StartPlan / StopPlan
//   DayOfPlan          time-zone-aware day arithmetic
//
// These run against a temp dir per test — atomic writes through
// internal/atomicio should leave a clean .granit/ tree behind.

func TestPlans_ReturnsCatalogue(t *testing.T) {
	plans := Plans()
	if len(plans) < 3 {
		t.Fatalf("expected at least 3 bundled plans (M'Cheyne + chrono NT + 90-day NT), got %d", len(plans))
	}
	// Catalogue should always include the canonical IDs the
	// handlers / frontend hardcode.
	wantIDs := map[string]bool{"mcheyne": true, "chrono-nt": true, "nt-90day": true}
	for _, p := range plans {
		delete(wantIDs, p.ID)
	}
	if len(wantIDs) > 0 {
		t.Fatalf("missing plan ids: %v", wantIDs)
	}
}

func TestPlans_EachReadingsListsAllDays(t *testing.T) {
	for _, p := range Plans() {
		if p.LengthDays <= 0 {
			t.Errorf("plan %q has LengthDays=%d, want > 0", p.ID, p.LengthDays)
			continue
		}
		if len(p.Readings) != p.LengthDays {
			t.Errorf("plan %q has %d Readings, LengthDays=%d — every day must be covered",
				p.ID, len(p.Readings), p.LengthDays)
		}
		// Spot-check day 1, day mid, day last all have non-empty
		// passages — a silently-empty day means the generator
		// has a fencepost bug.
		if len(p.Readings) > 0 {
			if len(p.Readings[0].Passages) == 0 {
				t.Errorf("plan %q day 1 has no passages", p.ID)
			}
			if len(p.Readings[len(p.Readings)-1].Passages) == 0 {
				t.Errorf("plan %q final day has no passages", p.ID)
			}
		}
	}
}

func TestGet_FoundAndMissing(t *testing.T) {
	if _, ok := Get("mcheyne"); !ok {
		t.Error("Get(\"mcheyne\") returned !ok, want bundled plan")
	}
	if _, ok := Get("nonexistent-plan"); ok {
		t.Error("Get(\"nonexistent-plan\") returned ok, want false")
	}
}

// LoadState should treat a missing file as an empty State, NOT an
// error — a fresh vault has no plans started yet.
func TestLoadState_MissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	s, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState on empty vault: %v", err)
	}
	if len(s.Active) != 0 {
		t.Errorf("expected empty Active, got %d entries", len(s.Active))
	}
}

func TestStartPlan_RejectsUnknownID(t *testing.T) {
	dir := t.TempDir()
	err := StartPlan(dir, "this-plan-does-not-exist")
	if err == nil {
		t.Fatal("StartPlan with unknown id: expected error, got nil")
	}
}

func TestStartPlan_PersistsAcrossLoad(t *testing.T) {
	dir := t.TempDir()
	if err := StartPlan(dir, "mcheyne"); err != nil {
		t.Fatalf("StartPlan: %v", err)
	}
	s, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(s.Active) != 1 || s.Active[0].PlanID != "mcheyne" {
		t.Errorf("expected exactly one active plan with id=mcheyne, got %+v", s.Active)
	}
	// File should actually land on disk.
	if _, err := os.Stat(filepath.Join(dir, ".granit", "lectionary-state.json")); err != nil {
		t.Errorf("state file not written: %v", err)
	}
}

// Calling StartPlan twice for the same ID is the documented "restart
// from day 1" path — should NOT duplicate the entry, just bump StartedAt.
func TestStartPlan_RestartReplacesEntry(t *testing.T) {
	dir := t.TempDir()
	if err := StartPlan(dir, "mcheyne"); err != nil {
		t.Fatalf("first StartPlan: %v", err)
	}
	s1, _ := LoadState(dir)
	firstStart := s1.Active[0].StartedAt

	// Sleep a millisecond so the second StartedAt is strictly later
	// — without this the test can't distinguish "replaced" from
	// "untouched" on a fast clock.
	time.Sleep(2 * time.Millisecond)

	if err := StartPlan(dir, "mcheyne"); err != nil {
		t.Fatalf("second StartPlan: %v", err)
	}
	s2, _ := LoadState(dir)
	if len(s2.Active) != 1 {
		t.Errorf("restart produced %d entries, want 1", len(s2.Active))
	}
	if !s2.Active[0].StartedAt.After(firstStart) {
		t.Errorf("restart should bump StartedAt; got %v, was %v", s2.Active[0].StartedAt, firstStart)
	}
}

func TestStopPlan_RemovesEntry(t *testing.T) {
	dir := t.TempDir()
	_ = StartPlan(dir, "mcheyne")
	_ = StartPlan(dir, "chrono-nt")
	if err := StopPlan(dir, "mcheyne"); err != nil {
		t.Fatalf("StopPlan: %v", err)
	}
	s, _ := LoadState(dir)
	if len(s.Active) != 1 || s.Active[0].PlanID != "chrono-nt" {
		t.Errorf("expected only chrono-nt active, got %+v", s.Active)
	}
}

// Stopping a plan that isn't active is documented as a no-op success
// (matches REST DELETE-on-missing semantics).
func TestStopPlan_AlreadyGoneIsNoOp(t *testing.T) {
	dir := t.TempDir()
	if err := StopPlan(dir, "mcheyne"); err != nil {
		t.Errorf("StopPlan on empty state should be no-op success, got %v", err)
	}
}

// DayOfPlan is the bit users notice most: "what reading am I on
// today?". The key invariant: a user who starts at 23:50 should NOT
// flip to day 2 at 00:00 ten minutes later — they flip at midnight
// of the next *calendar* day. We test by comparing calendar dates
// in the local zone.
func TestDayOfPlan_StartDayIsOne(t *testing.T) {
	start := time.Now()
	a := ActivePlan{PlanID: "mcheyne", StartedAt: start}
	if got := DayOfPlan(a, start); got != 1 {
		t.Errorf("DayOfPlan(start, start) = %d, want 1", got)
	}
}

func TestDayOfPlan_TomorrowIsTwo(t *testing.T) {
	// Anchor at 09:00 today, ask for 09:00 tomorrow.
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.Local)
	tomorrow := start.AddDate(0, 0, 1)
	a := ActivePlan{PlanID: "mcheyne", StartedAt: start}
	if got := DayOfPlan(a, tomorrow); got != 2 {
		t.Errorf("DayOfPlan(start, start+1d) = %d, want 2", got)
	}
}

// The "late-night start doesn't tick at midnight" case — explicitly.
func TestDayOfPlan_LateStartHoldsAtDayOneTillMidnight(t *testing.T) {
	// Start at 23:50, check 5 minutes later (23:55) — still day 1.
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 23, 50, 0, 0, time.Local)
	fiveMinLater := start.Add(5 * time.Minute)
	a := ActivePlan{PlanID: "mcheyne", StartedAt: start}
	if got := DayOfPlan(a, fiveMinLater); got != 1 {
		t.Errorf("DayOfPlan still on day 1 at 23:55 same day, got %d", got)
	}
	// Now jump past midnight to 00:01 of the NEXT day — should be day 2.
	afterMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 1, 0, 0, time.Local)
	if got := DayOfPlan(a, afterMidnight); got != 2 {
		t.Errorf("DayOfPlan at 00:01 next day, got %d, want 2", got)
	}
}

// A `when` before `StartedAt` is degenerate (the user shouldn't be
// asking what day they're on before they started). Clamp to day 1
// rather than returning 0 or a negative — keeps callers from having
// to bounds-check.
func TestDayOfPlan_ClampsNegativeToOne(t *testing.T) {
	start := time.Now()
	a := ActivePlan{PlanID: "mcheyne", StartedAt: start}
	yesterday := start.AddDate(0, 0, -1)
	if got := DayOfPlan(a, yesterday); got != 1 {
		t.Errorf("DayOfPlan with when before start, got %d, want 1 (clamped)", got)
	}
}
