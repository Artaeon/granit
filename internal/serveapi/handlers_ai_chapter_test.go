package serveapi

import (
	"strings"
	"testing"
)

// Pure-function tests for the helpers. The full HTTP roundtrip is
// blocked behind the AI runtime (real provider call); we exercise
// the parts that don't need a live LLM.

func TestCleanGeneratedChapter_StripsPreamble(t *testing.T) {
	in := "Sure! Here's the chapter on closures:\n\n# Closures\n\nA closure is..."
	got := cleanGeneratedChapter(in, "Closures")
	if !strings.HasPrefix(got, "# Closures") {
		t.Errorf("expected output to start with # heading, got %q", got[:40])
	}
	if strings.Contains(got, "Sure!") {
		t.Errorf("preamble not stripped: %q", got[:40])
	}
}

func TestCleanGeneratedChapter_StripsCodeFence(t *testing.T) {
	in := "```markdown\n# Chapter Title\nContent here.\n```"
	got := cleanGeneratedChapter(in, "Chapter Title")
	if strings.Contains(got, "```") {
		t.Errorf("code fence not stripped: %q", got)
	}
	if !strings.HasPrefix(got, "# Chapter Title") {
		t.Errorf("heading dropped: %q", got)
	}
}

func TestCleanGeneratedChapter_PrependsHeadingWhenMissing(t *testing.T) {
	in := "A closure is a function that captures its surrounding scope.\n\nExample: ..."
	got := cleanGeneratedChapter(in, "Closures")
	if !strings.HasPrefix(got, "# Closures\n\n") {
		t.Errorf("expected prepended heading, got %q", got[:40])
	}
}

// When the model returns the SAME topic with cosmetic drift (smart
// punctuation, casing), keep the model's polished form.
func TestCleanGeneratedChapter_AcceptsCosmeticTitleDrift(t *testing.T) {
	requested := "lexical scoping and closures"
	cases := []string{
		"# Lexical Scoping and Closures\n\nA closure is...",
		"# Lexical Scoping & Closures\n\nA closure is...",
		"# Lexical scoping, and closures\n\nA closure is...",
	}
	for _, in := range cases {
		got := cleanGeneratedChapter(in, requested)
		// Must keep the model's heading, not rewrite to the user's
		// lowercase form.
		firstLine := strings.SplitN(got, "\n", 2)[0]
		if firstLine == "# "+requested {
			t.Errorf("cosmetic drift was rewritten to requested form, lost polish: %q from %q", firstLine, in)
		}
	}
}

// When the model wrote a different chapter entirely (real failure
// mode), replace the heading with the requested title so the saved
// file matches the wikilink the user clicked.
func TestCleanGeneratedChapter_RewritesWrongTopic(t *testing.T) {
	requested := "Closures"
	in := "# Higher-order functions\n\nA higher-order function takes another function..."
	got := cleanGeneratedChapter(in, requested)
	if !strings.HasPrefix(got, "# Closures\n") {
		t.Errorf("expected heading rewritten to # Closures, got %q", got[:40])
	}
	// Body content should survive intact (just the heading line changed).
	if !strings.Contains(got, "A higher-order function takes another function") {
		t.Errorf("body content lost: %q", got)
	}
}

func TestTopicallyEqual(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"Closures", "closures", true},                    // case
		{"A and B", "A & B", true},                        // & vs and
		{"Foo: Bar", "Foo Bar", true},                     // colon vs none
		{"  spaces  collapse  ", "spaces collapse", true}, // whitespace
		{"Closures", "Higher-order functions", false},     // genuinely different
		{"", "", true},
		{"x", "", false},
	}
	for _, c := range cases {
		if got := topicallyEqual(c.a, c.b); got != c.want {
			t.Errorf("topicallyEqual(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestCleanGeneratedChapter_EmptyInput(t *testing.T) {
	if got := cleanGeneratedChapter("", "Chapter"); got != "" {
		t.Errorf("empty in → empty out; got %q", got)
	}
	if got := cleanGeneratedChapter("   \n  ", "Chapter"); got != "" {
		t.Errorf("whitespace-only → empty; got %q", got)
	}
}

func TestDeriveChapterPath_NestedUnderParentName(t *testing.T) {
	// Parent at Research/Go.md → child at Research/Go/<slug>.md
	got := deriveChapterPath("Research/Go.md", "Lexical scoping and closures")
	want := "Research/Go/Lexical-scoping-and-closures.md"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDeriveChapterPath_NoParentDefaultsToChaptersDir(t *testing.T) {
	got := deriveChapterPath("", "Foundations")
	if got != "Chapters/Foundations.md" {
		t.Errorf("got %q, want Chapters/Foundations.md", got)
	}
}

func TestSlugifyChapter_StripsBadCharsAndHyphenates(t *testing.T) {
	cases := map[string]string{
		"Foo Bar":                "Foo-Bar",
		"OO/RPC and HTTP/2":      "OORPC-and-HTTP2",
		"  spaces   collapse  ":  "spaces-collapse",
		`a:b*c?"d<e>f|g\h`:       "abcdefgh",
		"":                       "",
	}
	for in, want := range cases {
		if got := slugifyChapter(in); got != want {
			t.Errorf("slugifyChapter(%q) = %q, want %q", in, got, want)
		}
	}
}
