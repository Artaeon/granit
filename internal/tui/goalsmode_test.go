package tui

import "testing"

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
