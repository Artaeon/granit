package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// Constructor & Initial State
// ---------------------------------------------------------------------------

func TestFindReplace_NewFindReplace(t *testing.T) {
	fr := NewFindReplace()
	if fr.active {
		t.Error("expected inactive after construction")
	}
	if fr.resultLine != -1 {
		t.Errorf("expected resultLine=-1, got %d", fr.resultLine)
	}
	if fr.historyIdx != -1 {
		t.Errorf("expected historyIdx=-1, got %d", fr.historyIdx)
	}
	if fr.findQuery != "" {
		t.Error("expected empty findQuery")
	}
	if fr.replaceText != "" {
		t.Error("expected empty replaceText")
	}
	if len(fr.matches) != 0 {
		t.Error("expected no matches")
	}
	if fr.regexMode {
		t.Error("expected regex mode off")
	}
}

// ---------------------------------------------------------------------------
// Open / Close / IsActive
// ---------------------------------------------------------------------------

func TestFindReplace_OpenFindCloseIsActive(t *testing.T) {
	fr := NewFindReplace()
	if fr.IsActive() {
		t.Error("should be inactive before Open")
	}

	fr.OpenFind(t.TempDir())
	if !fr.IsActive() {
		t.Error("should be active after OpenFind")
	}
	if fr.mode != 0 {
		t.Error("OpenFind should set mode=0 (find)")
	}

	fr.Close()
	if fr.IsActive() {
		t.Error("should be inactive after Close")
	}
}

func TestFindReplace_OpenReplace(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenReplace(t.TempDir())
	if !fr.IsActive() {
		t.Error("should be active after OpenReplace")
	}
	if fr.mode != 1 {
		t.Error("OpenReplace should set mode=1 (replace)")
	}
}

// ---------------------------------------------------------------------------
// UpdateMatches — plain text
// ---------------------------------------------------------------------------

func TestFindReplace_SetContentAndFindMatches(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenFind(t.TempDir())

	content := []string{
		"Hello world",
		"hello again",
		"HELLO HELLO",
		"no match here",
	}

	// Type query "hello"
	fr.findQuery = "hello"
	fr.UpdateMatches(content)

	// "hello" appears case-insensitively: line 0 (1), line 1 (1), line 2 (2) = 4
	if len(fr.matches) != 4 {
		t.Errorf("expected 4 matches, got %d", len(fr.matches))
	}
}

func TestFindReplace_MatchCountMultiplePerLine(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenFind(t.TempDir())

	content := []string{"aaa bbb aaa ccc aaa"}
	fr.findQuery = "aaa"
	fr.UpdateMatches(content)

	if len(fr.matches) != 3 {
		t.Errorf("expected 3 matches for 'aaa', got %d", len(fr.matches))
	}
}

// ---------------------------------------------------------------------------
// Empty query & no matches
// ---------------------------------------------------------------------------

func TestFindReplace_EmptyQuery(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenFind(t.TempDir())

	content := []string{"some text", "more text"}
	fr.findQuery = ""
	fr.UpdateMatches(content)

	if len(fr.matches) != 0 {
		t.Errorf("expected 0 matches for empty query, got %d", len(fr.matches))
	}
}

func TestFindReplace_NoMatches(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenFind(t.TempDir())

	content := []string{"some text", "more text"}
	fr.findQuery = "zzzzz"
	fr.UpdateMatches(content)

	if len(fr.matches) != 0 {
		t.Errorf("expected 0 matches for non-existent query, got %d", len(fr.matches))
	}
	if fr.matchIdx != 0 {
		t.Errorf("expected matchIdx=0 when no matches, got %d", fr.matchIdx)
	}
}

// ---------------------------------------------------------------------------
// Match navigation — next/prev with wrap-around
// ---------------------------------------------------------------------------

