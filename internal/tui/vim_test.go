package tui

import "testing"

// helper: enabled VimState in normal mode.
func enabledVim() *VimState {
	vs := NewVimState()
	vs.SetEnabled(true)
	return vs
}

// sampleContent returns a small multi-line document for testing motions.
func sampleContent() []string {
	return []string{
		"hello world",      // 0
		"foo bar baz",      // 1
		"  indented line",  // 2
		"last line",        // 3
	}
}

// ---------------------------------------------------------------------------
// NewVimState
// ---------------------------------------------------------------------------

func TestNewVimState(t *testing.T) {
	vs := NewVimState()

	t.Run("starts in normal mode", func(t *testing.T) {
		if vs.Mode() != VimNormal {
			t.Errorf("expected VimNormal, got %d", vs.Mode())
		}
	})

	t.Run("disabled by default", func(t *testing.T) {
		if vs.IsEnabled() {
			t.Error("expected vim to be disabled by default")
		}
	})

	t.Run("mode string is NORMAL", func(t *testing.T) {
		if vs.ModeString() != "NORMAL" {
			t.Errorf("expected 'NORMAL', got %q", vs.ModeString())
		}
	})
}

// ---------------------------------------------------------------------------
// Mode transitions
// ---------------------------------------------------------------------------

func TestModeTransitions(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run("Normal to Insert via i", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("i", content, 0, 0, height)
		if vs.Mode() != VimInsert {
			t.Error("expected VimInsert mode")
		}
		if !r.EnterInsert {
			t.Error("expected EnterInsert flag")
		}
	})

	t.Run("Insert to Normal via Esc", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("i", content, 0, 0, height) // enter insert
		r := vs.HandleKey("esc", content, 0, 0, height)
		if vs.Mode() != VimNormal {
			t.Error("expected VimNormal mode after Esc")
		}
		if !r.EnterNormal {
			t.Error("expected EnterNormal flag")
		}
	})

	t.Run("Normal to Visual via v", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("v", content, 0, 0, height)
		if vs.Mode() != VimVisual {
			t.Error("expected VimVisual mode")
		}
		if !r.EnterVisual {
			t.Error("expected EnterVisual flag")
		}
	})

	t.Run("Visual to Normal via Esc", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("v", content, 0, 0, height)
		r := vs.HandleKey("esc", content, 0, 0, height)
		if vs.Mode() != VimNormal {
			t.Error("expected VimNormal after Esc from Visual")
		}
		if !r.EnterNormal {
			t.Error("expected EnterNormal flag")
		}
	})

	t.Run("Normal to Command via colon", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey(":", content, 0, 0, height)
		if vs.Mode() != VimCommand {
			t.Error("expected VimCommand mode")
		}
		if !r.EnterCommand {
			t.Error("expected EnterCommand flag")
		}
	})

	t.Run("Command to Normal via Esc", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		r := vs.HandleKey("esc", content, 0, 0, height)
		if vs.Mode() != VimNormal {
			t.Error("expected VimNormal after Esc from Command")
		}
		if !r.EnterNormal {
			t.Error("expected EnterNormal flag")
		}
	})
}

// ---------------------------------------------------------------------------
// ModeString
// ---------------------------------------------------------------------------

func TestModeString(t *testing.T) {
	vs := enabledVim()
	content := sampleContent()
	height := 24

	tests := []struct {
		name string
		key  string
		want string
	}{
		{"insert", "i", "INSERT"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vs2 := enabledVim()
			vs2.HandleKey(tc.key, content, 0, 0, height)
			if vs2.ModeString() != tc.want {
				t.Errorf("expected %q, got %q", tc.want, vs2.ModeString())
			}
		})
	}

	// Visual
	vs.HandleKey("v", content, 0, 0, height)
	if vs.ModeString() != "VISUAL" {
		t.Errorf("expected 'VISUAL', got %q", vs.ModeString())
	}

	// Command
	vs3 := enabledVim()
	vs3.HandleKey(":", content, 0, 0, height)
	if vs3.ModeString() != "COMMAND" {
		t.Errorf("expected 'COMMAND', got %q", vs3.ModeString())
	}
}

// ---------------------------------------------------------------------------
// Normal mode motions: h/j/k/l
// ---------------------------------------------------------------------------

