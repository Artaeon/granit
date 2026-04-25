package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/tasks"
)

func mkTriageStore(t *testing.T, taskLines []string) *tasks.TaskStore {
	t.Helper()
	vault := t.TempDir()
	tasksMd := strings.Join(taskLines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(vault, "Tasks.md"), []byte(tasksMd), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := tasks.Load(vault, func() []tasks.NoteContent {
		data, _ := os.ReadFile(filepath.Join(vault, "Tasks.md"))
		return []tasks.NoteContent{{Path: "Tasks.md", Content: string(data)}}
	})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestTriageQueue_OpenSnapshotsInboxTasks(t *testing.T) {
	store := mkTriageStore(t, []string{
		"- [ ] alpha",
		"- [ ] beta",
		"- [ ] gamma",
	})
	q := NewTriageQueue(store)
	q.Open()
	if !q.IsActive() {
		t.Error("Open did not activate")
	}
	if len(q.inbox) != 3 {
		t.Errorf("expected 3 inbox tasks, got %d", len(q.inbox))
	}
}

func TestTriageQueue_TriageAdvancesAndPersists(t *testing.T) {
	store := mkTriageStore(t, []string{"- [ ] one", "- [ ] two"})
	q := NewTriageQueue(store)
	q.Open()
	firstID := q.inbox[0].ID

	q, _ = q.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})

	if q.cursor != 1 {
		t.Errorf("cursor should advance to 1 after t, got %d", q.cursor)
	}
	got, _ := store.GetByID(firstID)
	if got.Triage != "triaged" {
		t.Errorf("first task triage: got %q want triaged", got.Triage)
	}
}

func TestTriageQueue_DropMarksDropped(t *testing.T) {
	store := mkTriageStore(t, []string{"- [ ] kill me"})
	q := NewTriageQueue(store)
	q.Open()
	id := q.inbox[0].ID
	q, _ = q.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	got, _ := store.GetByID(id)
	if got.Triage != tasks.TriageDropped {
		t.Errorf("triage state: got %q want dropped", got.Triage)
	}
}

func TestTriageQueue_ScheduleOpensPickerThenAppliesChoice(t *testing.T) {
	store := mkTriageStore(t, []string{"- [ ] schedule me"})
	q := NewTriageQueue(store)
	q.Open()
	id := q.inbox[0].ID
	// `s` opens the picker — task should NOT be triaged yet.
	q, _ = q.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if !q.picker.IsActive() {
		t.Fatal("expected picker active after s")
	}
	got, _ := store.GetByID(id)
	if got.Triage == tasks.TriageScheduled {
		t.Error("schedule should NOT apply until user picks a duration")
	}
	// Pick "today" (key 1).
	q, _ = q.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	if q.picker.IsActive() {
		t.Error("picker should close after pick")
	}
	got, _ = store.GetByID(id)
	if got.Triage != tasks.TriageScheduled {
		t.Errorf("triage: got %q want scheduled", got.Triage)
	}
	if got.ScheduledStart == nil {
		t.Error("ScheduledStart should be set after picking today")
	}
}

func TestTriageQueue_SnoozeOpensPickerThenAppliesChoice(t *testing.T) {
	store := mkTriageStore(t, []string{"- [ ] later"})
	q := NewTriageQueue(store)
	q.Open()
	id := q.inbox[0].ID
	q, _ = q.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
	if !q.picker.IsActive() {
		t.Fatal("expected picker active after z")
	}
	// Pick "+3d" (key 2 in snoozeOptions = 3 days).
	q, _ = q.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	got, _ := store.GetByID(id)
	if got.Triage != tasks.TriageSnoozed {
		t.Errorf("triage: got %q want snoozed", got.Triage)
	}
	if got.ScheduledStart == nil {
		t.Fatal("ScheduledStart should be set after pick")
	}
	// Should be roughly 3 days in the future.
	hoursAhead := got.ScheduledStart.Sub(time.Now()).Hours()
	if hoursAhead < 70 || hoursAhead > 74 {
		t.Errorf("ScheduledStart should be ~72h ahead, got %.1fh", hoursAhead)
	}
}