func TestFindReplace_MatchNavigationNextPrev(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenFind(t.TempDir())

	content := []string{"foo bar", "foo baz", "foo qux"}
	fr.findQuery = "foo"
	fr.UpdateMatches(content)

	if len(fr.matches) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(fr.matches))
	}

	// Initial position
	if fr.matchIdx != 0 {
		t.Errorf("expected matchIdx=0, got %d", fr.matchIdx)
	}

	// Navigate down/next
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyDown})
	if fr.matchIdx != 1 {
		t.Errorf("after down, expected matchIdx=1, got %d", fr.matchIdx)
	}

	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyDown})
	if fr.matchIdx != 2 {
		t.Errorf("after down, expected matchIdx=2, got %d", fr.matchIdx)
	}

	// Wrap around to first
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyDown})
	if fr.matchIdx != 0 {
		t.Errorf("after wrap, expected matchIdx=0, got %d", fr.matchIdx)
	}

	// Navigate up wraps to last
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyUp})
	if fr.matchIdx != 2 {
		t.Errorf("after up wrap, expected matchIdx=2, got %d", fr.matchIdx)
	}

	// Navigate up again
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyUp})
	if fr.matchIdx != 1 {
		t.Errorf("after up, expected matchIdx=1, got %d", fr.matchIdx)
	}
}

func TestFindReplace_WrapAround(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenFind(t.TempDir())

	content := []string{"x y x"}
	fr.findQuery = "x"
	fr.UpdateMatches(content)

	if len(fr.matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(fr.matches))
	}

	// Forward: 0 -> 1 -> 0
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyDown})
	if fr.matchIdx != 1 {
		t.Errorf("expected 1, got %d", fr.matchIdx)
	}
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyDown})
	if fr.matchIdx != 0 {
		t.Errorf("expected wrap to 0, got %d", fr.matchIdx)
	}

	// Backward: 0 -> 1 (wrap)
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyUp})
	if fr.matchIdx != 1 {
		t.Errorf("expected wrap to 1, got %d", fr.matchIdx)
	}
}

// ---------------------------------------------------------------------------
// Replace & Replace All flags
// ---------------------------------------------------------------------------

func TestFindReplace_Replace(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenReplace(t.TempDir())

	content := []string{"hello world hello"}
	fr.findQuery = "hello"
	fr.UpdateMatches(content)

	// Type replacement text
	fr.focusField = 1
	fr.replaceText = "hi"

	// Press Enter on replace field triggers single replace
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !fr.ShouldReplace() {
		t.Error("expected doReplace=true after Enter on replace field")
	}
	// ShouldReplace resets the flag
	if fr.ShouldReplace() {
		t.Error("ShouldReplace should be false after first read")
	}

	if fr.GetFindQuery() != "hello" {
		t.Errorf("expected findQuery='hello', got '%s'", fr.GetFindQuery())
	}
	if fr.GetReplaceText() != "hi" {
		t.Errorf("expected replaceText='hi', got '%s'", fr.GetReplaceText())
	}
}

func TestFindReplace_ReplaceAll(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenReplace(t.TempDir())

	content := []string{"aaa bbb aaa"}
	fr.findQuery = "aaa"
	fr.UpdateMatches(content)
	fr.replaceText = "ccc"

	// Ctrl+A triggers replace all in replace mode
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	if !fr.ShouldReplaceAll() {
		t.Error("expected doReplaceAll=true after Ctrl+A")
	}
	// Resets after read
	if fr.ShouldReplaceAll() {
		t.Error("ShouldReplaceAll should be false after first read")
	}
}

func TestFindReplace_ReplaceAllNotInFindMode(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenFind(t.TempDir()) // mode=0, not replace

	content := []string{"aaa bbb aaa"}
	fr.findQuery = "aaa"
	fr.UpdateMatches(content)

	// Ctrl+A should NOT trigger replace all in find mode
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	if fr.ShouldReplaceAll() {
		t.Error("replace all should not trigger in find-only mode")
	}
}

// ---------------------------------------------------------------------------
// Case sensitivity — plain search is always case-insensitive
// ---------------------------------------------------------------------------

func TestFindReplace_CaseInsensitivePlain(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenFind(t.TempDir())

	content := []string{"Hello", "HELLO", "hello", "hElLo"}
	fr.findQuery = "hello"
	fr.UpdateMatches(content)

	if len(fr.matches) != 4 {
		t.Errorf("expected 4 case-insensitive matches, got %d", len(fr.matches))
	}
}

func TestFindReplace_CaseInsensitiveUpperQuery(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenFind(t.TempDir())

	content := []string{"hello world"}
	fr.findQuery = "HELLO"
	fr.UpdateMatches(content)

	if len(fr.matches) != 1 {
		t.Errorf("expected 1 match for uppercase query, got %d", len(fr.matches))
	}
}

// ---------------------------------------------------------------------------
// Regex mode
// ---------------------------------------------------------------------------

