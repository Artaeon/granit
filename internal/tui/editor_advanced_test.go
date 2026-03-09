package tui

import (
	"strings"
	"testing"
	"time"
)

// ===========================================================================
// SmartPaste tests
// ===========================================================================

func TestSmartPaste_PlainText(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("some text", "/test.md")
	e.focused = true
	e.col = 4

	// SmartPaste with plain text (not a URL) should return false
	result := e.SmartPaste("just plain text")
	if result {
		t.Error("SmartPaste should return false for plain text (non-URL)")
	}
	// Content should be unchanged since SmartPaste declined
	if e.GetContent() != "some text" {
		t.Errorf("content should be unchanged, got %q", e.GetContent())
	}
}

func TestSmartPaste_URL_NoSelection(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("hello world", "/test.md")
	e.focused = true
	e.col = 5
	e.lastSnapshot = time.Time{}

	result := e.SmartPaste("https://example.com")
	if !result {
		t.Fatal("SmartPaste should return true for a URL")
	}
	content := e.GetContent()
	if !strings.Contains(content, "[https://example.com](https://example.com)") {
		t.Errorf("expected [url](url) format, got %q", content)
	}
}

func TestSmartPaste_URL_WithTextSelected(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("click here for info", "/test.md")
	e.focused = true
	e.lastSnapshot = time.Time{}

	// Select "here" (cols 6..10)
	e.selectionActive = true
	e.selectionStart = CursorPos{Line: 0, Col: 6}
	e.selectionEnd = CursorPos{Line: 0, Col: 10}
	e.cursor = 0
	e.col = 10

	result := e.SmartPaste("https://example.com")
	if !result {
		t.Fatal("SmartPaste should return true for URL with selection")
	}
	content := e.GetContent()
	if !strings.Contains(content, "[here](https://example.com)") {
		t.Errorf("expected [selected](url) format, got %q", content)
	}
}

func TestSmartPaste_PlainText_SelectedURL(t *testing.T) {
	// Reverse case: clipboard is plain text, selected text is a URL
	e := focusedEditor()
	e.LoadContent("visit https://example.com today", "/test.md")
	e.focused = true
	e.lastSnapshot = time.Time{}

	// Select "https://example.com" (cols 6..25)
	e.selectionActive = true
	e.selectionStart = CursorPos{Line: 0, Col: 6}
	e.selectionEnd = CursorPos{Line: 0, Col: 25}
	e.cursor = 0
	e.col = 25

	result := e.SmartPaste("My Link")
	if !result {
		t.Fatal("SmartPaste should return true when selected text is a URL")
	}
	content := e.GetContent()
	if !strings.Contains(content, "[My Link](https://example.com)") {
		t.Errorf("expected [clipboard](selected_url) format, got %q", content)
	}
}

func TestSmartPaste_MultiLine(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("start", "/test.md")
	e.focused = true
	e.col = 0

	// Multi-line non-URL text: SmartPaste should return false
	result := e.SmartPaste("line one\nline two\nline three")
	if result {
		t.Error("SmartPaste should return false for multi-line non-URL text")
	}
	if e.GetContent() != "start" {
		t.Errorf("content should be unchanged, got %q", e.GetContent())
	}
}

func TestSmartPaste_TabToSpaces(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("content", "/test.md")
	e.focused = true
	e.col = 0

	// Text with tabs is not a URL, SmartPaste returns false
	result := e.SmartPaste("col1\tcol2\tcol3")
	if result {
		t.Error("SmartPaste should return false for text with tabs (non-URL)")
	}
	if e.GetContent() != "content" {
		t.Errorf("content should be unchanged, got %q", e.GetContent())
	}
}

// ===========================================================================
// findMatchingBracket tests
// ===========================================================================

func TestFindMatchingBracket_Parens(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("foo(bar)", "/test.md")
	e.focused = true

	// Cursor on '('
	e.cursor = 0
	e.col = 3
	e.codeFenceCacheDirty = true
	ln, col, found := e.findMatchingBracket()
	if !found {
		t.Fatal("expected to find matching bracket")
	}
	if ln != 0 || col != 7 {
		t.Errorf("expected match at (0,7), got (%d,%d)", ln, col)
	}

	// Cursor on ')'
	e.col = 7
	e.codeFenceCacheDirty = true
	ln, col, found = e.findMatchingBracket()
	if !found {
		t.Fatal("expected to find matching bracket for ')'")
	}
	if ln != 0 || col != 3 {
		t.Errorf("expected match at (0,3), got (%d,%d)", ln, col)
	}
}

