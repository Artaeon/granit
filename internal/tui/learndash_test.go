package tui

import "testing"

func TestLearnDashboard_RecordReview_StartsStreak(t *testing.T) {
	ld := NewLearnDashboard(t.TempDir())
	ld.RecordReview("2026-04-09")

	if ld.stats.StreakDays != 1 {
		t.Errorf("expected streak=1, got %d", ld.stats.StreakDays)
	}
	if ld.stats.TotalReviews != 1 {
		t.Errorf("expected total=1, got %d", ld.stats.TotalReviews)
	}
}

func TestLearnDashboard_RecordReview_ConsecutiveDaysIncrement(t *testing.T) {
	ld := NewLearnDashboard(t.TempDir())
	ld.RecordReview("2026-04-07")
	ld.RecordReview("2026-04-08")
	ld.RecordReview("2026-04-09")

	if ld.stats.StreakDays != 3 {
		t.Errorf("expected streak=3 for consecutive days, got %d", ld.stats.StreakDays)
	}
}

func TestLearnDashboard_RecordReview_GapResetsStreak(t *testing.T) {
	ld := NewLearnDashboard(t.TempDir())
	ld.RecordReview("2026-04-07")
	ld.RecordReview("2026-04-10") // 3 day gap

	if ld.stats.StreakDays != 1 {
		t.Errorf("expected streak reset to 1, got %d", ld.stats.StreakDays)
	}
}

func TestLearnDashboard_RecordReview_SameDayNoChange(t *testing.T) {
	ld := NewLearnDashboard(t.TempDir())
	ld.RecordReview("2026-04-09")
	ld.RecordReview("2026-04-09")

	if ld.stats.StreakDays != 1 {
		t.Errorf("same day should not change streak, got %d", ld.stats.StreakDays)
	}
	if ld.stats.TotalReviews != 2 {
		t.Errorf("total should increment, got %d", ld.stats.TotalReviews)
	}
}

func TestLearnDashboard_RecordQuiz_MaxHistory(t *testing.T) {
	ld := NewLearnDashboard(t.TempDir())
	for i := 0; i < 25; i++ {
		ld.RecordQuiz(QuizScore{Correct: i, Total: 10})
	}

	if len(ld.stats.QuizScores) != 20 {
		t.Errorf("expected 20 scores (capped), got %d", len(ld.stats.QuizScores))
	}
}

func TestLearnDashboard_SetCardStats(t *testing.T) {
	ld := NewLearnDashboard(t.TempDir())
	ld.SetCardStats(100, 10, 5, 50)

	if ld.cards.total != 100 {
		t.Errorf("expected 100 total cards, got %d", ld.cards.total)
	}
	if ld.cards.due != 10 {
		t.Errorf("expected 10 due, got %d", ld.cards.due)
	}
}

func TestLearnDashboard_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	ld := NewLearnDashboard(dir)
	ld.RecordReview("2026-04-09")
	ld.RecordQuiz(QuizScore{Correct: 8, Total: 10})

	// Reload
	ld2 := NewLearnDashboard(dir)
	if ld2.stats.TotalReviews != 1 {
		t.Errorf("expected 1 review after reload, got %d", ld2.stats.TotalReviews)
	}
	if len(ld2.stats.QuizScores) != 1 {
		t.Errorf("expected 1 quiz after reload, got %d", len(ld2.stats.QuizScores))
	}
}
