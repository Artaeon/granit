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
