package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// keyMsg builds a tea.KeyMsg for a printable rune character.
func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "delete":
		return tea.KeyMsg{Type: tea.KeyDelete}
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	default:
		if len(s) == 1 {
			return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
		}
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// focusedEditor returns a new editor with focus enabled and a given size.
func focusedEditor() Editor {
	e := NewEditor()
	e.focused = true
	e.SetSize(80, 24)
	return e
}

// sendKey sends a key to the editor and returns the updated editor.
func sendKey(e Editor, s string) Editor {
	e, _ = e.Update(keyMsg(s))
	return e
}

// ---------------------------------------------------------------------------
// NewEditor
// ---------------------------------------------------------------------------

func TestNewEditor(t *testing.T) {
	e := NewEditor()

	t.Run("content has one empty line", func(t *testing.T) {
		if len(e.content) != 1 || e.content[0] != "" {
			t.Errorf("expected [\"\"], got %v", e.content)
		}
	})

	t.Run("cursor at origin", func(t *testing.T) {
		line, col := e.GetCursor()
		if line != 0 || col != 0 {
			t.Errorf("expected cursor (0,0), got (%d,%d)", line, col)
		}
	})

	t.Run("not modified", func(t *testing.T) {
		if e.IsModified() {
			t.Error("new editor should not be modified")
		}
	})

	t.Run("word count is zero", func(t *testing.T) {
		if e.GetWordCount() != 0 {
			t.Errorf("expected word count 0, got %d", e.GetWordCount())
		}
	})
}

// ---------------------------------------------------------------------------
// SetContent / GetContent roundtrip
// ---------------------------------------------------------------------------

func TestSetContent_GetContent_Roundtrip(t *testing.T) {
	e := NewEditor()

	tests := []struct {
		name    string
		content string
	}{
		{"single line", "hello world"},
		{"multiline", "line one\nline two\nline three"},
		{"empty", ""},
		{"with special chars", "# Heading\n- bullet\n> quote"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e.SetContent(tc.content)
			got := e.GetContent()
			if got != tc.content {
				t.Errorf("roundtrip failed: set %q, got %q", tc.content, got)
			}
		})
	}
}

func TestSetContent_MarksModified(t *testing.T) {
	e := NewEditor()
	e.SetContent("something")
	if !e.IsModified() {
		t.Error("SetContent should mark editor as modified")
	}
}

// ---------------------------------------------------------------------------
// LoadContent
// ---------------------------------------------------------------------------

func TestLoadContent(t *testing.T) {
	e := NewEditor()
	e.LoadContent("alpha\nbeta\ngamma", "/tmp/test.md")

	t.Run("content matches", func(t *testing.T) {
		if e.GetContent() != "alpha\nbeta\ngamma" {
			t.Errorf("unexpected content: %q", e.GetContent())
		}
	})

	t.Run("cursor reset to origin", func(t *testing.T) {
		line, col := e.GetCursor()
		if line != 0 || col != 0 {
			t.Errorf("expected cursor (0,0), got (%d,%d)", line, col)
		}
	})

	t.Run("not marked as modified", func(t *testing.T) {
		if e.IsModified() {
			t.Error("LoadContent should not mark editor as modified")
		}
	})
}

// ---------------------------------------------------------------------------
// SetSize
// ---------------------------------------------------------------------------

func TestSetSize(t *testing.T) {
	e := NewEditor()
	e.SetSize(120, 40)
	if e.width != 120 || e.height != 40 {
		t.Errorf("expected size (120,40), got (%d,%d)", e.width, e.height)
	}
}

// ---------------------------------------------------------------------------
// InsertChar (via Update with single character keys)
// ---------------------------------------------------------------------------

func TestInsertChar(t *testing.T) {
	e := focusedEditor()
	e = sendKey(e, "H")
	e = sendKey(e, "i")

	if e.GetContent() != "Hi" {
		t.Errorf("expected 'Hi', got %q", e.GetContent())
	}
	line, col := e.GetCursor()
	if line != 0 || col != 2 {
		t.Errorf("expected cursor (0,2), got (%d,%d)", line, col)
	}
}

func TestInsertChar_MidLine(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("ac", "/test.md")
	e.focused = true
	// Move cursor to col 1 (between 'a' and 'c')
	e.cursor = 0
	e.col = 1
	e = sendKey(e, "b")

	if e.GetContent() != "abc" {
		t.Errorf("expected 'abc', got %q", e.GetContent())
	}
}