func TestNormalMotions_HJKL(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run("h moves left", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("h", content, 0, 5, height)
		if !r.CursorSet || r.NewCol != 4 {
			t.Errorf("expected col 4, got %d (CursorSet=%v)", r.NewCol, r.CursorSet)
		}
	})

	t.Run("h at col 0 stays at 0", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("h", content, 0, 0, height)
		if r.NewCol != 0 {
			t.Errorf("expected col 0, got %d", r.NewCol)
		}
	})

	t.Run("l moves right", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("l", content, 0, 0, height)
		if !r.CursorSet || r.NewCol != 1 {
			t.Errorf("expected col 1, got %d", r.NewCol)
		}
	})

	t.Run("l clamps to line length - 1", func(t *testing.T) {
		vs := enabledVim()
		// "hello world" has 11 chars, max col = 10
		r := vs.HandleKey("l", content, 0, 10, height)
		if r.NewCol != 10 {
			t.Errorf("expected col 10 (clamped), got %d", r.NewCol)
		}
	})

	t.Run("j moves down", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("j", content, 0, 0, height)
		if !r.CursorSet || r.NewCursor != 1 {
			t.Errorf("expected cursor line 1, got %d", r.NewCursor)
		}
	})

	t.Run("j at last line stays", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("j", content, 3, 0, height)
		if r.NewCursor != 3 {
			t.Errorf("expected cursor line 3, got %d", r.NewCursor)
		}
	})

	t.Run("k moves up", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("k", content, 2, 0, height)
		if !r.CursorSet || r.NewCursor != 1 {
			t.Errorf("expected cursor line 1, got %d", r.NewCursor)
		}
	})

	t.Run("k at first line stays", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("k", content, 0, 0, height)
		if r.NewCursor != 0 {
			t.Errorf("expected cursor line 0, got %d", r.NewCursor)
		}
	})
}

// ---------------------------------------------------------------------------
// Word motions: w, b, e
// ---------------------------------------------------------------------------

func TestWordMotions(t *testing.T) {
	content := []string{"hello world foo"}
	height := 24

	t.Run("w moves to next word", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("w", content, 0, 0, height)
		if !r.CursorSet {
			t.Fatal("expected CursorSet")
		}
		// "hello" ends at 4, next word "world" starts at 6
		if r.NewCol != 6 {
			t.Errorf("expected col 6, got %d", r.NewCol)
		}
	})

	t.Run("b moves to previous word", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("b", content, 0, 6, height)
		if !r.CursorSet {
			t.Fatal("expected CursorSet")
		}
		if r.NewCol != 0 {
			t.Errorf("expected col 0, got %d", r.NewCol)
		}
	})

	t.Run("e moves to end of word", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("e", content, 0, 0, height)
		if !r.CursorSet {
			t.Fatal("expected CursorSet")
		}
		// end of "hello" is col 4
		if r.NewCol != 4 {
			t.Errorf("expected col 4, got %d", r.NewCol)
		}
	})

	t.Run("w wraps to next line", func(t *testing.T) {
		content2 := []string{"hello", "world"}
		vs := enabledVim()
		// cursor at end of "hello" (col 4), w should go to next line col 0
		r := vs.HandleKey("w", content2, 0, 4, height)
		if r.NewCursor != 1 {
			t.Errorf("expected line 1, got %d", r.NewCursor)
		}
	})
}

// ---------------------------------------------------------------------------
// Line motions: 0, $, ^
// ---------------------------------------------------------------------------

func TestLineMotions(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run("0 goes to start of line", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("0", content, 1, 5, height)
		if !r.CursorSet || r.NewCol != 0 {
			t.Errorf("expected col 0, got %d", r.NewCol)
		}
	})

	t.Run("$ goes to end of line", func(t *testing.T) {
		vs := enabledVim()
		// "foo bar baz" has 11 chars, last col = 10
		r := vs.HandleKey("$", content, 1, 0, height)
		if !r.CursorSet || r.NewCol != 10 {
			t.Errorf("expected col 10, got %d", r.NewCol)
		}
	})

	t.Run("^ goes to first non-space", func(t *testing.T) {
		vs := enabledVim()
		// "  indented line" has first non-space at col 2
		r := vs.HandleKey("^", content, 2, 8, height)
		if !r.CursorSet || r.NewCol != 2 {
			t.Errorf("expected col 2, got %d", r.NewCol)
		}
	})
}

// ---------------------------------------------------------------------------
// Document motions: gg, G
// ---------------------------------------------------------------------------

func TestDocumentMotions(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run("gg goes to top", func(t *testing.T) {
		vs := enabledVim()
		// gg is a two-key combo: g then g
		vs.HandleKey("g", content, 3, 0, height)
		r := vs.HandleKey("g", content, 3, 0, height)
		if !r.CursorSet || r.NewCursor != 0 {
			t.Errorf("expected cursor line 0, got %d", r.NewCursor)
		}
	})

	t.Run("G goes to bottom", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("G", content, 0, 0, height)
		if !r.CursorSet || r.NewCursor != 3 {
			t.Errorf("expected cursor line 3, got %d", r.NewCursor)
		}
	})

	t.Run("gg from middle goes to first line", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("g", content, 2, 5, height) // pending g
		r := vs.HandleKey("g", content, 2, 5, height)
		if !r.CursorSet || r.NewCursor != 0 {
			t.Errorf("expected cursor line 0, got %d", r.NewCursor)
		}
		if r.NewCol != 0 {
			t.Errorf("expected col 0, got %d", r.NewCol)
		}
	})
}

