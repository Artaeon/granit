package tui

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// AIProjectPlanner.parseAIResponse — tests the AI response parser that
// turns LLM output into a Project + parsed task list. This was 0% covered.
// ---------------------------------------------------------------------------

func newTestAIPlanner() *AIProjectPlanner {
	return &AIProjectPlanner{
		nameInput: "fallback name",
		descInput: "fallback description",
	}
}

func TestParseAIResponse_ProjectName(t *testing.T) {
	ap := newTestAIPlanner()
	ap.parseAIResponse("PROJECT: Build a TUI app")

	if ap.parsedProject.Name != "Build a TUI app" {
		t.Errorf("expected 'Build a TUI app', got %q", ap.parsedProject.Name)
	}
}

func TestParseAIResponse_FallsBackToUserInputForName(t *testing.T) {
	ap := newTestAIPlanner()
	ap.parseAIResponse("CATEGORY: development")

	if ap.parsedProject.Name != "fallback name" {
		t.Errorf("expected fallback name, got %q", ap.parsedProject.Name)
	}
}

func TestParseAIResponse_ValidCategoryAccepted(t *testing.T) {
	ap := newTestAIPlanner()
	ap.parseAIResponse("CATEGORY: development")

	if ap.parsedProject.Category != "development" {
		t.Errorf("expected 'development', got %q", ap.parsedProject.Category)
	}
}

func TestParseAIResponse_InvalidCategoryFallsBackToOther(t *testing.T) {
	ap := newTestAIPlanner()
	ap.parseAIResponse("CATEGORY: notarealcategory")

	if ap.parsedProject.Category != "other" {
		t.Errorf("expected 'other', got %q", ap.parsedProject.Category)
	}
}

func TestParseAIResponse_CategoryCaseInsensitive(t *testing.T) {
	ap := newTestAIPlanner()
	ap.parseAIResponse("CATEGORY: DEVELOPMENT")

	if ap.parsedProject.Category != "development" {
		t.Errorf("expected 'development', got %q", ap.parsedProject.Category)
	}
}

func TestParseAIResponse_TagsParsed(t *testing.T) {
	ap := newTestAIPlanner()
	ap.parseAIResponse("TAGS: alpha, beta, #gamma")

	if len(ap.parsedProject.Tags) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(ap.parsedProject.Tags))
	}
	// '#' prefix must be stripped.
	for _, tag := range ap.parsedProject.Tags {
		if strings.HasPrefix(tag, "#") {
			t.Errorf("tag %q should have leading # stripped", tag)
		}
	}
}

func TestParseAIResponse_FolderParsed(t *testing.T) {
	ap := newTestAIPlanner()
	ap.parseAIResponse("FOLDER: projects/my-app")

	if ap.parsedProject.Folder != "projects/my-app" {
		t.Errorf("expected 'projects/my-app', got %q", ap.parsedProject.Folder)
	}
}

func TestParseAIResponse_FolderFallback(t *testing.T) {
	ap := newTestAIPlanner()
	ap.nameInput = "My Cool Project"
	ap.parseAIResponse("PROJECT: My Cool Project")

	// Falls back to slugified name
	if ap.parsedProject.Folder != "my-cool-project" {
		t.Errorf("expected 'my-cool-project', got %q", ap.parsedProject.Folder)
	}
}

func TestParseAIResponse_PhasesAndMilestones(t *testing.T) {
	response := strings.Join([]string{
		"PROJECT: Test",
		"PHASES:",
		"Phase 1: Discovery (due: 2026-04-30)",
		"- [ ] Research existing tools",
		"- [ ] Write spec",
		"Phase 2: Build (due: 2026-05-31)",
		"- [ ] Implement core",
		"- [x] Set up CI",
	}, "\n")
	ap := newTestAIPlanner()
	ap.parseAIResponse(response)

	if len(ap.parsedProject.Goals) != 2 {
		t.Fatalf("expected 2 phases, got %d", len(ap.parsedProject.Goals))
	}
	if ap.parsedProject.Goals[0].Title != "Discovery" {
		t.Errorf("phase 1 title wrong: %q", ap.parsedProject.Goals[0].Title)
	}
	if len(ap.parsedProject.Goals[0].Milestones) != 2 {
		t.Errorf("expected 2 milestones in phase 1, got %d", len(ap.parsedProject.Goals[0].Milestones))
	}
	if len(ap.parsedProject.Goals[1].Milestones) != 2 {
		t.Errorf("expected 2 milestones in phase 2, got %d", len(ap.parsedProject.Goals[1].Milestones))
	}
}