// ---------------------------------------------------------------------------
// Backspace
// ---------------------------------------------------------------------------

func TestBackspace_RemovesChar(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("abc", "/test.md")
	e.focused = true
	e.col = 3 // cursor at end
	e = sendKey(e, "backspace")

	if e.GetContent() != "ab" {
		t.Errorf("expected 'ab', got %q", e.GetContent())
	}
	if _, col := e.GetCursor(); col != 2 {
		t.Errorf("expected col 2, got %d", col)
	}
}

func TestBackspace_AtStartOfLine_JoinLines(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("hello\nworld", "/test.md")
	e.focused = true
	e.cursor = 1
	e.col = 0
	e = sendKey(e, "backspace")

	if e.GetContent() != "helloworld" {
		t.Errorf("expected 'helloworld', got %q", e.GetContent())
	}
	line, col := e.GetCursor()
	if line != 0 || col != 5 {
		t.Errorf("expected cursor (0,5), got (%d,%d)", line, col)
	}
}

func TestBackspace_AtOrigin_NoOp(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("text", "/test.md")
	e.focused = true
	e.cursor = 0
	e.col = 0
	e = sendKey(e, "backspace")

	if e.GetContent() != "text" {
		t.Errorf("expected no change, got %q", e.GetContent())
	}
}

// ---------------------------------------------------------------------------
// Enter — splits line at cursor
// ---------------------------------------------------------------------------

func TestEnter_SplitsLine(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("abcd", "/test.md")
	e.focused = true
	e.col = 2
	e = sendKey(e, "enter")

	if e.GetContent() != "ab\ncd" {
		t.Errorf("expected 'ab\\ncd', got %q", e.GetContent())
	}
	line, col := e.GetCursor()
	if line != 1 || col != 0 {
		t.Errorf("expected cursor (1,0), got (%d,%d)", line, col)
	}
}

func TestEnter_AtEndOfLine(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("line1", "/test.md")
	e.focused = true
	e.col = 5
	e = sendKey(e, "enter")

	if e.GetContent() != "line1\n" {
		t.Errorf("expected 'line1\\n', got %q", e.GetContent())
	}
}

func TestEnter_AtStartOfLine(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("line1", "/test.md")
	e.focused = true
	e.col = 0
	e = sendKey(e, "enter")

	if e.GetContent() != "\nline1" {
		t.Errorf("expected '\\nline1', got %q", e.GetContent())
	}
}

// ---------------------------------------------------------------------------
// Cursor movement
// ---------------------------------------------------------------------------

func TestCursorMovement_Down(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("line1\nline2\nline3", "/test.md")
	e.focused = true
	e = sendKey(e, "down")

	line, _ := e.GetCursor()
	if line != 1 {
		t.Errorf("expected line 1, got %d", line)
	}
}

func TestCursorMovement_Up(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("line1\nline2", "/test.md")
	e.focused = true
	e.cursor = 1
	e = sendKey(e, "up")

	line, _ := e.GetCursor()
	if line != 0 {
		t.Errorf("expected line 0, got %d", line)
	}
}

func TestCursorMovement_Right(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("abc", "/test.md")
	e.focused = true
	e.col = 0
	e = sendKey(e, "right")

	_, col := e.GetCursor()
	if col != 1 {
		t.Errorf("expected col 1, got %d", col)
	}
}

func TestCursorMovement_Left(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("abc", "/test.md")
	e.focused = true
	e.col = 2
	e = sendKey(e, "left")

	_, col := e.GetCursor()
	if col != 1 {
		t.Errorf("expected col 1, got %d", col)
	}
}

func TestCursorMovement_RightWrapsToNextLine(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("ab\ncd", "/test.md")
	e.focused = true
	e.col = 2 // end of "ab"
	e = sendKey(e, "right")

	line, col := e.GetCursor()
	if line != 1 || col != 0 {
		t.Errorf("expected cursor (1,0), got (%d,%d)", line, col)
	}
}

func TestCursorMovement_LeftWrapsToEndOfPrevLine(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("ab\ncd", "/test.md")
	e.focused = true
	e.cursor = 1
	e.col = 0
	e = sendKey(e, "left")

	line, col := e.GetCursor()
	if line != 0 || col != 2 {
		t.Errorf("expected cursor (0,2), got (%d,%d)", line, col)
	}
}

// ---------------------------------------------------------------------------
// Cursor bounds checking
// ---------------------------------------------------------------------------