func TestFindMatchingBracket_Braces(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("func() { return }", "/test.md")
	e.focused = true

	// Cursor on '{'
	e.cursor = 0
	e.col = 7
	e.codeFenceCacheDirty = true
	ln, col, found := e.findMatchingBracket()
	if !found {
		t.Fatal("expected to find matching brace")
	}
	if ln != 0 || col != 16 {
		t.Errorf("expected match at (0,16), got (%d,%d)", ln, col)
	}
}

func TestFindMatchingBracket_Brackets(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("arr[0]", "/test.md")
	e.focused = true

	// Cursor on '['
	e.cursor = 0
	e.col = 3
	e.codeFenceCacheDirty = true
	ln, col, found := e.findMatchingBracket()
	if !found {
		t.Fatal("expected to find matching bracket")
	}
	if ln != 0 || col != 5 {
		t.Errorf("expected match at (0,5), got (%d,%d)", ln, col)
	}
}

func TestFindMatchingBracket_Nested(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("f(g(x), h(y))", "/test.md")
	e.focused = true

	// Cursor on outermost '(' at col 1
	e.cursor = 0
	e.col = 1
	e.codeFenceCacheDirty = true
	ln, col, found := e.findMatchingBracket()
	if !found {
		t.Fatal("expected to find matching bracket for outer '('")
	}
	// The outermost ')' is at col 12
	if ln != 0 || col != 12 {
		t.Errorf("expected match at (0,12), got (%d,%d)", ln, col)
	}

	// Cursor on inner '(' at col 3
	e.col = 3
	e.codeFenceCacheDirty = true
	ln, col, found = e.findMatchingBracket()
	if !found {
		t.Fatal("expected to find matching bracket for inner '('")
	}
	// Inner ')' at col 5
	if ln != 0 || col != 5 {
		t.Errorf("expected match at (0,5), got (%d,%d)", ln, col)
	}
}

func TestFindMatchingBracket_MultiLine(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("if (\n  true\n)", "/test.md")
	e.focused = true

	// Cursor on '(' at line 0, col 3
	e.cursor = 0
	e.col = 3
	e.codeFenceCacheDirty = true
	ln, col, found := e.findMatchingBracket()
	if !found {
		t.Fatal("expected to find matching bracket across lines")
	}
	if ln != 2 || col != 0 {
		t.Errorf("expected match at (2,0), got (%d,%d)", ln, col)
	}
}

func TestFindMatchingBracket_NoBracket(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("hello world", "/test.md")
	e.focused = true

	e.cursor = 0
	e.col = 3 // on 'l', not a bracket
	e.codeFenceCacheDirty = true
	_, _, found := e.findMatchingBracket()
	if found {
		t.Error("expected no matching bracket when cursor is not on a bracket")
	}
}

func TestFindMatchingBracket_NoBracket_EmptyLine(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("", "/test.md")
	e.focused = true

	e.cursor = 0
	e.col = 0
	e.codeFenceCacheDirty = true
	_, _, found := e.findMatchingBracket()
	if found {
		t.Error("expected no matching bracket on empty line")
	}
}

// ===========================================================================
// wordUnderCursor tests
// ===========================================================================

func TestWordUnderCursor_Middle(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("hello world", "/test.md")
	e.focused = true

	// Cursor in the middle of "hello" (col 2 = 'l')
	e.cursor = 0
	e.col = 2
	word, start := e.wordUnderCursor()
	if word != "hello" {
		t.Errorf("expected word 'hello', got %q", word)
	}
	if start != 0 {
		t.Errorf("expected start col 0, got %d", start)
	}
}

