package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// makeEditedEditor builds an editor with a content-after-edit state plus
// a saved undo snapshot of the prior state. The exact saving order
// matters: we need a snapshot of "hello" sitting on the undo stack so
// the next Undo() (or Ctrl+Z key) can restore it.
func makeEditedEditor(t *testing.T) Editor {
	t.Helper()
	e := NewEditor()
	e.focused = true
	e.SetContent("hello")
	// Force snapshot save (saveSnapshot is debounced 500ms).
	e.lastSnapshot = time.Now().Add(-time.Second)
	// Move cursor to end of "hello" so InsertText appends.
	e.cursor = 0
	e.col = 5
	e.InsertText(" world")
	if got := e.content[0]; got != "hello world" {
		t.Fatalf("setup: expected 'hello world', got %q", got)
	}
	return e
}

// TestEditor_CtrlZ_TriggersUndo verifies that Ctrl+Z runs Undo() when
// the editor receives the keystroke. Regression: the binding moved
// from Ctrl+U-only to Ctrl+Z (universal convention) + Ctrl+U alias.
func TestEditor_CtrlZ_TriggersUndo(t *testing.T) {
	e := makeEditedEditor(t)
	updated, _ := e.Update(tea.KeyMsg{Type: tea.KeyCtrlZ})
	if got := updated.content[0]; got != "hello" {
		t.Fatalf("expected ctrl+z to restore 'hello', got %q", got)
	}
}

func TestEditor_CtrlU_StillTriggersUndo(t *testing.T) {
	e := makeEditedEditor(t)
	updated, _ := e.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	if got := updated.content[0]; got != "hello" {
		t.Fatalf("expected ctrl+u to undo, got %q", got)
	}
}

func TestEditor_CtrlY_StillTriggersRedo(t *testing.T) {
	e := makeEditedEditor(t)
	e.Undo()
	if e.content[0] != "hello" {
		t.Fatalf("setup: undo failed: %q", e.content[0])
	}
	updated, _ := e.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	if got := updated.content[0]; got != "hello world" {
		t.Fatalf("expected ctrl+y to redo, got %q", got)
	}
}