// ---------------------------------------------------------------------------
// Delete operations: dd, x
// ---------------------------------------------------------------------------

func TestDeleteOperations(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run("dd deletes current line", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("d", content, 1, 0, height) // pending d
		r := vs.HandleKey("d", content, 1, 0, height)
		if !r.DeleteLine {
			t.Error("expected DeleteLine flag for dd")
		}
	})

	t.Run("dd stores deleted line in register", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("d", content, 1, 0, height)
		vs.HandleKey("d", content, 1, 0, height)
		if vs.register != "foo bar baz" {
			t.Errorf("expected register 'foo bar baz', got %q", vs.register)
		}
	})

	t.Run("x deletes char under cursor", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("x", content, 0, 2, height)
		if r.StatusMsg != "delete_char" {
			t.Errorf("expected status 'delete_char', got %q", r.StatusMsg)
		}
		// Register should hold the deleted char 'l'
		if vs.register != "l" {
			t.Errorf("expected register 'l', got %q", vs.register)
		}
	})
}

// ---------------------------------------------------------------------------
// Yank: yy
// ---------------------------------------------------------------------------

func TestYank(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run("yy yanks current line", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("y", content, 0, 0, height)
		r := vs.HandleKey("y", content, 0, 0, height)
		if r.StatusMsg != "yanked" {
			t.Errorf("expected status 'yanked', got %q", r.StatusMsg)
		}
		if vs.register != "hello world" {
			t.Errorf("expected register 'hello world', got %q", vs.register)
		}
	})

	t.Run("yy on last line yanks that line", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("y", content, 3, 0, height)
		r := vs.HandleKey("y", content, 3, 0, height)
		if r.StatusMsg != "yanked" {
			t.Errorf("expected status 'yanked', got %q", r.StatusMsg)
		}
		if vs.register != "last line" {
			t.Errorf("expected register 'last line', got %q", vs.register)
		}
	})
}

// ---------------------------------------------------------------------------
// Paste: p (paste after)
// ---------------------------------------------------------------------------

func TestPaste(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run("p pastes below", func(t *testing.T) {
		vs := enabledVim()
		vs.register = "pasted text"
		r := vs.HandleKey("p", content, 0, 0, height)
		if !r.PasteBelow {
			t.Error("expected PasteBelow flag")
		}
		if r.PasteText != "pasted text" {
			t.Errorf("expected PasteText 'pasted text', got %q", r.PasteText)
		}
	})

	t.Run("P pastes above", func(t *testing.T) {
		vs := enabledVim()
		vs.register = "above"
		r := vs.HandleKey("P", content, 1, 0, height)
		if !r.PasteAbove {
			t.Error("expected PasteAbove flag")
		}
		if r.PasteText != "above" {
			t.Errorf("expected PasteText 'above', got %q", r.PasteText)
		}
	})
}

// ---------------------------------------------------------------------------
// Insert entry points: i, a, o, O
// ---------------------------------------------------------------------------

func TestInsertEntryPoints(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run("i enters insert before cursor", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("i", content, 0, 3, height)
		if !r.EnterInsert {
			t.Error("expected EnterInsert")
		}
		if vs.Mode() != VimInsert {
			t.Error("expected VimInsert mode")
		}
		// 'i' does not move cursor
		if r.CursorSet {
			t.Error("'i' should not set cursor position")
		}
	})

	t.Run("a enters insert after cursor", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("a", content, 0, 3, height)
		if !r.EnterInsert {
			t.Error("expected EnterInsert")
		}
		if !r.CursorSet || r.NewCol != 4 {
			t.Errorf("expected col 4, got %d (CursorSet=%v)", r.NewCol, r.CursorSet)
		}
	})

	t.Run("o opens new line below and enters insert", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("o", content, 1, 0, height)
		if !r.EnterInsert {
			t.Error("expected EnterInsert")
		}
		if !r.CursorSet || r.NewCursor != 2 {
			t.Errorf("expected cursor line 2, got %d", r.NewCursor)
		}
		if r.NewCol != 0 {
			t.Errorf("expected col 0, got %d", r.NewCol)
		}
	})

	t.Run("O opens new line above and enters insert", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("O", content, 1, 0, height)
		if !r.EnterInsert {
			t.Error("expected EnterInsert")
		}
		if !r.PasteAbove {
			t.Error("expected PasteAbove flag for O")
		}
		if !r.CursorSet || r.NewCursor != 1 {
			t.Errorf("expected cursor line 1, got %d", r.NewCursor)
		}
	})

	t.Run("I enters insert at first non-space", func(t *testing.T) {
		vs := enabledVim()
		// "  indented line" first non-space is col 2
		r := vs.HandleKey("I", content, 2, 8, height)
		if !r.EnterInsert {
			t.Error("expected EnterInsert")
		}
		if !r.CursorSet || r.NewCol != 2 {
			t.Errorf("expected col 2, got %d", r.NewCol)
		}
	})

	t.Run("A enters insert at end of line", func(t *testing.T) {
		vs := enabledVim()
		// "hello world" has 11 chars
		r := vs.HandleKey("A", content, 0, 3, height)
		if !r.EnterInsert {
			t.Error("expected EnterInsert")
		}
		if !r.CursorSet || r.NewCol != 11 {
			t.Errorf("expected col 11, got %d", r.NewCol)
		}
	})
}

