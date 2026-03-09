package tui

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Toast Message Queue
// ---------------------------------------------------------------------------

func TestStatusBar_SetMessage(t *testing.T) {
	sb := NewStatusBar()
	sb.SetWidth(120)
	sb.SetMessage("File saved")

	view := sb.View()
	if !strings.Contains(view, "File saved") {
		t.Errorf("expected View to contain %q, got:\n%s", "File saved", view)
	}
}

func TestStatusBar_SetWarning(t *testing.T) {
	sb := NewStatusBar()
	sb.SetWidth(120)

	sb.SetMessage("info message")
	sb.SetWarning("warning message")

	toast, ok := sb.topStatusToast()
	if !ok {
		t.Fatal("expected a toast, got none")
	}
	if toast.priority != ToastWarning {
		t.Errorf("expected priority ToastWarning (%d), got %d", ToastWarning, toast.priority)
	}
	if toast.text != "warning message" {
		t.Errorf("expected text %q, got %q", "warning message", toast.text)
	}
}

func TestStatusBar_SetError(t *testing.T) {
	sb := NewStatusBar()
	sb.SetWidth(120)

	sb.SetWarning("warning message")
	sb.SetError("error message")

	toast, ok := sb.topStatusToast()
	if !ok {
		t.Fatal("expected a toast, got none")
	}
	if toast.priority != ToastError {
		t.Errorf("expected priority ToastError (%d), got %d", ToastError, toast.priority)
	}
	if toast.text != "error message" {
		t.Errorf("expected text %q, got %q", "error message", toast.text)
	}
}

func TestStatusBar_MultiplePriorities(t *testing.T) {
	sb := NewStatusBar()
	sb.SetWidth(120)

	sb.SetMessage("info one")
	sb.SetWarning("warn one")
	sb.SetMessage("info two")
	sb.SetError("err one")
	sb.SetWarning("warn two")

	toast, ok := sb.topStatusToast()
	if !ok {
		t.Fatal("expected a toast, got none")
	}
	// Error has the highest priority; it should always be returned.
	if toast.priority != ToastError {
		t.Errorf("expected highest priority to be ToastError (%d), got %d", ToastError, toast.priority)
	}
	if toast.text != "err one" {
		t.Errorf("expected error toast text %q, got %q", "err one", toast.text)
	}
}

func TestStatusBar_SetActiveNote(t *testing.T) {
	sb := NewStatusBar()
	sb.SetWidth(120)
	sb.SetActiveNote("projects/roadmap.md")

	view := sb.View()
	if !strings.Contains(view, "roadmap.md") {
		t.Errorf("expected View to contain the active note name, got:\n%s", view)
	}
}

func TestStatusBar_SetNoteCount(t *testing.T) {
	sb := NewStatusBar()
	sb.SetWidth(120)
	sb.SetNoteCount(42)

	view := sb.View()
	if !strings.Contains(view, "42") {
		t.Errorf("expected View to contain note count '42', got:\n%s", view)
	}
}
