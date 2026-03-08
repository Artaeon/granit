package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// Macro recording: StartRecording / StopRecording / IsRecording
// ---------------------------------------------------------------------------

func TestMacro_StartRecording(t *testing.T) {
	vs := NewVimState()

	t.Run("IsRecording returns true after StartRecording", func(t *testing.T) {
		vs.StartRecording('a')
		if !vs.IsRecording() {
			t.Error("expected IsRecording() to return true after StartRecording")
		}
	})

	t.Run("record register is set correctly", func(t *testing.T) {
		vs.StartRecording('z')
		if vs.recordRegister != 'z' {
			t.Errorf("expected recordRegister 'z', got %q", vs.recordRegister)
		}
	})

	t.Run("record buffer is reset on start", func(t *testing.T) {
		vs.recordBuffer = []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune{'x'}}}
		vs.StartRecording('b')
		if len(vs.recordBuffer) != 0 {
			t.Errorf("expected empty record buffer after StartRecording, got %d items", len(vs.recordBuffer))
		}
	})
}

func TestMacro_StopRecording(t *testing.T) {
	t.Run("saves buffer to register on stop", func(t *testing.T) {
		vs := NewVimState()
		vs.StartRecording('a')
		vs.RecordKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		vs.RecordKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		vs.StopRecording()

		if vs.IsRecording() {
			t.Error("expected IsRecording() to return false after StopRecording")
		}

		macro := vs.GetMacro('a')
		if len(macro) != 2 {
			t.Fatalf("expected 2 keys in macro, got %d", len(macro))
		}
		if string(macro[0].Runes) != "j" {
			t.Errorf("expected first key 'j', got %q", string(macro[0].Runes))
		}
		if string(macro[1].Runes) != "k" {
			t.Errorf("expected second key 'k', got %q", string(macro[1].Runes))
		}
	})

	t.Run("stop when not recording is a no-op", func(t *testing.T) {
		vs := NewVimState()
		// Should not panic
		vs.StopRecording()
		if vs.IsRecording() {
			t.Error("should still not be recording")
		}
	})
}

// ---------------------------------------------------------------------------
// RecordingStatus
// ---------------------------------------------------------------------------

func TestMacro_RecordingStatus(t *testing.T) {
	vs := NewVimState()

	t.Run("empty when not recording", func(t *testing.T) {
		status := vs.RecordingStatus()
		if status != "" {
			t.Errorf("expected empty status when not recording, got %q", status)
		}
	})

	t.Run("shows register when recording", func(t *testing.T) {
		vs.StartRecording('a')
		status := vs.RecordingStatus()
		if status != "recording @a" {
			t.Errorf("expected 'recording @a', got %q", status)
		}
	})

	t.Run("shows correct register letter", func(t *testing.T) {
		vs.StartRecording('m')
		status := vs.RecordingStatus()
		if status != "recording @m" {
			t.Errorf("expected 'recording @m', got %q", status)
		}
	})
}

// ---------------------------------------------------------------------------
// Record and replay: record keys, then retrieve with GetMacro
// ---------------------------------------------------------------------------

func TestMacro_RecordAndReplay(t *testing.T) {
	vs := NewVimState()
	vs.StartRecording('c')

	keys := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'d'}},
		{Type: tea.KeyRunes, Runes: []rune{'d'}},
		{Type: tea.KeyRunes, Runes: []rune{'j'}},
	}

	for _, k := range keys {
		vs.RecordKey(k)
	}
	vs.StopRecording()

	macro := vs.GetMacro('c')
	if len(macro) != len(keys) {
		t.Fatalf("expected %d keys in macro, got %d", len(keys), len(macro))
	}

	for i, k := range keys {
		if string(macro[i].Runes) != string(k.Runes) {
			t.Errorf("key %d: expected runes %q, got %q", i, string(k.Runes), string(macro[i].Runes))
		}
		if macro[i].Type != k.Type {
			t.Errorf("key %d: expected type %d, got %d", i, k.Type, macro[i].Type)
		}
	}
}

// ---------------------------------------------------------------------------
// Multiple registers: record into 'a' and 'b' independently
// ---------------------------------------------------------------------------