func TestParseAIResponse_PhaseDueDateExtracted(t *testing.T) {
	response := strings.Join([]string{
		"PROJECT: Test",
		"PHASES:",
		"Phase 1: First (due: 2026-04-15)",
		"Phase 2: Second (due: 2026-06-30)",
	}, "\n")
	ap := newTestAIPlanner()
	ap.parseAIResponse(response)

	// Project DueDate should be the latest phase date
	if ap.parsedProject.DueDate != "2026-06-30" {
		t.Errorf("expected DueDate 2026-06-30, got %q", ap.parsedProject.DueDate)
	}
}

func TestParseAIResponse_DoneMilestonesMarkedDone(t *testing.T) {
	response := strings.Join([]string{
		"PROJECT: Test",
		"PHASES:",
		"Phase 1: First",
		"- [x] Already done",
		"- [ ] Not done",
		"- [X] Capital X",
	}, "\n")
	ap := newTestAIPlanner()
	ap.parseAIResponse(response)

	if len(ap.parsedProject.Goals) != 1 {
		t.Fatalf("expected 1 phase, got %d", len(ap.parsedProject.Goals))
	}
	ms := ap.parsedProject.Goals[0].Milestones
	if len(ms) != 3 {
		t.Fatalf("expected 3 milestones, got %d", len(ms))
	}
	if !ms[0].Done {
		t.Error("first milestone should be done (- [x])")
	}
	if ms[1].Done {
		t.Error("second milestone should NOT be done")
	}
	if !ms[2].Done {
		t.Error("third milestone should be done (- [X])")
	}
}

func TestParseAIResponse_TasksParsed(t *testing.T) {
	response := strings.Join([]string{
		"PROJECT: Test",
		"TASKS:",
		"- [ ] First task",
		"- [ ] Second task #urgent",
		"- [x] Already done task",
	}, "\n")
	ap := newTestAIPlanner()
	ap.parseAIResponse(response)

	if len(ap.parsedTasks) != 3 {
		t.Errorf("expected 3 parsed tasks, got %d", len(ap.parsedTasks))
	}
	if ap.parsedTasks[0] != "First task" {
		t.Errorf("first task wrong: %q", ap.parsedTasks[0])
	}
}

func TestParseAIResponse_DefaultsForMissingFields(t *testing.T) {
	ap := newTestAIPlanner()
	ap.parseAIResponse("PROJECT: Just a name")

	// Status, Color, Priority should have defaults set in parseAIResponse.
	if ap.parsedProject.Status != "active" {
		t.Errorf("expected status 'active', got %q", ap.parsedProject.Status)
	}
	if ap.parsedProject.Color != "blue" {
		t.Errorf("expected color 'blue', got %q", ap.parsedProject.Color)
	}
	if ap.parsedProject.Priority != 2 {
		t.Errorf("expected priority 2, got %d", ap.parsedProject.Priority)
	}
	if ap.parsedProject.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
}

func TestParseAIResponse_EmptyResponse(t *testing.T) {
	ap := newTestAIPlanner()
	ap.parseAIResponse("")

	// Should still produce a valid Project struct via fallbacks.
	if ap.parsedProject.Name != "fallback name" {
		t.Errorf("expected fallback name on empty response, got %q", ap.parsedProject.Name)
	}
	if ap.parsedProject.Category != "other" {
		t.Errorf("expected 'other' category fallback, got %q", ap.parsedProject.Category)
	}
}

// Regression: parseAIResponse must reset parsedTasks each call so a
// second invocation does not accumulate tasks from the first.
func TestParseAIResponse_ResetsParsedTasks(t *testing.T) {
	ap := newTestAIPlanner()
	ap.parseAIResponse("PROJECT: A\nTASKS:\n- [ ] one")
	ap.parseAIResponse("PROJECT: B\nTASKS:\n- [ ] two")

	if len(ap.parsedTasks) != 1 {
		t.Errorf("expected 1 task after second parse, got %d (tasks=%v)", len(ap.parsedTasks), ap.parsedTasks)
	}
	if ap.parsedTasks[0] != "two" {
		t.Errorf("expected 'two', got %q", ap.parsedTasks[0])
	}
}

// Regression: tags with extra whitespace must be trimmed.
func TestParseAIResponse_TagsTrimmed(t *testing.T) {
	ap := newTestAIPlanner()
	ap.parseAIResponse("TAGS:   alpha  ,   beta   , gamma")

	if len(ap.parsedProject.Tags) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(ap.parsedProject.Tags))
	}
	for _, tag := range ap.parsedProject.Tags {
		if tag != strings.TrimSpace(tag) {
			t.Errorf("tag %q has untrimmed whitespace", tag)
		}
	}
}