func TestFindReplace_RegexMode(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenFind(t.TempDir())

	if fr.IsRegexMode() {
		t.Error("regex mode should be off by default")
	}

	fr.ToggleRegex()
	if !fr.IsRegexMode() {
		t.Error("regex mode should be on after toggle")
	}

	content := []string{"foo123", "bar456", "baz"}
	fr.findQuery = `\d+`
	fr.UpdateMatches(content)

	if len(fr.matches) != 2 {
		t.Errorf("expected 2 regex matches for \\d+, got %d", len(fr.matches))
	}

	// Toggle back off
	fr.ToggleRegex()
	if fr.IsRegexMode() {
		t.Error("regex mode should be off after second toggle")
	}
}

func TestFindReplace_RegexToggleViaUpdate(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenFind(t.TempDir())

	// alt+r toggles regex mode
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}, Alt: true})
	if !fr.regexMode {
		t.Error("alt+r should enable regex mode")
	}

	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}, Alt: true})
	if fr.regexMode {
		t.Error("alt+r again should disable regex mode")
	}
}

func TestFindReplace_RegexInvalidPattern(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenFind(t.TempDir())
	fr.ToggleRegex()

	content := []string{"hello world"}
	fr.findQuery = "[invalid"
	fr.UpdateMatches(content)

	if fr.regexErr == "" {
		t.Error("expected regex error for invalid pattern")
	}
	if len(fr.matches) != 0 {
		t.Error("expected no matches for invalid regex")
	}
}

func TestFindReplace_RegexCaseInsensitive(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenFind(t.TempDir())
	fr.ToggleRegex()

	content := []string{"Hello", "HELLO", "hello"}
	fr.findQuery = "hello"
	fr.UpdateMatches(content)

	// Regex uses (?i) prefix, so all should match
	if len(fr.matches) != 3 {
		t.Errorf("expected 3 case-insensitive regex matches, got %d", len(fr.matches))
	}
}

// ---------------------------------------------------------------------------
// Typing characters and backspace via Update
// ---------------------------------------------------------------------------

func TestFindReplace_TypingAndBackspace(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenFind(t.TempDir())

	// Type "hi"
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if fr.findQuery != "hi" {
		t.Errorf("expected findQuery='hi', got '%s'", fr.findQuery)
	}

	// Backspace
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if fr.findQuery != "h" {
		t.Errorf("expected findQuery='h' after backspace, got '%s'", fr.findQuery)
	}

	// Backspace again
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if fr.findQuery != "" {
		t.Errorf("expected empty findQuery after double backspace, got '%s'", fr.findQuery)
	}

	// Backspace on empty — no crash
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if fr.findQuery != "" {
		t.Error("backspace on empty should remain empty")
	}
}

func TestFindReplace_TypingInReplaceField(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenReplace(t.TempDir())

	// Switch to replace field
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyTab})
	if fr.focusField != 1 {
		t.Errorf("expected focusField=1 after Tab, got %d", fr.focusField)
	}

	// Type "ab"
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if fr.replaceText != "ab" {
		t.Errorf("expected replaceText='ab', got '%s'", fr.replaceText)
	}

	// Backspace in replace field
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if fr.replaceText != "a" {
		t.Errorf("expected replaceText='a', got '%s'", fr.replaceText)
	}
}

// ---------------------------------------------------------------------------
// Esc closes overlay
// ---------------------------------------------------------------------------

func TestFindReplace_EscCloses(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenFind(t.TempDir())

	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if fr.IsActive() {
		t.Error("Esc should close find/replace")
	}
}

// ---------------------------------------------------------------------------
// Enter on find field jumps to match
// ---------------------------------------------------------------------------

func TestFindReplace_EnterJumpsToMatch(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenFind(t.TempDir())

	content := []string{"line0", "line1 target", "line2"}
	fr.findQuery = "target"
	fr.UpdateMatches(content)

	if len(fr.matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(fr.matches))
	}

	// Enter on find field should set resultLine
	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyEnter})
	jumpLine := fr.GetJumpLine()
	if jumpLine != 1 {
		t.Errorf("expected jump to line 1, got %d", jumpLine)
	}

	// GetJumpLine resets
	if fr.GetJumpLine() != -1 {
		t.Error("GetJumpLine should return -1 after first read")
	}
}

