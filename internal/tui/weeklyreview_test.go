package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ── Step Constants ─────────────────────────────────────────────

func TestWeeklyReview_StepOrder(t *testing.T) {
	if wrStepTasks != 0 {
		t.Error("wrStepTasks should be 0")
	}
	if wrStepAI != 4 {
		t.Error("wrStepAI should be 4")
	}
	if wrStepSummary != 5 {
		t.Error("wrStepSummary should be 5")
	}
}

// ── AI Step Skipping ───────────────────────────────────────────

func TestWeeklyReview_AIStepSkippedWhenLocal(t *testing.T) {
	wr := NewWeeklyReview()
	wr.active = true
	wr.step = wrStepTasks
	wr.ai = AIConfig{Provider: "local"}

	// Advance through all steps to wrStepNext
	wr.step = wrStepNext
	wr.inputBuf = "priorities"

	// Tab from text input should skip AI step
	wr, _ = wr.updateTextInput(tea.KeyMsg{Type: tea.KeyTab})

	if wr.step != wrStepSummary {
		t.Errorf("expected wrStepSummary (5), got step %d", wr.step)
	}
}

func TestWeeklyReview_AIStepSkippedWhenEmpty(t *testing.T) {
	wr := NewWeeklyReview()
	wr.active = true
	wr.step = wrStepNext
	wr.ai = AIConfig{Provider: ""}

	wr, _ = wr.updateTextInput(tea.KeyMsg{Type: tea.KeyTab})

	if wr.step != wrStepSummary {
		t.Errorf("expected wrStepSummary, got step %d", wr.step)
	}
}

func TestWeeklyReview_AIStepEnteredWhenOllama(t *testing.T) {
	wr := NewWeeklyReview()
	wr.active = true
	wr.step = wrStepNext
	wr.ai = AIConfig{Provider: "ollama"}

	wr, cmd := wr.updateTextInput(tea.KeyMsg{Type: tea.KeyTab})

	if wr.step != wrStepAI {
		t.Errorf("expected wrStepAI (4), got step %d", wr.step)
	}
	if cmd == nil {
		t.Error("should return a cmd to start AI call")
	}
}

// ── AI Step Navigation ─────────────────────────────────────────

func TestWeeklyReview_AIStepEscAdvances(t *testing.T) {
	wr := NewWeeklyReview()
	wr.active = true
	wr.step = wrStepAI

	wr, _ = wr.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if wr.step != wrStepSummary {
		t.Errorf("Esc in AI step should go to summary, got step %d", wr.step)
	}
}

func TestWeeklyReview_AIStepTabAdvances(t *testing.T) {
	wr := NewWeeklyReview()
	wr.active = true
	wr.step = wrStepAI

	wr, _ = wr.Update(tea.KeyMsg{Type: tea.KeyTab})

	if wr.step != wrStepSummary {
		t.Errorf("Tab in AI step should go to summary, got step %d", wr.step)
	}
}

func TestWeeklyReview_ShiftTabSkipsAIWhenLocal(t *testing.T) {
	wr := NewWeeklyReview()
	wr.active = true
	wr.step = wrStepSummary
	wr.ai = AIConfig{Provider: "local"}

	// Shift+tab from Summary should skip AI step
	wr, _ = wr.Update(tea.KeyMsg{Type: tea.KeyShiftTab})

	if wr.step != wrStepNext {
		t.Errorf("shift+tab from summary should skip AI and land on wrStepNext (3), got %d", wr.step)
	}
}

func TestWeeklyReview_ShiftTabEntersAIWhenConfigured(t *testing.T) {
	wr := NewWeeklyReview()
	wr.active = true
	wr.step = wrStepSummary
	wr.ai = AIConfig{Provider: "ollama"}
	wr.aiSynthesis = "previous result"

	wr, _ = wr.Update(tea.KeyMsg{Type: tea.KeyShiftTab})

	if wr.step != wrStepAI {
		t.Errorf("shift+tab from summary should land on wrStepAI (4), got %d", wr.step)
	}
}

// ── AI Result Message ──────────────────────────────────────────

func TestWeeklyReview_AIResultMsg(t *testing.T) {
	wr := NewWeeklyReview()
	wr.active = true
	wr.step = wrStepAI

	wr, _ = wr.Update(weeklyReviewAIMsg{synthesis: "SCORE: 8/10"})

	if wr.aiSynthesis != "SCORE: 8/10" {
		t.Errorf("unexpected aiSynthesis: %q", wr.aiSynthesis)
	}
}

func TestWeeklyReview_AIResultMsg_Error(t *testing.T) {
	wr := NewWeeklyReview()
	wr.active = true
	wr.step = wrStepAI

	wr, _ = wr.Update(weeklyReviewAIMsg{err: errWRTest})

	if !strings.Contains(wr.aiSynthesis, "AI unavailable") {
		t.Errorf("should show error: %q", wr.aiSynthesis)
	}
}

// ── AI Scroll ──────────────────────────────────────────────────

func TestWeeklyReview_AIScrollBounds(t *testing.T) {
	wr := NewWeeklyReview()
	wr.active = true
	wr.step = wrStepAI
	wr.aiSynthesis = "Line 1\nLine 2\nLine 3"

	// Scroll down
	wr, _ = wr.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if wr.aiScroll != 1 {
		t.Errorf("aiScroll should be 1, got %d", wr.aiScroll)
	}

	wr, _ = wr.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if wr.aiScroll != 2 {
		t.Errorf("aiScroll should be 2, got %d", wr.aiScroll)
	}

	// Should not exceed last line
	wr, _ = wr.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if wr.aiScroll != 2 {
		t.Errorf("aiScroll should stay at 2, got %d", wr.aiScroll)
	}

	// Scroll up
	wr, _ = wr.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if wr.aiScroll != 1 {
		t.Errorf("aiScroll should be 1, got %d", wr.aiScroll)
	}
}

func TestWeeklyReview_AIScrollEmptySynthesis(t *testing.T) {
	wr := NewWeeklyReview()
	wr.active = true
	wr.step = wrStepAI
	wr.aiSynthesis = ""

	// Should not panic or increment
	wr, _ = wr.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if wr.aiScroll != 0 {
		t.Error("aiScroll should stay 0 with empty synthesis")
	}
}

var errWRTest = &wrTestError{}

type wrTestError struct{}

func (e *wrTestError) Error() string { return "test error" }
