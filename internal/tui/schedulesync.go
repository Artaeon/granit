package tui

// Unified schedule-persistence layer.
//
// A "scheduled task" has two surface representations that must stay in sync:
//
//  1. The ⏰ HH:MM-HH:MM marker on the task line in its source note.
//     Read by TaskManager (via Task.ScheduledTime) to drive the Plan view
//     time-block grouping.
//
//  2. A block in Planner/{date}.md (format: - HH:MM-HH:MM | text | type).
//     Read by Calendar (via loadPlannerBlocks) and DailyPlanner to render
//     the day's hour grid.
//
// Historically, each caller updated only one representation, so a task
// scheduled via TaskManager was invisible to Calendar and vice-versa.
// SetTaskSchedule / ClearTaskSchedule in this file are the single entry
// points that keep both surfaces consistent.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ScheduleRef locates the source task line for precise upsert/remove. When
// NotePath+LineNum are both set, they are authoritative. If only Text is
// set, matching falls back to exact-text search (fragile — identical tasks
// collide).
type ScheduleRef struct {
	NotePath string // vault-relative path, e.g. "projects/work.md"
	LineNum  int    // 1-indexed line number
	Text     string // task text without the ⏰ marker
}

func (r ScheduleRef) hasLocation() bool {
	return r.NotePath != "" && r.LineNum > 0
}

// SetTaskSchedule assigns a [start,end] time block to a task on the given
// date. It writes the ⏰ marker on the source line AND upserts a matching
// block in Planner/{date}.md (blockType tags the block, e.g. "task",
// "focus", "meeting").
//
// On partial failure it returns the first error but still attempts both
// writes — a half-updated state is better than silent divergence.
func SetTaskSchedule(vaultRoot, date string, ref ScheduleRef, start, end, blockType string) error {
	if vaultRoot == "" {
		return fmt.Errorf("schedule: empty vault root")
	}
	if !validTimeRange(start, end) {
		return fmt.Errorf("schedule: invalid time range %q-%q", start, end)
	}
	if date == "" {
		return fmt.Errorf("schedule: empty date")
	}

	var firstErr error
	if err := writeTaskScheduleMarker(vaultRoot, ref, start, end); err != nil {
		firstErr = err
	}
	block := PlannerBlock{
		Date:      date,
		StartTime: start,
		EndTime:   end,
		Text:      ref.Text,
		BlockType: blockType,
		SourceRef: ref,
	}
	if err := UpsertPlannerBlock(vaultRoot, date, ref, block); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// ClearTaskSchedule removes the ⏰ marker from the source line AND the
// matching block from Planner/{date}.md. An empty date skips the planner
// side (useful when the task has no known date).
func ClearTaskSchedule(vaultRoot, date string, ref ScheduleRef) error {
	if vaultRoot == "" {
		return fmt.Errorf("schedule: empty vault root")
	}
	var firstErr error
	if err := clearTaskScheduleMarker(vaultRoot, ref); err != nil {
		firstErr = err
	}
	if date != "" {
		if err := RemovePlannerBlock(vaultRoot, date, ref); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ---------------------------------------------------------------------------
// Time-range validation
// ---------------------------------------------------------------------------

var hhmmRe = regexp.MustCompile(`^\d{2}:\d{2}$`)

func validTimeRange(start, end string) bool {
	if !hhmmRe.MatchString(start) || !hhmmRe.MatchString(end) {
		return false
	}
	s := slotToMinutes(start)
	e := slotToMinutes(end)
	return s >= 0 && e > s && e <= 24*60
}

// ---------------------------------------------------------------------------
// Source-file ⏰ marker — write / clear
// ---------------------------------------------------------------------------

// schedMarkerRe matches the ⏰ marker plus any leading whitespace so it can
// be cleanly stripped from a line.
var schedMarkerRe = regexp.MustCompile(`\s*⏰\s*\d{2}:\d{2}-\d{2}:\d{2}`)

// writeTaskScheduleMarker replaces (or adds) the ⏰ marker on a task line.
// Prefers ref.NotePath+LineNum (fast, precise); falls back to vault-wide
// text search when the location is unknown.
func writeTaskScheduleMarker(vaultRoot string, ref ScheduleRef, start, end string) error {
	marker := " ⏰ " + start + "-" + end
	set := func(line string) string {
		cleaned := schedMarkerRe.ReplaceAllString(line, "")
		return strings.TrimRight(cleaned, " ") + marker
	}

	if ref.hasLocation() {
		return editLineAt(vaultRoot, ref.NotePath, ref.LineNum, set)
	}
	if ref.Text == "" {
		return fmt.Errorf("schedule: ref has neither location nor text")
	}
	updateTaskScheduleInFile(vaultRoot, ref.Text, start, end)
	return nil
}

// clearTaskScheduleMarker strips the ⏰ marker from a task line.
func clearTaskScheduleMarker(vaultRoot string, ref ScheduleRef) error {
	clr := func(line string) string {
		return schedMarkerRe.ReplaceAllString(line, "")
	}
	if ref.hasLocation() {
		return editLineAt(vaultRoot, ref.NotePath, ref.LineNum, clr)
	}
	if ref.Text == "" {
		return fmt.Errorf("schedule: ref has neither location nor text")
	}
	needle := strings.TrimSpace(schedMarkerRe.ReplaceAllString(ref.Text, ""))
	return walkVaultMarkdown(vaultRoot, func(path string) bool {
		data, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "- [") {
				continue
			}
			idx := strings.Index(trimmed, "] ")
			if idx < 0 {
				continue
			}
			lineTask := strings.TrimSpace(schedMarkerRe.ReplaceAllString(trimmed[idx+2:], ""))
			if lineTask == needle {
				lines[i] = clr(line)
				_ = atomicWriteNote(path, strings.Join(lines, "\n"))
				return true
			}
		}
		return false
	})
}

// editLineAt applies transform to lineNum (1-indexed) of path and writes
// atomically. Returns nil if the file or line does not exist — a stale
// ref shouldn't abort a larger operation.
func editLineAt(vaultRoot, notePath string, lineNum int, transform func(string) string) error {
	full := filepath.Join(vaultRoot, notePath)
	data, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lines := strings.Split(string(data), "\n")
	if lineNum < 1 || lineNum > len(lines) {
		return nil
	}
	newLine := transform(lines[lineNum-1])
	if newLine == lines[lineNum-1] {
		return nil
	}
	lines[lineNum-1] = newLine
	return atomicWriteNote(full, strings.Join(lines, "\n"))
}

// walkVaultMarkdown walks .md files under vaultRoot (skipping hidden dirs)
// and calls visit for each. If visit returns true, the walk stops.
func walkVaultMarkdown(vaultRoot string, visit func(path string) bool) error {
	return filepath.Walk(vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") && path != vaultRoot {
			return filepath.SkipDir
		}
		if info.IsDir() || !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			return nil
		}
		if visit(path) {
			return filepath.SkipAll
		}
		return nil
	})
}

