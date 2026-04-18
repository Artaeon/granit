package tui

import (
	"strings"
	"testing"
)

// ── cmdFuzzyMatch ──

func TestCmdFuzzyMatch_ExactMatch(t *testing.T) {
	if !cmdFuzzyMatch("daily note", "daily note") {
		t.Error("exact match should return true")
	}
}

func TestCmdFuzzyMatch_PrefixMatch(t *testing.T) {
	if !cmdFuzzyMatch("daily note", "daily") {
		t.Error("prefix match should return true")
	}
}

func TestCmdFuzzyMatch_SubstringMatch(t *testing.T) {
	// "note" characters appear in order inside "daily note"
	if !cmdFuzzyMatch("daily note", "note") {
		t.Error("substring match should return true")
	}
}

func TestCmdFuzzyMatch_FuzzyCharacters(t *testing.T) {
	// "dnt" — d(aily) n(o)t(e)
	if !cmdFuzzyMatch("daily note", "dnt") {
		t.Error("fuzzy character subsequence should return true")
	}
}

func TestCmdFuzzyMatch_CaseInsensitive(t *testing.T) {
	// cmdFuzzyMatch itself is case-sensitive; the caller lowercases both sides.
	// Verify the lowercase-before-call pattern works.
	if !cmdFuzzyMatch("daily note", "daily note") {
		t.Error("same-case match should return true")
	}
	// Upper vs lower should fail when not pre-lowered.
	if cmdFuzzyMatch("DAILY NOTE", "daily note") {
		t.Error("case-sensitive mismatch should return false without lowercasing")
	}
}

func TestCmdFuzzyMatch_NoMatch(t *testing.T) {
	if cmdFuzzyMatch("daily note", "xyz") {
		t.Error("non-matching pattern should return false")
	}
}

func TestCmdFuzzyMatch_EmptyQuery(t *testing.T) {
	if !cmdFuzzyMatch("daily note", "") {
		t.Error("empty pattern should match anything")
	}
	if !cmdFuzzyMatch("", "") {
		t.Error("empty pattern and empty string should match")
	}
}

func TestCmdFuzzyMatch_EmptyString(t *testing.T) {
	if cmdFuzzyMatch("", "abc") {
		t.Error("non-empty pattern against empty string should return false")
	}
}

func TestCmdFuzzyMatch_SpecialCharacters(t *testing.T) {
	if !cmdFuzzyMatch("ctrl+p", "ctrl+p") {
		t.Error("special characters should match exactly")
	}
	if !cmdFuzzyMatch("ctrl+p", "c+p") {
		t.Error("fuzzy match across special characters should work")
	}
	if !cmdFuzzyMatch("find & replace", "f&r") {
		t.Error("ampersand in fuzzy match should work")
	}
}

func TestCmdFuzzyMatch_PatternLongerThanStr(t *testing.T) {
	if cmdFuzzyMatch("ab", "abcde") {
		t.Error("pattern longer than string should return false")
	}
}

func TestCmdFuzzyMatch_SingleCharacter(t *testing.T) {
	if !cmdFuzzyMatch("daily note", "d") {
		t.Error("single character at start should match")
	}
	if !cmdFuzzyMatch("daily note", "e") {
		t.Error("single character at end should match")
	}
	if cmdFuzzyMatch("daily note", "z") {
		t.Error("non-existing single character should not match")
	}
}

// ── AllCommands completeness ──

func TestAllCommands_KeyCommandsExist(t *testing.T) {
	required := map[CommandAction]string{
		CmdDailyNote:     "CmdDailyNote",
		CmdWeeklyNote:    "CmdWeeklyNote",
		CmdNewNote:       "CmdNewNote",
		CmdSaveNote:      "CmdSaveNote",
		CmdToggleView:    "CmdToggleView",
		CmdSettings:      "CmdSettings",
		CmdOpenFile:      "CmdOpenFile",
		CmdPrevDailyNote: "CmdPrevDailyNote",
		CmdNextDailyNote: "CmdNextDailyNote",
	}

	found := make(map[CommandAction]bool)
	for _, cmd := range AllCommands {
		found[cmd.Action] = true
	}

	for action, name := range required {
		if !found[action] {
			t.Errorf("AllCommands is missing required command: %s", name)
		}
	}
}

