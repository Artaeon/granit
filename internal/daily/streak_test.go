package daily

import "testing"

func TestComputeStreak_Empty(t *testing.T) {
	got := ComputeStreak(nil, "2026-05-11")
	if got.Current != 0 || got.Longest != 0 || got.TodayLogged {
		t.Errorf("empty input must return zero streak: %+v", got)
	}
}

func TestComputeStreak_TodayLogged(t *testing.T) {
	// Three consecutive days ending today — current = longest = 3.
	dates := []string{"2026-05-09", "2026-05-10", "2026-05-11"}
	got := ComputeStreak(dates, "2026-05-11")
	if got.Current != 3 || got.Longest != 3 || !got.TodayLogged {
		t.Errorf("expected current=3 longest=3 todayLogged=true, got %+v", got)
	}
	if got.LastDate != "2026-05-11" {
		t.Errorf("LastDate = %q, want 2026-05-11", got.LastDate)
	}
}

func TestComputeStreak_TodayNotYet_YesterdayLogged(t *testing.T) {
	// User hasn't written today. The streak is still alive because
	// yesterday is logged — today is "in progress", not broken.
	dates := []string{"2026-05-08", "2026-05-09", "2026-05-10"}
	got := ComputeStreak(dates, "2026-05-11")
	if got.Current != 3 || got.Longest != 3 || got.TodayLogged {
		t.Errorf("expected current=3 longest=3 todayLogged=false, got %+v", got)
	}
}

func TestComputeStreak_Broken(t *testing.T) {
	// Last entry was day before yesterday; today and yesterday are
	// both missing → streak fully broken. Longest still reflects
	// the historical best.
	dates := []string{"2026-05-08", "2026-05-09"}
	got := ComputeStreak(dates, "2026-05-11")
	if got.Current != 0 {
		t.Errorf("expected current=0, got %d", got.Current)
	}
	if got.Longest != 2 {
		t.Errorf("expected longest=2 (the historical pair), got %d", got.Longest)
	}
}

func TestComputeStreak_HandlesGapsForLongest(t *testing.T) {
	// Historical pattern: a long-ago 5-day run, then sporadic
	// recent activity. Longest must surface the 5-day run even
	// though current is tiny.
	dates := []string{
		"2026-01-01", "2026-01-02", "2026-01-03", "2026-01-04", "2026-01-05",
		"2026-03-10",
		"2026-05-10", "2026-05-11",
	}
	got := ComputeStreak(dates, "2026-05-11")
	if got.Longest != 5 {
		t.Errorf("expected longest=5 from Jan run, got %d", got.Longest)
	}
	if got.Current != 2 {
		t.Errorf("expected current=2 (May 10+11), got %d", got.Current)
	}
}

func TestComputeStreak_DropsFutureDates(t *testing.T) {
	// A user post-dating a draft for tomorrow must not extend the
	// streak — the streak is honoured habit days only.
	dates := []string{"2026-05-11", "2026-05-12"}
	got := ComputeStreak(dates, "2026-05-11")
	if got.Current != 1 {
		t.Errorf("expected current=1 (today only, no future), got %d", got.Current)
	}
	if got.LastDate != "2026-05-11" {
		t.Errorf("LastDate must clamp at today, got %q", got.LastDate)
	}
}

func TestComputeStreak_DedupesSameDate(t *testing.T) {
	// Duplicate inputs (theoretical: two daily notes for the same
	// date in different folders) shouldn't inflate the count.
	dates := []string{"2026-05-10", "2026-05-10", "2026-05-11", "2026-05-11"}
	got := ComputeStreak(dates, "2026-05-11")
	if got.Current != 2 || got.Longest != 2 {
		t.Errorf("expected current=longest=2 after dedupe, got %+v", got)
	}
}

func TestComputeStreak_MonthBoundary(t *testing.T) {
	// prevDay must handle calendar month rollover (Apr 30 → May 1
	// going forward, May 1 → Apr 30 going backward). A naive
	// "-1 from the day component" would break the streak count here.
	dates := []string{"2026-04-29", "2026-04-30", "2026-05-01", "2026-05-02"}
	got := ComputeStreak(dates, "2026-05-02")
	if got.Current != 4 || got.Longest != 4 {
		t.Errorf("expected current=longest=4 across April→May boundary, got %+v", got)
	}
}

func TestComputeStreak_LeapYearFebToMar(t *testing.T) {
	// Feb 28 → Feb 29 (leap) → Mar 1 in 2024.
	dates := []string{"2024-02-28", "2024-02-29", "2024-03-01"}
	got := ComputeStreak(dates, "2024-03-01")
	if got.Current != 3 || got.Longest != 3 {
		t.Errorf("leap-year Feb→Mar streak miscomputed: %+v", got)
	}
}

func TestComputeStreak_IgnoresMalformedEntries(t *testing.T) {
	// Empty strings and odd values shouldn't crash or pollute the count.
	dates := []string{"", "2026-05-10", "2026-05-11"}
	got := ComputeStreak(dates, "2026-05-11")
	if got.Current != 2 {
		t.Errorf("empty string entry must be skipped: %+v", got)
	}
}
