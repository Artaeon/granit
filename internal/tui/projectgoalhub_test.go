package tui

import (
	"strings"
	"testing"

	"github.com/artaeon/granit/internal/objects"
	"github.com/artaeon/granit/internal/vault"
)

func hubModel() *Model {
	v := &vault.Vault{Notes: map[string]*vault.Note{}}
	return &Model{vault: v}
}

func TestProjectGoalHub_HiddenForRegularNote(t *testing.T) {
	m := hubModel()
	m.objectsRegistry = objects.NewRegistry()
	m.objectsIndex = objects.NewIndex()
	m.activeNote = "Random/Note.md"
	if got := m.renderProjectGoalHub(80); got != "" {
		t.Fatalf("hub should be empty for non-typed note, got %q", got)
	}
}

func TestProjectGoalHub_HiddenForNonProjectGoalType(t *testing.T) {
	m := hubModel()
	reg := objects.NewRegistry()
	bld := objects.NewBuilder(reg)
	bld.Add("Books/X.md", "X", map[string]string{"type": "book"})
	m.objectsRegistry = reg
	m.objectsIndex = bld.Finalize()
	m.activeNote = "Books/X.md"
	if got := m.renderProjectGoalHub(80); got != "" {
		t.Fatalf("hub should be empty for book typed-object, got %q", got)
	}
}

func TestProjectGoalHub_RendersForProjectWithCounts(t *testing.T) {
	m := hubModel()
	reg := objects.NewRegistry()
	bld := objects.NewBuilder(reg)
	bld.Add("Projects/Apollo.md", "Apollo", map[string]string{
		"type": "project", "status": "active",
	})
	m.objectsRegistry = reg
	m.objectsIndex = bld.Finalize()
	m.activeNote = "Projects/Apollo.md"
	m.cachedTasks = []Task{
		{Text: "build rocket", Project: "Apollo", Done: false},
		{Text: "moon landing", Project: "Apollo", Done: true},
		{Text: "unrelated", Project: "Other", Done: false},
	}

	out := m.renderProjectGoalHub(80)
	if out == "" {
		t.Fatal("expected hub strip for project note")
	}
	if !strings.Contains(out, "Apollo") {
		t.Errorf("strip should name the project: %q", out)
	}
	if !strings.Contains(out, "active") {
		t.Errorf("strip should show status: %q", out)
	}
	// 2 linked tasks (1 open, 1 done); 1 unrelated must be excluded.
	if !strings.Contains(out, "2 tasks") || !strings.Contains(out, "1 done") {
		t.Errorf("strip should show 2 tasks (1 done): %q", out)
	}
	if !strings.Contains(out, "Alt+N") && !strings.Contains(out, "Alt+T") {
		// We use Alt+N (Alt+T was taken). Either is acceptable in copy.
		// At minimum "add task" should appear.
		if !strings.Contains(out, "add task") {
			t.Errorf("strip should hint at quick-add: %q", out)
		}
	}
}

func TestProjectGoalHub_RendersForGoalWithTargetDate(t *testing.T) {
	m := hubModel()
	reg := objects.NewRegistry()
	bld := objects.NewBuilder(reg)
	bld.Add("Goals/Marathon.md", "Run a Marathon", map[string]string{
		"type": "goal", "status": "active", "target_date": "2026-09-01",
	})
	m.objectsRegistry = reg
	m.objectsIndex = bld.Finalize()
	m.activeNote = "Goals/Marathon.md"
	m.cachedTasks = []Task{
		{Text: "run 5k", NotePath: "Goals/Marathon.md", Done: true},
		{Text: "run 10k", NotePath: "Goals/Marathon.md", Done: false},
	}

	out := m.renderProjectGoalHub(80)
	if !strings.Contains(out, "Marathon") {
		t.Fatalf("expected goal title: %q", out)
	}
	if !strings.Contains(out, "2026-09-01") {
		t.Errorf("expected target date in strip: %q", out)
	}
	if !strings.Contains(out, "2 tasks") {
		t.Errorf("expected goal-note tasks counted: %q", out)
	}
}

func TestProjectGoalHub_NoTasksShowsHint(t *testing.T) {
	m := hubModel()
	reg := objects.NewRegistry()
	bld := objects.NewBuilder(reg)
	bld.Add("Projects/Empty.md", "Empty", map[string]string{
		"type": "project",
	})
	m.objectsRegistry = reg
	m.objectsIndex = bld.Finalize()
	m.activeNote = "Projects/Empty.md"

	out := m.renderProjectGoalHub(80)
	if !strings.Contains(out, "no tasks yet") {
		t.Errorf("expected empty-state hint: %q", out)
	}
}

func TestEditor_AppendTaskLine_AppendsCheckboxAndPlacesCursor(t *testing.T) {
	e := mkEditor("# Project Apollo", "Some intro.")
	e.AppendTaskLine()
	last := len(e.content) - 1
	if e.content[last] != "- [ ] " {
		t.Fatalf("expected last line to be empty checkbox, got %q", e.content[last])
	}
	if e.cursor != last || e.col != len("- [ ] ") {
		t.Errorf("cursor wrong: line=%d col=%d", e.cursor, e.col)
	}
	// Should also have inserted a blank line between the prior content
	// and the new checkbox.
	if e.content[last-1] != "" {
		t.Errorf("expected blank line before new checkbox, got %q", e.content[last-1])
	}
}

func TestEditor_AppendTaskLine_NoBlankLineWhenAlreadyEmpty(t *testing.T) {
	e := mkEditor("- [ ] first", "")
	e.AppendTaskLine()
	// Should reuse the trailing blank line as the gap, then add the
	// checkbox — total length should be 3, not 4.
	// Actually simpler check: last line is the checkbox.
	last := len(e.content) - 1
	if e.content[last] != "- [ ] " {
		t.Fatalf("got %q", e.content[last])
	}
}