// ---------------------------------------------------------------------------
// Insert mode passes through non-Esc keys
// ---------------------------------------------------------------------------

func TestInsertMode_PassThrough(t *testing.T) {
	vs := enabledVim()
	content := sampleContent()
	height := 24
	vs.HandleKey("i", content, 0, 0, height) // enter insert

	t.Run("printable chars pass through", func(t *testing.T) {
		r := vs.HandleKey("a", content, 0, 0, height)
		if !r.PassThrough {
			t.Error("expected PassThrough for printable char in insert mode")
		}
	})

	t.Run("enter passes through", func(t *testing.T) {
		r := vs.HandleKey("enter", content, 0, 0, height)
		if !r.PassThrough {
			t.Error("expected PassThrough for enter in insert mode")
		}
	})
}

// ---------------------------------------------------------------------------
// Command mode: :w, :q, :wq, :{number}
// ---------------------------------------------------------------------------

func TestCommandMode(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run(":w saves", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		vs.HandleKey("w", content, 0, 0, height)
		r := vs.HandleKey("enter", content, 0, 0, height)
		if r.StatusMsg != "save" {
			t.Errorf("expected status 'save', got %q", r.StatusMsg)
		}
		if vs.Mode() != VimNormal {
			t.Error("expected return to normal mode after :w")
		}
	})

	t.Run(":q quits", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		vs.HandleKey("q", content, 0, 0, height)
		r := vs.HandleKey("enter", content, 0, 0, height)
		if r.StatusMsg != "quit" {
			t.Errorf("expected status 'quit', got %q", r.StatusMsg)
		}
	})

	t.Run(":wq saves and quits", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		vs.HandleKey("w", content, 0, 0, height)
		vs.HandleKey("q", content, 0, 0, height)
		r := vs.HandleKey("enter", content, 0, 0, height)
		if r.StatusMsg != "save_quit" {
			t.Errorf("expected status 'save_quit', got %q", r.StatusMsg)
		}
	})

	t.Run(":10 goes to line 10", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		vs.HandleKey("1", content, 0, 0, height)
		vs.HandleKey("0", content, 0, 0, height)
		r := vs.HandleKey("enter", content, 0, 0, height)
		// Line 10 = index 9, but we only have 4 lines, so clamp to 3
		if !r.CursorSet || r.NewCursor != 3 {
			t.Errorf("expected cursor line 3, got %d (CursorSet=%v)", r.NewCursor, r.CursorSet)
		}
	})

	t.Run(":2 goes to line 2", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		vs.HandleKey("2", content, 0, 0, height)
		r := vs.HandleKey("enter", content, 0, 0, height)
		// Line 2 = index 1
		if !r.CursorSet || r.NewCursor != 1 {
			t.Errorf("expected cursor line 1, got %d", r.NewCursor)
		}
	})

	t.Run("unknown command shows error", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		vs.HandleKey("z", content, 0, 0, height)
		vs.HandleKey("z", content, 0, 0, height)
		r := vs.HandleKey("enter", content, 0, 0, height)
		if r.StatusMsg != "unknown command: zz" {
			t.Errorf("expected unknown command message, got %q", r.StatusMsg)
		}
	})

	t.Run("backspace in command mode removes last char", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		vs.HandleKey("w", content, 0, 0, height)
		vs.HandleKey("q", content, 0, 0, height)
		r := vs.HandleKey("backspace", content, 0, 0, height)
		if r.StatusMsg != ":w" {
			t.Errorf("expected ':w' after backspace, got %q", r.StatusMsg)
		}
	})

	t.Run("backspace on empty cmd buffer returns to normal", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		vs.HandleKey("x", content, 0, 0, height)
		vs.HandleKey("backspace", content, 0, 0, height) // removes 'x', buf empty
		if vs.Mode() != VimNormal {
			t.Error("expected return to normal when cmd buffer emptied by backspace")
		}
	})
}

// ---------------------------------------------------------------------------
// Dot repeat
// ---------------------------------------------------------------------------

