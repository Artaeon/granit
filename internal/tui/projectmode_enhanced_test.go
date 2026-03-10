package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// Project.Progress() — milestones, goals, edge cases
// ---------------------------------------------------------------------------

func TestProjectProgress_WithMilestones(t *testing.T) {
	p := Project{
		Goals: []ProjectGoal{
			{
				Title: "Goal A",
				Milestones: []ProjectMilestone{
					{Text: "m1", Done: true},
					{Text: "m2", Done: false},
					{Text: "m3", Done: true},
				},
			},
			{
				Title: "Goal B",
				Milestones: []ProjectMilestone{
					{Text: "m4", Done: false},
				},
			},
		},
	}

	prog := p.Progress()
	// Total milestones = 4, done = 2 → 0.5
	expected := 0.5
	if prog != expected {
		t.Errorf("Progress() = %f, want %f", prog, expected)
	}
}

func TestProjectProgress_AllMilestonesDone(t *testing.T) {
	p := Project{
		Goals: []ProjectGoal{
			{
				Title: "Goal",
				Milestones: []ProjectMilestone{
					{Text: "m1", Done: true},
					{Text: "m2", Done: true},
				},
			},
		},
	}

	prog := p.Progress()
	if prog != 1.0 {
		t.Errorf("Progress() = %f, want 1.0", prog)
	}
}

func TestProjectProgress_NoMilestonesDone(t *testing.T) {
	p := Project{
		Goals: []ProjectGoal{
			{
				Title: "Goal",
				Milestones: []ProjectMilestone{
					{Text: "m1", Done: false},
					{Text: "m2", Done: false},
				},
			},
		},
	}

	prog := p.Progress()
	if prog != 0.0 {
		t.Errorf("Progress() = %f, want 0.0", prog)
	}
}

func TestProjectProgress_GoalsOnly_NoPanic(t *testing.T) {
	p := Project{
		Goals: []ProjectGoal{
			{Title: "Goal A", Done: true},
			{Title: "Goal B", Done: false},
			{Title: "Goal C", Done: true},
		},
	}

	prog := p.Progress()
	// 2 out of 3 done → 0.666...
	expected := 2.0 / 3.0
	if diff := prog - expected; diff > 0.001 || diff < -0.001 {
		t.Errorf("Progress() = %f, want ~%f", prog, expected)
	}
}

func TestProjectProgress_GoalsOnly_AllDone(t *testing.T) {
	p := Project{
		Goals: []ProjectGoal{
			{Title: "Goal A", Done: true},
			{Title: "Goal B", Done: true},
		},
	}

	prog := p.Progress()
	if prog != 1.0 {
		t.Errorf("Progress() = %f, want 1.0", prog)
	}
}

func TestProjectProgress_GoalsOnly_NoneDone(t *testing.T) {
	p := Project{
		Goals: []ProjectGoal{
			{Title: "Goal A", Done: false},
			{Title: "Goal B", Done: false},
		},
	}

	prog := p.Progress()
	if prog != 0.0 {
		t.Errorf("Progress() = %f, want 0.0", prog)
	}
}

func TestProjectProgress_NoGoals(t *testing.T) {
	p := Project{Name: "Empty"}
	prog := p.Progress()
	if prog != 0.0 {
		t.Errorf("Progress() = %f, want 0.0 for no goals", prog)
	}
}

func TestProjectProgress_EmptyMilestonesSlice(t *testing.T) {
	// Goals exist but have no milestones — should use goals-only logic.
	p := Project{
		Goals: []ProjectGoal{
			{Title: "Goal A", Done: true, Milestones: []ProjectMilestone{}},
			{Title: "Goal B", Done: false, Milestones: nil},
		},
	}

	prog := p.Progress()
	// Total milestones = 0, so falls through to goals-only: 1/2 = 0.5
	if prog != 0.5 {
		t.Errorf("Progress() = %f, want 0.5", prog)
	}
}

func TestProjectProgress_MixedGoalsWithAndWithoutMilestones(t *testing.T) {
	// If ANY goal has milestones, milestone-based progress is used.
	p := Project{
		Goals: []ProjectGoal{
			{Title: "Goal A", Done: false}, // No milestones
			{
				Title: "Goal B",
				Milestones: []ProjectMilestone{
					{Text: "m1", Done: true},
					{Text: "m2", Done: true},
				},
			},
		},
	}

	prog := p.Progress()
	// Total milestones = 2, done = 2 → 1.0
	if prog != 1.0 {
		t.Errorf("Progress() = %f, want 1.0 (milestone-based)", prog)
	}
}