func TestAllCommands_NonEmpty(t *testing.T) {
	if len(AllCommands) == 0 {
		t.Fatal("AllCommands should not be empty")
	}
}

func TestAllCommands_HasSubstantialCount(t *testing.T) {
	// The command list should have at least 50 commands based on the source.
	if len(AllCommands) < 50 {
		t.Errorf("expected at least 50 commands, got %d", len(AllCommands))
	}
}

// ── Command filtering — non-empty Label and Desc ──

func TestAllCommands_LabelsNotEmpty(t *testing.T) {
	for i, cmd := range AllCommands {
		if cmd.Label == "" {
			t.Errorf("AllCommands[%d] (action=%d) has empty Label", i, cmd.Action)
		}
	}
}

func TestAllCommands_DescsNotEmpty(t *testing.T) {
	for i, cmd := range AllCommands {
		if cmd.Desc == "" {
			t.Errorf("AllCommands[%d] (Label=%q) has empty Desc", i, cmd.Label)
		}
	}
}

func TestAllCommands_ActionsNotNone(t *testing.T) {
	for i, cmd := range AllCommands {
		if cmd.Action == CmdNone {
			t.Errorf("AllCommands[%d] (Label=%q) has CmdNone action", i, cmd.Label)
		}
	}
}

// ── Shortcut uniqueness ──

func TestAllCommands_ShortcutUniqueness(t *testing.T) {
	seen := make(map[string]string) // shortcut -> label
	for _, cmd := range AllCommands {
		if cmd.Shortcut == "" {
			continue // empty shortcuts are allowed
		}
		if prev, exists := seen[cmd.Shortcut]; exists {
			t.Errorf("duplicate shortcut %q: %q and %q", cmd.Shortcut, prev, cmd.Label)
		}
		seen[cmd.Shortcut] = cmd.Label
	}
}

// ── CommandPalette ──

func TestNewCommandPalette_InitialState(t *testing.T) {
	cp := NewCommandPalette()
	if cp.IsActive() {
		t.Error("new command palette should not be active")
	}
	if len(cp.filtered) != len(AllCommands) {
		t.Errorf("filtered should contain all commands initially, got %d, want %d",
			len(cp.filtered), len(AllCommands))
	}
	if cp.cursor != 0 {
		t.Errorf("cursor should be 0, got %d", cp.cursor)
	}
}

func TestCommandPalette_OpenClose(t *testing.T) {
	cp := NewCommandPalette()

	cp.Open()
	if !cp.IsActive() {
		t.Error("Open should activate palette")
	}
	if cp.query != "" {
		t.Error("Open should reset query")
	}
	if len(cp.filtered) != len(AllCommands) {
		t.Error("Open should reset filtered to all commands")
	}

	cp.Close()
	if cp.IsActive() {
		t.Error("Close should deactivate palette")
	}
}

func TestCommandPalette_SetSize(t *testing.T) {
	cp := NewCommandPalette()
	cp.SetSize(120, 40)
	if cp.width != 120 {
		t.Errorf("width should be 120, got %d", cp.width)
	}
	if cp.height != 40 {
		t.Errorf("height should be 40, got %d", cp.height)
	}
}

func TestCommandPalette_Result(t *testing.T) {
	cp := NewCommandPalette()
	cp.result = CmdDailyNote

	r := cp.Result()
	if r != CmdDailyNote {
		t.Errorf("Result() should return CmdDailyNote, got %d", r)
	}

	// Second call should return CmdNone (consumed)
	r = cp.Result()
	if r != CmdNone {
		t.Errorf("Result() should return CmdNone after consumption, got %d", r)
	}
}

