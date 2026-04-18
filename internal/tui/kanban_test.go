package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

// Regression: pressing 'x' on an empty kanban must not panic.
func TestKanban_ToggleDone_EmptyDoesNotPanic(t *testing.T) {
	kb := NewKanban()
	// Force columns to be empty so colCursor=0 indexes nothing.
	kb.columns = nil
	kb.colCursor = 0
	kb.cardCursor = 0
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("kbToggleDone panicked on empty kanban: %v", r)
		}
	}()
	kb.kbToggleDone()
}

// Regression: invalid colCursor must not panic kbToggleDone.
func TestKanban_ToggleDone_InvalidCursorDoesNotPanic(t *testing.T) {
	kb := NewKanban()
	noteContents := map[string]string{"a.md": "- [ ] task"}
	kb.SetTasks(noteContents)
	kb.colCursor = -1
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("kbToggleDone panicked with negative colCursor: %v", r)
		}
	}()
	kb.kbToggleDone()
}

// ── New cursor actions: open / delete / priority cycle ──

func TestKanban_OpenKey_QueuesOpenRequestAndCloses(t *testing.T) {
	kb := NewKanban()
	kb.active = true
	kb.colCursor = 0
	kb.cardCursor = 0
	kb.columns[0].Cards = []KanbanCard{{Text: "Ship", Source: "Tasks.md", Line: 5}}

	kb, _ = kb.Update(teaKey('o'))

	if kb.active {
		t.Error("o should close the overlay")
	}
	notePath, line, ok := kb.GetOpenRequest()
	if !ok || notePath != "Tasks.md" || line != 5 {
		t.Errorf("expected open(Tasks.md:5), got (%q, %d, %v)", notePath, line, ok)
	}
}

func TestKanban_DeleteKey_QueuesDeleteRequest(t *testing.T) {
	kb := NewKanban()
	kb.active = true
	kb.colCursor = 0
	kb.cardCursor = 0
	kb.columns[0].Cards = []KanbanCard{{Text: "Drop me", Source: "Tasks.md", Line: 9}}

	kb, _ = kb.Update(teaKey('d'))

	if !kb.active {
		t.Error("d should NOT close the overlay (user keeps editing)")
	}
	notePath, line, text, ok := kb.GetDeleteRequest()
	if !ok || notePath != "Tasks.md" || line != 9 || text != "Drop me" {
		t.Errorf("expected delete(Tasks.md:9, 'Drop me'), got (%q, %d, %q, %v)", notePath, line, text, ok)
	}
}

func TestKanban_PriorityKey_CyclesPriorityAndQueuesUpdate(t *testing.T) {
	kb := NewKanban()
	kb.active = true
	kb.colCursor = 0
	kb.cardCursor = 0
	kb.columns[0].Cards = []KanbanCard{{Text: "Plan", Source: "Tasks.md", Line: 3, Priority: 2}}

	kb, _ = kb.Update(teaKey('p'))

	if got := kb.columns[0].Cards[0].Priority; got != 3 {
		t.Errorf("expected priority cycled to 3 (med→high), got %d", got)
	}
	notePath, line, level, ok := kb.GetPriorityRequest()
	if !ok || notePath != "Tasks.md" || line != 3 || level != 3 {
		t.Errorf("expected priority(Tasks.md:3, 3), got (%q, %d, %d, %v)", notePath, line, level, ok)
	}
}

func TestKanban_PriorityCycle_WrapsToZero(t *testing.T) {
	kb := NewKanban()
	kb.active = true
	kb.colCursor = 0
	kb.cardCursor = 0
	kb.columns[0].Cards = []KanbanCard{{Text: "Last", Priority: 4}}

	kb, _ = kb.Update(teaKey('p'))

	if got := kb.columns[0].Cards[0].Priority; got != 0 {
		t.Errorf("highest (4) should wrap to none (0), got %d", got)
	}
}

func TestKanban_HasPendingActions_TracksAllKinds(t *testing.T) {
	kb := NewKanban()
	if kb.HasPendingActions() {
		t.Error("fresh kanban should have no pending actions")
	}
	kb.pendingDelete = true
	if !kb.HasPendingActions() {
		t.Error("should detect pending delete")
	}
	_ , _, _, _ = kb.GetDeleteRequest() // drain
	if kb.HasPendingActions() {
		t.Error("should be empty after drain")
	}
}

// ── setTaskLinePriority — line rewriting ──

func TestSetTaskLinePriority_AddsAndStripsEmoji(t *testing.T) {
	original := "- [ ] Ship the release"
	withHigh := setTaskLinePriority(original, 3)
	if !contains(withHigh, "⏫") {
		t.Errorf("level 3 should add ⏫, got %q", withHigh)
	}
	cleared := setTaskLinePriority(withHigh, 0)
	if contains(cleared, "⏫") {
		t.Errorf("level 0 should strip the emoji, got %q", cleared)
	}
	// Cycling from highest to medium should swap, not append.
	withHighest := setTaskLinePriority(original, 4)
	withMed := setTaskLinePriority(withHighest, 2)
	if contains(withMed, "🔺") {
		t.Errorf("med should strip prior 🔺, got %q", withMed)
	}
	if !contains(withMed, "🔼") {
		t.Errorf("med should add 🔼, got %q", withMed)
	}
}

// teaKey builds a single-rune key message for tests.
func teaKey(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

// contains is a tiny stand-in for strings.Contains used by these tests.
func contains(s, substr string) bool { return len(s) >= len(substr) && (substr == "" || stringsIndex(s, substr) >= 0) }

func stringsIndex(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
