package tui

import (
	"fmt"
	"os"
	"path/filepath"
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

// ── AI Goal Coach ──────────────────────────────────────────────

func TestGoalsMode_CoachMsg_SetsText(t *testing.T) {
	gm := NewGoalsMode()
	gm.active = true
	gm.aiPending = true

	gm, _ = gm.Update(gmAICoachMsg{analysis: "## Report\nAll good"})

	if gm.aiPending {
		t.Error("aiPending should be false after coach msg")
	}
	if !gm.showCoach {
		t.Error("showCoach should be true after successful coach msg")
	}
	if gm.coachText != "## Report\nAll good" {
		t.Errorf("unexpected coachText: %q", gm.coachText)
	}
}

func TestGoalsMode_CoachMsg_Error(t *testing.T) {
	gm := NewGoalsMode()
	gm.active = true
	gm.aiPending = true

	gm, _ = gm.Update(gmAICoachMsg{err: fmt.Errorf("timeout")})

	if gm.aiPending {
		t.Error("aiPending should be false after error")
	}
	if gm.showCoach {
		t.Error("showCoach should be false on error")
	}
	if !strings.Contains(gm.statusMsg, "timeout") {
		t.Errorf("statusMsg should contain error: %q", gm.statusMsg)
	}
}

func TestGoalsMode_CoachEscDismisses(t *testing.T) {
	gm := NewGoalsMode()
	gm.active = true
	gm.showCoach = true
	gm.coachText = "some analysis"

	gm, _ = gm.updateNormal("esc")

	if gm.showCoach {
		t.Error("Esc should dismiss coach")
	}
	if gm.coachText != "" {
		t.Error("coachText should be cleared on dismiss")
	}
	if !gm.IsActive() {
		t.Error("goals should still be active after dismissing coach")
	}
}

func TestGoalsMode_CoachRenderNotPanic(t *testing.T) {
	gm := NewGoalsMode()
	gm.active = true
	gm.showCoach = true
	gm.coachText = ""
	gm.width = 80
	gm.height = 40

	// Should not panic with empty coach text
	view := gm.View()
	if view == "" {
		t.Error("View should not be empty when active")
	}
}

// ---------------------------------------------------------------------------
// saveAllGoals — atomic save (regression for non-atomic write)
// ---------------------------------------------------------------------------

func TestSaveAllGoals_RoundTrip(t *testing.T) {
	vault := t.TempDir()

	goals := []Goal{
		{ID: "g1", Title: "Ship granit", Status: GoalStatusActive},
		{ID: "g2", Title: "Read more", Status: GoalStatusActive,
			Milestones: []GoalMilestone{{Text: "Book 1", Done: true}, {Text: "Book 2"}}},
	}
	if !saveAllGoals(vault, goals) {
		t.Fatal("saveAllGoals returned false")
	}

	loaded := loadAllGoals(vault)
	if len(loaded) != 2 {
		t.Fatalf("expected 2 goals, got %d", len(loaded))
	}
	if loaded[0].Title != "Ship granit" {
		t.Errorf("first goal lost title: %q", loaded[0].Title)
	}
	if len(loaded[1].Milestones) != 2 {
		t.Errorf("second goal lost milestones: %v", loaded[1].Milestones)
	}
}

// Regression: saveAllGoals must use atomic write — no leftover .tmp file.
func TestSaveAllGoals_AtomicNoTmp(t *testing.T) {
	vault := t.TempDir()
	if !saveAllGoals(vault, []Goal{{ID: "x", Title: "y"}}) {
		t.Fatal("save failed")
	}
	tmp := filepath.Join(vault, ".granit", "goals.json.tmp")
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("expected no .tmp file, stat err = %v", err)
	}
}

func TestSaveAllGoals_OverwritesPrevious(t *testing.T) {
	vault := t.TempDir()
	saveAllGoals(vault, []Goal{{ID: "old", Title: "Old goal"}})
	saveAllGoals(vault, []Goal{{ID: "new", Title: "New goal"}})

	loaded := loadAllGoals(vault)
	if len(loaded) != 1 {
		t.Fatalf("expected 1 goal after overwrite, got %d", len(loaded))
	}
	if loaded[0].Title != "New goal" {
		t.Errorf("expected 'New goal', got %q", loaded[0].Title)
	}
}

func TestSaveAllGoals_EmptyList(t *testing.T) {
	vault := t.TempDir()
	if !saveAllGoals(vault, []Goal{}) {
		t.Fatal("expected save to succeed for empty list")
	}
	loaded := loadAllGoals(vault)
	if len(loaded) != 0 {
		t.Errorf("expected 0 goals, got %d", len(loaded))
	}
}

func TestLoadAllGoals_MissingFile(t *testing.T) {
	vault := t.TempDir()
	loaded := loadAllGoals(vault)
	if len(loaded) != 0 {
		t.Errorf("expected empty result for missing file, got %d", len(loaded))
	}
}

// Regression: malformed goals.json must not crash; load returns empty.
func TestLoadAllGoals_MalformedJSON(t *testing.T) {
	vault := t.TempDir()
	dir := filepath.Join(vault, ".granit")
	_ = os.MkdirAll(dir, 0755)
	_ = os.WriteFile(filepath.Join(dir, "goals.json"), []byte("{not json"), 0644)

	loaded := loadAllGoals(vault)
	if len(loaded) != 0 {
		t.Errorf("expected empty result for malformed JSON, got %v", loaded)
	}
}

// ---------------------------------------------------------------------------
// addMilestoneToGoal
// ---------------------------------------------------------------------------

func TestAddMilestoneToGoal_AppendsAndPersists(t *testing.T) {
	vault := t.TempDir()
	saveAllGoals(vault, []Goal{
		{ID: "g1", Title: "Goal 1"},
	})

	addMilestoneToGoal(vault, "g1", "First step", "2026-04-15")

	loaded := loadAllGoals(vault)
	if len(loaded) != 1 {
		t.Fatalf("expected 1 goal, got %d", len(loaded))
	}
	if len(loaded[0].Milestones) != 1 {
		t.Fatalf("expected 1 milestone, got %d", len(loaded[0].Milestones))
	}
	if loaded[0].Milestones[0].Text != "First step" {
		t.Errorf("milestone text wrong: %q", loaded[0].Milestones[0].Text)
	}
}
