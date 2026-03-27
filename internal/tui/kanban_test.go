package tui

import (
	"testing"
)

// ---------------------------------------------------------------------------
// SetTaskProjects — verify cards get project names from matched tasks
// ---------------------------------------------------------------------------

func TestKanban_SetTaskProjects(t *testing.T) {
	kb := NewKanban()

	noteContents := map[string]string{
		"alpha/tasks.md": "- [ ] Task one\n- [x] Task two",
		"beta/tasks.md":  "- [ ] Task three",
	}
	kb.SetTasks(noteContents)

	// Verify cards were created
	totalCards := 0
	for _, col := range kb.columns {
		totalCards += len(col.Cards)
	}
	if totalCards != 3 {
		t.Fatalf("expected 3 cards, got %d", totalCards)
	}

	// Apply project names via matching tasks
	tasks := []Task{
		{Text: "Task one", NotePath: "alpha/tasks.md", LineNum: 1, Project: "Alpha"},
		{Text: "Task two", NotePath: "alpha/tasks.md", LineNum: 2, Project: "Alpha"},
		{Text: "Task three", NotePath: "beta/tasks.md", LineNum: 1, Project: "Beta"},
	}
	kb.SetTaskProjects(tasks)

	// Check that cards in columns have project names
	for _, col := range kb.columns {
		for _, card := range col.Cards {
			switch card.Source {
			case "alpha/tasks.md":
				if card.Project != "Alpha" {
					t.Errorf("card from alpha/tasks.md: expected project 'Alpha', got %q", card.Project)
				}
			case "beta/tasks.md":
				if card.Project != "Beta" {
					t.Errorf("card from beta/tasks.md: expected project 'Beta', got %q", card.Project)
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Priority sorting — cards sorted by priority within columns
// ---------------------------------------------------------------------------

func TestKanban_PrioritySorting(t *testing.T) {
	kb := NewKanban()

	// Use emoji markers for priority
	noteContents := map[string]string{
		"tasks.md": "- [ ] Low priority \U0001F53D\n" +
			"- [ ] Highest priority \U0001F53A\n" +
			"- [ ] Medium priority \U0001F53C\n" +
			"- [ ] No priority\n" +
			"- [ ] High priority \u23EB",
	}
	kb.SetTasks(noteContents)

	// All tasks are undone and none have #wip, so they should all be in Backlog
	backlog := kb.columns[0].Cards
	if len(backlog) < 2 {
		t.Fatalf("expected at least 2 backlog cards, got %d", len(backlog))
	}

	// Verify sorted by priority descending
	for i := 1; i < len(backlog); i++ {
		if backlog[i].Priority > backlog[i-1].Priority {
			t.Errorf("card %d (priority %d) should not come after card %d (priority %d)",
				i, backlog[i].Priority, i-1, backlog[i-1].Priority)
		}
	}

	// The first card should be the highest priority
	if backlog[0].Priority != 4 {
		t.Errorf("expected first backlog card to have priority 4, got %d", backlog[0].Priority)
	}
}

// ---------------------------------------------------------------------------
// WIP tag routing
// ---------------------------------------------------------------------------

func TestKanban_WipTagRouting(t *testing.T) {
	kb := NewKanban()

	noteContents := map[string]string{
		"tasks.md": "- [ ] Regular task\n- [ ] Active task #wip\n- [x] Done task",
	}
	kb.SetTasks(noteContents)

	// Backlog: regular task
	if len(kb.columns[0].Cards) != 1 {
		t.Errorf("expected 1 backlog card, got %d", len(kb.columns[0].Cards))
	}
	// In Progress: #wip task
	if len(kb.columns[1].Cards) != 1 {
		t.Errorf("expected 1 in-progress card, got %d", len(kb.columns[1].Cards))
	}
	// Done: done task
	if len(kb.columns[2].Cards) != 1 {
		t.Errorf("expected 1 done card, got %d", len(kb.columns[2].Cards))
	}
}