func TestDotRepeat(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run("dot repeats x (delete char)", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("x", content, 0, 0, height)
		if vs.lastAction != "x" {
			t.Fatalf("expected lastAction 'x', got %q", vs.lastAction)
		}
		r := vs.HandleKey(".", content, 0, 0, height)
		if r.StatusMsg != "delete_char" {
			t.Errorf("expected dot to repeat x, got status %q", r.StatusMsg)
		}
	})

	t.Run("dd sets lastAction to dd", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("d", content, 0, 0, height)
		vs.HandleKey("d", content, 0, 0, height)
		if vs.lastAction != "dd" {
			t.Errorf("expected lastAction 'dd', got %q", vs.lastAction)
		}
	})

	t.Run("dot repeats o (open line below)", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("o", content, 0, 0, height)
		if vs.lastAction != "o" {
			t.Fatalf("expected lastAction 'o', got %q", vs.lastAction)
		}
		// Return to normal mode to use dot
		vs.HandleKey("esc", content, 1, 0, height)
		r := vs.HandleKey(".", content, 1, 0, height)
		if !r.EnterInsert {
			t.Error("expected dot to repeat o with EnterInsert flag")
		}
	})

	t.Run("dot with no previous action is noop", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey(".", content, 0, 0, height)
		// Should return empty result
		if r.CursorSet || r.DeleteLine || r.EnterInsert {
			t.Error("dot with no lastAction should be noop")
		}
	})
}

// ---------------------------------------------------------------------------
// Fold commands: za, zM, zR
// ---------------------------------------------------------------------------

func TestFoldCommands(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run("za toggles fold", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("z", content, 0, 0, height) // pending z
		r := vs.HandleKey("a", content, 0, 0, height)
		if !r.FoldToggle {
			t.Error("expected FoldToggle flag for za")
		}
	})

	t.Run("zM folds all", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("z", content, 0, 0, height)
		r := vs.HandleKey("M", content, 0, 0, height)
		if !r.FoldAll {
			t.Error("expected FoldAll flag for zM")
		}
	})

	t.Run("zR unfolds all", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("z", content, 0, 0, height)
		r := vs.HandleKey("R", content, 0, 0, height)
		if !r.UnfoldAll {
			t.Error("expected UnfoldAll flag for zR")
		}
	})

	t.Run("z followed by unknown key is noop", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("z", content, 0, 0, height)
		r := vs.HandleKey("x", content, 0, 0, height)
		if r.FoldToggle || r.FoldAll || r.UnfoldAll {
			t.Error("unexpected fold action for zx")
		}
	})
}

// ---------------------------------------------------------------------------
// Numeric count prefix
// ---------------------------------------------------------------------------

func TestNumericCount(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run("3j moves down 3 lines", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("3", content, 0, 0, height)
		r := vs.HandleKey("j", content, 0, 0, height)
		if !r.CursorSet || r.NewCursor != 3 {
			t.Errorf("expected cursor line 3, got %d", r.NewCursor)
		}
	})

	t.Run("2k moves up 2 lines", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("2", content, 3, 0, height)
		r := vs.HandleKey("k", content, 3, 0, height)
		if !r.CursorSet || r.NewCursor != 1 {
			t.Errorf("expected cursor line 1, got %d", r.NewCursor)
		}
	})

	t.Run("5l moves right (clamped to line end)", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("5", content, 0, 0, height)
		r := vs.HandleKey("l", content, 0, 0, height)
		if !r.CursorSet || r.NewCol != 5 {
			t.Errorf("expected col 5, got %d", r.NewCol)
		}
	})
}

// ---------------------------------------------------------------------------
// Undo / Redo signals
// ---------------------------------------------------------------------------

func TestUndoRedo_Signals(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run("u signals undo", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("u", content, 0, 0, height)
		if !r.Undo {
			t.Error("expected Undo flag")
		}
	})

	t.Run("ctrl+r signals redo", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("ctrl+r", content, 0, 0, height)
		if !r.Redo {
			t.Error("expected Redo flag")
		}
	})
}

// ---------------------------------------------------------------------------
// Join line: J
// ---------------------------------------------------------------------------

func TestJoinLine(t *testing.T) {
	content := sampleContent()
	height := 24

	vs := enabledVim()
	r := vs.HandleKey("J", content, 0, 0, height)
	if !r.JoinLine {
		t.Error("expected JoinLine flag")
	}
}

// ---------------------------------------------------------------------------
// Disabled vim passes through
// ---------------------------------------------------------------------------

func TestDisabledVim_PassThrough(t *testing.T) {
	vs := NewVimState() // disabled by default
	content := sampleContent()
	r := vs.HandleKey("j", content, 0, 0, 24)
	if !r.PassThrough {
		t.Error("disabled vim should pass through all keys")
	}
}

// ---------------------------------------------------------------------------
// Visual mode motions
// ---------------------------------------------------------------------------

