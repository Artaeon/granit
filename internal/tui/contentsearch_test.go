package tui

import (
	"testing"
)

func newTestContentSearch(notes map[string]string) *ContentSearch {
	cs := &ContentSearch{
		noteContents: notes,
		maxResults:   50,
	}
	return cs
}

func TestContentSearch_PlainSearch(t *testing.T) {
	cs := newTestContentSearch(map[string]string{
		"notes/hello.md":   "Hello World\nThis is a test\nHello again",
		"notes/goodbye.md": "Goodbye World\nNothing here",
	})
	cs.query = "hello"
	cs.search()

	if len(cs.results) != 2 {
		t.Fatalf("expected 2 results for 'hello', got %d", len(cs.results))
	}
	// Exact case matches should come first
	if cs.results[0].Line != 0 {
		t.Errorf("expected first result at line 0, got %d", cs.results[0].Line)
	}
	if cs.results[0].FilePath != "notes/hello.md" {
		t.Errorf("expected first result in hello.md, got %q", cs.results[0].FilePath)
	}
}

func TestContentSearch_CaseInsensitive(t *testing.T) {
	cs := newTestContentSearch(map[string]string{
		"note.md": "The Quick Brown Fox",
	})
	cs.query = "quick"
	cs.search()

	if len(cs.results) != 1 {
		t.Fatalf("expected 1 case-insensitive result, got %d", len(cs.results))
	}
	if cs.results[0].Col != 4 {
		t.Errorf("expected match at col 4, got %d", cs.results[0].Col)
	}
}

func TestContentSearch_NoResults(t *testing.T) {
	cs := newTestContentSearch(map[string]string{
		"note.md": "Hello World",
	})
	cs.query = "xyz"
	cs.search()

	if len(cs.results) != 0 {
		t.Errorf("expected 0 results, got %d", len(cs.results))
	}
}

func TestContentSearch_EmptyQuery(t *testing.T) {
	cs := newTestContentSearch(map[string]string{
		"note.md": "Hello",
	})
	cs.query = ""
	cs.search()

	// Empty query should return no results
	if len(cs.results) != 0 {
		t.Errorf("expected 0 results for empty query, got %d", len(cs.results))
	}
}

func TestContentSearch_MaxResults(t *testing.T) {
	cs := newTestContentSearch(map[string]string{
		"note.md": "word\nword\nword\nword\nword\nword",
	})
	cs.maxResults = 3
	cs.query = "word"
	cs.search()

	if len(cs.results) > 3 {
		t.Errorf("expected at most 3 results, got %d", len(cs.results))
	}
}

func TestContentSearch_RegexSearch(t *testing.T) {
	cs := newTestContentSearch(map[string]string{
		"note.md": "foo123bar\nbaz456qux\nhello",
	})
	cs.query = `\d+`
	cs.regexMode = true
	cs.search()

	if len(cs.results) != 2 {
		t.Fatalf("expected 2 regex matches, got %d", len(cs.results))
	}
}

func TestContentSearch_InvalidRegex(t *testing.T) {
	cs := newTestContentSearch(map[string]string{
		"note.md": "hello",
	})
	cs.query = `[invalid`
	cs.regexMode = true
	cs.search()

	if cs.regexErr == "" {
		t.Error("expected regex error for invalid pattern")
	}
	if len(cs.results) != 0 {
		t.Errorf("expected 0 results on regex error, got %d", len(cs.results))
	}
}

func TestContentSearch_FilenameMode(t *testing.T) {
	cs := newTestContentSearch(map[string]string{
		"notes/meeting.md": "content",
		"notes/tasks.md":   "content",
		"journal/day.md":   "content",
	})
	cs.filenameMode = true
	cs.query = "meet"
	cs.search()

	if len(cs.results) != 1 {
		t.Fatalf("expected 1 filename match, got %d", len(cs.results))
	}
	if cs.results[0].FilePath != "notes/meeting.md" {
		t.Errorf("expected meeting.md, got %q", cs.results[0].FilePath)
	}
}

func TestContentSearch_ExactCaseFirst(t *testing.T) {
	cs := newTestContentSearch(map[string]string{
		"note.md": "hello world\nHello World\nhELLO wORLD",
	})
	cs.query = "Hello"
	cs.search()

	if len(cs.results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(cs.results))
	}
	// Exact case match "Hello World" should come before fuzzy "hello world"
	if cs.results[0].Context != "Hello World" {
		t.Errorf("expected exact case match first, got %q", cs.results[0].Context)
	}
}
