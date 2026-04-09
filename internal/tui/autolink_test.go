package tui

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

func TestIsWordBoundary_StartOfLine(t *testing.T) {
	if !isWordBoundary("hello world", 0, 5) {
		t.Error("word at start of line should be a boundary")
	}
}

func TestIsWordBoundary_EndOfLine(t *testing.T) {
	if !isWordBoundary("hello world", 6, 5) {
		t.Error("word at end of line should be a boundary")
	}
}

func TestIsWordBoundary_PartOfWord(t *testing.T) {
	if isWordBoundary("unhappy", 2, 3) {
		t.Error("'hap' inside 'unhappy' should NOT be a word boundary")
	}
}

func TestIsWordBoundary_WithPunctuation(t *testing.T) {
	if !isWordBoundary("hello, world", 7, 5) {
		t.Error("word after comma+space should be a boundary")
	}
}

func TestIsInsideWikilink(t *testing.T) {
	if !isInsideWikilink("see [[hello world]]", 6) {
		t.Error("position inside [[ ]] should return true")
	}
	if isInsideWikilink("see [[hello]] world", 15) {
		t.Error("position after ]] should return false")
	}
	if isInsideWikilink("no links here", 5) {
		t.Error("no [[ should return false")
	}
}

func TestIsInCodeSpan(t *testing.T) {
	if !isInCodeSpan("text `code here` end", 8) {
		t.Error("position inside backticks should be in code span")
	}
	if isInCodeSpan("text `code` end", 13) {
		t.Error("position after closing backtick should not be in code span")
	}
	if isInCodeSpan("no code here", 5) {
		t.Error("no backticks should return false")
	}
}

func TestIsAlphaNum(t *testing.T) {
	for _, ch := range []byte{'a', 'z', 'A', 'Z', '0', '9', '_', '-'} {
		if !isAlphaNum(ch) {
			t.Errorf("expected %c to be alphanumeric", ch)
		}
	}
	for _, ch := range []byte{' ', '.', ',', '!', '(', ')', '[', ']'} {
		if isAlphaNum(ch) {
			t.Errorf("expected %c to NOT be alphanumeric", ch)
		}
	}
}

// ---------------------------------------------------------------------------
// FindUnlinkedMentions — fenced code block protection
// ---------------------------------------------------------------------------

func TestAutoLinker_SkipsFencedCodeBlocks(t *testing.T) {
	al := &AutoLinker{}
	al.SetNotes([]string{"MyProject.md"})

	content := "# Notes\n\nMyProject is great.\n\n```\nMyProject reference in code\n```\n\nMore text about MyProject.\n"
	suggestions := al.FindUnlinkedMentions(content, "other.md")

	// Should find mentions in regular text but NOT inside fenced code
	for _, s := range suggestions {
		if s.Line == 5 { // line inside ```
			t.Error("should not suggest links inside fenced code blocks")
		}
	}
}

func TestAutoLinker_SkipsSelfReferences(t *testing.T) {
	al := &AutoLinker{}
	al.SetNotes([]string{"notes/hello.md", "notes/world.md"})

	content := "# Hello\n\nThis note mentions hello and world.\n"
	suggestions := al.FindUnlinkedMentions(content, "notes/hello.md")

	for _, s := range suggestions {
		if strings.Contains(strings.ToLower(s.NotePath), "hello") {
			t.Error("should not suggest linking to self")
		}
	}
}

func TestAutoLinker_SkipsExistingWikilinks(t *testing.T) {
	al := &AutoLinker{}
	al.SetNotes([]string{"ProjectX.md"})

	content := "See [[ProjectX]] for details. Also ProjectX is mentioned.\n"
	suggestions := al.FindUnlinkedMentions(content, "other.md")

	// The mention inside [[ ]] should be skipped, but the second one
	// (if detected as a separate occurrence) could be suggested
	for _, s := range suggestions {
		if s.Col >= 4 && s.Col <= 15 {
			t.Error("should not suggest links for text already in wikilinks")
		}
	}
}

func TestAutoLinker_EmptyContent(t *testing.T) {
	al := &AutoLinker{}
	al.SetNotes([]string{"test.md"})

	suggestions := al.FindUnlinkedMentions("", "other.md")
	if len(suggestions) != 0 {
		t.Errorf("expected 0 suggestions for empty content, got %d", len(suggestions))
	}
}

func TestAutoLinker_NoNotes(t *testing.T) {
	al := &AutoLinker{}

	suggestions := al.FindUnlinkedMentions("some content", "note.md")
	if len(suggestions) != 0 {
		t.Errorf("expected 0 suggestions with no notes, got %d", len(suggestions))
	}
}

func TestAutoLinker_ShortNamesSkipped(t *testing.T) {
	al := &AutoLinker{}
	al.SetNotes([]string{"AI.md", "Go.md", "notes/longname.md"})

	content := "AI and Go are mentioned. Also longname.\n"
	suggestions := al.FindUnlinkedMentions(content, "other.md")

	for _, s := range suggestions {
		noteName := strings.TrimSuffix(s.NotePath, ".md")
		if idx := strings.LastIndex(noteName, "/"); idx >= 0 {
			noteName = noteName[idx+1:]
		}
		if len(noteName) < 3 {
			t.Errorf("should skip short note names (< 3 chars): %q", noteName)
		}
	}
}