func TestMacro_MultipleRegisters(t *testing.T) {
	vs := NewVimState()

	// Record macro into register 'a'
	vs.StartRecording('a')
	vs.RecordKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	vs.StopRecording()

	// Record macro into register 'b'
	vs.StartRecording('b')
	vs.RecordKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	vs.RecordKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	vs.StopRecording()

	macroA := vs.GetMacro('a')
	macroB := vs.GetMacro('b')

	if len(macroA) != 1 {
		t.Fatalf("expected 1 key in register 'a', got %d", len(macroA))
	}
	if string(macroA[0].Runes) != "j" {
		t.Errorf("register 'a': expected 'j', got %q", string(macroA[0].Runes))
	}

	if len(macroB) != 2 {
		t.Fatalf("expected 2 keys in register 'b', got %d", len(macroB))
	}
	if string(macroB[0].Runes) != "k" {
		t.Errorf("register 'b' key 0: expected 'k', got %q", string(macroB[0].Runes))
	}
	if string(macroB[1].Runes) != "k" {
		t.Errorf("register 'b' key 1: expected 'k', got %q", string(macroB[1].Runes))
	}
}

// ---------------------------------------------------------------------------
// Overwrite register: recording into same register replaces old macro
// ---------------------------------------------------------------------------

func TestMacro_OverwriteRegister(t *testing.T) {
	vs := NewVimState()

	// First recording into 'a'
	vs.StartRecording('a')
	vs.RecordKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	vs.RecordKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	vs.StopRecording()

	if len(vs.GetMacro('a')) != 2 {
		t.Fatalf("expected 2 keys after first recording, got %d", len(vs.GetMacro('a')))
	}

	// Second recording into 'a' — should replace
	vs.StartRecording('a')
	vs.RecordKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	vs.StopRecording()

	macro := vs.GetMacro('a')
	if len(macro) != 1 {
		t.Fatalf("expected 1 key after overwrite, got %d", len(macro))
	}
	if string(macro[0].Runes) != "o" {
		t.Errorf("expected 'o' after overwrite, got %q", string(macro[0].Runes))
	}
}

// ---------------------------------------------------------------------------
// Empty macro: start and immediately stop
// ---------------------------------------------------------------------------

func TestMacro_EmptyMacro(t *testing.T) {
	vs := NewVimState()
	vs.StartRecording('e')
	vs.StopRecording()

	macro := vs.GetMacro('e')
	if macro == nil {
		t.Fatal("expected non-nil macro for empty recording")
	}
	if len(macro) != 0 {
		t.Errorf("expected 0 keys in empty macro, got %d", len(macro))
	}
}

// ---------------------------------------------------------------------------
// Get nonexistent macro: should return nil
// ---------------------------------------------------------------------------

func TestMacro_GetNonexistent(t *testing.T) {
	vs := NewVimState()
	macro := vs.GetMacro('z')
	if macro != nil {
		t.Errorf("expected nil for unrecorded register, got %v", macro)
	}
}

// ---------------------------------------------------------------------------
// Last macro register: verify @@ tracking
// ---------------------------------------------------------------------------

func TestMacro_LastMacroRegister(t *testing.T) {
	vs := NewVimState()

	t.Run("initial last register is zero", func(t *testing.T) {
		if vs.LastMacroRegister() != 0 {
			t.Errorf("expected initial lastMacroRegister to be 0, got %d", vs.LastMacroRegister())
		}
	})

	t.Run("SetLastMacroRegister stores register", func(t *testing.T) {
		vs.SetLastMacroRegister('a')
		if vs.LastMacroRegister() != 'a' {
			t.Errorf("expected lastMacroRegister 'a', got %c", vs.LastMacroRegister())
		}
	})

	t.Run("SetLastMacroRegister updates to new register", func(t *testing.T) {
		vs.SetLastMacroRegister('b')
		if vs.LastMacroRegister() != 'b' {
			t.Errorf("expected lastMacroRegister 'b', got %c", vs.LastMacroRegister())
		}
	})
}

// ---------------------------------------------------------------------------
// Recursive prevention: playingMacro flag prevents nested recording
// ---------------------------------------------------------------------------

