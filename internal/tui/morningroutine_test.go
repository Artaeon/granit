package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// getSelectedTasks — due-suffix deduplication
// ---------------------------------------------------------------------------

func TestGetSelectedTasks_BasicSelection(t *testing.T) {
	mr := MorningRoutine{
		allTasks: []Task{
			{Text: "Buy groceries", DueDate: "2026-04-09"},
			{Text: "Call dentist", DueDate: ""},
			{Text: "Submit report", DueDate: "2026-04-10"},
		},
		taskSelected: []bool{true, true, false},
	}

	result := mr.getSelectedTasks()
	if len(result) != 2 {
		t.Fatalf("expected 2 selected tasks, got %d", len(result))
	}
	if result[0] != "Buy groceries (due: 2026-04-09)" {
		t.Errorf("expected due suffix, got %q", result[0])
	}
	if result[1] != "Call dentist" {
		t.Errorf("expected no due suffix for empty date, got %q", result[1])
	}
}

func TestGetSelectedTasks_NoDuplicateDueSuffix(t *testing.T) {
	mr := MorningRoutine{
		allTasks: []Task{
			{Text: "NOR: online shop live 📅 2026-03-31 (due: 2026-03-31)", DueDate: "2026-03-31"},
		},
		taskSelected: []bool{true},
	}

	result := mr.getSelectedTasks()
	if len(result) != 1 {
		t.Fatalf("expected 1 task, got %d", len(result))
	}
	// Must NOT double the (due: ...) suffix
	count := strings.Count(result[0], "(due: 2026-03-31)")
	if count != 1 {
		t.Errorf("expected exactly 1 occurrence of due suffix, got %d in %q", count, result[0])
	}
}

func TestGetSelectedTasks_IncludesCreatedTasks(t *testing.T) {
	mr := MorningRoutine{
		allTasks:     []Task{{Text: "Existing task", DueDate: ""}},
		taskSelected: []bool{false},
		createdTasks: []string{"New task from morning routine"},
	}

	result := mr.getSelectedTasks()
	if len(result) != 1 {
		t.Fatalf("expected 1 task (created), got %d", len(result))
	}
	if result[0] != "New task from morning routine" {
		t.Errorf("expected created task, got %q", result[0])
	}
}

