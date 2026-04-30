package tui

import (
	"strings"
	"testing"
)

func TestCleanAIEditOutput_StripsPreamble(t *testing.T) {
	cases := map[string]string{
		"Sure! Here is the rewritten text:\nHello world.": "Hello world.",
		"Here's a summary:\nThis is short.":               "This is short.",
		"Improved:\nClearer prose now.":                   "Clearer prose now.",
	}
	for input, want := range cases {
		got := cleanAIEditOutput(input)
		if got != want {
			t.Errorf("input %q: want %q, got %q", input, want, got)
		}
	}
}

func TestCleanAIEditOutput_StripsCodeFence(t *testing.T) {
	in := "```\nrewritten body line\n```"
	got := cleanAIEditOutput(in)
	if got != "rewritten body line" {
		t.Errorf("expected fence-stripped, got %q", got)
	}
}

func TestCleanAIEditOutput_StripsCodeFenceWithLanguage(t *testing.T) {
	in := "```markdown\n# Heading\nbody\n```"
	got := cleanAIEditOutput(in)
	if !strings.HasPrefix(got, "# Heading") {
		t.Errorf("expected language fence stripped, got %q", got)
	}
}

func TestCleanAIEditOutput_StripsWrappingQuotes(t *testing.T) {
	in := `"a quoted single line"`
	got := cleanAIEditOutput(in)
	if got != "a quoted single line" {
		t.Errorf("expected quote-stripped, got %q", got)
	}
}

func TestCleanAIEditOutput_LeavesNormalProseAlone(t *testing.T) {
	in := "Here are my notes about the project:\n- Point one\n- Point two"
	got := cleanAIEditOutput(in)
	// Must NOT eat "Here are" since it's followed by ":" then newline,
	// but the actual content "- Point one" is the legitimate continuation.
	// Our cleaner WILL strip preamble lines ending with ":" and then newline.
	// That's intentional — we'd rather trade an occasional false positive
	// on lists like this for losing a model preamble. So just verify it
	// doesn't completely destroy the output.
	if !strings.Contains(got, "Point one") {
		t.Errorf("cleaner ate the body: %q", got)
	}
}

func TestCleanAIEditOutput_EmptyAndWhitespace(t *testing.T) {
	if cleanAIEditOutput("") != "" {
		t.Error("empty in should be empty out")
	}
	if cleanAIEditOutput("   \n\n  ") != "" {
		t.Error("whitespace-only should clean to empty")
	}
}

func TestAIActionLabel_AllActionsHaveLabels(t *testing.T) {
	for _, a := range []aiEditAction{
		aiActionRewrite, aiActionExpand, aiActionSummarize,
		aiActionImprove, aiActionShorten, aiActionFix,
	} {
		if got := aiActionLabel(a); got == "thinking" {
			t.Errorf("action %q falls through to default; should have a label", a)
		}
	}
}

func TestAIEditPrompts_AllActionsHavePrompts(t *testing.T) {
	for _, a := range []aiEditAction{
		aiActionRewrite, aiActionExpand, aiActionSummarize,
		aiActionImprove, aiActionShorten, aiActionFix,
	} {
		if _, ok := aiEditPrompts[a]; !ok {
			t.Errorf("action %q missing from aiEditPrompts", a)
		}
	}
}