func TestCursorBounds_CannotGoAboveFirstLine(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("only line", "/test.md")
	e.focused = true
	e.cursor = 0
	e = sendKey(e, "up")

	line, _ := e.GetCursor()
	if line != 0 {
		t.Errorf("cursor should stay at line 0, got %d", line)
	}
}

func TestCursorBounds_CannotGoBelowLastLine(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("line1\nline2", "/test.md")
	e.focused = true
	e.cursor = 1
	e = sendKey(e, "down")

	line, _ := e.GetCursor()
	if line != 1 {
		t.Errorf("cursor should stay at line 1, got %d", line)
	}
}

func TestCursorBounds_LeftAtOrigin(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("text", "/test.md")
	e.focused = true
	e.cursor = 0
	e.col = 0

	// Single-line, at origin: left with no previous line should stay
	// Actually the editor wraps to prev line end, but at line 0 col 0 it stays
	e = sendKey(e, "left")
	line, col := e.GetCursor()
	if line != 0 || col != 0 {
		t.Errorf("expected (0,0), got (%d,%d)", line, col)
	}
}

func TestCursorBounds_DownClampsColumn(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("long line here\nhi", "/test.md")
	e.focused = true
	e.col = 14 // end of "long line here"
	e = sendKey(e, "down")

	_, col := e.GetCursor()
	// "hi" has length 2, so col should be clamped to 2
	if col != 2 {
		t.Errorf("expected col clamped to 2, got %d", col)
	}
}

// ---------------------------------------------------------------------------
// Undo / Redo
// ---------------------------------------------------------------------------

func TestUndo_RestoresContent(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("original", "/test.md")
	e.focused = true

	// Force enough time gap so snapshot is saved
	e.lastSnapshot = time.Time{}
	e.col = 8
	e = sendKey(e, "!")

	if e.GetContent() != "original!" {
		t.Fatalf("expected 'original!', got %q", e.GetContent())
	}

	e.Undo()
	if e.GetContent() != "original" {
		t.Errorf("expected 'original' after undo, got %q", e.GetContent())
	}
}

func TestRedo_RestoresUndoneContent(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("original", "/test.md")
	e.focused = true

	e.lastSnapshot = time.Time{}
	e.col = 8
	e = sendKey(e, "!")

	e.Undo()
	if e.GetContent() != "original" {
		t.Fatalf("undo failed, got %q", e.GetContent())
	}

	e.Redo()
	if e.GetContent() != "original!" {
		t.Errorf("expected 'original!' after redo, got %q", e.GetContent())
	}
}

func TestUndo_OnEmptyStack_NoOp(t *testing.T) {
	e := NewEditor()
	before := e.GetContent()
	e.Undo()
	if e.GetContent() != before {
		t.Error("undo on empty stack should be a no-op")
	}
}

func TestRedo_OnEmptyStack_NoOp(t *testing.T) {
	e := NewEditor()
	before := e.GetContent()
	e.Redo()
	if e.GetContent() != before {
		t.Error("redo on empty stack should be a no-op")
	}
}

// ---------------------------------------------------------------------------
// SetContent pushes undo snapshot (undo is available after SetContent)
// ---------------------------------------------------------------------------

func TestSetContent_PushesUndoSnapshot(t *testing.T) {
	e := NewEditor()
	e.LoadContent("first", "/test.md")

	// Ensure the snapshot timer allows saving
	e.lastSnapshot = time.Time{}

	e.SetContent("second")
	if e.GetContent() != "second" {
		t.Fatalf("expected 'second', got %q", e.GetContent())
	}

	e.Undo()
	if e.GetContent() != "first" {
		t.Errorf("expected 'first' after undo, got %q", e.GetContent())
	}
}

// ---------------------------------------------------------------------------
// Line count tracking
// ---------------------------------------------------------------------------

