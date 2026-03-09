package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// Goal Parsing
// ---------------------------------------------------------------------------

func TestFocusMode_GoalParsing(t *testing.T) {
	fm := NewFocusMode()
	fm.Open(0)
	fm.OpenGoalPrompt()

	// Type "500" then press Enter
	for _, r := range "500" {
		fm, _ = fm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	fm, _ = fm.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if fm.targetWords != 500 {
		t.Errorf("expected targetWords=500, got %d", fm.targetWords)
	}
	if fm.IsSettingGoal() {
		t.Error("goal prompt should be closed after Enter")
	}
}

func TestFocusMode_GoalInvalid(t *testing.T) {
	fm := NewFocusMode()
	fm.Open(0)

	fm.OpenGoalPrompt()
	// Type "abc" (non-digit characters should be rejected by the input filter)
	for _, r := range "abc" {
		fm, _ = fm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	fm, _ = fm.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Only digits are accepted, so goalInput stays empty → Atoi fails → target cleared to 0
	if fm.targetWords != 0 {
		t.Errorf("expected targetWords=0 for non-digit input, got %d", fm.targetWords)
	}
}

func TestFocusMode_GoalNegative(t *testing.T) {
	fm := NewFocusMode()
	fm.Open(0)

	fm.OpenGoalPrompt()
	// The input only accepts digits (0-9), so typing "-100" will only
	// register "100" — the minus sign is silently ignored.
	for _, r := range "-100" {
		fm, _ = fm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	fm, _ = fm.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Since the minus is filtered out, "100" is parsed as a positive int.
	if fm.targetWords != 100 {
		t.Errorf("expected targetWords=100 (minus filtered), got %d", fm.targetWords)
	}
}

func TestFocusMode_GoalZero(t *testing.T) {
	fm := NewFocusMode()
	fm.Open(0)

	fm.OpenGoalPrompt()
	fm, _ = fm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	fm, _ = fm.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// n <= 0 clears the target
	if fm.targetWords != 0 {
		t.Errorf("expected targetWords=0 for zero input, got %d", fm.targetWords)
	}
}

func TestFocusMode_ProgressCalculation(t *testing.T) {
	fm := NewFocusMode()
	fm.Open(50) // start with 50 words already written
	fm.SetTargetWords(100)
	fm.SetSize(120, 40)

	// Simulate 100 current words → progress = 100-50 = 50 out of 100 = 50%
	fm.checkGoalReached(100)

	if fm.goalReached {
		t.Error("goal should not be reached at 50% progress")
	}

	// Verify the buildStatus output contains progress info
	status := fm.buildStatus(100, 78)
	if status == "" {
		t.Error("expected non-empty status line")
	}
	// The status should contain "50/100" progress
	if !containsSubstring(status, "50/100") {
		t.Errorf("expected status to contain progress '50/100', got: %s", status)
	}
}

func TestFocusMode_GoalReached(t *testing.T) {
	fm := NewFocusMode()
	fm.Open(10)           // started at 10 words
	fm.SetTargetWords(50) // target is 50 new words
	fm.SetSize(120, 40)

	// Simulate reaching the goal: 10 start + 50 target = 60 words needed
	fm.checkGoalReached(59)
	if fm.goalReached {
		t.Error("goal should not be reached at 49/50 progress")
	}

	fm.checkGoalReached(60)
	if !fm.goalReached {
		t.Error("goal should be reached at 50/50 progress")
	}

	// Verify the congratulations banner appears in RenderEditor
	rendered := fm.RenderEditor("some text", 60)
	if !containsSubstring(rendered, "Goal reached") {
		t.Error("expected congratulations message in rendered output")
	}
}

// containsSubstring checks if s contains sub, stripping ANSI escape codes.
func containsSubstring(s, sub string) bool {
	// Simple check: lipgloss output contains ANSI codes, but the raw text
	// should still be present somewhere in the output.
	return len(s) > 0 && len(sub) > 0 && findInANSI(s, sub)
}

// findInANSI strips ANSI escape sequences and searches for sub.
func findInANSI(s, sub string) bool {
	// Strip ANSI escape codes: \x1b[ ... m
	clean := make([]byte, 0, len(s))
	inEsc := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if s[i] == 'm' {
				inEsc = false
			}
			continue
		}
		clean = append(clean, s[i])
	}
	return containsBytes(clean, []byte(sub))
}

func containsBytes(haystack, needle []byte) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
