package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ── AI Summary Phase ───────────────────────────────────────────

func TestDailyReview_AIPhaseSkippedWhenLocal(t *testing.T) {
	dr := DailyReview{}
	dr.active = true
	dr.phase = reviewReflect
	dr.ai = AIConfig{Provider: "local"}

	// Simulate Enter in reflect phase — should skip AI and go to saved
	dr, _ = dr.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if dr.phase != reviewSaved {
		t.Errorf("expected reviewSaved, got phase %d", dr.phase)
	}
}

func TestDailyReview_AIPhaseSkippedWhenEmpty(t *testing.T) {
	dr := DailyReview{}
	dr.active = true
	dr.phase = reviewReflect
	dr.ai = AIConfig{Provider: ""}

	dr, _ = dr.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if dr.phase != reviewSaved {
		t.Errorf("expected reviewSaved, got phase %d", dr.phase)
	}
}

func TestDailyReview_AIPhaseEnteredWhenOllama(t *testing.T) {
	dr := DailyReview{}
	dr.active = true
	dr.phase = reviewReflect
	dr.ai = AIConfig{Provider: "ollama"}
	dr.rescheduleMap = make(map[int]string)

	dr, cmd := dr.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if dr.phase != reviewAISummary {
		t.Errorf("expected reviewAISummary, got phase %d", dr.phase)
	}
	if cmd == nil {
		t.Error("should return a cmd to start AI call")
	}
}

func TestDailyReview_AIResultMsg(t *testing.T) {
	dr := DailyReview{}
	dr.active = true
	dr.phase = reviewAISummary
	dr.scroll = 5

	dr, _ = dr.Update(dailyReviewAIMsg{summary: "WIN: Shipped parser"})

	if dr.aiSummary != "WIN: Shipped parser" {
		t.Errorf("unexpected aiSummary: %q", dr.aiSummary)
	}
	if dr.scroll != 0 {
		t.Error("scroll should reset on new result")
	}
}

func TestDailyReview_AIResultMsg_Error(t *testing.T) {
	dr := DailyReview{}
	dr.active = true
	dr.phase = reviewAISummary

	dr, _ = dr.Update(dailyReviewAIMsg{err: errDRTest})

	if !strings.Contains(dr.aiSummary, "AI unavailable") {
		t.Errorf("should show error: %q", dr.aiSummary)
	}
}

func TestDailyReview_AISummaryEscAdvances(t *testing.T) {
	dr := DailyReview{}
	dr.active = true
	dr.phase = reviewAISummary

	dr, _ = dr.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if dr.phase != reviewSaved {
		t.Errorf("Esc in AI summary should go to saved, got phase %d", dr.phase)
	}
}

func TestDailyReview_AISummaryScrollBounds(t *testing.T) {
	dr := DailyReview{}
	dr.active = true
	dr.phase = reviewAISummary
	dr.aiSummary = "A\nB"

	// Scroll down
	dr, _ = dr.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if dr.scroll != 1 {
		t.Errorf("scroll should be 1, got %d", dr.scroll)
	}

	// Should not go past last line
	dr, _ = dr.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if dr.scroll != 1 {
		t.Errorf("scroll should stay at 1, got %d", dr.scroll)
	}

	// Scroll up
	dr, _ = dr.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if dr.scroll != 0 {
		t.Errorf("scroll should be 0, got %d", dr.scroll)
	}
}

func TestDailyReview_PhaseNames(t *testing.T) {
	// Verify phase names array matches iota constants
	if reviewCompleted != 0 {
		t.Error("reviewCompleted should be 0")
	}
	if reviewAISummary != 4 {
		t.Error("reviewAISummary should be 4")
	}
	if reviewSaved != 5 {
		t.Error("reviewSaved should be 5")
	}
}

var errDRTest = &drTestError{}

type drTestError struct{}

func (e *drTestError) Error() string { return "test error" }

// ---------------------------------------------------------------------------
// summarizeDayTime — planned vs actual aggregation for Time Audit section
// ---------------------------------------------------------------------------

func TestSummarizeDayTime_AggregatesPlannedAndActual(t *testing.T) {
	blocks := []PlannerBlock{
		{StartTime: "09:00", EndTime: "10:00", BlockType: "task"},
		{StartTime: "10:30", EndTime: "11:15", BlockType: "focus"},
		{StartTime: "11:00", EndTime: "11:25", BlockType: "pomodoro"},
		{StartTime: "11:30", EndTime: "11:55", BlockType: "pomodoro"},
		{StartTime: "12:00", EndTime: "13:00", BlockType: "break"},        // ignored
		{StartTime: "14:00", EndTime: "14:30", BlockType: "admin"},
	}
	planned, actual, pomodoros := summarizeDayTime(blocks)
	if planned != 60+45+30 { // task + focus + admin
		t.Errorf("planned = %d, want %d", planned, 60+45+30)
	}
	if actual != 25+25 {
		t.Errorf("actual = %d, want %d", actual, 25+25)
	}
	if pomodoros != 2 {
		t.Errorf("pomodoros = %d, want 2", pomodoros)
	}
}

func TestSummarizeDayTime_IgnoresInvertedOrZeroRanges(t *testing.T) {
	blocks := []PlannerBlock{
		{StartTime: "10:00", EndTime: "10:00", BlockType: "task"}, // zero
		{StartTime: "11:00", EndTime: "10:00", BlockType: "task"}, // inverted
		{StartTime: "09:00", EndTime: "10:00", BlockType: "task"}, // valid
	}
	planned, _, _ := summarizeDayTime(blocks)
	if planned != 60 {
		t.Errorf("planned = %d, want 60 (invalid ranges should be skipped)", planned)
	}
}
