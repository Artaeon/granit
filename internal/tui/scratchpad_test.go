package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScratchpad_OpenAndClose(t *testing.T) {
	dir := t.TempDir()
	s := NewScratchpad()
	s.Open(dir)

	if !s.IsActive() {
		t.Error("expected active after Open")
	}

	s.Close()
	if s.IsActive() {
		t.Error("expected inactive after Close")
	}

	// File should be saved
	if _, err := os.Stat(filepath.Join(dir, ".granit", "scratchpad.md")); os.IsNotExist(err) {
		t.Error("scratchpad file should be created on close")
	}
}

func TestScratchpad_PersistContent(t *testing.T) {
	dir := t.TempDir()
	s := NewScratchpad()
	s.Open(dir)
	s.content = []string{"Line 1", "Line 2", "Line 3"}
	s.Close()

	// Reopen and verify
	s2 := NewScratchpad()
	s2.Open(dir)
	if len(s2.content) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(s2.content))
	}
	if s2.content[1] != "Line 2" {
		t.Errorf("expected 'Line 2', got %q", s2.content[1])
	}
}

func TestScratchpad_GetContent(t *testing.T) {
	s := NewScratchpad()
	s.content = []string{"Hello", "World"}
	if got := s.GetContent(); got != "Hello\nWorld" {
		t.Errorf("expected 'Hello\\nWorld', got %q", got)
	}
}

func TestScratchpad_ClampCursor(t *testing.T) {
	s := NewScratchpad()
	s.content = []string{"ab", "cde"}

	// Out of bounds line
	s.cursorLine = 5
	s.cursorCol = 10
	s.clampCursor()
	if s.cursorLine != 1 {
		t.Errorf("expected line clamped to 1, got %d", s.cursorLine)
	}
	if s.cursorCol != 3 {
		t.Errorf("expected col clamped to 3 (len of 'cde'), got %d", s.cursorCol)
	}

	// Negative cursor
	s.cursorLine = -1
	s.cursorCol = -1
	s.clampCursor()
	if s.cursorLine != 0 || s.cursorCol != 0 {
		t.Error("negative cursor should clamp to 0")
	}
}

func TestScratchpad_EmptyVaultRoot(t *testing.T) {
	s := NewScratchpad()
	s.vaultRoot = ""
	s.content = []string{"test"}
	s.save() // should not panic or create files
}

func TestScratchpad_LoadMissing(t *testing.T) {
	s := NewScratchpad()
	s.vaultRoot = t.TempDir()
	s.load()
	if len(s.content) != 1 || s.content[0] != "" {
		t.Error("missing file should initialize to single empty line")
	}
}