// ---------------------------------------------------------------------------
// Inactive Update is no-op
// ---------------------------------------------------------------------------

func TestFindReplace_InactiveUpdateNoop(t *testing.T) {
	fr := NewFindReplace()
	// Not opened — Update should be a no-op
	fr2, cmd := fr.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if cmd != nil {
		t.Error("expected nil cmd for inactive update")
	}
	if fr2.findQuery != "" {
		t.Error("inactive update should not change state")
	}
}

// ---------------------------------------------------------------------------
// Tab switches focus only in replace mode
// ---------------------------------------------------------------------------

func TestFindReplace_TabSwitchesFocusInReplaceMode(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenReplace(t.TempDir())

	if fr.focusField != 0 {
		t.Error("should start on find field")
	}

	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyTab})
	if fr.focusField != 1 {
		t.Error("Tab should switch to replace field")
	}

	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyTab})
	if fr.focusField != 0 {
		t.Error("Tab should cycle back to find field")
	}
}

func TestFindReplace_TabNoOpInFindMode(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenFind(t.TempDir())

	fr, _ = fr.Update(tea.KeyMsg{Type: tea.KeyTab})
	// In find mode (mode=0), Tab should not change focus
	if fr.focusField != 0 {
		t.Errorf("Tab in find mode should not change focusField, got %d", fr.focusField)
	}
}

// ---------------------------------------------------------------------------
// Match positions are correct
// ---------------------------------------------------------------------------

func TestFindReplace_MatchPositions(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenFind(t.TempDir())

	content := []string{"abc def abc", "xyz abc"}
	fr.findQuery = "abc"
	fr.UpdateMatches(content)

	if len(fr.matches) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(fr.matches))
	}

	// First match: line 0, col 0
	if fr.matches[0].line != 0 || fr.matches[0].col != 0 {
		t.Errorf("match 0: expected line=0 col=0, got line=%d col=%d", fr.matches[0].line, fr.matches[0].col)
	}
	// Second match: line 0, col 8
	if fr.matches[1].line != 0 || fr.matches[1].col != 8 {
		t.Errorf("match 1: expected line=0 col=8, got line=%d col=%d", fr.matches[1].line, fr.matches[1].col)
	}
	// Third match: line 1, col 4
	if fr.matches[2].line != 1 || fr.matches[2].col != 4 {
		t.Errorf("match 2: expected line=1 col=4, got line=%d col=%d", fr.matches[2].line, fr.matches[2].col)
	}
}

// ---------------------------------------------------------------------------
// matchIdx resets when matches shrink
// ---------------------------------------------------------------------------

func TestFindReplace_MatchIdxResetsOnNewQuery(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenFind(t.TempDir())

	content := []string{"aaa", "aaa", "aaa", "bbb"}

	fr.findQuery = "aaa"
	fr.UpdateMatches(content)
	// Navigate to last match
	fr.matchIdx = 2

	// Now search for something with fewer matches
	fr.findQuery = "bbb"
	fr.UpdateMatches(content)
	if fr.matchIdx != 0 {
		t.Errorf("expected matchIdx reset to 0, got %d", fr.matchIdx)
	}
}

// ---------------------------------------------------------------------------
// SetSize
// ---------------------------------------------------------------------------

func TestFindReplace_SetSize(t *testing.T) {
	fr := NewFindReplace()
	fr.SetSize(120, 40)
	if fr.width != 120 {
		t.Errorf("expected width=120, got %d", fr.width)
	}
	if fr.height != 40 {
		t.Errorf("expected height=40, got %d", fr.height)
	}
}

// ---------------------------------------------------------------------------
// View does not panic
// ---------------------------------------------------------------------------

func TestFindReplace_ViewNoPanic(t *testing.T) {
	fr := NewFindReplace()
	fr.OpenFind(t.TempDir())
	fr.SetSize(100, 30)

	// View with no matches
	_ = fr.View()

	// View with matches
	content := []string{"hello world", "hello there"}
	fr.findQuery = "hello"
	fr.UpdateMatches(content)
	v := fr.View()
	if v == "" {
		t.Error("expected non-empty view")
	}

	// View in replace mode
	fr.OpenReplace(t.TempDir())
	fr.SetSize(100, 30)
	fr.findQuery = "hello"
	fr.UpdateMatches(content)
	v = fr.View()
	if v == "" {
		t.Error("expected non-empty view in replace mode")
	}
}
