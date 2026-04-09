package tui

import "testing"

func TestCaseInsensitiveReplaceFirst_Basic(t *testing.T) {
	result := caseInsensitiveReplaceFirst("Hello World Hello", "hello", "Hi")
	if result != "Hi World Hello" {
		t.Errorf("expected 'Hi World Hello', got %q", result)
	}
}

func TestCaseInsensitiveReplaceFirst_CasePreserved(t *testing.T) {
	result := caseInsensitiveReplaceFirst("The QUICK brown fox", "quick", "slow")
	if result != "The slow brown fox" {
		t.Errorf("expected 'The slow brown fox', got %q", result)
	}
}

func TestCaseInsensitiveReplaceFirst_NoMatch(t *testing.T) {
	result := caseInsensitiveReplaceFirst("Hello World", "xyz", "abc")
	if result != "Hello World" {
		t.Errorf("expected unchanged, got %q", result)
	}
}

func TestCaseInsensitiveReplaceFirst_Empty(t *testing.T) {
	result := caseInsensitiveReplaceFirst("Hello", "", "x")
	if result != "Hello" {
		t.Errorf("empty old should return unchanged, got %q", result)
	}
}

func TestCaseInsensitiveReplaceFirst_OnlyFirst(t *testing.T) {
	result := caseInsensitiveReplaceFirst("aaa", "a", "b")
	if result != "baa" {
		t.Errorf("should only replace first, got %q", result)
	}
}

func TestCaseInsensitiveReplaceAll_Basic(t *testing.T) {
	result := caseInsensitiveReplaceAll("Hello hello HELLO", "hello", "Hi")
	if result != "Hi Hi Hi" {
		t.Errorf("expected 'Hi Hi Hi', got %q", result)
	}
}

func TestCaseInsensitiveReplaceAll_NoMatch(t *testing.T) {
	result := caseInsensitiveReplaceAll("Hello World", "xyz", "abc")
	if result != "Hello World" {
		t.Errorf("expected unchanged, got %q", result)
	}
}

func TestCaseInsensitiveReplaceAll_Empty(t *testing.T) {
	result := caseInsensitiveReplaceAll("Hello", "", "x")
	if result != "Hello" {
		t.Errorf("empty old should return unchanged, got %q", result)
	}
}

func TestCaseInsensitiveReplaceAll_AllOccurrences(t *testing.T) {
	result := caseInsensitiveReplaceAll("Go is great. go is fast. GO is typed.", "go", "Rust")
	if result != "Rust is great. Rust is fast. Rust is typed." {
		t.Errorf("expected all replaced, got %q", result)
	}
}

func TestGlobalReplace_HighlightMatch(t *testing.T) {
	gr := &GlobalReplace{findQuery: "hello"}

	highlighted := gr.highlightMatch("say hello world", 80)
	if highlighted == "" {
		t.Error("expected non-empty highlight output")
	}
}

func TestGlobalReplace_ModifiedFiles_Empty(t *testing.T) {
	gr := NewGlobalReplace()
	mods := gr.ModifiedFiles()
	if len(mods) != 0 {
		t.Errorf("expected 0 modified files, got %d", len(mods))
	}
}

func TestGlobalReplace_GetJumpResult_NoResult(t *testing.T) {
	gr := NewGlobalReplace()
	_, _, ok := gr.GetJumpResult()
	if ok {
		t.Error("expected no jump result for new global replace")
	}
}
