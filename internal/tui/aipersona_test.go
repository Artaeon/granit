package tui

import (
	"strings"
	"testing"
)

func TestDeepCovenIntro_FormatsRoleIntoSentence(t *testing.T) {
	got := DeepCovenIntro("end-of-day coach")
	want := "You are DEEPCOVEN, a direct and honest end-of-day coach."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDeepCovenIntro_DefaultsOnEmptyRole(t *testing.T) {
	got := DeepCovenIntro("")
	if !strings.Contains(got, "personal assistant") {
		t.Errorf("empty role should fall back to 'personal assistant', got %q", got)
	}
}

func TestDeepCovenSystem_JoinsWithBlankLine(t *testing.T) {
	got := DeepCovenSystem("habit coach", "Analyze the data.")
	if !strings.HasPrefix(got, "You are DEEPCOVEN, a direct and honest habit coach.") {
		t.Errorf("intro missing or malformed: %q", got)
	}
	if !strings.Contains(got, "\n\nAnalyze the data.") {
		t.Errorf("task should be separated by a blank line: %q", got)
	}
}

func TestDeepCovenSystem_OmitsBlankWhenTaskEmpty(t *testing.T) {
	got := DeepCovenSystem("project advisor", "")
	if strings.HasSuffix(got, "\n\n") {
		t.Errorf("empty task should not leave a trailing blank line: %q", got)
	}
}

func TestDeepCovenLongPreamble_MentionsCorePrinciples(t *testing.T) {
	for _, must := range []string{"DEEPCOVEN", "100% honesty", "100% service"} {
		if !strings.Contains(DeepCovenLongPreamble, must) {
			t.Errorf("long preamble missing %q", must)
		}
	}
}