func TestVisualMode_Motions(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run("j extends selection down", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("v", content, 1, 3, height)
		r := vs.HandleKey("j", content, 1, 3, height)
		if !r.CursorSet || r.NewCursor != 2 {
			t.Errorf("expected cursor line 2, got %d", r.NewCursor)
		}
	})

	t.Run("k extends selection up", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("v", content, 2, 0, height)
		r := vs.HandleKey("k", content, 2, 0, height)
		if !r.CursorSet || r.NewCursor != 1 {
			t.Errorf("expected cursor line 1, got %d", r.NewCursor)
		}
	})

	t.Run("d in visual deletes range", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("v", content, 0, 0, height)
		// move to line 1 then delete
		vs.HandleKey("j", content, 0, 0, height)
		r := vs.HandleKey("d", content, 1, 0, height)
		if r.DeleteRange != [2]int{0, 1} {
			t.Errorf("expected DeleteRange [0,1], got %v", r.DeleteRange)
		}
		if vs.Mode() != VimNormal {
			t.Error("expected return to normal after visual delete")
		}
	})

	t.Run("y in visual yanks range", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey("v", content, 1, 0, height)
		r := vs.HandleKey("y", content, 1, 0, height)
		if r.StatusMsg != "yanked" {
			t.Errorf("expected status 'yanked', got %q", r.StatusMsg)
		}
		if vs.Mode() != VimNormal {
			t.Error("expected return to normal after visual yank")
		}
	})
}

// ---------------------------------------------------------------------------
// Search initiation
// ---------------------------------------------------------------------------

func TestSearchInitiation(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run("/ starts forward search", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("/", content, 0, 0, height)
		if r.StatusMsg != "/" {
			t.Errorf("expected status '/', got %q", r.StatusMsg)
		}
	})

	t.Run("? starts backward search", func(t *testing.T) {
		vs := enabledVim()
		r := vs.HandleKey("?", content, 0, 0, height)
		if r.StatusMsg != "?" {
			t.Errorf("expected status '?', got %q", r.StatusMsg)
		}
	})
}

// ---------------------------------------------------------------------------
// Ex commands: :x, :q!, :e, :s, :%s, :set, :noh
// ---------------------------------------------------------------------------

func TestExCommand_X(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run(":x saves and quits", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		vs.HandleKey("x", content, 0, 0, height)
		r := vs.HandleKey("enter", content, 0, 0, height)
		if r.StatusMsg != "save_quit" {
			t.Errorf("expected status 'save_quit', got %q", r.StatusMsg)
		}
		if vs.Mode() != VimNormal {
			t.Error("expected return to normal mode after :x")
		}
	})
}

func TestExCommand_ForceQuit(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run(":q! force quits", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		vs.HandleKey("q", content, 0, 0, height)
		vs.HandleKey("!", content, 0, 0, height)
		r := vs.HandleKey("enter", content, 0, 0, height)
		if !r.ExForceQuit {
			t.Error("expected ExForceQuit flag")
		}
	})
}

func TestExCommand_OpenFile(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run(":e opens file", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		for _, c := range "e notes.md" {
			vs.HandleKey(string(c), content, 0, 0, height)
		}
		r := vs.HandleKey("enter", content, 0, 0, height)
		if r.ExOpenFile != "notes.md" {
			t.Errorf("expected ExOpenFile 'notes.md', got %q", r.ExOpenFile)
		}
	})

	t.Run(":e without filename shows error", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		vs.HandleKey("e", content, 0, 0, height)
		r := vs.HandleKey("enter", content, 0, 0, height)
		if r.StatusMsg != "no file name" {
			t.Errorf("expected 'no file name', got %q", r.StatusMsg)
		}
	})
}