func TestMacro_RecursivePrevention(t *testing.T) {
	vs := NewVimState()

	t.Run("SetPlayingMacro sets flag", func(t *testing.T) {
		vs.SetPlayingMacro(true)
		if !vs.IsPlayingMacro() {
			t.Error("expected IsPlayingMacro() to return true")
		}
	})

	t.Run("RecordKey skips when playingMacro is true", func(t *testing.T) {
		vs.StartRecording('a')
		vs.SetPlayingMacro(true)
		vs.RecordKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		vs.RecordKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		vs.SetPlayingMacro(false)
		vs.StopRecording()

		macro := vs.GetMacro('a')
		if len(macro) != 0 {
			t.Errorf("expected 0 keys recorded while playingMacro, got %d", len(macro))
		}
	})

	t.Run("RecordKey works normally when playingMacro is false", func(t *testing.T) {
		vs2 := NewVimState()
		vs2.StartRecording('b')
		vs2.SetPlayingMacro(false)
		vs2.RecordKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		vs2.StopRecording()

		macro := vs2.GetMacro('b')
		if len(macro) != 1 {
			t.Errorf("expected 1 key recorded, got %d", len(macro))
		}
	})

	t.Run("clearing playingMacro flag", func(t *testing.T) {
		vs.SetPlayingMacro(false)
		if vs.IsPlayingMacro() {
			t.Error("expected IsPlayingMacro() to return false after clearing")
		}
	})
}

// ---------------------------------------------------------------------------
// HandleKey integration: q{reg} starts macro, q stops it
// ---------------------------------------------------------------------------

func TestMacro_HandleKey_QStartStop(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run("q followed by register letter returns MacroStart", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("q", content, 0, 0, height) // sets pending "q"
		r := vs.HandleKey("a", content, 0, 0, height)
		if r.MacroStart != 'a' {
			t.Errorf("expected MacroStart 'a', got %d", r.MacroStart)
		}
	})

	t.Run("q while recording returns MacroStop", func(t *testing.T) {
		vs := enabledVim()
		vs.StartRecording('a')
		r := vs.HandleKey("q", content, 0, 0, height)
		if !r.MacroStop {
			t.Error("expected MacroStop flag when pressing q while recording")
		}
	})

	t.Run("q followed by invalid char cancels", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("q", content, 0, 0, height)
		r := vs.HandleKey("1", content, 0, 0, height)
		if r.MacroStart != 0 {
			t.Error("expected no MacroStart for invalid register")
		}
		if r.MacroStop {
			t.Error("expected no MacroStop for invalid register")
		}
	})
}

// ---------------------------------------------------------------------------
// HandleKey integration: @{reg} replays macro, @@ replays last
// ---------------------------------------------------------------------------

func TestMacro_HandleKey_AtReplay(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run("@ followed by register returns MacroReplay", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("@", content, 0, 0, height)
		r := vs.HandleKey("a", content, 0, 0, height)
		if r.MacroReplay != 'a' {
			t.Errorf("expected MacroReplay 'a', got %d", r.MacroReplay)
		}
	})

	t.Run("@@ replays last macro register", func(t *testing.T) {
		vs := enabledVim()
		vs.SetLastMacroRegister('m')
		vs.HandleKey("@", content, 0, 0, height)
		r := vs.HandleKey("@", content, 0, 0, height)
		if r.MacroReplay != 'm' {
			t.Errorf("expected MacroReplay 'm' for @@, got %d", r.MacroReplay)
		}
	})

	t.Run("@@ with no last register does nothing", func(t *testing.T) {
		vs := enabledVim()
		// lastMacroRegister is 0 by default
		vs.HandleKey("@", content, 0, 0, height)
		r := vs.HandleKey("@", content, 0, 0, height)
		if r.MacroReplay != 0 {
			t.Errorf("expected no MacroReplay for @@ with no last register, got %d", r.MacroReplay)
		}
	})

	t.Run("@ followed by invalid char cancels", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("@", content, 0, 0, height)
		r := vs.HandleKey("1", content, 0, 0, height)
		if r.MacroReplay != 0 {
			t.Error("expected no MacroReplay for invalid register")
		}
	})
}
