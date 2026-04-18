package tui

import (
	"reflect"
	"testing"
)

// ── Tokenizer ──

func TestTokenizeSearchQuery_PreservesQuotedPhrases(t *testing.T) {
	got := tokenizeSearchQuery(`foo "shipped on Tuesday" bar`)
	want := []string{`foo`, `"shipped on Tuesday"`, `bar`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTokenizeSearchQuery_HandlesUnclosedQuote(t *testing.T) {
	// Half-typed query — must not panic and must keep the partial token.
	got := tokenizeSearchQuery(`foo "incomplete`)
	if len(got) != 2 {
		t.Errorf("got %q, want 2 tokens", got)
	}
}

func TestTokenizeSearchQuery_CollapsesWhitespace(t *testing.T) {
	got := tokenizeSearchQuery("  a   b\tc")
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ── Parser ──

func TestParseSearchQuery_AllOperators(t *testing.T) {
	q := ParseSearchQuery(`tag:work path:projects/ -draft "ship it" review`)
	if len(q.Tags) != 1 || q.Tags[0] != "work" {
		t.Errorf("tags = %+v", q.Tags)
	}
	if len(q.Paths) != 1 || q.Paths[0] != "projects/" {
		t.Errorf("paths = %+v", q.Paths)
	}
	if len(q.Excludes) != 1 || q.Excludes[0] != "draft" {
		t.Errorf("excludes = %+v", q.Excludes)
	}
	if len(q.Phrases) != 1 || q.Phrases[0] != "ship it" {
		t.Errorf("phrases = %+v", q.Phrases)
	}
	if len(q.Terms) != 1 || q.Terms[0] != "review" {
		t.Errorf("terms = %+v", q.Terms)
	}
}

func TestParseSearchQuery_PlainQueryHasNoOperators(t *testing.T) {
	q := ParseSearchQuery("just plain words")
	if q.HasOperators() {
		t.Errorf("plain query reports HasOperators=true: %+v", q)
	}
	if got := q.PlainQuery(); got != "just plain words" {
		t.Errorf("PlainQuery = %q", got)
	}
}

func TestParseSearchQuery_TagAcceptsHashPrefix(t *testing.T) {
	q := ParseSearchQuery("tag:#work")
	if len(q.Tags) != 1 || q.Tags[0] != "work" {
		t.Errorf("expected tag without leading #, got %+v", q.Tags)
	}
}

// ── Filter helpers ──

func TestSearchQuery_MatchesPath(t *testing.T) {
	q := ParseSearchQuery("path:projects/ path:work/")
	if !q.MatchesPath("projects/foo.md") {
		t.Error("projects/foo.md should match path:projects/")
	}
	if !q.MatchesPath("/abs/work/notes.md") {
		t.Error("any path: matches OR semantics")
	}
	if q.MatchesPath("random/elsewhere.md") {
		t.Error("non-matching path should be rejected")
	}
}

func TestSearchQuery_MatchesTags_AndSemantics(t *testing.T) {
	q := ParseSearchQuery("tag:work tag:review")
	if !q.MatchesTags("a #work b #review c") {
		t.Error("both tags present — should match")
	}
	if q.MatchesTags("a #work only") {
		t.Error("only one tag present — AND semantics should reject")
	}
}

func TestSearchQuery_MatchesExcludes(t *testing.T) {
	q := ParseSearchQuery("foo -draft")
	if !q.MatchesExcludes("a clean note") {
		t.Error("no excluded terms — should pass")
	}
	if q.MatchesExcludes("contains DRAFT marker") {
		t.Error("excluded term present — should reject (case-insensitive)")
	}
}

func TestSearchQuery_MatchesPhrases(t *testing.T) {
	q := ParseSearchQuery(`"exact phrase"`)
	if !q.MatchesPhrases("we wrote the EXACT phrase here") {
		t.Error("phrase present (case-insensitive) — should match")
	}
	if q.MatchesPhrases("words in different order") {
		t.Error("phrase absent — should reject")
	}
}