// ---------------------------------------------------------------------------
// Planner block — upsert / remove (text-matched; precise ref matching
// arrives with PlannerBlock.SourceRef in a follow-up change).
// ---------------------------------------------------------------------------

// blockMatchesRef returns true if a parsed planner block refers to the
// same logical task as ref. Precise match (NotePath+LineNum) wins; we fall
// back to normalised text when the block has no recorded source ref.
func blockMatchesRef(b PlannerBlock, ref ScheduleRef) bool {
	if ref.hasLocation() && b.SourceRef.hasLocation() {
		return b.SourceRef.NotePath == ref.NotePath && b.SourceRef.LineNum == ref.LineNum
	}
	if ref.Text == "" {
		return false
	}
	return normalizeBlockText(b.Text) == normalizeBlockText(ref.Text)
}

func normalizeBlockText(s string) string {
	return strings.TrimSpace(schedMarkerRe.ReplaceAllString(s, ""))
}

// UpsertPlannerBlock writes block into Planner/{date}.md, replacing any
// existing block that matches ref. Blocks are kept sorted by start time.
// Callers that already own the source-file write (e.g. TaskManager's
// undo-aware writeLineChange) can use this to update just the planner side.
func UpsertPlannerBlock(vaultRoot, date string, ref ScheduleRef, block PlannerBlock) error {
	blocks := readPlannerScheduleBlocks(vaultRoot, date)
	replaced := false
	for i, b := range blocks {
		if blockMatchesRef(b, ref) {
			blocks[i] = block
			replaced = true
			break
		}
	}
	if !replaced {
		blocks = append(blocks, block)
	}
	sortBlocksByStart(blocks)
	return writePlannerScheduleBlocks(vaultRoot, date, blocks)
}