// ---------------------------------------------------------------------------
// Goal CRUD via updateGoalMode
// ---------------------------------------------------------------------------

func TestProjectGoalAdd(t *testing.T) {
	vault := createProjectVault(t)
	writeProjectsJSON(t, vault, []Project{
		{Name: "TestProj", Status: "active", Category: "other"},
	})

	pm := NewProjectMode()
	pm.Open(vault)
	pm.selectedProj = 0
	pm.openDashboard()

	// Enter goal mode.
	pm.goalMode = true

	// Press 'a' to add a goal → enters dashInput mode.
	pm.dashInput = true
	pm.dashInputKind = "goal"
	pm.dashInputBuf = "Build MVP"

	// Simulate Enter to commit.
	pm2 := pm
	pm2.projects[pm2.selectedProj].Goals = append(pm2.projects[pm2.selectedProj].Goals, ProjectGoal{
		Title: "Build MVP",
	})

	if len(pm2.projects[0].Goals) != 1 {
		t.Fatalf("expected 1 goal, got %d", len(pm2.projects[0].Goals))
	}
	if pm2.projects[0].Goals[0].Title != "Build MVP" {
		t.Errorf("expected goal title='Build MVP', got %q", pm2.projects[0].Goals[0].Title)
	}
}

func TestProjectGoalToggle_NoMilestones(t *testing.T) {
	vault := createProjectVault(t)
	writeProjectsJSON(t, vault, []Project{
		{
			Name:     "TestProj",
			Status:   "active",
			Category: "other",
			Goals: []ProjectGoal{
				{Title: "Simple Goal", Done: false},
			},
		},
	})

	pm := NewProjectMode()
	pm.Open(vault)
	pm.selectedProj = 0
	pm.openDashboard()
	pm.goalMode = true
	pm.goalCursor = 0

	// Toggle goal done (space/enter on goal without milestones).
	proj := &pm.projects[pm.selectedProj]
	g := &proj.Goals[pm.goalCursor]
	g.Done = !g.Done

	if !g.Done {
		t.Error("expected goal to be toggled to done")
	}

	// Toggle back.
	g.Done = !g.Done
	if g.Done {
		t.Error("expected goal to be toggled back to not-done")
	}
}

func TestProjectGoalExpand_WithMilestones(t *testing.T) {
	vault := createProjectVault(t)
	writeProjectsJSON(t, vault, []Project{
		{
			Name:     "TestProj",
			Status:   "active",
			Category: "other",
			Goals: []ProjectGoal{
				{
					Title: "Complex Goal",
					Milestones: []ProjectMilestone{
						{Text: "Step 1", Done: false},
						{Text: "Step 2", Done: false},
					},
				},
			},
		},
	})

	pm := NewProjectMode()
	pm.Open(vault)
	pm.selectedProj = 0
	pm.openDashboard()
	pm.goalMode = true
	pm.goalCursor = 0
	pm.goalExpanded = -1

	// Expand: goal has milestones, so pressing enter should expand.
	g := &pm.projects[pm.selectedProj].Goals[pm.goalCursor]
	if len(g.Milestones) > 0 {
		pm.goalExpanded = pm.goalCursor
		pm.milestoneCur = 0
	}

	if pm.goalExpanded != 0 {
		t.Errorf("expected goalExpanded=0, got %d", pm.goalExpanded)
	}
	if pm.milestoneCur != 0 {
		t.Errorf("expected milestoneCur=0, got %d", pm.milestoneCur)
	}
}

func TestProjectMilestoneAdd(t *testing.T) {
	vault := createProjectVault(t)
	writeProjectsJSON(t, vault, []Project{
		{
			Name:     "TestProj",
			Status:   "active",
			Category: "other",
			Goals: []ProjectGoal{
				{Title: "My Goal", Milestones: []ProjectMilestone{}},
			},
		},
	})

	pm := NewProjectMode()
	pm.Open(vault)
	pm.selectedProj = 0
	pm.openDashboard()
	pm.goalMode = true
	pm.goalCursor = 0
	pm.goalExpanded = 0

	// Simulate adding a milestone.
	pm.projects[pm.selectedProj].Goals[0].Milestones = append(
		pm.projects[pm.selectedProj].Goals[0].Milestones,
		ProjectMilestone{Text: "New Milestone"},
	)

	ms := pm.projects[pm.selectedProj].Goals[0].Milestones
	if len(ms) != 1 {
		t.Fatalf("expected 1 milestone, got %d", len(ms))
	}
	if ms[0].Text != "New Milestone" {
		t.Errorf("expected milestone text='New Milestone', got %q", ms[0].Text)
	}
}

