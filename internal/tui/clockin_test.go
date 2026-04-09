package tui

import (
	"testing"
	"time"
)

func TestClockIn_StatusString_Inactive(t *testing.T) {
	c := ClockIn{active: false}
	if got := c.StatusString(); got != "" {
		t.Errorf("expected empty status when inactive, got %q", got)
	}
}

func TestClockIn_StatusString_Active(t *testing.T) {
	c := ClockIn{
		active:  true,
		elapsed: 1*time.Hour + 30*time.Minute + 15*time.Second,
	}
	got := c.StatusString()
	if got != "⏱ 1:30:15" {
		t.Errorf("expected '⏱ 1:30:15', got %q", got)
	}
}

func TestClockIn_StatusString_WithProject(t *testing.T) {
	c := ClockIn{
		active:  true,
		elapsed: 5 * time.Minute,
		project: "MealTime",
	}
	got := c.StatusString()
	if got != "⏱ 0:05:00 · MealTime" {
		t.Errorf("expected '⏱ 0:05:00 · MealTime', got %q", got)
	}
}

func TestClockIn_TodayTotal_IncludesActive(t *testing.T) {
	c := ClockIn{
		active:     true,
		elapsed:    30 * time.Minute,
		todayTotal: 2 * time.Hour,
	}
	total := c.TodayTotal()
	expected := 2*time.Hour + 30*time.Minute
	if total != expected {
		t.Errorf("expected %v, got %v", expected, total)
	}
}

func TestClockIn_TodayTotal_InactiveExcludesElapsed(t *testing.T) {
	c := ClockIn{
		active:     false,
		elapsed:    30 * time.Minute, // leftover from previous session
		todayTotal: 2 * time.Hour,
	}
	if c.TodayTotal() != 2*time.Hour {
		t.Errorf("expected 2h when inactive, got %v", c.TodayTotal())
	}
}

func TestClockIn_DoubleClockIn(t *testing.T) {
	c := ClockIn{
		vaultPath:  t.TempDir(),
		active:     true,
		firedToday: make(map[string]bool),
	}
	cmd := c.ClockInCmd("test")
	if cmd != nil {
		t.Error("should not allow double clock-in")
	}
}

func TestClockIn_ClockOutWhenInactive(t *testing.T) {
	c := ClockIn{
		vaultPath:  t.TempDir(),
		active:     false,
		firedToday: make(map[string]bool),
	}
	cmd := c.ClockOutCmd()
	if cmd != nil {
		t.Error("should not allow clock-out when inactive")
	}
}
