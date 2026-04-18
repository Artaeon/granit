package tui

import "testing"

func TestNormaliseBlockType_FoldsAliases(t *testing.T) {
	cases := map[string]BlockType{
		"task":      BlockTypeTask,
		"TASK":      BlockTypeTask,
		"  task ":   BlockTypeTask,
		"todo":      BlockTypeTask,
		"deep-work": BlockTypeDeepWork,
		"deep_work": BlockTypeDeepWork,
		"deepwork":  BlockTypeDeepWork,
		"DEEP-WORK": BlockTypeDeepWork,
		"focus":     BlockTypeFocus,
		"break":     BlockTypeBreak,
		"lunch":     BlockTypeLunch,
		"meeting":   BlockTypeMeeting,
		"event":     BlockTypeEvent,
		"admin":     BlockTypeAdmin,
		"habit":     BlockTypeHabit,
		"review":    BlockTypeReview,
		"pomodoro":  BlockTypePomodoro,
	}
	for in, want := range cases {
		if got := NormaliseBlockType(in); got != want {
			t.Errorf("NormaliseBlockType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormaliseBlockType_UnknownPassesThrough(t *testing.T) {
	// Unknown kinds lowercase-through so downstream exact-match logic
	// still works without ad-hoc ToLower calls.
	if got := NormaliseBlockType("CustomKind"); got != BlockType("customkind") {
		t.Errorf("expected lowercased pass-through, got %q", got)
	}
	// Empty input preserves the BlockTypeUnknown sentinel.
	if got := NormaliseBlockType(""); got != BlockTypeUnknown {
		t.Errorf("expected BlockTypeUnknown (empty), got %q", got)
	}
}

func TestBlockType_IsTaskLike(t *testing.T) {
	taskLike := []BlockType{BlockTypeTask, BlockTypeDeepWork, BlockTypeAdmin, BlockTypeFocus}
	other := []BlockType{
		BlockTypeBreak, BlockTypeLunch, BlockTypeMeeting, BlockTypeEvent,
		BlockTypeHabit, BlockTypeReview, BlockTypePomodoro, BlockTypeUnknown,
		BlockType("unknown"),
	}
	for _, b := range taskLike {
		if !b.IsTaskLike() {
			t.Errorf("%q should be task-like", b)
		}
	}
	for _, b := range other {
		if b.IsTaskLike() {
			t.Errorf("%q should NOT be task-like", b)
		}
	}
}

func TestBlockType_IsTaskLike_IsCanonical(t *testing.T) {
	// Callers should normalise before calling IsTaskLike. Verify that a
	// raw alias doesn't accidentally match. Catches the bug where a
	// future dev would use `if BlockType("deep_work").IsTaskLike()` —
	// only the canonical "deep-work" is recognised.
	if BlockType("deep_work").IsTaskLike() {
		t.Error("IsTaskLike should require canonical BlockType; callers must Normalise first")
	}
	if !NormaliseBlockType("deep_work").IsTaskLike() {
		t.Error("NormaliseBlockType + IsTaskLike should fold the alias")
	}
}