func TestExCommand_Substitute(t *testing.T) {
	height := 24

	t.Run(":s/old/new/ on current line", func(t *testing.T) {
		content := []string{"foo bar foo", "baz foo baz"}
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		for _, c := range "s/foo/qux/" {
			vs.HandleKey(string(c), content, 0, 0, height)
		}
		r := vs.HandleKey("enter", content, 0, 0, height)
		if r.ExSubstitute == nil {
			t.Fatal("expected ExSubstitute to be non-nil")
		}
		if r.ExSubstitute.Count != 1 {
			t.Errorf("expected 1 substitution, got %d", r.ExSubstitute.Count)
		}
		if r.ExSubstitute.NewLines[0] != "qux bar foo" {
			t.Errorf("expected 'qux bar foo', got %q", r.ExSubstitute.NewLines[0])
		}
		// Line 1 should be unchanged
		if r.ExSubstitute.NewLines[1] != "baz foo baz" {
			t.Errorf("line 1 should be unchanged, got %q", r.ExSubstitute.NewLines[1])
		}
	})

	t.Run(":s/old/new/g replaces all on current line", func(t *testing.T) {
		content := []string{"foo bar foo", "baz foo baz"}
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		for _, c := range "s/foo/qux/g" {
			vs.HandleKey(string(c), content, 0, 0, height)
		}
		r := vs.HandleKey("enter", content, 0, 0, height)
		if r.ExSubstitute == nil {
			t.Fatal("expected ExSubstitute to be non-nil")
		}
		if r.ExSubstitute.Count != 2 {
			t.Errorf("expected 2 substitutions, got %d", r.ExSubstitute.Count)
		}
		if r.ExSubstitute.NewLines[0] != "qux bar qux" {
			t.Errorf("expected 'qux bar qux', got %q", r.ExSubstitute.NewLines[0])
		}
	})

	t.Run(":%s/old/new/g replaces all in file", func(t *testing.T) {
		content := []string{"foo bar foo", "baz foo baz"}
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		for _, c := range "%s/foo/qux/g" {
			vs.HandleKey(string(c), content, 0, 0, height)
		}
		r := vs.HandleKey("enter", content, 0, 0, height)
		if r.ExSubstitute == nil {
			t.Fatal("expected ExSubstitute to be non-nil")
		}
		if r.ExSubstitute.Count != 3 {
			t.Errorf("expected 3 substitutions, got %d", r.ExSubstitute.Count)
		}
		if r.ExSubstitute.NewLines[0] != "qux bar qux" {
			t.Errorf("expected 'qux bar qux', got %q", r.ExSubstitute.NewLines[0])
		}
		if r.ExSubstitute.NewLines[1] != "baz qux baz" {
			t.Errorf("expected 'baz qux baz', got %q", r.ExSubstitute.NewLines[1])
		}
	})

	t.Run(":s with no match shows error", func(t *testing.T) {
		content := []string{"hello world"}
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		for _, c := range "s/xyz/abc/" {
			vs.HandleKey(string(c), content, 0, 0, height)
		}
		r := vs.HandleKey("enter", content, 0, 0, height)
		if r.ExSubstitute != nil {
			t.Error("expected nil ExSubstitute when pattern not found")
		}
		if r.StatusMsg != "pattern not found: xyz" {
			t.Errorf("expected pattern not found message, got %q", r.StatusMsg)
		}
	})

	t.Run(":s with escaped delimiter", func(t *testing.T) {
		content := []string{"a/b/c"}
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		for _, c := range `s/a\/b/x` {
			vs.HandleKey(string(c), content, 0, 0, height)
		}
		vs.HandleKey("/", content, 0, 0, height)
		r := vs.HandleKey("enter", content, 0, 0, height)
		if r.ExSubstitute == nil {
			t.Fatal("expected ExSubstitute to be non-nil")
		}
		if r.ExSubstitute.NewLines[0] != "x/c" {
			t.Errorf("expected 'x/c', got %q", r.ExSubstitute.NewLines[0])
		}
	})
}

func TestExCommand_SetOptions(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run(":set number", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		for _, c := range "set number" {
			vs.HandleKey(string(c), content, 0, 0, height)
		}
		r := vs.HandleKey("enter", content, 0, 0, height)
		if r.ExSetOption != "number" {
			t.Errorf("expected ExSetOption 'number', got %q", r.ExSetOption)
		}
	})

	t.Run(":set nonumber", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		for _, c := range "set nonumber" {
			vs.HandleKey(string(c), content, 0, 0, height)
		}
		r := vs.HandleKey("enter", content, 0, 0, height)
		if r.ExSetOption != "nonumber" {
			t.Errorf("expected ExSetOption 'nonumber', got %q", r.ExSetOption)
		}
	})

	t.Run(":set nonu (abbreviation)", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		for _, c := range "set nonu" {
			vs.HandleKey(string(c), content, 0, 0, height)
		}
		r := vs.HandleKey("enter", content, 0, 0, height)
		if r.ExSetOption != "nonumber" {
			t.Errorf("expected ExSetOption 'nonumber', got %q", r.ExSetOption)
		}
	})

	t.Run(":set wrap", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		for _, c := range "set wrap" {
			vs.HandleKey(string(c), content, 0, 0, height)
		}
		r := vs.HandleKey("enter", content, 0, 0, height)
		if r.ExSetOption != "wrap" {
			t.Errorf("expected ExSetOption 'wrap', got %q", r.ExSetOption)
		}
	})

	t.Run(":set nowrap", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		for _, c := range "set nowrap" {
			vs.HandleKey(string(c), content, 0, 0, height)
		}
		r := vs.HandleKey("enter", content, 0, 0, height)
		if r.ExSetOption != "nowrap" {
			t.Errorf("expected ExSetOption 'nowrap', got %q", r.ExSetOption)
		}
	})
}

