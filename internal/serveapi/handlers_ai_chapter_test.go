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

// Long-title slug must be capped at 100 chars to stay
// filesystem-safe (eCryptfs / FAT32 / some ZFS encrypted volumes
// have segment-length limits around 143 bytes). Prefer a word-
// boundary cut where possible.
func TestSlugifyChapter_CapsLongTitleAtWordBoundary(t *testing.T) {
	long := strings.Repeat("Foundations of asynchronous IO and event loops ", 5)
	got := slugifyChapter(long)
	if len(got) > 100 {
		t.Errorf("slug exceeds 100 chars: %d (%q)", len(got), got)
	}
	// Word-boundary cut means the last char shouldn't be a partial word.
	if strings.HasSuffix(got, "-") {
		t.Errorf("slug ends in dangling hyphen, expected clean word-boundary cut: %q", got)
	}
}

// A single-word title longer than 100 chars (no hyphens to cut at)
// should still be capped — accepting a hard mid-word cut as the
// fallback when no word boundary exists in the last 30 chars.
func TestSlugifyChapter_CapsLongUnbrokenWord(t *testing.T) {
	long := strings.Repeat("A", 200)
	got := slugifyChapter(long)
	if len(got) > 100 {
		t.Errorf("slug exceeds 100 chars: %d", len(got))
	}
}

func TestNormaliseChapterTargetPath_HappyPath(t *testing.T) {
	got, err := normaliseChapterTargetPath("Research/Foo.md", "", "Foo")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Research/Foo.md" {
		t.Errorf("got %q", got)
	}
}

func TestNormaliseChapterTargetPath_AppendsMdExtension(t *testing.T) {
	got, err := normaliseChapterTargetPath("Research/Foo", "", "Foo")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Research/Foo.md" {
		t.Errorf("expected .md appended, got %q", got)
	}
}

func TestNormaliseChapterTargetPath_BackslashNormalised(t *testing.T) {
	got, err := normaliseChapterTargetPath("Research\\Foo.md", "", "Foo")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Research/Foo.md" {
		t.Errorf("backslash not normalised: got %q", got)
	}
}

func TestNormaliseChapterTargetPath_RejectsAbsolute(t *testing.T) {
	for _, in := range []string{
		"/etc/passwd",
		"/foo.md",
		// On Windows filepath.IsAbs catches C:\... — the test
		// passes the forward-slashed form; the function first
		// normalises backslashes to forward slashes BEFORE
		// IsAbs, so a Windows-style "C:/foo" would still be
		// rejected because filepath.IsAbs("C:/foo")=true on
		// Windows. On Linux we accept "C:/foo" as a relative
		// path which is correct for non-Windows clients.
	} {
		_, err := normaliseChapterTargetPath(in, "", "x")
		if err == nil {
			t.Errorf("expected rejection for %q", in)
		}
	}
}

func TestNormaliseChapterTargetPath_RejectsParentSegment(t *testing.T) {
	for _, in := range []string{
		"../etc/passwd",
		"Research/../../etc",
		"..",
		"foo/../bar/..",
	} {
		_, err := normaliseChapterTargetPath(in, "", "x")
		if err == nil {
			t.Errorf("expected rejection for %q", in)
		}
	}
}

// Chapter titles containing ".." in prose are NOT rejected — the
// segment-level check distinguishes a real path-traversal attempt
// from "Patterns .. Anti-patterns" or "Foo..Bar".
func TestNormaliseChapterTargetPath_AcceptsTitlesWithDotDotInProse(t *testing.T) {
	got, err := normaliseChapterTargetPath("Research/Patterns..Anti-patterns.md", "", "x")
	if err != nil {
		t.Fatalf("unexpected rejection: %v", err)
	}
	if got != "Research/Patterns..Anti-patterns.md" {
		t.Errorf("got %q", got)
	}
}

func TestNormaliseChapterTargetPath_EmptyDerivesFromParent(t *testing.T) {
	got, err := normaliseChapterTargetPath("", "Research/Outline.md", "First chapter")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Research/Outline/First-chapter.md" {
		t.Errorf("expected derived nested path, got %q", got)
	}
}