func TestWordUnderCursor_Start(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("hello world", "/test.md")
	e.focused = true

	// Cursor at start of "hello" (col 0 = 'h')
	e.cursor = 0
	e.col = 0
	word, start := e.wordUnderCursor()
	if word != "hello" {
		t.Errorf("expected word 'hello', got %q", word)
	}
	if start != 0 {
		t.Errorf("expected start col 0, got %d", start)
	}
}

func TestWordUnderCursor_End(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("hello world", "/test.md")
	e.focused = true

	// Cursor at end of "hello" (col 5 = space), should try col-1 and find 'o'
	e.cursor = 0
	e.col = 5
	word, start := e.wordUnderCursor()
	if word != "hello" {
		t.Errorf("expected word 'hello', got %q", word)
	}
	if start != 0 {
		t.Errorf("expected start col 0, got %d", start)
	}
}

func TestWordUnderCursor_SecondWord(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("hello world", "/test.md")
	e.focused = true

	// Cursor in middle of "world" (col 8 = 'r')
	e.cursor = 0
	e.col = 8
	word, start := e.wordUnderCursor()
	if word != "world" {
		t.Errorf("expected word 'world', got %q", word)
	}
	if start != 6 {
		t.Errorf("expected start col 6, got %d", start)
	}
}

func TestWordUnderCursor_Empty(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("", "/test.md")
	e.focused = true

	e.cursor = 0
	e.col = 0
	word, _ := e.wordUnderCursor()
	if word != "" {
		t.Errorf("expected empty word on empty line, got %q", word)
	}
}

func TestWordUnderCursor_Unicode(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("héllo wörld", "/test.md")
	e.focused = true

	// Cursor in the middle of "héllo" (col 2 = 'l')
	e.cursor = 0
	e.col = 2
	word, start := e.wordUnderCursor()
	if word != "héllo" {
		t.Errorf("expected word 'héllo', got %q", word)
	}
	if start != 0 {
		t.Errorf("expected start col 0, got %d", start)
	}
}

func TestWordUnderCursor_Underscore(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("my_var = 42", "/test.md")
	e.focused = true

	// Cursor on '_' (col 2)
	e.cursor = 0
	e.col = 2
	word, start := e.wordUnderCursor()
	if word != "my_var" {
		t.Errorf("expected word 'my_var', got %q", word)
	}
	if start != 0 {
		t.Errorf("expected start col 0, got %d", start)
	}
}

// ===========================================================================
// findAndAddNextOccurrence tests
// ===========================================================================

func TestFindAndAddNextOccurrence_Basic(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("foo bar foo baz foo", "/test.md")
	e.focused = true

	// Set up as if Ctrl+D selected "foo" at col 0
	e.cursor = 0
	e.col = 0
	e.multiWord = "foo"

	initialCursorCount := len(e.cursors) // 0 additional cursors

	// Find next occurrence: should add a cursor at the second "foo" (col 8)
	e.findAndAddNextOccurrence()

	if len(e.cursors) != initialCursorCount+1 {
		t.Fatalf("expected %d additional cursors, got %d", initialCursorCount+1, len(e.cursors))
	}
	newCursor := e.cursors[len(e.cursors)-1]
	if newCursor.Line != 0 || newCursor.Col != 8 {
		t.Errorf("expected new cursor at (0,8), got (%d,%d)", newCursor.Line, newCursor.Col)
	}
}

func TestFindAndAddNextOccurrence_MultiLine(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("foo bar\nbaz foo\nqux", "/test.md")
	e.focused = true

	// Select "foo" on line 0 col 0
	e.cursor = 0
	e.col = 0
	e.multiWord = "foo"

	e.findAndAddNextOccurrence()

	if len(e.cursors) != 1 {
		t.Fatalf("expected 1 additional cursor, got %d", len(e.cursors))
	}
	newCursor := e.cursors[0]
	if newCursor.Line != 1 || newCursor.Col != 4 {
		t.Errorf("expected new cursor at (1,4), got (%d,%d)", newCursor.Line, newCursor.Col)
	}
}

func TestFindAndAddNextOccurrence_NoMore(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("foo bar baz", "/test.md")
	e.focused = true

	// Only one "foo" in the content
	e.cursor = 0
	e.col = 0
	e.multiWord = "foo"

	cursorsBefore := len(e.cursors)
	e.findAndAddNextOccurrence()

	if len(e.cursors) != cursorsBefore {
		t.Errorf("expected no new cursors when no more occurrences, got %d additional", len(e.cursors)-cursorsBefore)
	}
}