// AppendPlannerBlock adds block to Planner/{date}.md without attempting to
// match or replace an existing entry. Use this when two overlapping blocks
// are meaningful — e.g. a "task" block (planned) and a "pomodoro" block
// (actual focus session) on the same time range. Blocks are kept sorted
// by start time.
func AppendPlannerBlock(vaultRoot, date string, block PlannerBlock) error {
	blocks := readPlannerScheduleBlocks(vaultRoot, date)
	blocks = append(blocks, block)
	sortBlocksByStart(blocks)
	return writePlannerScheduleBlocks(vaultRoot, date, blocks)
}

// RemovePlannerBlock removes any block matching ref from Planner/{date}.md.
// Returns nil if nothing matched (absence is success).
func RemovePlannerBlock(vaultRoot, date string, ref ScheduleRef) error {
	blocks := readPlannerScheduleBlocks(vaultRoot, date)
	kept := blocks[:0]
	removed := false
	for _, b := range blocks {
		if !removed && blockMatchesRef(b, ref) {
			removed = true
			continue
		}
		kept = append(kept, b)
	}
	if !removed {
		return nil
	}
	return writePlannerScheduleBlocks(vaultRoot, date, kept)
}

// CurrentPlannerBlock returns the planner block containing nowMins (minutes
// from midnight) in Planner/{date}.md, or nil if no block is active. Ties
// are broken in file order (the first matching block wins). Callers like
// the pomodoro timer use this to answer "what should I be working on
// right now?" against the day's planned schedule.
func CurrentPlannerBlock(vaultRoot, date string, nowMins int) *PlannerBlock {
	for _, b := range readPlannerScheduleBlocks(vaultRoot, date) {
		startMin := slotToMinutes(b.StartTime)
		endMin := slotToMinutes(b.EndTime)
		if endMin <= startMin {
			continue
		}
		if nowMins >= startMin && nowMins < endMin {
			block := b
			return &block
		}
	}
	return nil
}

// isTaskSlot reports whether a daySlot.Type (or planner BlockType) describes
// a user task that should carry a ⏰ marker on its source line. Non-task
// kinds like "break", "meeting", "habit", "review" only exist on the
// planner side and have no source task to annotate.
func isTaskSlot(slotType string) bool {
	switch strings.ToLower(slotType) {
	case "task", "deep-work", "deep_work", "admin", "focus":
		return true
	}
	return false
}

// scheduleRefForSlotText resolves a slot's task text back to a source-task
// reference by matching against the parsed task list. Returns a text-only
// ref when nothing matches (caller should treat the slot as planner-only).
//
// Matching order:
//  1. Exact normalised-text equality.
//  2. Either-direction substring containment — the AI scheduler sometimes
//     trims or paraphrases task text. Among candidates, the longest
//     match wins so "Review" doesn't beat "Review PR description" when
//     both contain the needle. Result order is then stable (task file
//     order) and doesn't flip across re-scans.
func scheduleRefForSlotText(taskText string, tasks []Task) ScheduleRef {
	if taskText == "" {
		return ScheduleRef{}
	}
	needle := normalizeBlockText(taskText)
	for _, t := range tasks {
		if normalizeBlockText(t.Text) == needle {
			return ScheduleRef{NotePath: t.NotePath, LineNum: t.LineNum, Text: t.Text}
		}
	}
	best := -1
	bestLen := 0
	for i, t := range tasks {
		norm := normalizeBlockText(t.Text)
		if norm == "" {
			continue
		}
		if !(strings.Contains(norm, needle) || strings.Contains(needle, norm)) {
			continue
		}
		if len(norm) > bestLen {
			best = i
			bestLen = len(norm)
		}
	}
	if best >= 0 {
		t := tasks[best]
		return ScheduleRef{NotePath: t.NotePath, LineNum: t.LineNum, Text: t.Text}
	}
	return ScheduleRef{Text: taskText}
}

func sortBlocksByStart(blocks []PlannerBlock) {
	sort.SliceStable(blocks, func(i, j int) bool {
		return slotToMinutes(blocks[i].StartTime) < slotToMinutes(blocks[j].StartTime)
	})
}