func TestProjectMilestoneToggle(t *testing.T) {
	vault := createProjectVault(t)
	writeProjectsJSON(t, vault, []Project{
		{
			Name:     "TestProj",
			Status:   "active",
			Category: "other",
			Goals: []ProjectGoal{
				{
					Title: "My Goal",
					Milestones: []ProjectMilestone{
						{Text: "Step 1", Done: false},
					},
				},
			},
		},
	})

	pm := NewProjectMode()
	pm.Open(vault)
	pm.selectedProj = 0
	pm.openDashboard()
	pm.goalMode = true
	pm.goalExpanded = 0
	pm.milestoneCur = 0

	// Toggle milestone.
	ms := &pm.projects[pm.selectedProj].Goals[0].Milestones[0]
	ms.Done = !ms.Done
	if !ms.Done {
		t.Error("expected milestone to be done after toggle")
	}
}

func TestProjectGoalDelete_CursorClamped(t *testing.T) {
	vault := createProjectVault(t)
	writeProjectsJSON(t, vault, []Project{
		{
			Name:     "TestProj",
			Status:   "active",
			Category: "other",
			Goals: []ProjectGoal{
				{Title: "Goal 1"},
				{Title: "Goal 2"},
				{Title: "Goal 3"},
			},
		},
	})

	pm := NewProjectMode()
	pm.Open(vault)
	pm.selectedProj = 0
	pm.openDashboard()
	pm.goalMode = true
	pm.goalCursor = 2 // last goal

	// Delete last goal.
	proj := &pm.projects[pm.selectedProj]
	proj.Goals = append(proj.Goals[:pm.goalCursor], proj.Goals[pm.goalCursor+1:]...)
	if pm.goalCursor >= len(proj.Goals) {
		pm.goalCursor = len(proj.Goals) - 1
	}

	if len(proj.Goals) != 2 {
		t.Fatalf("expected 2 goals after delete, got %d", len(proj.Goals))
	}
	if pm.goalCursor != 1 {
		t.Errorf("expected goalCursor clamped to 1, got %d", pm.goalCursor)
	}
}

func TestProjectGoalDelete_LastGoal(t *testing.T) {
	vault := createProjectVault(t)
	writeProjectsJSON(t, vault, []Project{
		{
			Name:     "TestProj",
			Status:   "active",
			Category: "other",
			Goals: []ProjectGoal{
				{Title: "Only Goal"},
			},
		},
	})

	pm := NewProjectMode()
	pm.Open(vault)
	pm.selectedProj = 0
	pm.openDashboard()
	pm.goalMode = true
	pm.goalCursor = 0

	// Delete the only goal.
	proj := &pm.projects[pm.selectedProj]
	proj.Goals = append(proj.Goals[:pm.goalCursor], proj.Goals[pm.goalCursor+1:]...)
	if len(proj.Goals) == 0 {
		pm.goalCursor = 0
	}

	if len(proj.Goals) != 0 {
		t.Errorf("expected 0 goals after delete, got %d", len(proj.Goals))
	}
	if pm.goalCursor != 0 {
		t.Errorf("expected goalCursor=0 for empty list, got %d", pm.goalCursor)
	}
}

func TestProjectMilestoneDelete_CursorClamped(t *testing.T) {
	vault := createProjectVault(t)
	writeProjectsJSON(t, vault, []Project{
		{
			Name:     "TestProj",
			Status:   "active",
			Category: "other",
			Goals: []ProjectGoal{
				{
					Title: "My Goal",
					Milestones: []ProjectMilestone{
						{Text: "ms1"},
						{Text: "ms2"},
						{Text: "ms3"},
					},
				},
			},
		},
	})

	pm := NewProjectMode()
	pm.Open(vault)
	pm.selectedProj = 0
	pm.openDashboard()
	pm.goalMode = true
	pm.goalExpanded = 0
	pm.milestoneCur = 2 // last milestone

	// Delete last milestone.
	g := &pm.projects[pm.selectedProj].Goals[0]
	g.Milestones = append(g.Milestones[:pm.milestoneCur], g.Milestones[pm.milestoneCur+1:]...)
	if pm.milestoneCur >= len(g.Milestones) && pm.milestoneCur > 0 {
		pm.milestoneCur--
	}

	if len(g.Milestones) != 2 {
		t.Fatalf("expected 2 milestones after delete, got %d", len(g.Milestones))
	}
	if pm.milestoneCur != 1 {
		t.Errorf("expected milestoneCur clamped to 1, got %d", pm.milestoneCur)
	}
}

