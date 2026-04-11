package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFocusSession_NewDefaults(t *testing.T) {
	fs := NewFocusSession()
	if fs.IsActive() {
		t.Error("new session should be inactive")
	}
}

func TestFocusSession_Open(t *testing.T) {
	fs := NewFocusSession()
	fs.Open(t.TempDir())

	if !fs.IsActive() {
		t.Error("expected active after Open")
	}
	if fs.phase != fsPhaseSetup {
		t.Errorf("expected setup phase, got %d", fs.phase)
	}
}

func TestFocusSession_OpenWithTask(t *testing.T) {
	fs := NewFocusSession()
	fs.OpenWithTask(t.TempDir(), "Write docs")

	if !fs.IsActive() {
		t.Error("expected active")
	}
	if fs.sessionTask != "Write docs" {
		t.Errorf("expected task 'Write docs', got %q", fs.sessionTask)
	}
}

func TestFocusSession_HandleTick_ActiveComplete(t *testing.T) {
	fs := NewFocusSession()
	fs.phase = fsPhaseActive
	fs.duration = 1 * time.Minute
	fs.startTime = time.Now().Add(-2 * time.Minute) // started 2 min ago, duration is 1 min

	fs, _ = fs.handleTick()

	if fs.phase != fsPhaseBreak {
		t.Errorf("expected phase to transition to break when time is up, got %d", fs.phase)
	}
}

func TestFocusSession_HandleTick_Paused(t *testing.T) {
	fs := NewFocusSession()
	fs.phase = fsPhaseActive
	fs.paused = true
	fs.duration = 25 * time.Minute
	fs.startTime = time.Now()

	before := fs.elapsed
	fs, _ = fs.handleTick()

	if fs.elapsed != before {
		t.Error("elapsed should not change while paused")
	}
}

