package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// Initialization
// ---------------------------------------------------------------------------

func TestDevotional_NewDefaults(t *testing.T) {
	d := NewDevotional()
	if d.IsActive() {
		t.Error("new devotional should be inactive")
	}
	if d.loading {
		t.Error("new devotional should not be loading")
	}
	if d.scroll != 0 {
		t.Error("new devotional scroll should be 0")
	}
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

func TestDevotional_OpenActivates(t *testing.T) {
	d := NewDevotional()
	dir := t.TempDir()
	ai := AIConfig{Provider: "local"}
	goals := []Goal{{Title: "Test Goal", Status: GoalStatusActive}}

	_ = d.Open(dir, ai, goals)

	if !d.IsActive() {
		t.Error("devotional should be active after Open")
	}
	if !d.loading {
		t.Error("devotional should be loading after Open")
	}
	if d.scripture.Text == "" {
		t.Error("devotional should have loaded a scripture")
	}
	if len(d.goals) != 1 {
		t.Errorf("expected 1 goal, got %d", len(d.goals))
	}
}

func TestDevotional_OpenResetsState(t *testing.T) {
	d := NewDevotional()
	d.reflection = "old data"
	d.scroll = 5
	d.lines = []string{"stale"}
	d.loadingTick = 99

	dir := t.TempDir()
	_ = d.Open(dir, AIConfig{Provider: "local"}, nil)

	if d.reflection != "" {
		t.Error("Open should reset reflection")
	}
	if d.scroll != 0 {
		t.Error("Open should reset scroll")
	}
	if d.lines != nil {
		t.Error("Open should reset lines")
	}
	if d.loadingTick != 0 {
		t.Error("Open should reset loadingTick")
	}
}

// ---------------------------------------------------------------------------
// Message handling
// ---------------------------------------------------------------------------

func TestDevotional_ResultMsg_Success(t *testing.T) {
	d := NewDevotional()
	dir := t.TempDir()
	_ = d.Open(dir, AIConfig{Provider: "local"}, nil)

	d, _ = d.Update(devotionalResultMsg{reflection: "Line 1\nLine 2\nLine 3"})

	if d.loading {
		t.Error("should not be loading after result")
	}
	if d.reflection != "Line 1\nLine 2\nLine 3" {
		t.Errorf("unexpected reflection: %q", d.reflection)
	}
	if len(d.lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(d.lines))
	}
	if d.scroll != 0 {
		t.Error("scroll should reset to 0 on new result")
	}
}

func TestDevotional_ResultMsg_Error(t *testing.T) {
	d := NewDevotional()
	dir := t.TempDir()
	_ = d.Open(dir, AIConfig{Provider: "local"}, nil)

	d, _ = d.Update(devotionalResultMsg{err: errDevTest})

	if d.loading {
		t.Error("should not be loading after error")
	}
	if !strings.Contains(d.reflection, "AI unavailable") {
		t.Errorf("error should be shown in reflection: %q", d.reflection)
	}
}

func TestDevotional_TickMsg_WhileLoading(t *testing.T) {
	d := NewDevotional()
	dir := t.TempDir()
	_ = d.Open(dir, AIConfig{Provider: "local"}, nil)

	before := d.loadingTick
	d, cmd := d.Update(devotionalTickMsg{})

	if d.loadingTick != before+1 {
		t.Error("tick should increment loadingTick while loading")
	}
	if cmd == nil {
		t.Error("tick should return another tick cmd while loading")
	}
}

func TestDevotional_TickMsg_AfterLoaded(t *testing.T) {
	d := NewDevotional()
	dir := t.TempDir()
	_ = d.Open(dir, AIConfig{Provider: "local"}, nil)
	d.loading = false

	d, cmd := d.Update(devotionalTickMsg{})

	if cmd != nil {
		t.Error("tick should return nil cmd when not loading")
	}
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------

func TestDevotional_EscCloses(t *testing.T) {
	d := NewDevotional()
	dir := t.TempDir()
	_ = d.Open(dir, AIConfig{Provider: "local"}, nil)

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if d.IsActive() {
		t.Error("Esc should close devotional")
	}
}

func TestDevotional_ScrollBounds(t *testing.T) {
	d := NewDevotional()
	dir := t.TempDir()
	_ = d.Open(dir, AIConfig{Provider: "local"}, nil)
	d, _ = d.Update(devotionalResultMsg{reflection: "A\nB\nC"})

	// Scroll down
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if d.scroll != 1 {
		t.Errorf("scroll should be 1, got %d", d.scroll)
	}

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if d.scroll != 2 {
		t.Errorf("scroll should be 2, got %d", d.scroll)
	}

	// Should not go past last line
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if d.scroll != 2 {
		t.Errorf("scroll should stay at 2, got %d", d.scroll)
	}

	// Scroll up
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if d.scroll != 1 {
		t.Errorf("scroll should be 1, got %d", d.scroll)
	}
}

func TestDevotional_ScrollEmptyLines(t *testing.T) {
	d := NewDevotional()
	dir := t.TempDir()
	_ = d.Open(dir, AIConfig{Provider: "local"}, nil)
	// Result not yet received, lines is nil
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if d.scroll != 0 {
		t.Error("scroll should stay 0 with nil lines")
	}
}

// ---------------------------------------------------------------------------
// View rendering
// ---------------------------------------------------------------------------

func TestDevotional_ViewInactive(t *testing.T) {
	d := NewDevotional()
	if d.View() != "" {
		t.Error("inactive devotional should render empty string")
	}
}

func TestDevotional_ViewLoading(t *testing.T) {
	d := NewDevotional()
	d.SetSize(100, 40)
	dir := t.TempDir()
	_ = d.Open(dir, AIConfig{Provider: "local"}, nil)

	view := d.View()
	if !strings.Contains(view, "Reflecting on this verse") {
		t.Error("loading view should show reflecting message")
	}
	if !strings.Contains(view, "DEEPCOVEN") {
		t.Error("view should show DEEPCOVEN header")
	}
}

func TestDevotional_ViewWithResult(t *testing.T) {
	d := NewDevotional()
	d.SetSize(100, 40)
	dir := t.TempDir()
	_ = d.Open(dir, AIConfig{Provider: "local"}, nil)
	d, _ = d.Update(devotionalResultMsg{reflection: "## Verse Insight\nThis verse teaches us."})

	view := d.View()
	if !strings.Contains(view, "Verse Insight") {
		t.Error("view should show heading text")
	}
	if !strings.Contains(view, "This verse teaches us") {
		t.Error("view should show body text")
	}
}

// test error sentinel
var errDevTest = &devTestError{}

type devTestError struct{}

func (e *devTestError) Error() string { return "test error" }