func TestFindAndAddNextOccurrence_WrapsAround(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("foo bar foo baz foo", "/test.md")
	e.focused = true

	// Start at the last "foo" (col 16)
	e.cursor = 0
	e.col = 16
	e.multiWord = "foo"

	// Should wrap around and find "foo" at col 0
	e.findAndAddNextOccurrence()

	if len(e.cursors) != 1 {
		t.Fatalf("expected 1 additional cursor after wrap, got %d", len(e.cursors))
	}
	newCursor := e.cursors[0]
	if newCursor.Line != 0 || newCursor.Col != 0 {
		t.Errorf("expected wrapped cursor at (0,0), got (%d,%d)", newCursor.Line, newCursor.Col)
	}
}

func TestFindAndAddNextOccurrence_WholeWordOnly(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("foo foobar foo", "/test.md")
	e.focused = true

	// Select "foo" at col 0
	e.cursor = 0
	e.col = 0
	e.multiWord = "foo"

	// First next should skip "foobar" and find the second standalone "foo" at col 11
	e.findAndAddNextOccurrence()

	if len(e.cursors) != 1 {
		t.Fatalf("expected 1 additional cursor, got %d", len(e.cursors))
	}
	newCursor := e.cursors[0]
	if newCursor.Line != 0 || newCursor.Col != 11 {
		t.Errorf("expected cursor at (0,11) skipping 'foobar', got (%d,%d)", newCursor.Line, newCursor.Col)
	}
}

// ===========================================================================
// Undo/Redo deep copy integrity (no corruption)
// ===========================================================================

func TestUndoRedo_NoCorruption(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("", "/test.md")
	e.focused = true

	// Step 1: Type "AAA"
	e.lastSnapshot = time.Time{}
	e.col = 0
	for _, ch := range "AAA" {
		e = sendKey(e, string(ch))
	}
	if e.GetContent() != "AAA" {
		t.Fatalf("expected 'AAA', got %q", e.GetContent())
	}

	// Step 2: Undo -> should go back to ""
	e.Undo()
	state1 := e.GetContent()
	if state1 != "" {
		t.Fatalf("expected empty after first undo, got %q", state1)
	}

	// Step 3: Type "BBB" (diverging from original timeline)
	e.lastSnapshot = time.Time{}
	e.col = 0
	for _, ch := range "BBB" {
		e = sendKey(e, string(ch))
	}
	if e.GetContent() != "BBB" {
		t.Fatalf("expected 'BBB', got %q", e.GetContent())
	}

	// Step 4: Undo -> should go back to "" (the state before "BBB")
	e.Undo()
	state2 := e.GetContent()
	if state2 != "" {
		t.Errorf("expected empty after second undo (no corruption), got %q", state2)
	}

	// Step 5: Redo -> should go forward to "BBB" (not "AAA")
	e.Redo()
	state3 := e.GetContent()
	if state3 != "BBB" {
		t.Errorf("expected 'BBB' after redo, got %q", state3)
	}
}

func TestUndoRedo_MultipleSteps_Integrity(t *testing.T) {
	e := focusedEditor()
	e.LoadContent("base", "/test.md")
	e.focused = true

	// Record several distinct states
	states := []string{"base"}
	words := []string{"first", "second", "third"}
	for _, word := range words {
		e.lastSnapshot = time.Time{}
		e.SetContent("base " + word)
		states = append(states, e.GetContent())
	}

	// Undo all the way back
	for i := len(words) - 1; i >= 0; i-- {
		e.Undo()
		got := e.GetContent()
		expected := states[i]
		if got != expected {
			t.Errorf("undo step %d: expected %q, got %q", i, expected, got)
		}
	}

	// Redo all the way forward
	for i := 0; i < len(words); i++ {
		e.Redo()
		got := e.GetContent()
		expected := states[i+1]
		if got != expected {
			t.Errorf("redo step %d: expected %q, got %q", i, expected, got)
		}
	}
}