func TestGetSelectedTasks_NoneSelected(t *testing.T) {
	mr := MorningRoutine{
		allTasks:     []Task{{Text: "A"}, {Text: "B"}},
		taskSelected: []bool{false, false},
	}

	result := mr.getSelectedTasks()
	if len(result) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// getSelectedHabits
// ---------------------------------------------------------------------------

func TestGetSelectedHabits(t *testing.T) {
	mr := MorningRoutine{
		allHabits:     []habitEntry{{Name: "Gym"}, {Name: "Pray"}, {Name: "Write"}},
		habitSelected: []bool{true, false, true},
	}

	result := mr.getSelectedHabits()
	if len(result) != 2 {
		t.Fatalf("expected 2 habits, got %d", len(result))
	}
	if result[0] != "Gym" || result[1] != "Write" {
		t.Errorf("expected [Gym, Write], got %v", result)
	}
}

// ---------------------------------------------------------------------------
// buildDailyPlanMarkdown — output format
// ---------------------------------------------------------------------------

func TestBuildDailyPlanMarkdown_FullPlan(t *testing.T) {
	mr := MorningRoutine{
		scripture: Scripture{Text: "Be strong and courageous", Source: "Joshua 1:9"},
		todayGoal: "Ship the release",
		allTasks: []Task{
			{Text: "Fix bug #42", DueDate: "2026-04-09"},
			{Text: "Update docs", DueDate: ""},
		},
		taskSelected: []bool{true, true},
		allHabits:    []habitEntry{{Name: "Gym"}, {Name: "Pray"}},
		habitSelected: []bool{true, true},
		thoughts:     "Feeling focused today.",
	}

	md := mr.buildDailyPlanMarkdown()

	// Heading
	if !strings.HasPrefix(md, "## Daily Plan") {
		t.Error("expected markdown to start with ## Daily Plan heading")
	}

	// Scripture
	if !strings.Contains(md, "Be strong and courageous") {
		t.Error("expected scripture text")
	}
	if !strings.Contains(md, "Joshua 1:9") {
		t.Error("expected scripture source")
	}

	// Goal
	if !strings.Contains(md, "### Today's Goal") {
		t.Error("expected goal heading")
	}
	if !strings.Contains(md, "Ship the release") {
		t.Error("expected goal text")
	}

	// Tasks — emitted as plain bullets (no "[ ]" checkbox) so the daily-plan
	// recap doesn't get reparsed as fresh tasks by ParseAllTasks. The real
	// task lines live in the source notes; this section is informational.
	if !strings.Contains(md, "### Tasks") {
		t.Error("expected tasks heading")
	}
	if !strings.Contains(md, "- Fix bug #42 (due: 2026-04-09)") {
		t.Error("expected task with due date as plain bullet")
	}
	if !strings.Contains(md, "- Update docs") {
		t.Error("expected task without due date as plain bullet")
	}
	if strings.Contains(md, "- [ ] Fix bug #42") || strings.Contains(md, "- [ ] Update docs") {
		t.Errorf("tasks must not be emitted as '- [ ]' checkboxes — would duplicate into global task list. md:\n%s", md)
	}

	// Habits
	if !strings.Contains(md, "### Habits") {
		t.Error("expected habits heading")
	}
	if !strings.Contains(md, "- [ ] Gym") {
		t.Error("expected gym habit")
	}

	// Thoughts
	if !strings.Contains(md, "### Thoughts") {
		t.Error("expected thoughts heading")
	}
	if !strings.Contains(md, "Feeling focused today.") {
		t.Error("expected thoughts text")
	}
}

func TestBuildDailyPlanMarkdown_EmptyOptionalSections(t *testing.T) {
	mr := MorningRoutine{
		scripture:     Scripture{Text: "Test", Source: "Test 1:1"},
		todayGoal:     "",
		allTasks:      []Task{},
		taskSelected:  []bool{},
		allHabits:     []habitEntry{},
		habitSelected: []bool{},
		thoughts:      "",
	}

	md := mr.buildDailyPlanMarkdown()

	// Should have heading and scripture but NOT optional sections
	if !strings.Contains(md, "## Daily Plan") {
		t.Error("expected heading")
	}
	if strings.Contains(md, "### Today's Goal") {
		t.Error("goal section should be omitted when empty")
	}
	if strings.Contains(md, "### Tasks") {
		t.Error("tasks section should be omitted when no tasks selected")
	}
	if strings.Contains(md, "### Habits") {
		t.Error("habits section should be omitted when no habits selected")
	}
	if strings.Contains(md, "### Thoughts") {
		t.Error("thoughts section should be omitted when empty")
	}
}

func TestBuildDailyPlanMarkdown_TasksWithExistingDueSuffix(t *testing.T) {
	mr := MorningRoutine{
		scripture: Scripture{Text: "Test", Source: "Test 1:1"},
		allTasks: []Task{
			{Text: "Task A (due: 2026-04-09)", DueDate: "2026-04-09"},
		},
		taskSelected: []bool{true},
	}

	md := mr.buildDailyPlanMarkdown()

	// Count occurrences of the due suffix — should appear exactly once
	count := strings.Count(md, "(due: 2026-04-09)")
	if count != 1 {
		t.Errorf("expected 1 due suffix occurrence, got %d in:\n%s", count, md)
	}
}

// ---------------------------------------------------------------------------
// AI refine handoff — pressing 'P' on the complete screen flags the host
// to continue into Plan My Day.
// ---------------------------------------------------------------------------

func TestMorningRoutine_AIRefine_PressP_FlagsAndCloses(t *testing.T) {
	mr := NewMorningRoutine()
	mr.active = true
	mr.phase = morningComplete

	mr, _ = mr.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

	if mr.active {
		t.Error("pressing P should close the overlay")
	}
	if !mr.ConsumeAIRefineRequest() {
		t.Error("expected AI refine flag set after pressing P")
	}
	// Second call must return false — consumed-once semantics.
	if mr.ConsumeAIRefineRequest() {
		t.Error("flag should be cleared after first consume")
	}
}

func TestMorningRoutine_AIRefine_PressEnter_ClosesWithoutFlag(t *testing.T) {
	mr := NewMorningRoutine()
	mr.active = true
	mr.phase = morningComplete

	mr, _ = mr.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if mr.active {
		t.Error("enter should close the overlay")
	}
	if mr.ConsumeAIRefineRequest() {
		t.Error("enter must not set the AI refine flag")
	}
}