func TestTriageQueue_PickerEscCancelsWithoutChange(t *testing.T) {
	store := mkTriageStore(t, []string{"- [ ] keep me"})
	q := NewTriageQueue(store)
	q.Open()
	id := q.inbox[0].ID
	q, _ = q.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	q, _ = q.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if q.picker.IsActive() {
		t.Error("esc should close picker")
	}
	got, _ := store.GetByID(id)
	if got.Triage == tasks.TriageScheduled {
		t.Error("esc on picker must not commit a schedule")
	}
	// Cursor should NOT have advanced — the user cancelled.
	if q.cursor != 0 {
		t.Errorf("cursor advanced after picker cancel: %d", q.cursor)
	}
}

func TestTriageQueue_SpaceSkipsWithoutMutating(t *testing.T) {
	store := mkTriageStore(t, []string{"- [ ] keep state", "- [ ] next"})
	q := NewTriageQueue(store)
	q.Open()
	firstID := q.inbox[0].ID
	q, _ = q.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if q.cursor != 1 {
		t.Errorf("cursor should advance, got %d", q.cursor)
	}
	got, _ := store.GetByID(firstID)
	if got.Triage != "" && got.Triage != tasks.TriageInbox {
		t.Errorf("space should not change triage, got %q", got.Triage)
	}
}

func TestTriageQueue_AutoCloseAfterLast(t *testing.T) {
	store := mkTriageStore(t, []string{"- [ ] only one"})
	q := NewTriageQueue(store)
	q.Open()
	q, _ = q.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if !q.AutoClose() {
		t.Error("expected AutoClose to fire after last task")
	}
	if q.IsActive() {
		t.Error("queue should be closed after AutoClose")
	}
}

func TestTriageQueue_OpenRequestPersistsAcrossPendingOpen(t *testing.T) {
	store := mkTriageStore(t, []string{"- [ ] open me"})
	q := NewTriageQueue(store)
	q.Open()
	q, _ = q.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if q.IsActive() {
		t.Error("o should close the queue")
	}
	path, ok := q.PendingOpen()
	if !ok {
		t.Fatal("PendingOpen should report a path")
	}
	if path != "Tasks.md" {
		t.Errorf("path: got %q want Tasks.md", path)
	}
	// Consumed-once
	if _, ok := q.PendingOpen(); ok {
		t.Error("PendingOpen should be consumed-once")
	}
}

func TestTriageQueue_EmptyInboxRendersZeroMessage(t *testing.T) {
	store := mkTriageStore(t, nil)
	q := NewTriageQueue(store)
	q.Open()
	out := q.View()
	if !strings.Contains(out, "Inbox zero") {
		t.Errorf("empty inbox should mention 'Inbox zero', got: %q", out)
	}
}

func TestTriageQueue_KbackMovesCursorBack(t *testing.T) {
	store := mkTriageStore(t, []string{"- [ ] a", "- [ ] b"})
	q := NewTriageQueue(store)
	q.Open()
	q, _ = q.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if q.cursor != 1 {
		t.Fatalf("expected cursor=1, got %d", q.cursor)
	}
	q, _ = q.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if q.cursor != 0 {
		t.Errorf("expected cursor=0 after k, got %d", q.cursor)
	}
}

func TestTriageQueue_QClosesWithoutMutation(t *testing.T) {
	store := mkTriageStore(t, []string{"- [ ] one"})
	q := NewTriageQueue(store)
	q.Open()
	id := q.inbox[0].ID
	q, _ = q.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if q.IsActive() {
		t.Error("q should close")
	}
	got, _ := store.GetByID(id)
	if got.Triage != "" && got.Triage != tasks.TriageInbox {
		t.Errorf("q must not mutate triage, got %q", got.Triage)
	}
}
