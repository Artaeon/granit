package tui

// Typed enum for planner block kinds.
//
// 121 string literals ("task", "focus", "break", "meeting", "lunch",
// "deep-work", "admin", "habit", "review", "pomodoro") are scattered
// across 27 files — the AI scheduler, calendar renderer, Plan My Day
// output, pomodoro session logger, standup generator, kanban board,
// and more. Typos like "medium" vs "med" or "deep-work" vs "deepwork"
// can't be caught by the compiler when every comparison is against a
// bare string literal.
//
// This file defines:
//
//   1. The canonical `BlockType` typed-string enum with a constant per
//      kind, plus a group predicate (IsTaskLike).
//   2. NormaliseBlockType(s) which folds the known aliases
//      ("deep-work" / "deep_work") to a single canonical form so
//      downstream comparisons can be literal.
//
// PlannerBlock.BlockType stays as `string` for now — changing it would
// cascade through ~60 read sites and risk breaking round-trips with
// existing planner files. New code should use BlockType at API
// boundaries; string-typed call sites coerce automatically.

import "strings"

// BlockType names the semantic category of a scheduled block.
type BlockType string

// Canonical block-type values. On-disk representation is these exact
// lowercase strings; any variant seen in AI or user input should be
// routed through NormaliseBlockType first.
const (
	BlockTypeUnknown  BlockType = ""
	BlockTypeTask     BlockType = "task"
	BlockTypeDeepWork BlockType = "deep-work"
	BlockTypeAdmin    BlockType = "admin"
	BlockTypeFocus    BlockType = "focus"
	BlockTypeBreak    BlockType = "break"
	BlockTypeLunch    BlockType = "lunch"
	BlockTypeMeeting  BlockType = "meeting"
	BlockTypeEvent    BlockType = "event"
	BlockTypeHabit    BlockType = "habit"
	BlockTypeReview   BlockType = "review"
	BlockTypePomodoro BlockType = "pomodoro"
)

// NormaliseBlockType folds case + known aliases to the canonical form.
// Unknown kinds pass through lowercased so exact-string-match logic can
// still compare without ad-hoc lower-casing.
func NormaliseBlockType(s string) BlockType {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "task", "todo":
		return BlockTypeTask
	case "deep-work", "deep_work", "deepwork":
		return BlockTypeDeepWork
	case "admin":
		return BlockTypeAdmin
	case "focus":
		return BlockTypeFocus
	case "break":
		return BlockTypeBreak
	case "lunch":
		return BlockTypeLunch
	case "meeting":
		return BlockTypeMeeting
	case "event":
		return BlockTypeEvent
	case "habit":
		return BlockTypeHabit
	case "review":
		return BlockTypeReview
	case "pomodoro":
		return BlockTypePomodoro
	}
	return BlockType(strings.ToLower(strings.TrimSpace(s)))
}

// IsTaskLike reports whether the block describes user work that should
// carry a ⏰ marker on its source task line. Non-task kinds (break,
// meeting, habit, review) only live on the planner side.
//
// Used by the schedule layer when routing a slot through
// SetTaskSchedule vs. UpsertPlannerBlock.
func (b BlockType) IsTaskLike() bool {
	switch b {
	case BlockTypeTask, BlockTypeDeepWork, BlockTypeAdmin, BlockTypeFocus:
		return true
	}
	return false
}