func TestExCommand_NoHighlight(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run(":noh clears search highlights", func(t *testing.T) {
		vs := enabledVim()
		// First do a search to set up highlights
		vs.searchActive = true
		vs.searchMatches = []SearchMatch{{Line: 0, StartCol: 0, EndCol: 5}}

		vs.HandleKey(":", content, 0, 0, height)
		for _, c := range "noh" {
			vs.HandleKey(string(c), content, 0, 0, height)
		}
		r := vs.HandleKey("enter", content, 0, 0, height)
		if !r.ExClearSearch {
			t.Error("expected ExClearSearch flag")
		}
		if vs.searchActive {
			t.Error("expected searchActive to be false after :noh")
		}
	})

	t.Run(":nohlsearch also works", func(t *testing.T) {
		vs := enabledVim()
		vs.searchActive = true

		vs.HandleKey(":", content, 0, 0, height)
		for _, c := range "nohlsearch" {
			vs.HandleKey(string(c), content, 0, 0, height)
		}
		r := vs.HandleKey("enter", content, 0, 0, height)
		if !r.ExClearSearch {
			t.Error("expected ExClearSearch flag")
		}
	})
}

func TestExCommand_GetCmdBuffer(t *testing.T) {
	content := sampleContent()
	height := 24

	t.Run("returns empty when not in command mode", func(t *testing.T) {
		vs := enabledVim()
		if vs.GetCmdBuffer() != "" {
			t.Error("expected empty buffer in normal mode")
		}
	})

	t.Run("returns buffer in command mode", func(t *testing.T) {
		vs := enabledVim()
		vs.HandleKey(":", content, 0, 0, height)
		vs.HandleKey("w", content, 0, 0, height)
		vs.HandleKey("q", content, 0, 0, height)
		if vs.GetCmdBuffer() != "wq" {
			t.Errorf("expected buffer 'wq', got %q", vs.GetCmdBuffer())
		}
	})
}

func TestSplitSubParts(t *testing.T) {
	t.Run("simple split", func(t *testing.T) {
		parts := splitSubParts("old/new/g", '/')
		if len(parts) != 3 {
			t.Fatalf("expected 3 parts, got %d", len(parts))
		}
		if parts[0] != "old" || parts[1] != "new" || parts[2] != "g" {
			t.Errorf("unexpected parts: %v", parts)
		}
	})

	t.Run("escaped delimiter", func(t *testing.T) {
		parts := splitSubParts(`a\/b/new/`, '/')
		if len(parts) != 3 {
			t.Fatalf("expected 3 parts, got %d", len(parts))
		}
		if parts[0] != "a/b" {
			t.Errorf("expected 'a/b', got %q", parts[0])
		}
	})

	t.Run("no trailing delimiter", func(t *testing.T) {
		parts := splitSubParts("old/new", '/')
		if len(parts) != 2 {
			t.Fatalf("expected 2 parts, got %d", len(parts))
		}
	})
}

// ---------------------------------------------------------------------------
// Helper functions (exported for vim.go)
// ---------------------------------------------------------------------------

func TestLineLength(t *testing.T) {
	content := []string{"hello", "ab", ""}

	tests := []struct {
		line int
		want int
	}{
		{0, 5},
		{1, 2},
		{2, 0},
		{-1, 0},
		{99, 0},
	}

	for _, tc := range tests {
		got := lineLength(content, tc.line)
		if got != tc.want {
			t.Errorf("lineLength(content, %d) = %d, want %d", tc.line, got, tc.want)
		}
	}
}

func TestFirstNonSpace(t *testing.T) {
	content := []string{
		"hello",
		"  indented",
		"\ttabbed",
		"   ",
		"",
	}

	tests := []struct {
		line int
		want int
	}{
		{0, 0},
		{1, 2},
		{2, 1},
		{3, 0}, // all spaces, returns 0
		{4, 0}, // empty line
	}

	for _, tc := range tests {
		got := firstNonSpace(content, tc.line)
		if got != tc.want {
			t.Errorf("firstNonSpace(content, %d) = %d, want %d", tc.line, got, tc.want)
		}
	}
}

func TestJoinRange(t *testing.T) {
	content := []string{"a", "b", "c", "d"}

	tests := []struct {
		start, end int
		want       string
	}{
		{0, 0, "a"},
		{0, 2, "a\nb\nc"},
		{1, 3, "b\nc\nd"},
		{3, 1, ""},     // start > end
		{-1, 2, "a\nb\nc"}, // negative start clamped to 0
	}

	for _, tc := range tests {
		got := joinRange(content, tc.start, tc.end)
		if got != tc.want {
			t.Errorf("joinRange(%d,%d) = %q, want %q", tc.start, tc.end, got, tc.want)
		}
	}
}

func TestParseNumber(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"0", 0},
		{"1", 1},
		{"42", 42},
		{"100", 100},
		{"", 0},
		{"abc", 0},
		{"12x", 0},
	}

	for _, tc := range tests {
		got := parseNumber(tc.input)
		if got != tc.want {
			t.Errorf("parseNumber(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestVimIntToStr(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{1000, "1000"},
	}

	for _, tc := range tests {
		got := vimIntToStr(tc.input)
		if got != tc.want {
			t.Errorf("vimIntToStr(%d) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