func TestCommandPalette_FilterCommands(t *testing.T) {
	cp := NewCommandPalette()
	cp.Open()

	// Filter with a specific query
	cp.query = "daily"
	cp.filterCommands()

	if len(cp.filtered) == 0 {
		t.Fatal("filtering by 'daily' should return at least one command")
	}

	// All results should match "daily" in label or desc (fuzzy, case insensitive)
	for _, cmd := range cp.filtered {
		labelMatch := cmdFuzzyMatch(toLower(cmd.Label), "daily")
		descMatch := cmdFuzzyMatch(toLower(cmd.Desc), "daily")
		if !labelMatch && !descMatch {
			t.Errorf("filtered command %q (desc: %q) should match 'daily'", cmd.Label, cmd.Desc)
		}
	}
}

func TestCommandPalette_FilterCommands_EmptyQuery(t *testing.T) {
	cp := NewCommandPalette()
	cp.Open()

	cp.query = ""
	cp.filterCommands()

	if len(cp.filtered) != len(AllCommands) {
		t.Errorf("empty query should show all commands, got %d want %d",
			len(cp.filtered), len(AllCommands))
	}
}

func TestCommandPalette_FilterCommands_NoMatch(t *testing.T) {
	cp := NewCommandPalette()
	cp.Open()

	cp.query = "zzzzzzzzzzz"
	cp.filterCommands()

	if len(cp.filtered) != 0 {
		t.Errorf("nonsense query should return 0 results, got %d", len(cp.filtered))
	}
}

func TestCommandPalette_FilterCommands_CursorClamped(t *testing.T) {
	cp := NewCommandPalette()
	cp.Open()

	// Set cursor beyond filtered bounds
	cp.cursor = 999
	cp.query = "quit"
	cp.filterCommands()

	if cp.cursor >= len(cp.filtered) && len(cp.filtered) > 0 {
		t.Errorf("cursor should be clamped to filtered length-1, got %d (len=%d)",
			cp.cursor, len(cp.filtered))
	}
}

// helper used in test
func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		result[i] = c
	}
	return string(result)
}

// ── cmdFuzzyScore — ranking semantics ──

func TestCmdFuzzyScore_ExactBeatsEverything(t *testing.T) {
	exact := cmdFuzzyScore("daily note", "daily note")
	prefix := cmdFuzzyScore("daily note overlay", "daily note")
	if exact <= prefix {
		t.Errorf("exact (%d) should beat prefix (%d)", exact, prefix)
	}
}

func TestCmdFuzzyScore_PrefixBeatsMidStringFuzzy(t *testing.T) {
	prefix := cmdFuzzyScore("task manager", "ta")
	mid := cmdFuzzyScore("auto-link suggester", "ta") // 'a' then 't' as subseq
	if prefix <= mid {
		t.Errorf("prefix (%d) should beat mid-string fuzzy (%d)", prefix, mid)
	}
}

func TestCmdFuzzyScore_WordBoundaryBoost(t *testing.T) {
	// "tm" matches both, but "task manager" has 'm' at a word boundary.
	wordBoundary := cmdFuzzyScore("task manager", "tm")
	midstring := cmdFuzzyScore("automaticshrub", "tm") // 't' and 'm' inside one word
	if wordBoundary <= midstring {
		t.Errorf("word-boundary match (%d) should beat mid-word fuzzy (%d)", wordBoundary, midstring)
	}
}

func TestCmdFuzzyScore_NoMatchReturnsZero(t *testing.T) {
	if got := cmdFuzzyScore("daily note", "xyz"); got != 0 {
		t.Errorf("expected 0 for non-match, got %d", got)
	}
}

// ── filterCommands — ranking integration ──

func TestFilterCommands_RanksLabelMatchesAboveDescMatches(t *testing.T) {
	// "task" should put "Task Manager" (label hit) above any command whose
	// description happens to mention task.
	cp := NewCommandPalette()
	cp.query = "task"
	cp.filterCommands()
	if len(cp.filtered) == 0 {
		t.Fatal("expected at least one match")
	}
	first := cp.filtered[0]
	if !strings.Contains(strings.ToLower(first.Label), "task") {
		t.Errorf("first result label should contain 'task', got %q", first.Label)
	}
}
