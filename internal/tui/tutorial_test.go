package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/config"
)

// newTestConfig returns a Config whose Save writes to a temp directory.
func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.SetFilePath(filepath.Join(dir, "config.json"))
	return &cfg
}

// ---------------------------------------------------------------------------
// Tutorial State Machine
// ---------------------------------------------------------------------------

func TestTutorial_InitialState(t *testing.T) {
	cfg := newTestConfig(t)
	tut := NewTutorial(cfg)

	if tut.IsActive() {
		t.Error("new tutorial should not be active")
	}
	if tut.page != 0 {
		t.Errorf("expected page 0, got %d", tut.page)
	}
}

func TestTutorial_Navigation(t *testing.T) {
	cfg := newTestConfig(t)
	tut := NewTutorial(cfg)
	tut.Open()
	tut.SetSize(120, 40)

	if !tut.IsActive() {
		t.Fatal("tutorial should be active after Open")
	}
	if tut.page != 0 {
		t.Fatalf("expected page 0 after Open, got %d", tut.page)
	}

	// Right arrow advances
	tut, _ = tut.Update(tea.KeyMsg{Type: tea.KeyRight})
	if tut.page != 1 {
		t.Errorf("expected page 1 after right, got %d", tut.page)
	}

	// Left arrow goes back
	tut, _ = tut.Update(tea.KeyMsg{Type: tea.KeyRight})
	tut, _ = tut.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if tut.page != 1 {
		t.Errorf("expected page 1 after right-right-left, got %d", tut.page)
	}
}

func TestTutorial_Bounds(t *testing.T) {
	cfg := newTestConfig(t)
	tut := NewTutorial(cfg)
	tut.Open()
	tut.SetSize(120, 40)

	// Can't go before page 0
	tut, _ = tut.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if tut.page != 0 {
		t.Errorf("expected page to stay at 0, got %d", tut.page)
	}

	// Advance to the last page
	for i := 0; i < tut.totalPages-1; i++ {
		tut, _ = tut.Update(tea.KeyMsg{Type: tea.KeyRight})
	}
	if tut.page != tut.totalPages-1 {
		t.Fatalf("expected to be on last page %d, got %d", tut.totalPages-1, tut.page)
	}

	// Can't go past the last page
	tut, _ = tut.Update(tea.KeyMsg{Type: tea.KeyRight})
	if tut.page != tut.totalPages-1 {
		t.Errorf("expected page to stay at %d, got %d", tut.totalPages-1, tut.page)
	}
}

func TestTutorial_EscCloses(t *testing.T) {
	cfg := newTestConfig(t)
	tut := NewTutorial(cfg)
	tut.Open()
	tut.SetSize(120, 40)

	if !tut.IsActive() {
		t.Fatal("tutorial should be active after Open")
	}

	tut, _ = tut.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if tut.IsActive() {
		t.Error("tutorial should be closed after Esc")
	}
}

func TestTutorial_MarkComplete(t *testing.T) {
	cfg := newTestConfig(t)
	tut := NewTutorial(cfg)
	tut.Open()
	tut.SetSize(120, 40)

	// Close via Esc which calls MarkComplete
	tut, cmd := tut.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if tut.IsActive() {
		t.Error("tutorial should be closed after Esc")
	}
	if !cfg.TutorialCompleted {
		t.Error("expected TutorialCompleted to be true after closing")
	}

	// The config should have been saved to disk (cmd is nil on success)
	if cmd != nil {
		// If cmd is non-nil, it means Save returned an error
		t.Log("MarkComplete returned a non-nil cmd (save may have produced an error msg)")
	}

	// Verify the file was written
	data, err := os.ReadFile(cfg.GetFilePath())
	if err != nil {
		t.Fatalf("expected config file to exist: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty config file")
	}
}
