package tui

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Dashboard.parseHabits — extracts today's habit status from a habits file
// ---------------------------------------------------------------------------

func TestDashboardParseHabits_FromCachedContent(t *testing.T) {
	habitsContent := strings.Join([]string{
		"# Habits",
		"",
		"## Habits",
		"| Habit | Description | Streak |",
		"|-------|-------------|--------|",
		"| Read  | 30 min      | 12     |",
		"| Walk  | 10k steps   | 5      |",
		"",
		"## Log",
		"| Date       | Completed |",
		"|------------|-----------|",
		"| 2026-04-10 | Read, Walk |",
	}, "\n")

	d := &Dashboard{habitFileContent: habitsContent}
	d.parseHabits("2026-04-10")

	if len(d.todayHabits) != 2 {
		t.Fatalf("expected 2 habits, got %d", len(d.todayHabits))
	}
	if d.todayHabits[0].Name != "Read" || d.todayHabits[0].Streak != 12 || !d.todayHabits[0].Completed {
		t.Errorf("Read habit wrong: %+v", d.todayHabits[0])
	}
	if d.todayHabits[1].Name != "Walk" || d.todayHabits[1].Streak != 5 || !d.todayHabits[1].Completed {
		t.Errorf("Walk habit wrong: %+v", d.todayHabits[1])
	}
}

func TestDashboardParseHabits_OnlyTodayCounted(t *testing.T) {
	// Yesterday's log entries must NOT mark today's habits complete.
	habitsContent := strings.Join([]string{
		"## Habits",
		"| Habit | Desc | Streak |",
		"| Meditate | morning | 3 |",
		"## Log",
		"| Date       | Completed |",
		"| 2026-04-09 | Meditate |",
	}, "\n")

	d := &Dashboard{habitFileContent: habitsContent}
	d.parseHabits("2026-04-10")

	if len(d.todayHabits) != 1 {
		t.Fatalf("expected 1 habit, got %d", len(d.todayHabits))
	}
	if d.todayHabits[0].Completed {
		t.Errorf("habit should NOT be marked complete (yesterday's log), got %+v", d.todayHabits[0])
	}
}

func TestDashboardParseHabits_NoLogEntry(t *testing.T) {
	habitsContent := strings.Join([]string{
		"## Habits",
		"| Habit | Desc | Streak |",
		"| Stretch | 5 min | 0 |",
	}, "\n")

	d := &Dashboard{habitFileContent: habitsContent}
	d.parseHabits("2026-04-10")

	if len(d.todayHabits) != 1 {
		t.Fatalf("expected 1 habit, got %d", len(d.todayHabits))
	}
	if d.todayHabits[0].Completed {
		t.Error("habit should not be completed without a log entry")
	}
	if d.todayHabits[0].Streak != 0 {
		t.Errorf("expected streak 0, got %d", d.todayHabits[0].Streak)
	}
}

func TestDashboardParseHabits_SkipsHeaderAndSeparatorRows(t *testing.T) {
	// Header rows ('Habit') and separator rows ('---') must be skipped.
	habitsContent := strings.Join([]string{
		"## Habits",
		"| Habit | Description | Streak |",
		"|-------|-------------|--------|",
		"| Coffee | morning brew | 100 |",
	}, "\n")

	d := &Dashboard{habitFileContent: habitsContent}
	d.parseHabits("2026-04-10")

	if len(d.todayHabits) != 1 {
		t.Errorf("expected 1 habit (header skipped), got %d", len(d.todayHabits))
	}
	if d.todayHabits[0].Name != "Coffee" {
		t.Errorf("expected 'Coffee', got %q", d.todayHabits[0].Name)
	}
}

func TestDashboardParseHabits_EmptyContent(t *testing.T) {
	d := &Dashboard{habitFileContent: ""}
	d.parseHabits("2026-04-10")
	// With empty content and no fallback file, no habits.
	if len(d.todayHabits) != 0 {
		t.Errorf("expected 0 habits for empty content, got %d", len(d.todayHabits))
	}
}

func TestDashboardParseHabits_NoHabitsSection(t *testing.T) {
	// File with no ## Habits section.
	d := &Dashboard{habitFileContent: "# Just a regular note\n\nNo habits here."}
	d.parseHabits("2026-04-10")
	if len(d.todayHabits) != 0 {
		t.Errorf("expected 0 habits, got %d", len(d.todayHabits))
	}
}

func TestDashboardParseHabits_SectionBreak(t *testing.T) {
	// Habit parsing must stop at the next ## section.
	content := strings.Join([]string{
		"## Habits",
		"| Habit | Desc | Streak |",
		"| First | x | 1 |",
		"## Other Section",
		"| Habit | Desc | Streak |",
		"| Bogus | this should be ignored | 99 |",
	}, "\n")

	d := &Dashboard{habitFileContent: content}
	d.parseHabits("2026-04-10")

	if len(d.todayHabits) != 1 {
		t.Errorf("expected 1 habit, got %d (should not cross section boundary)", len(d.todayHabits))
	}
}

func TestDashboardParseHabits_StreakNonNumeric(t *testing.T) {
	// A streak value that doesn't parse to int defaults to 0.
	content := strings.Join([]string{
		"## Habits",
		"| Habit | Desc | Streak |",
		"| Read | book | abc |",
	}, "\n")

	d := &Dashboard{habitFileContent: content}
	d.parseHabits("2026-04-10")

	if len(d.todayHabits) != 1 {
		t.Fatal("expected 1 habit")
	}
	if d.todayHabits[0].Streak != 0 {
		t.Errorf("expected streak 0 for non-numeric, got %d", d.todayHabits[0].Streak)
	}
}

func TestDashboardParseHabits_MultipleCompletedHabits(t *testing.T) {
	content := strings.Join([]string{
		"## Habits",
		"| Habit | Desc | Streak |",
		"| Habit A | x | 1 |",
		"| Habit B | y | 2 |",
		"| Habit C | z | 3 |",
		"## Log",
		"| Date       | Completed |",
		"| 2026-04-10 | Habit A, Habit C |",
	}, "\n")

	d := &Dashboard{habitFileContent: content}
	d.parseHabits("2026-04-10")

	if len(d.todayHabits) != 3 {
		t.Fatalf("expected 3 habits, got %d", len(d.todayHabits))
	}
	if !d.todayHabits[0].Completed {
		t.Error("Habit A should be completed")
	}
	if d.todayHabits[1].Completed {
		t.Error("Habit B should NOT be completed")
	}
	if !d.todayHabits[2].Completed {
		t.Error("Habit C should be completed")
	}
}
