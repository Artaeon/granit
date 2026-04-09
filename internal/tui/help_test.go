package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHelpOverlay_Toggle(t *testing.T) {
	h := NewHelpOverlay()
	if h.IsActive() {
		t.Error("new overlay should be inactive")
	}

	h.Toggle()
	if !h.IsActive() {
		t.Error("should be active after toggle")
	}

	h.Toggle()
	if h.IsActive() {
		t.Error("should be inactive after second toggle")
	}
}

func TestHelpOverlay_ToggleResetsState(t *testing.T) {
	h := NewHelpOverlay()
	h.scroll = 10
	h.searching = true
	h.query = "test"

	h.Toggle()

	if h.scroll != 0 {
		t.Error("scroll should reset on toggle")
	}
	if h.searching {
		t.Error("searching should reset on toggle")
	}
	if h.query != "" {
		t.Error("query should reset on toggle")
	}
}

func TestHelpOverlay_BuildLines(t *testing.T) {
	h := NewHelpOverlay()
	h.SetSize(80, 40)

	lines := h.buildLines()
	if len(lines) == 0 {
		t.Fatal("expected help content lines")
	}
}

func TestHelpOverlay_ScrollBounds(t *testing.T) {
	h := NewHelpOverlay()
	h.SetSize(80, 20)
	h.Toggle()

	// Scroll up from 0 should stay at 0
	h, _ = h.Update(tea.KeyMsg{Type: tea.KeyUp})
	if h.scroll != 0 {
		t.Errorf("scroll should not go below 0, got %d", h.scroll)
	}

	// Scroll down
	for i := 0; i < 100; i++ {
		h, _ = h.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	// Should be capped at max
	lines := h.buildLines()
	maxScroll := len(lines) - h.visibleHeight()
	if maxScroll < 0 {
		maxScroll = 0
	}
	if h.scroll > maxScroll {
		t.Errorf("scroll should be capped at %d, got %d", maxScroll, h.scroll)
	}
}

func TestHelpOverlay_EscCloses(t *testing.T) {
	h := NewHelpOverlay()
	h.Toggle()

	h, _ = h.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if h.IsActive() {
		t.Error("Esc should close help overlay")
	}
}

func TestHelpOverlay_VisibleHeight(t *testing.T) {
	h := NewHelpOverlay()
	h.SetSize(80, 40)
	vh := h.visibleHeight()
	if vh <= 0 {
		t.Errorf("visible height should be positive, got %d", vh)
	}
}
