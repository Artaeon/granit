package tui

import (
	"testing"
	"time"
)

// ── CommandHistory — frecency math ──

func TestCommandHistory_NilSafe(t *testing.T) {
	var h *CommandHistory
	// Methods must not panic on nil receiver — palette runs without
	// history when the on-disk file is unreadable.
	h.Record(CmdDailyNote)
	h.Save()
	if got := h.FrecencyScore(CmdDailyNote); got != 0 {
		t.Errorf("nil receiver should score 0, got %d", got)
	}
}

func TestCommandHistory_RecordIncrements(t *testing.T) {
	h := &CommandHistory{Usage: map[string]commandUsage{}}
	h.Record(CmdDailyNote)
	h.Record(CmdDailyNote)
	if got := h.Usage[commandKey(CmdDailyNote)].Count; got != 2 {
		t.Errorf("Count = %d, want 2", got)
	}
}

func TestCommandHistory_FrecencyZeroForUntracked(t *testing.T) {
	h := &CommandHistory{Usage: map[string]commandUsage{}}
	if got := h.FrecencyScore(CmdDailyNote); got != 0 {
		t.Errorf("untracked = %d, want 0", got)
	}
}

func TestCommandHistory_FrecencyDecaysWithAge(t *testing.T) {
	now := time.Now()
	h := &CommandHistory{Usage: map[string]commandUsage{
		commandKey(CmdDailyNote):     {LastUsed: now, Count: 5},
		commandKey(CmdMorningRoutine): {LastUsed: now.AddDate(0, 0, -30), Count: 5},
	}}
	fresh := h.FrecencyScore(CmdDailyNote)
	stale := h.FrecencyScore(CmdMorningRoutine)
	if fresh <= stale {
		t.Errorf("recent (%d) should outrank 30-day-old (%d) at equal counts", fresh, stale)
	}
}

func TestCommandHistory_FrecencyRewardsRepeats(t *testing.T) {
	now := time.Now()
	h := &CommandHistory{Usage: map[string]commandUsage{
		commandKey(CmdDailyNote):     {LastUsed: now, Count: 50},
		commandKey(CmdMorningRoutine): {LastUsed: now, Count: 1},
	}}
	heavy := h.FrecencyScore(CmdDailyNote)
	light := h.FrecencyScore(CmdMorningRoutine)
	if heavy <= light {
		t.Errorf("50-use cmd (%d) should outrank 1-use (%d) at same recency", heavy, light)
	}
	// But not by 50× — log curve should keep things bounded.
	if heavy > light*10 {
		t.Errorf("frequency boost too aggressive: heavy=%d light=%d", heavy, light)
	}
}

// ── Palette integration — empty query rank ──

func TestCommandPalette_EmptyQuery_RanksByFrecency(t *testing.T) {
	cp := NewCommandPalette()
	cp.history = &CommandHistory{Usage: map[string]commandUsage{
		commandKey(CmdDailyNote): {LastUsed: time.Now(), Count: 100},
	}}
	cp.Open()

	if len(cp.filtered) == 0 {
		t.Fatal("expected non-empty palette")
	}
	if cp.filtered[0].Action != CmdDailyNote {
		t.Errorf("expected CmdDailyNote first, got %v (%q)", cp.filtered[0].Action, cp.filtered[0].Label)
	}
}

func TestCommandPalette_QueryMatch_FrecencyBoostsTies(t *testing.T) {
	// Two commands both fuzzy-match — the more-used one should win.
	cp := NewCommandPalette()
	cp.history = &CommandHistory{Usage: map[string]commandUsage{
		commandKey(CmdDailyNote): {LastUsed: time.Now(), Count: 100},
	}}
	cp.query = "dai"
	cp.filterCommands()

	if len(cp.filtered) == 0 {
		t.Fatal("expected matches for 'dai'")
	}
	if cp.filtered[0].Action != CmdDailyNote {
		t.Errorf("expected boosted CmdDailyNote first, got %v (%q)", cp.filtered[0].Action, cp.filtered[0].Label)
	}
}