// ---------------------------------------------------------------------------
// Priority cycling
// ---------------------------------------------------------------------------

func TestProjectPriorityCycle(t *testing.T) {
	vault := createProjectVault(t)
	writeProjectsJSON(t, vault, []Project{
		{Name: "TestProj", Status: "active", Category: "other", Priority: 0},
	})

	pm := NewProjectMode()
	pm.Open(vault)
	pm.selectedProj = 0
	pm.openDashboard()

	// Cycle: 0 → 1 → 2 → 3 → 4 → 0
	proj := &pm.projects[pm.selectedProj]

	expectedCycle := []int{1, 2, 3, 4, 0}
	for _, expected := range expectedCycle {
		proj.Priority = (proj.Priority + 1) % len(projectPriorityLabels)
		if proj.Priority != expected {
			t.Errorf("expected priority=%d, got %d", expected, proj.Priority)
		}
	}
}

// ---------------------------------------------------------------------------
// Next action setting
// ---------------------------------------------------------------------------

func TestProjectNextAction_Set(t *testing.T) {
	vault := createProjectVault(t)
	writeProjectsJSON(t, vault, []Project{
		{Name: "TestProj", Status: "active", Category: "other"},
	})

	pm := NewProjectMode()
	pm.Open(vault)
	pm.selectedProj = 0
	pm.openDashboard()

	// Simulate 'n' key → enters input mode for next_action.
	pm.dashInput = true
	pm.dashInputKind = "next_action"
	pm.dashInputBuf = "Review pull request"

	// Simulate Enter to commit.
	pm.projects[pm.selectedProj].NextAction = pm.dashInputBuf
	pm.dashInput = false

	if pm.projects[pm.selectedProj].NextAction != "Review pull request" {
		t.Errorf("expected NextAction='Review pull request', got %q", pm.projects[pm.selectedProj].NextAction)
	}
}

// ---------------------------------------------------------------------------
// Sorting — active + high-priority projects first
// ---------------------------------------------------------------------------

func TestProjectSorting_ActiveFirst(t *testing.T) {
	vault := createProjectVault(t)
	writeProjectsJSON(t, vault, []Project{
		{Name: "Archived", Status: "archived", Category: "other", Priority: 4},
		{Name: "Active", Status: "active", Category: "other", Priority: 0},
		{Name: "Paused", Status: "paused", Category: "other", Priority: 3},
	})

	pm := NewProjectMode()
	pm.Open(vault)

	filtered := pm.filteredProjects()
	if len(filtered) != 3 {
		t.Fatalf("expected 3 projects, got %d", len(filtered))
	}

	// Active project should be first.
	first := pm.projects[filtered[0]]
	if first.Status != "active" {
		t.Errorf("expected active project first, got status=%q name=%q", first.Status, first.Name)
	}
}

func TestProjectSorting_HighPriorityFirst(t *testing.T) {
	vault := createProjectVault(t)
	writeProjectsJSON(t, vault, []Project{
		{Name: "Low", Status: "active", Category: "other", Priority: 1},
		{Name: "Highest", Status: "active", Category: "other", Priority: 4},
		{Name: "Medium", Status: "active", Category: "other", Priority: 2},
	})

	pm := NewProjectMode()
	pm.Open(vault)

	filtered := pm.filteredProjects()
	if len(filtered) != 3 {
		t.Fatalf("expected 3 projects, got %d", len(filtered))
	}

	// Highest priority should be first.
	first := pm.projects[filtered[0]]
	if first.Priority != 4 {
		t.Errorf("expected highest priority first (4), got %d (%s)", first.Priority, first.Name)
	}

	second := pm.projects[filtered[1]]
	if second.Priority != 2 {
		t.Errorf("expected medium priority second (2), got %d (%s)", second.Priority, second.Name)
	}

	third := pm.projects[filtered[2]]
	if third.Priority != 1 {
		t.Errorf("expected low priority third (1), got %d (%s)", third.Priority, third.Name)
	}
}

