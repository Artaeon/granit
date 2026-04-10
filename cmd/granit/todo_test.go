package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// parseDueDate
// ---------------------------------------------------------------------------

func TestParseDueDate_Today(t *testing.T) {
	want := time.Now().Format("2006-01-02")
	if got := parseDueDate("today"); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestParseDueDate_TodayCaseInsensitive(t *testing.T) {
	want := time.Now().Format("2006-01-02")
	if got := parseDueDate("TODAY"); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestParseDueDate_Tomorrow(t *testing.T) {
	want := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	if got := parseDueDate("tomorrow"); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestParseDueDate_Yesterday(t *testing.T) {
	want := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	if got := parseDueDate("yesterday"); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestParseDueDate_FullDate(t *testing.T) {
	if got := parseDueDate("2026-12-25"); got != "2026-12-25" {
		t.Errorf("expected '2026-12-25', got %q", got)
	}
}

func TestParseDueDate_MonthDayInPast_RollsToNextYear(t *testing.T) {
	// Pick a date 2 months ago — should roll to next year.
	twoMonthsAgo := time.Now().AddDate(0, -2, 0)
	input := twoMonthsAgo.Format("01-02")
	got := parseDueDate(input)

	// Result should be in the future (this year or next year).
	parsed, err := time.Parse("2006-01-02", got)
	if err != nil {
		t.Fatalf("could not parse result %q: %v", got, err)
	}
	if parsed.Before(time.Now().Add(-24 * time.Hour)) {
		t.Errorf("expected future date, got %s for input %s", got, input)
	}
}

func TestParseDueDate_MonthDayInFuture(t *testing.T) {
	// Pick a date 2 months from now — should be this year.
	twoMonthsAhead := time.Now().AddDate(0, 2, 0)
	input := twoMonthsAhead.Format("01-02")
	got := parseDueDate(input)

	parsed, err := time.Parse("2006-01-02", got)
	if err != nil {
		t.Fatalf("could not parse result %q: %v", got, err)
	}
	if parsed.Year() != time.Now().Year() && parsed.Year() != time.Now().Year()+1 {
		t.Errorf("year wrong: got %d", parsed.Year())
	}
}

func TestParseDueDate_Invalid(t *testing.T) {
	if got := parseDueDate("not a date"); got != "" {
		t.Errorf("expected empty for unparseable input, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// nextWeekday
// ---------------------------------------------------------------------------

func TestNextWeekday_AlwaysInFuture(t *testing.T) {
	for _, day := range []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"} {
		t.Run(day, func(t *testing.T) {
			got := nextWeekday(day)
			now := time.Now()
			if !got.After(now) {
				t.Errorf("nextWeekday(%q) = %v, expected to be after now (%v)", day, got, now)
			}
			// Should be at most 7 days away.
			if got.After(now.AddDate(0, 0, 8)) {
				t.Errorf("nextWeekday(%q) too far in future: %v", day, got)
			}
		})
	}
}

func TestNextWeekday_CaseInsensitive(t *testing.T) {
	a := nextWeekday("monday")
	b := nextWeekday("MONDAY")
	c := nextWeekday("Monday")
	if a.Format("2006-01-02") != b.Format("2006-01-02") || b.Format("2006-01-02") != c.Format("2006-01-02") {
		t.Error("nextWeekday should be case-insensitive")
	}
}

// Regression: nextWeekday("monday") on a Monday must return NEXT Monday,
// not today (the daysUntil==0 case must roll forward by 7).
func TestNextWeekday_TodayRollsForward(t *testing.T) {
	now := time.Now()
	todayName := strings.ToLower(now.Weekday().String())
	got := nextWeekday(todayName)
	if got.Format("2006-01-02") == now.Format("2006-01-02") {
		t.Errorf("nextWeekday(%q) on a %s should be 7 days away, got today", todayName, todayName)
	}
}

func TestNextWeekday_UnknownReturnsNow(t *testing.T) {
	got := nextWeekday("zzz")
	// Falls through to time.Now() — should be very close to current time.
	if time.Since(got) > time.Second {
		t.Errorf("expected ~now for unknown weekday, got %v ago", time.Since(got))
	}
}

// ---------------------------------------------------------------------------
// collectTodoText
// ---------------------------------------------------------------------------

func TestCollectTodoText_PositionalsOnly(t *testing.T) {
	got := collectTodoText([]string{"buy", "milk", "today"})
	if got != "buy milk today" {
		t.Errorf("expected 'buy milk today', got %q", got)
	}
}

func TestCollectTodoText_SkipsFlags(t *testing.T) {
	got := collectTodoText([]string{"--priority", "high", "buy", "milk"})
	if got != "buy milk" {
		t.Errorf("expected 'buy milk', got %q", got)
	}
}

func TestCollectTodoText_SkipsEqualsFlags(t *testing.T) {
	got := collectTodoText([]string{"--priority=high", "buy", "milk"})
	if got != "buy milk" {
		t.Errorf("expected 'buy milk', got %q", got)
	}
}

func TestCollectTodoText_Empty(t *testing.T) {
	if got := collectTodoText([]string{}); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// buildTaskLine
// ---------------------------------------------------------------------------

func TestBuildTaskLine_NoFlags(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"granit", "todo", "buy milk"}

	if got := buildTaskLine("buy milk"); got != "- [ ] buy milk" {
		t.Errorf("expected '- [ ] buy milk', got %q", got)
	}
}

func TestBuildTaskLine_WithPriority(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"granit", "todo", "buy milk", "--priority", "high"}

	got := buildTaskLine("buy milk")
	if !strings.Contains(got, "⏫") {
		t.Errorf("expected high priority marker, got %q", got)
	}
}

func TestBuildTaskLine_WithDueDate(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"granit", "todo", "buy milk", "--due", "today"}

	got := buildTaskLine("buy milk")
	today := time.Now().Format("2006-01-02")
	if !strings.Contains(got, "📅 "+today) {
		t.Errorf("expected due date marker for today, got %q", got)
	}
}

func TestBuildTaskLine_WithTag(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"granit", "todo", "buy milk", "--tag", "shopping"}

	got := buildTaskLine("buy milk")
	if !strings.Contains(got, "#shopping") {
		t.Errorf("expected #shopping tag, got %q", got)
	}
}

// Regression: a tag that already starts with # should not be doubled.
func TestBuildTaskLine_TagWithLeadingHash(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"granit", "todo", "x", "--tag", "#urgent"}

	got := buildTaskLine("x")
	if strings.Contains(got, "##urgent") {
		t.Errorf("tag was double-hashed: %q", got)
	}
	if !strings.Contains(got, "#urgent") {
		t.Errorf("expected #urgent, got %q", got)
	}
}

func TestBuildTaskLine_AllFlags(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"granit", "todo", "x", "--priority", "highest", "--due", "tomorrow", "--tag", "alpha", "--tag", "beta"}

	got := buildTaskLine("x")
	if !strings.Contains(got, "🔺") {
		t.Errorf("missing highest priority: %q", got)
	}
	if !strings.Contains(got, "📅") {
		t.Errorf("missing due date: %q", got)
	}
	if !strings.Contains(got, "#alpha") || !strings.Contains(got, "#beta") {
		t.Errorf("missing tags: %q", got)
	}
}
