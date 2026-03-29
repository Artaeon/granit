package tui

import (
	"strings"
	"testing"
	"time"
)

func TestGoalProgress(t *testing.T) {
	tests := []struct {
		name string
		goal Goal
		want int
	}{
		{"no milestones active", Goal{Status: GoalStatusActive}, 0},
		{"no milestones completed", Goal{Status: GoalStatusCompleted}, 100},
		{"all done", Goal{Milestones: []GoalMilestone{{Done: true}, {Done: true}}}, 100},
		{"half done", Goal{Milestones: []GoalMilestone{{Done: true}, {Done: false}}}, 50},
		{"none done", Goal{Milestones: []GoalMilestone{{Done: false}, {Done: false}}}, 0},
		{"one of three", Goal{Milestones: []GoalMilestone{{Done: true}, {Done: false}, {Done: false}}}, 33},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.goal.Progress()
			if got != tc.want {
				t.Errorf("Progress() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestGoalDoneCount(t *testing.T) {
	g := Goal{Milestones: []GoalMilestone{
		{Done: true}, {Done: false}, {Done: true},
	}}
	if g.DoneCount() != 2 {
		t.Errorf("DoneCount() = %d, want 2", g.DoneCount())
	}
}

func TestGoalIsOverdue(t *testing.T) {
	tests := []struct {
		name string
		goal Goal
		want bool
	}{
		{"no date", Goal{Status: GoalStatusActive}, false},
		{"future date", Goal{Status: GoalStatusActive, TargetDate: "2099-12-31"}, false},
		{"past date active", Goal{Status: GoalStatusActive, TargetDate: "2020-01-01"}, true},
		{"past date completed", Goal{Status: GoalStatusCompleted, TargetDate: "2020-01-01"}, false},
		{"past date archived", Goal{Status: GoalStatusArchived, TargetDate: "2020-01-01"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.goal.IsOverdue()
			if got != tc.want {
				t.Errorf("IsOverdue() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGoalDaysRemaining(t *testing.T) {
	g := Goal{TargetDate: "2099-12-31"}
	days := g.DaysRemaining()
	if days <= 0 {
		t.Errorf("DaysRemaining() = %d, want positive", days)
	}

	g2 := Goal{}
	if g2.DaysRemaining() != -1 {
		t.Errorf("DaysRemaining() with no date = %d, want -1", g2.DaysRemaining())
	}
}

func TestGoalsModeNextID(t *testing.T) {
	gm := &GoalsMode{
		goals: []Goal{
			{ID: "G001"},
			{ID: "G003"},
			{ID: "G002"},
		},
	}
	got := gm.nextID()
	if got != "G004" {
		t.Errorf("nextID() = %q, want %q", got, "G004")
	}
}

func TestGoalsModeNextID_Empty(t *testing.T) {
	gm := &GoalsMode{}
	got := gm.nextID()
	if got != "G001" {
		t.Errorf("nextID() = %q, want %q", got, "G001")
	}
}

func TestGoalsModeRebuildFiltered(t *testing.T) {
	gm := &GoalsMode{
		goals: []Goal{
			{ID: "G001", Status: GoalStatusActive},
			{ID: "G002", Status: GoalStatusCompleted},
			{ID: "G003", Status: GoalStatusPaused},
			{ID: "G004", Status: GoalStatusArchived},
		},
	}

	gm.view = goalViewAll
	gm.rebuildFiltered()
	if len(gm.filtered) != 2 {
		t.Errorf("goalViewAll: filtered = %d, want 2 (active + paused)", len(gm.filtered))
	}

	gm.view = goalViewCompleted
	gm.rebuildFiltered()
	if len(gm.filtered) != 2 {
		t.Errorf("goalViewCompleted: filtered = %d, want 2 (completed + archived)", len(gm.filtered))
	}
}

func TestGoalTimeframeLabel(t *testing.T) {
	tests := []struct {
		name     string
		days     int
		contains string
	}{
		{"overdue", -5, "overdue"},
		{"due today", 0, "due today"},
		{"1 day left", 1, "1d left"},
		{"7 days left", 7, "7d left"},
		{"3 weeks left", 21, "w left"},
		{"3 months left", 90, "mo left"},
		{"1 year left", 365, "y left"},
		{"1.5 years left", 548, "y"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
			target := today.AddDate(0, 0, tc.days)
			g := Goal{TargetDate: target.Format("2006-01-02"), Status: GoalStatusActive}
			got := g.TimeframeLabel()
			if !strings.Contains(got, tc.contains) {
				t.Errorf("TimeframeLabel() = %q, want to contain %q (days=%d)", got, tc.contains, tc.days)
			}
		})
	}
}

func TestGoalTimeframeLabel_NoDate(t *testing.T) {
	g := Goal{}
	got := g.TimeframeLabel()
	// DaysRemaining returns -1, so days < 0 triggers overdue path
	// but for no-date goals this shouldn't be called; just verify no crash
	if got == "" {
		t.Error("TimeframeLabel() should return something even for no-date")
	}
}

func TestGoalIsDueForReview_NeverReviewed(t *testing.T) {
	g := Goal{Status: GoalStatusActive, ReviewFrequency: "weekly", LastReviewed: ""}
	if !g.IsDueForReview() {
		t.Error("goal with frequency but no last review should be due")
	}
}

func TestGoalIsDueForReview_RecentlyReviewed(t *testing.T) {
	g := Goal{Status: GoalStatusActive, ReviewFrequency: "weekly", LastReviewed: time.Now().Format("2006-01-02")}
	if g.IsDueForReview() {
		t.Error("goal reviewed today should not be due")
	}
}

func TestGoalIsDueForReview_NoFrequency(t *testing.T) {
	g := Goal{Status: GoalStatusActive, ReviewFrequency: ""}
	if g.IsDueForReview() {
		t.Error("goal with no frequency should not be due")
	}
}

func TestGoalIsDueForReview_Completed(t *testing.T) {
	g := Goal{Status: GoalStatusCompleted, ReviewFrequency: "weekly", LastReviewed: "2020-01-01"}
	if g.IsDueForReview() {
		t.Error("completed goal should not be due for review")
	}
}

func TestGoalNextReviewDate(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	g := Goal{ReviewFrequency: "monthly", LastReviewed: today}
	next := g.NextReviewDate()
	if next == "" || next <= today {
		t.Errorf("NextReviewDate() = %q, should be after today", next)
	}
}

func TestGoalNextReviewDate_NoFrequency(t *testing.T) {
	g := Goal{}
	if g.NextReviewDate() != "" {
		t.Error("NextReviewDate() with no frequency should return empty")
	}
}

func TestGoalsModeUniqueCategories(t *testing.T) {
	gm := &GoalsMode{
		goals: []Goal{
			{Category: "Health"},
			{Category: "Career"},
			{Category: "Health"},
			{Category: ""},
		},
	}
	cats := gm.uniqueCategories()
	if len(cats) != 2 {
		t.Errorf("uniqueCategories() = %v, want 2 categories", cats)
	}
}
