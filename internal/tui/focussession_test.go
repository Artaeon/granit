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