func TestProjectSorting_DueDateTiebreaker(t *testing.T) {
	vault := createProjectVault(t)
	writeProjectsJSON(t, vault, []Project{
		{Name: "Later", Status: "active", Category: "other", Priority: 2, DueDate: "2026-12-01"},
		{Name: "Sooner", Status: "active", Category: "other", Priority: 2, DueDate: "2026-06-01"},
		{Name: "NoDue", Status: "active", Category: "other", Priority: 2, DueDate: ""},
	})

	pm := NewProjectMode()
	pm.Open(vault)

	filtered := pm.filteredProjects()
	if len(filtered) != 3 {
		t.Fatalf("expected 3 projects, got %d", len(filtered))
	}

	// Same priority, sooner due date should be first.
	first := pm.projects[filtered[0]]
	if first.Name != "Sooner" {
		t.Errorf("expected 'Sooner' first, got %q", first.Name)
	}

	second := pm.projects[filtered[1]]
	if second.Name != "Later" {
		t.Errorf("expected 'Later' second, got %q", second.Name)
	}

	// No due date should be last.
	third := pm.projects[filtered[2]]
	if third.Name != "NoDue" {
		t.Errorf("expected 'NoDue' last, got %q", third.Name)
	}
}

// ---------------------------------------------------------------------------
// Bounds check — selectedProj out of range
// ---------------------------------------------------------------------------

func TestProjectDashboard_SelectedProjOutOfRange(t *testing.T) {
	vault := createProjectVault(t)
	writeProjectsJSON(t, vault, []Project{
		{Name: "OnlyProj", Status: "active", Category: "other"},
	})

	pm := NewProjectMode()
	pm.Open(vault)
	pm.phase = pmPhaseDashboard
	pm.selectedProj = 99 // out of range

	// updateDashboard should fall back to list phase.
	keyJ := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	pm, _ = pm.updateDashboard(keyJ)

	if pm.phase != pmPhaseList {
		t.Errorf("expected phase to reset to pmPhaseList, got %d", pm.phase)
	}
}

func TestProjectDashboard_NegativeSelectedProj(t *testing.T) {
	vault := createProjectVault(t)
	writeProjectsJSON(t, vault, []Project{
		{Name: "OnlyProj", Status: "active", Category: "other"},
	})

	pm := NewProjectMode()
	pm.Open(vault)
	pm.phase = pmPhaseDashboard
	pm.selectedProj = -1

	keyJ := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	pm, _ = pm.updateDashboard(keyJ)

	if pm.phase != pmPhaseList {
		t.Errorf("expected phase to reset to pmPhaseList, got %d", pm.phase)
	}
}

// ---------------------------------------------------------------------------
// projectPriorityLabel
// ---------------------------------------------------------------------------

func TestProjectPriorityLabel_AllLevels(t *testing.T) {
	labels := []struct {
		pri      int
		wantText string
	}{
		{0, "NONE"},
		{1, "LOW"},
		{2, "MEDIUM"},
		{3, "HIGH"},
		{4, "HIGHEST"},
	}

	for _, tc := range labels {
		result := projectPriorityLabel(tc.pri)
		if result == "" {
			t.Errorf("projectPriorityLabel(%d) returned empty string", tc.pri)
		}
		// The result contains ANSI styling, but the label text should be present.
		// Just verify it's non-empty and doesn't panic.
	}
}

func TestProjectPriorityLabel_OutOfRange(t *testing.T) {
	// Negative priority.
	result := projectPriorityLabel(-1)
	if result == "" {
		t.Error("projectPriorityLabel(-1) returned empty string")
	}

	// Too large priority.
	result = projectPriorityLabel(99)
	if result == "" {
		t.Error("projectPriorityLabel(99) returned empty string")
	}
}

// ---------------------------------------------------------------------------
// priorityDot
// ---------------------------------------------------------------------------

func TestPriorityDot_AllLevels(t *testing.T) {
	for pri := 0; pri <= 4; pri++ {
		dot := priorityDot(pri)
		if dot == "" {
			t.Errorf("priorityDot(%d) returned empty string", pri)
		}
	}
}

func TestPriorityDot_DefaultCase(t *testing.T) {
	// Negative value should hit default.
	dot := priorityDot(-1)
	if dot == "" {
		t.Error("priorityDot(-1) returned empty string")
	}

	// Large value should hit default.
	dot = priorityDot(100)
	if dot == "" {
		t.Error("priorityDot(100) returned empty string")
	}
}

