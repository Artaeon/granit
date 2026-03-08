package tui

import (
	"strings"
	"testing"
	"time"
)

// ===========================================================================
// Editor edge case and stress tests
// ===========================================================================

// ---------------------------------------------------------------------------
// Inserting at every position in a multi-line file
// ---------------------------------------------------------------------------

func TestInsertAtEveryPosition(t *testing.T) {
	original := "abc\ndef\nghi"
	lines := strings.Split(original, "\n")

	for lineIdx, line := range lines {
		for col := 0; col <= len(line); col++ {
			e := focusedEditor()
			e.LoadContent(original, "/test.md")
			e.focused = true
			e.cursor = lineIdx
			e.col = col

			e = sendKey(e, "X")

			content := e.GetContent()
			resultLines := strings.Split(content, "\n")

			if len(resultLines) != 3 {
				t.Errorf("insert at (%d,%d): expected 3 lines, got %d",
					lineIdx, col, len(resultLines))
				continue
			}

			expectedLine := line[:col] + "X" + line[col:]
			if resultLines[lineIdx] != expectedLine {
				t.Errorf("insert at (%d,%d): expected line %q, got %q",
					lineIdx, col, expectedLine, resultLines[lineIdx])
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Rapid undo/redo cycles (100 operations)
// ---------------------------------------------------------------------------

func TestRapidUndoRedoCycles(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("start", "/test.md")
	e.focused = true

	// Perform 100 edits, each with a fresh snapshot timestamp
	for i := 0; i < 100; i++ {
		e.lastSnapshot = time.Time{} // force snapshot
		e.col = len(e.content[e.cursor])
		e = sendKey(e, "a")
	}

	finalContent := e.GetContent()
	if !strings.HasPrefix(finalContent, "start") {
		t.Fatalf("content should start with 'start', got %q", finalContent)
	}

	// Undo everything (stack limited to 100)
	for i := 0; i < 100; i++ {
		e.Undo()
	}
	undoneContent := e.GetContent()

	// Redo everything
	for i := 0; i < 100; i++ {
		e.Redo()
	}
	redoneContent := e.GetContent()

	// After redo, should return to the final state
	if redoneContent != finalContent {
		t.Errorf("after full redo, expected %q, got %q", finalContent, redoneContent)
	}

	// After undo, should be shorter
	if len(undoneContent) >= len(finalContent) {
		t.Errorf("after undo, content should be shorter: undo=%d final=%d",
			len(undoneContent), len(finalContent))
	}
}

// ---------------------------------------------------------------------------
// Paste of very large text (10,000 lines)
// ---------------------------------------------------------------------------

func TestPasteLargeText(t *testing.T) {
	e := NewEditor()
	e.lastSnapshot = time.Time{} // force snapshot

	var sb strings.Builder
	for i := 0; i < 10000; i++ {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("line content here")
	}
	largeText := sb.String()

	e.InsertText(largeText)

	content := e.GetContent()
	lineCount := strings.Count(content, "\n") + 1
	if lineCount != 10000 {
		t.Errorf("expected 10000 lines, got %d", lineCount)
	}

	line, col := e.GetCursor()
	if line != 9999 {
		t.Errorf("expected cursor at line 9999, got %d", line)
	}
	if col != len("line content here") {
		t.Errorf("expected cursor at col %d, got %d", len("line content here"), col)
	}
}

// ---------------------------------------------------------------------------
// Cursor movement on empty file
// ---------------------------------------------------------------------------

func TestCursorMovement_EmptyFile(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("", "/test.md")
	e.focused = true

	// All movements should be safe on empty content
	movements := []string{"up", "down", "left", "right", "home", "end"}
	for _, mv := range movements {
		e = sendKey(e, mv)
		line, col := e.GetCursor()
		if line != 0 {
			t.Errorf("after %s on empty file, expected line 0, got %d", mv, line)
		}
		if col < 0 {
			t.Errorf("after %s on empty file, col should not be negative: %d", mv, col)
		}
	}
}

// ---------------------------------------------------------------------------
// Backspace on empty file
// ---------------------------------------------------------------------------

func TestBackspace_EmptyFile(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("", "/test.md")
	e.focused = true

	// Should not panic or corrupt state
	for i := 0; i < 10; i++ {
		e = sendKey(e, "backspace")
	}

	if e.GetContent() != "" {
		t.Errorf("expected empty content after backspace on empty file, got %q", e.GetContent())
	}
	line, col := e.GetCursor()
	if line != 0 || col != 0 {
		t.Errorf("expected cursor (0,0), got (%d,%d)", line, col)
	}
}

// ---------------------------------------------------------------------------
// Multi-cursor with 50 cursors simultaneously
// ---------------------------------------------------------------------------

func TestMultiCursor_50Cursors(t *testing.T) {
	// Create a file with 50 identical lines
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("hello")
	}

	e := focusedEditor()
	e.LoadContent(sb.String(), "/test.md")
	e.focused = true
	e.cursor = 0
	e.col = 5 // end of "hello"

	// Add 49 additional cursors (one per remaining line)
	for i := 1; i < 50; i++ {
		e.cursors = append(e.cursors, CursorPos{Line: i, Col: 5})
	}

	if len(e.cursors) != 49 {
		t.Fatalf("expected 49 additional cursors, got %d", len(e.cursors))
	}

	// Insert a character at all 50 cursor positions
	e = sendKey(e, "!")

	content := e.GetContent()
	lines := strings.Split(content, "\n")

	if len(lines) != 50 {
		t.Fatalf("expected 50 lines, got %d", len(lines))
	}

	// Each line should now be "hello!"
	for i, line := range lines {
		if line != "hello!" {
			t.Errorf("line %d: expected 'hello!', got %q", i, line)
		}
	}
}

// ---------------------------------------------------------------------------
// Delete key on empty file
// ---------------------------------------------------------------------------

func TestDelete_EmptyFile(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("", "/test.md")
	e.focused = true

	// Should not panic
	for i := 0; i < 5; i++ {
		e = sendKey(e, "delete")
	}
	if e.GetContent() != "" {
		t.Errorf("expected empty content, got %q", e.GetContent())
	}
}

// ---------------------------------------------------------------------------
// Enter on single empty line
// ---------------------------------------------------------------------------

func TestEnter_EmptyFile(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("", "/test.md")
	e.focused = true

	e = sendKey(e, "enter")

	lines := strings.Split(e.GetContent(), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines after enter on empty file, got %d", len(lines))
	}
	line, col := e.GetCursor()
	if line != 1 || col != 0 {
		t.Errorf("expected cursor (1,0), got (%d,%d)", line, col)
	}
}

// ---------------------------------------------------------------------------
// Multi-cursor backspace
// ---------------------------------------------------------------------------

func TestMultiCursor_Backspace(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("ab\ncd\nef", "/test.md")
	e.focused = true
	e.cursor = 0
	e.col = 1 // after 'a'

	// Add cursors at col 1 of lines 1 and 2
	e.cursors = append(e.cursors, CursorPos{Line: 1, Col: 1})
	e.cursors = append(e.cursors, CursorPos{Line: 2, Col: 1})

	e = sendKey(e, "backspace")

	content := e.GetContent()
	lines := strings.Split(content, "\n")

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}

	// Each line should have its first char removed
	expected := []string{"b", "d", "f"}
	for i, exp := range expected {
		if lines[i] != exp {
			t.Errorf("line %d: expected %q, got %q", i, exp, lines[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Undo after multi-cursor edit
// ---------------------------------------------------------------------------

func TestUndoAfterMultiCursorEdit(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("aaa\nbbb\nccc", "/test.md")
	e.focused = true
	e.cursor = 0
	e.col = 3

	e.lastSnapshot = time.Time{} // force snapshot

	// Add cursors
	e.cursors = append(e.cursors, CursorPos{Line: 1, Col: 3})
	e.cursors = append(e.cursors, CursorPos{Line: 2, Col: 3})

	e = sendKey(e, "X")

	// Verify edit applied
	if !strings.Contains(e.GetContent(), "aaaX") {
		t.Fatalf("multi-cursor edit failed, got %q", e.GetContent())
	}

	// Undo should restore original
	e.Undo()
	if e.GetContent() != "aaa\nbbb\nccc" {
		t.Errorf("undo after multi-cursor: expected original, got %q", e.GetContent())
	}
}

// ---------------------------------------------------------------------------
// Cursor column clamping when moving between lines of different lengths
// ---------------------------------------------------------------------------

func TestCursorColumnClamping(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("short\nvery long line here", "/test.md")
	e.focused = true
	e.cursor = 1
	e.col = 19 // end of long line

	// Move up to short line
	e = sendKey(e, "up")
	_, col := e.GetCursor()
	if col > 5 {
		t.Errorf("cursor col should be clamped to 5, got %d", col)
	}
}

// ---------------------------------------------------------------------------
// Tab insertion
// ---------------------------------------------------------------------------

func TestTabInsertion(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("text", "/test.md")
	e.focused = true
	e.tabSize = 4
	e.col = 0

	e = sendKey(e, "tab")

	if !strings.HasPrefix(e.GetContent(), "    text") {
		t.Errorf("expected tab (4 spaces) at start, got %q", e.GetContent())
	}
}

// ---------------------------------------------------------------------------
// Content with only newlines
// ---------------------------------------------------------------------------

func TestContentOnlyNewlines(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("\n\n\n", "/test.md")
	e.focused = true

	// Should have 4 lines (3 newlines = 4 segments)
	if len(e.content) != 4 {
		t.Errorf("expected 4 lines from 3 newlines, got %d", len(e.content))
	}

	// Navigate through all lines
	for i := 0; i < 3; i++ {
		e = sendKey(e, "down")
	}
	line, _ := e.GetCursor()
	if line != 3 {
		t.Errorf("expected cursor at line 3, got %d", line)
	}
}