func TestFocusSession_SaveSession(t *testing.T) {
	dir := t.TempDir()
	fs := NewFocusSession()
	fs.vaultRoot = dir
	fs.startTime = time.Now().Add(-30 * time.Minute)
	fs.totalElapsed = 30 * time.Minute
	fs.sessionGoal = "Write tests"
	fs.sessionTask = "Testing"
	fs.sessionNotes = "Good session."

	fs.saveSession()

	// Verify file created
	today := time.Now().Format("2006-01-02")
	data, err := os.ReadFile(filepath.Join(dir, "FocusSessions", today+".md"))
	if err != nil {
		t.Fatalf("session file not created: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "## Session") {
		t.Error("expected session header")
	}
	if !strings.Contains(content, "Goal: Write tests") {
		t.Error("expected goal in session")
	}
	if !strings.Contains(content, "Duration: 30 min") {
		t.Error("expected duration")
	}
}

func TestFocusSession_SaveMultipleSessions(t *testing.T) {
	dir := t.TempDir()
	fs := NewFocusSession()
	fs.vaultRoot = dir
	fs.startTime = time.Now()
	fs.totalElapsed = 10 * time.Minute

	fs.saveSession()
	fs.saveSession() // second session should append, not overwrite

	today := time.Now().Format("2006-01-02")
	data, err := os.ReadFile(filepath.Join(dir, "FocusSessions", today+".md"))
	if err != nil {
		t.Fatal(err)
	}
	count := strings.Count(string(data), "## Session")
	if count != 2 {
		t.Errorf("expected 2 session entries, got %d", count)
	}
}

// Regression: backspace on a multi-byte rune must remove the whole rune
// (not just one byte) and leave a valid UTF-8 string.
func TestFocusSession_BackspaceMultibyteGoal(t *testing.T) {
	fs := NewFocusSession()
	fs.Open(t.TempDir())
	fs.setupField = 1
	fs.goalInput = "café" // 'é' is 2 bytes in UTF-8

	fs, _ = fs.updateSetup(tea.KeyMsg{Type: tea.KeyBackspace})

	if fs.goalInput != "caf" {
		t.Errorf("expected 'caf' after backspace, got %q", fs.goalInput)
	}
	if !utf8.ValidString(fs.goalInput) {
		t.Errorf("goalInput is not valid UTF-8: %q", fs.goalInput)
	}
}

// Regression: backspace on the active-phase scratchpad must be rune-aware.
func TestFocusSession_BackspaceMultibyteScratchpad(t *testing.T) {
	fs := NewFocusSession()
	fs.Open(t.TempDir())
	fs.phase = fsPhaseActive
	fs.scratchpad = "hi 🌟" // 4-byte rune at the end

	fs, _ = fs.updateActive(tea.KeyMsg{Type: tea.KeyBackspace})

	if fs.scratchpad != "hi " {
		t.Errorf("expected 'hi ' after backspace, got %q", fs.scratchpad)
	}
	if !utf8.ValidString(fs.scratchpad) {
		t.Errorf("scratchpad is not valid UTF-8: %q", fs.scratchpad)
	}
}

// Pressing 'n' in the review phase should save the current session AND
// reopen the setup phase with a clean slate, so the user can start
// another focus session without having to navigate back to the task
// manager. This is the fix for the "I have to start over from the
// task manager every time" complaint.
func TestFocusSession_ReviewN_SavesAndRestartsInSetup(t *testing.T) {
	dir := t.TempDir()
	fs := NewFocusSession()
	fs.Open(dir)

	// Pretend a session just finished.
	fs.phase = fsPhaseReview
	fs.startTime = time.Now().Add(-25 * time.Minute)
	fs.totalElapsed = 25 * time.Minute
	fs.sessionGoal = "Write tests"
	fs.sessionTask = "Testing"
	fs.sessionNotes = "First session done."
	fs.scratchpad = "First session done."
	fs.goalInput = "Write tests"
	fs.taskIdx = 0

	fs, _ = fs.updateReview(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	if !fs.IsActive() {
		t.Fatal("overlay should remain active after 'n'")
	}
	if fs.phase != fsPhaseSetup {
		t.Errorf("expected setup phase after 'n', got %d", fs.phase)
	}
	if fs.sessionTask != "" || fs.sessionGoal != "" || fs.scratchpad != "" {
		t.Errorf("transient state should be cleared, got task=%q goal=%q scratch=%q",
			fs.sessionTask, fs.sessionGoal, fs.scratchpad)
	}
	if fs.taskIdx != -1 {
		t.Errorf("taskIdx should reset to -1, got %d", fs.taskIdx)
	}

	// First session should still be on disk.
	today := time.Now().Format("2006-01-02")
	data, err := os.ReadFile(filepath.Join(dir, "FocusSessions", today+".md"))
	if err != nil {
		t.Fatalf("session file not created: %v", err)
	}
	if !strings.Contains(string(data), "Goal: Write tests") {
		t.Error("first session not persisted before restart")
	}
}

// Pressing 'r' in the review phase should save and restart with the SAME
// task pre-selected, so the user can do another pomodoro on the same
// item without having to pick it again.
func TestFocusSession_ReviewR_SavesAndRepeatsSameTask(t *testing.T) {
	dir := t.TempDir()
	// Seed Tasks.md so loadTasks finds something to match against.
	if err := os.WriteFile(filepath.Join(dir, "Tasks.md"),
		[]byte("# Tasks\n\n- [ ] Refactor parser\n- [ ] Write blog post\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fs := NewFocusSession()
	fs.Open(dir)
	fs.phase = fsPhaseReview
	fs.startTime = time.Now().Add(-25 * time.Minute)
	fs.totalElapsed = 25 * time.Minute
	fs.sessionTask = "Refactor parser"
	fs.sessionGoal = "Refactor parser"
	fs.sessionNotes = "Made progress."

	fs, _ = fs.updateReview(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if !fs.IsActive() {
		t.Fatal("overlay should remain active after 'r'")
	}
	if fs.phase != fsPhaseSetup {
		t.Errorf("expected setup phase after 'r', got %d", fs.phase)
	}
	if fs.sessionTask != "Refactor parser" {
		t.Errorf("expected sessionTask preserved, got %q", fs.sessionTask)
	}
	if fs.goalInput != "Refactor parser" {
		t.Errorf("expected goalInput preserved, got %q", fs.goalInput)
	}
	// taskIdx should point at the matching task in the loaded list so
	// "start" binds the new session to the same entry.
	if fs.taskIdx < 0 || fs.taskIdx >= len(fs.tasks) || fs.tasks[fs.taskIdx] != "Refactor parser" {
		t.Errorf("expected taskIdx to point at 'Refactor parser', got idx=%d list=%v",
			fs.taskIdx, fs.tasks)
	}
	// User should land on the duration/goal field, not the task picker.
	if fs.setupField != 1 {
		t.Errorf("expected setupField=1 (duration/goal), got %d", fs.setupField)
	}
}

// Esc in the review phase still discards without saving.
func TestFocusSession_ReviewEsc_ClosesWithoutSaving(t *testing.T) {
	dir := t.TempDir()
	fs := NewFocusSession()
	fs.Open(dir)
	fs.phase = fsPhaseReview
	fs.startTime = time.Now()
	fs.totalElapsed = 10 * time.Minute
	fs.sessionGoal = "Discarded"

	fs, _ = fs.updateReview(tea.KeyMsg{Type: tea.KeyEsc})

	if fs.IsActive() {
		t.Error("expected overlay closed after Esc")
	}
	today := time.Now().Format("2006-01-02")
	if _, err := os.Stat(filepath.Join(dir, "FocusSessions", today+".md")); !os.IsNotExist(err) {
		t.Error("Esc should not have written a session file")
	}
}

// Enter in the review phase saves and closes (existing contract).
func TestFocusSession_ReviewEnter_SavesAndCloses(t *testing.T) {
	dir := t.TempDir()
	fs := NewFocusSession()
	fs.Open(dir)
	fs.phase = fsPhaseReview
	fs.startTime = time.Now().Add(-15 * time.Minute)
	fs.totalElapsed = 15 * time.Minute
	fs.sessionGoal = "Final"

	fs, _ = fs.updateReview(tea.KeyMsg{Type: tea.KeyEnter})

	if fs.IsActive() {
		t.Error("expected overlay closed after Enter")
	}
	today := time.Now().Format("2006-01-02")
	data, err := os.ReadFile(filepath.Join(dir, "FocusSessions", today+".md"))
	if err != nil {
		t.Fatalf("session file not created: %v", err)
	}
	if !strings.Contains(string(data), "Final") {
		t.Error("session goal not persisted")
	}
}