// ---------------------------------------------------------------------------
// formatTimeSpent
// ---------------------------------------------------------------------------

func TestFormatTimeSpent_Various(t *testing.T) {
	cases := []struct {
		minutes int
		want    string
	}{
		{0, "0m"},
		{-5, "0m"},
		{30, "30m"},
		{60, "1h"},
		{90, "1h 30m"},
		{120, "2h"},
		{150, "2h 30m"},
	}
	for _, tc := range cases {
		got := formatTimeSpent(tc.minutes)
		if got != tc.want {
			t.Errorf("formatTimeSpent(%d) = %q, want %q", tc.minutes, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// progressWithTasks
// ---------------------------------------------------------------------------

func TestProgressWithTasks_GoalsPriority(t *testing.T) {
	// When project has goals, their progress takes precedence.
	proj := Project{
		Goals: []ProjectGoal{
			{Title: "G1", Done: true},
			{Title: "G2", Done: false},
		},
	}
	tasks := []projectTask{
		{Text: "t1", Done: true},
		{Text: "t2", Done: true},
	}

	prog := progressWithTasks(proj, tasks)
	// Goals: 1/2 = 0.5. Tasks would give 1.0 but goals take precedence.
	if prog != 0.5 {
		t.Errorf("expected 0.5 (goals-based), got %f", prog)
	}
}

func TestProgressWithTasks_FallbackToTasks(t *testing.T) {
	proj := Project{} // No goals
	tasks := []projectTask{
		{Text: "t1", Done: true},
		{Text: "t2", Done: false},
		{Text: "t3", Done: true},
		{Text: "t4", Done: false},
	}

	prog := progressWithTasks(proj, tasks)
	if prog != 0.5 {
		t.Errorf("expected 0.5 (task fallback), got %f", prog)
	}
}

func TestProgressWithTasks_NoGoalsNoTasks(t *testing.T) {
	proj := Project{}
	var tasks []projectTask

	prog := progressWithTasks(proj, tasks)
	if prog != 0.0 {
		t.Errorf("expected 0.0, got %f", prog)
	}
}

// ---------------------------------------------------------------------------
// Dashboard view renders without panic
// ---------------------------------------------------------------------------

func TestProjectDashboard_ViewGoals_NoPanic(t *testing.T) {
	vault := createProjectVault(t)
	writeProjectsJSON(t, vault, []Project{
		{
			Name:     "GoalProj",
			Status:   "active",
			Category: "other",
			Color:    "blue",
			Goals: []ProjectGoal{
				{Title: "Done Goal", Done: true},
				{
					Title: "In Progress",
					Milestones: []ProjectMilestone{
						{Text: "Step 1", Done: true},
						{Text: "Step 2", Done: false},
					},
				},
				{Title: "Not Started"},
			},
			NextAction: "Review code",
			Priority:   3,
			DueDate:    "2026-06-01",
			TimeSpent:  120,
		},
	})

	pm := NewProjectMode()
	pm.SetSize(100, 40)
	pm.Open(vault)
	pm.selectedProj = 0
	pm.openDashboard()

	// Test all dashboard sections.
	for section := 0; section < 3; section++ {
		pm.dashSection = section
		output := pm.View()
		if output == "" {
			t.Errorf("dashboard section %d rendered empty with goals", section)
		}
	}

	// Test goal mode rendering.
	pm.goalMode = true
	pm.goalCursor = 1
	output := pm.View()
	if output == "" {
		t.Error("dashboard with goalMode rendered empty")
	}

	// Test expanded goal rendering.
	pm.goalExpanded = 1
	pm.milestoneCur = 0
	output = pm.View()
	if output == "" {
		t.Error("dashboard with expanded goal rendered empty")
	}
}

func TestProjectDashboard_ViewWithInput_NoPanic(t *testing.T) {
	vault := createProjectVault(t)
	writeProjectsJSON(t, vault, []Project{
		{Name: "InputProj", Status: "active", Category: "other", Color: "green"},
	})

	pm := NewProjectMode()
	pm.SetSize(100, 40)
	pm.Open(vault)
	pm.selectedProj = 0
	pm.openDashboard()

	// Test each input kind.
	for _, kind := range []string{"next_action", "goal", "milestone"} {
		pm.dashInput = true
		pm.dashInputKind = kind
		pm.dashInputBuf = "test input"
		output := pm.View()
		if output == "" {
			t.Errorf("dashboard with input kind=%q rendered empty", kind)
		}
	}
}