func TestLineCount(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{"single line", "hello", 1},
		{"two lines", "a\nb", 2},
		{"five lines", "1\n2\n3\n4\n5", 5},
		{"empty string produces one empty line", "", 1},
		{"trailing newline", "a\n", 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEditor()
			e.LoadContent(tc.content, "/test.md")
			got := len(e.content)
			if got != tc.want {
				t.Errorf("expected %d lines, got %d", tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Word count
// ---------------------------------------------------------------------------

func TestWordCount(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{"empty", "", 0},
		{"single word", "hello", 1},
		{"two words", "hello world", 2},
		{"multiline", "one two\nthree four five", 5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEditor()
			e.LoadContent(tc.content, "/test.md")
			if e.GetWordCount() != tc.want {
				t.Errorf("expected word count %d, got %d", tc.want, e.GetWordCount())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// InsertText (multi-char + newline insertion)
// ---------------------------------------------------------------------------

func TestInsertText(t *testing.T) {
	e := NewEditor()
	e.lastSnapshot = time.Time{}
	e.InsertText("hello\nworld")

	if e.GetContent() != "hello\nworld" {
		t.Errorf("expected 'hello\\nworld', got %q", e.GetContent())
	}
	line, col := e.GetCursor()
	if line != 1 || col != 5 {
		t.Errorf("expected cursor (1,5), got (%d,%d)", line, col)
	}
}

// ---------------------------------------------------------------------------
// Delete key
// ---------------------------------------------------------------------------

func TestDelete_RemovesCharUnderCursor(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("abcd", "/test.md")
	e.focused = true
	e.col = 1 // cursor on 'b'
	e = sendKey(e, "delete")

	if e.GetContent() != "acd" {
		t.Errorf("expected 'acd', got %q", e.GetContent())
	}
}

func TestDelete_AtEndOfLine_JoinsNext(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("ab\ncd", "/test.md")
	e.focused = true
	e.col = 2 // at end of "ab"
	e = sendKey(e, "delete")

	if e.GetContent() != "abcd" {
		t.Errorf("expected 'abcd', got %q", e.GetContent())
	}
}

// ---------------------------------------------------------------------------
// Ghost text
// ---------------------------------------------------------------------------

func TestGhostText(t *testing.T) {
	e := NewEditor()
	e.SetGhostText("suggestion")
	if e.GetGhostText() != "suggestion" {
		t.Errorf("expected 'suggestion', got %q", e.GetGhostText())
	}

	e.SetGhostText("")
	if e.GetGhostText() != "" {
		t.Error("expected empty ghost text after clearing")
	}
}

// ---------------------------------------------------------------------------
// Multi-cursor basics
// ---------------------------------------------------------------------------

func TestHasMultiCursors(t *testing.T) {
	e := NewEditor()
	if e.HasMultiCursors() {
		t.Error("new editor should not have multi-cursors")
	}

	e.cursors = append(e.cursors, CursorPos{Line: 1, Col: 0})
	if !e.HasMultiCursors() {
		t.Error("expected HasMultiCursors to return true")
	}

	e.clearMultiCursors()
	if e.HasMultiCursors() {
		t.Error("expected no multi-cursors after clear")
	}
}

// ---------------------------------------------------------------------------
// Unfocused editor ignores input
// ---------------------------------------------------------------------------

func TestUnfocusedEditor_IgnoresInput(t *testing.T) {
	e := NewEditor()
	// focused is false by default
	e.LoadContent("original", "/test.md")
	e = sendKey(e, "x")
	if e.GetContent() != "original" {
		t.Error("unfocused editor should not process key input")
	}
}

// ---------------------------------------------------------------------------
// IsModified tracking
// ---------------------------------------------------------------------------

func TestModifiedFlag(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("text", "/test.md")
	e.focused = true

	if e.IsModified() {
		t.Error("should not be modified right after LoadContent")
	}

	e.col = 4
	e = sendKey(e, "!")
	if !e.IsModified() {
		t.Error("should be modified after inserting a character")
	}
}

// ---------------------------------------------------------------------------
// Home / End keys
// ---------------------------------------------------------------------------

func TestHomeKey(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("hello world", "/test.md")
	e.focused = true
	e.col = 5
	e = sendKey(e, "home")

	_, col := e.GetCursor()
	if col != 0 {
		t.Errorf("expected col 0 after Home, got %d", col)
	}
}

func TestEndKey(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("hello world", "/test.md")
	e.focused = true
	e.col = 0
	e = sendKey(e, "end")

	_, col := e.GetCursor()
	if col != 11 {
		t.Errorf("expected col 11 after End, got %d", col)
	}
}

// ---------------------------------------------------------------------------
// Multiple sequential edits
// ---------------------------------------------------------------------------

func TestSequentialEdits(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("", "/test.md")
	e.focused = true

	for _, ch := range "Go" {
		e = sendKey(e, string(ch))
	}
	if !strings.HasPrefix(e.GetContent(), "Go") {
		t.Errorf("expected content to start with 'Go', got %q", e.GetContent())
	}
}
