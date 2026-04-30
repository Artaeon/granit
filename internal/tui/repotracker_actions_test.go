package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/objects"
)

func TestRepoTracker_OQueuesOpenStatus(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "demo")
	_ = os.Mkdir(repoDir, 0o755)
	rtInitRepo(t, repoDir)

	r := NewRepoTracker()
	r.SetSize(100, 30)
	r.Open(root, objects.NewRegistry(), objects.NewIndex())

	r, _ = r.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	msg := r.ConsumePendingStatus()
	// On Linux without xdg-open, OpenFolder errors with a clear message;
	// either outcome is fine — we just want the action to surface SOME
	// status feedback rather than be silent.
	if msg == "" {
		t.Fatal("expected pendingStatus after 'o', got empty")
	}
	if !strings.Contains(msg, "Open") && !strings.Contains(msg, "Opened") &&
		!strings.Contains(msg, "open folder") {
		t.Errorf("expected open-related status, got %q", msg)
	}
	// Consumed-once.
	if r.ConsumePendingStatus() != "" {
		t.Error("status should be cleared after consumption")
	}
}

func TestRepoTracker_CCopiesPathToStatus(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "demo")
	_ = os.Mkdir(repoDir, 0o755)
	rtInitRepo(t, repoDir)

	r := NewRepoTracker()
	r.SetSize(100, 30)
	r.Open(root, objects.NewRegistry(), objects.NewIndex())

	r, _ = r.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	msg := r.ConsumePendingStatus()
	if msg == "" {
		t.Fatal("expected pendingStatus after 'c'")
	}
	// Either copy succeeded ("Copied …") or failed ("Clipboard copy
	// failed: …" — happens in CI without a clipboard). Both are valid
	// outcomes; we just verify feedback was queued.
	if !strings.Contains(msg, "Copied") && !strings.Contains(msg, "Clipboard") {
		t.Errorf("expected copy-related status, got %q", msg)
	}
}

func TestRepoTracker_ActionOnEmptyRowsNoCrash(t *testing.T) {
	r := NewRepoTracker()
	r.SetSize(100, 30)
	// No Open() call → rows empty, cursor at 0. Pressing o/c must not
	// panic.
	r.Activate()
	r.cursor = 0
	r, _ = r.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	r, _ = r.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if r.ConsumePendingStatus() != "" {
		t.Error("no row → no status; got something")
	}
}
