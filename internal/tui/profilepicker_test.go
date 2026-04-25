package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/profiles"
)

func mkPickerProfiles() []*profiles.Profile {
	return []*profiles.Profile{
		{ID: "classic", Name: "Classic", Description: "Everything on"},
		{ID: "daily_operator", Name: "Daily Operator", Description: "Planning loop"},
		{ID: "researcher", Name: "Researcher", Description: "Knowledge work"},
		{ID: "builder", Name: "Builder", Description: "Shipping"},
	}
}

func TestProfilePicker_OpenFocusesCurrent(t *testing.T) {
	p := NewProfilePicker()
	p.Open(mkPickerProfiles(), "researcher")
	if p.focused != 2 {
		t.Errorf("focused should land on researcher (index 2), got %d", p.focused)
	}
}

func TestProfilePicker_ArrowKeysCycle(t *testing.T) {
	p := NewProfilePicker()
	p.Open(mkPickerProfiles(), "classic")
	if p.focused != 0 {
		t.Fatalf("expected focused=0, got %d", p.focused)
	}
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRight})
	if p.focused != 1 {
		t.Errorf("right arrow should advance to 1, got %d", p.focused)
	}
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRight})
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRight})
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRight}) // already at last
	if p.focused != 3 {
		t.Errorf("right at end should clamp to 3, got %d", p.focused)
	}
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if p.focused != 2 {
		t.Errorf("left should go back to 2, got %d", p.focused)
	}
}

func TestProfilePicker_NumberKeysJump(t *testing.T) {
	p := NewProfilePicker()
	p.Open(mkPickerProfiles(), "classic")
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if p.focused != 2 {
		t.Errorf("'3' should jump to index 2, got %d", p.focused)
	}
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'9'}})
	if p.focused != 2 {
		t.Errorf("out-of-range '9' should not move focus, got %d", p.focused)
	}
}

func TestProfilePicker_EnterCommitsResult(t *testing.T) {
	p := NewProfilePicker()
	p.Open(mkPickerProfiles(), "classic")
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRight}) // → daily_operator
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.IsActive() {
		t.Error("enter should close the picker")
	}
	id, ok := p.Result()
	if !ok {
		t.Fatal("Result should be ok after enter")
	}
	if id != "daily_operator" {
		t.Errorf("got %q, want daily_operator", id)
	}
}

func TestProfilePicker_EscCancelsWithoutResult(t *testing.T) {
	p := NewProfilePicker()
	p.Open(mkPickerProfiles(), "classic")
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRight})
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if p.IsActive() {
		t.Error("esc should close the picker")
	}
	if _, ok := p.Result(); ok {
		t.Error("esc must not commit a result")
	}
}

func TestProfilePicker_ResultIsConsumedOnce(t *testing.T) {
	p := NewProfilePicker()
	p.Open(mkPickerProfiles(), "classic")
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := p.Result(); !ok {
		t.Fatal("first Result should succeed")
	}
	if _, ok := p.Result(); ok {
		t.Error("second Result should report not-ok (consumed)")
	}
}

func TestProfilePicker_ViewMentionsCurrentAndFocusedProfiles(t *testing.T) {
	p := NewProfilePicker()
	p.Open(mkPickerProfiles(), "researcher")
	out := p.View()
	for _, name := range []string{"Classic", "Daily Operator", "Researcher", "Builder"} {
		if !strings.Contains(out, name) {
			t.Errorf("view missing profile name %q", name)
		}
	}
	if !strings.Contains(out, "Switch Profile") {
		t.Error("view missing header")
	}
}
