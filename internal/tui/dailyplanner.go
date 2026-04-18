package tui

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Data types passed in from app.go
// ---------------------------------------------------------------------------

// PlannerTask represents a task from the vault with scheduling metadata.
type PlannerTask struct {
	Text     string
	Done     bool
	Priority int    // 0=none, 1=low, 2=medium, 3=high, 4=highest
	DueDate  string // YYYY-MM-DD
	Source   string // which file it came from
	NotePath string // relative path to source note (for bidirectional sync)
	LineNum  int    // 1-based line number in source note (matches Task.LineNum)
}

// PlannerEvent represents a calendar event for the planner.
type PlannerEvent struct {
	Title    string
	Time     string // "HH:MM" or empty
	Duration int    // minutes, 0 = 60 default
}

// PlannerHabit represents a recurring habit to track.
type PlannerHabit struct {
	Name   string
	Done   bool
	Streak int
}

// ---------------------------------------------------------------------------
// Internal types
// ---------------------------------------------------------------------------

// blockType distinguishes different kinds of scheduled entries.
type blockType int

const (
	blockEmpty blockType = iota
	blockTask
	blockEvent
	blockBreak
	blockFocus
)

// timeBlock is a single 30-minute slot in the schedule grid.
type timeBlock struct {
	Hour       int            // 0-23
	HalfHour   bool           // false=:00, true=:30
	TaskText   string         // assigned task/event text
	TaskType   blockType      // type of the block content
	Done       bool           // whether the block is completed
	Color      lipgloss.Color // color for rendering the block
	Priority   int            // priority for task blocks (0-4)
	SourcePath string         // relative path to source note (for bidirectional sync)
	SourceLine int            // 1-based line number in source note (matches Task.LineNum)
}

// plannerPanel tracks which sub-panel has focus.
type plannerPanel int

const (
	panelSchedule plannerPanel = iota
	panelUnscheduled
	panelHabits
)

// ---------------------------------------------------------------------------
// DailyPlanner
// ---------------------------------------------------------------------------

// TaskCompletion records a task toggle that should be synced back to its
// source file (e.g. Tasks.md).
//
// LineNum is 1-based to match Task.LineNum — producers pass that value
// directly, and consumers (app_update.go, app_helpers.go) must index with
// LineNum-1. Historically the field was documented 0-based, but every
// producer in the codebase has always passed 1-based values; the field
// comment is now aligned with reality.
type TaskCompletion struct {
	NotePath string
	LineNum  int // 1-based
	Text     string
	Done     bool
}

// DailyPlanner is a time-blocked daily schedule overlay that integrates with
// tasks, calendar events, and habits.
type DailyPlanner struct {
	active    bool
	width     int
	height    int
	vaultRoot string
	date      time.Time

	// Time grid (06:00 to 22:00 = 32 half-hour slots)
	blocks []timeBlock
	cursor int // which slot the cursor is on
	scroll int

	// Unscheduled tasks (not yet placed in a time slot)
	unscheduled   []PlannerTask
	unschedCursor int

	// Habits for today
	habits      []PlannerHabit
	habitCursor int

	// State
	focus    plannerPanel // 0=schedule, 1=unscheduled panel, 2=habits panel
	adding   bool
	addBuf   string
	addType  blockType // block type being added
	addSlots int       // number of 30-min slots for the block (1-6)

	// Moving
	moving   bool
	moveFrom int

	// Completed blocks count
	doneCount  int
	totalCount int

	// Result for starting a focus session
	focusTask      string
	focusDuration  int
	hasFocusResult bool

	// File save
	modified bool

	// Active goals for display
	activeGoals []Goal

	// Pending task completions to sync back to source files
	completedTasks []TaskCompletion
}

// ---------------------------------------------------------------------------
// Constructor / lifecycle
// ---------------------------------------------------------------------------

// NewDailyPlanner creates a DailyPlanner in its default inactive state.
func NewDailyPlanner() DailyPlanner {
	return DailyPlanner{}
}

// IsActive reports whether the daily planner overlay is visible.
func (dp DailyPlanner) IsActive() bool {
	return dp.active
}

// Open activates the daily planner, populating it with the supplied tasks,
// events, and habits.  If a saved planner file exists for the current date it
// is loaded first; events and tasks are merged on top.
func (dp *DailyPlanner) Open(vaultRoot string, tasks []PlannerTask, events []PlannerEvent, habits []PlannerHabit) {
	dp.active = true
	dp.vaultRoot = vaultRoot
	dp.date = time.Now()
	dp.cursor = 0
	dp.scroll = 0
	dp.focus = panelSchedule
	dp.adding = false
	dp.addBuf = ""
	dp.addType = blockTask
	dp.moving = false
	dp.moveFrom = -1
	dp.unschedCursor = 0
	dp.habitCursor = 0
	dp.hasFocusResult = false
	dp.modified = false

	dp.initBlocks()

	// Try to load saved schedule first
	loaded := dp.loadFromFile()

	// Set up habits
	dp.habits = make([]PlannerHabit, len(habits))
	copy(dp.habits, habits)

	// Place events that have specific times (only if not loaded from file,
	// or merge missing events)
	if !loaded {
		dp.placeEvents(events)
	}

	// Populate unscheduled tasks: tasks due today or overdue, not yet scheduled
	dp.unscheduled = nil
	todayStr := dp.date.Format("2006-01-02")
	for _, t := range tasks {
		if t.Done {
			continue
		}
		isDueToday := t.DueDate == todayStr
		isOverdue := t.DueDate != "" && t.DueDate < todayStr
		if (isDueToday || isOverdue) && !dp.isTextScheduled(t.Text) {
			dp.unscheduled = append(dp.unscheduled, t)
		}
	}

	// Sort unscheduled by priority descending
	sort.Slice(dp.unscheduled, func(i, j int) bool {
		return dp.unscheduled[i].Priority > dp.unscheduled[j].Priority
	})

	dp.recountProgress()
}

// SetSize updates the overlay dimensions.
func (dp *DailyPlanner) SetSize(w, h int) {
	dp.width = w
	dp.height = h
}

// GetFocusResult returns (task, duration, ok) for triggering a focus session.
// Consumed-once pattern: the result is cleared after retrieval.
func (dp *DailyPlanner) GetFocusResult() (task string, duration int, ok bool) {
	if !dp.hasFocusResult {
		return "", 0, false
	}
	t := dp.focusTask
	d := dp.focusDuration
	dp.hasFocusResult = false
	dp.focusTask = ""
	dp.focusDuration = 0
	return t, d, true
}

// GetCompletedTasks returns and clears the list of task completions that
// should be synced back to their source files. Consumed-once pattern.
func (dp *DailyPlanner) GetCompletedTasks() []TaskCompletion {
	if len(dp.completedTasks) == 0 {
		return nil
	}
	result := dp.completedTasks
	dp.completedTasks = nil
	return result
}

// HasPendingSync reports whether there are task completions waiting to be
// synced back to source files.
func (dp DailyPlanner) HasPendingSync() bool {
	return len(dp.completedTasks) > 0
}

// ---------------------------------------------------------------------------
// Initialization helpers
// ---------------------------------------------------------------------------

// initBlocks creates the 32 half-hour slots from 06:00 to 21:30.
func (dp *DailyPlanner) initBlocks() {
	dp.blocks = make([]timeBlock, 0, 32)
	for h := 6; h <= 21; h++ {
		dp.blocks = append(dp.blocks, timeBlock{Hour: h, HalfHour: false})
		dp.blocks = append(dp.blocks, timeBlock{Hour: h, HalfHour: true})
	}
}

// placeEvents inserts calendar events into their matching time slots.
func (dp *DailyPlanner) placeEvents(events []PlannerEvent) {
	for _, ev := range events {
		if ev.Time == "" {
			continue
		}
		idx := dp.timeToSlotIndex(ev.Time)
		if idx < 0 {
			continue
		}
		dur := ev.Duration
		if dur <= 0 {
			dur = 60
		}
		slots := dur / 30
		if slots < 1 {
			slots = 1
		}
		for s := 0; s < slots && idx+s < len(dp.blocks); s++ {
			dp.blocks[idx+s].TaskText = ev.Title
			dp.blocks[idx+s].TaskType = blockEvent
			dp.blocks[idx+s].Color = lavender
		}
	}
}

// isTextScheduled checks whether any block already contains the given text.
func (dp *DailyPlanner) isTextScheduled(text string) bool {
	for _, b := range dp.blocks {
		if b.TaskType != blockEmpty && b.TaskText == text {
			return true
		}
	}
	return false
}

// recountProgress tallies done vs total non-empty blocks.
func (dp *DailyPlanner) recountProgress() {
	dp.doneCount = 0
	dp.totalCount = 0
	seen := make(map[string]bool)
	for _, b := range dp.blocks {
		if b.TaskType == blockEmpty {
			continue
		}
		key := fmt.Sprintf("%d-%v-%s", b.Hour, b.HalfHour, b.TaskText)
		if seen[key] {
			continue
		}
		seen[key] = true
		dp.totalCount++
		if b.Done {
			dp.doneCount++
		}
	}
}

// timeToSlotIndex converts "HH:MM" to a slot index, or -1.
func (dp *DailyPlanner) timeToSlotIndex(t string) int {
	var h, m int
	if _, err := fmt.Sscanf(t, "%d:%d", &h, &m); err != nil {
		return -1
	}
	if h < 6 || h > 21 {
		return -1
	}
	idx := (h - 6) * 2
	if m >= 30 {
		idx++
	}
	if idx < 0 || idx >= len(dp.blocks) {
		return -1
	}
	return idx
}

// slotTime returns the "HH:MM" string for a given slot index.
func (dp *DailyPlanner) slotTime(idx int) string {
	if idx < 0 || idx >= len(dp.blocks) {
		return "??:??"
	}
	b := dp.blocks[idx]
	m := 0
	if b.HalfHour {
		m = 30
	}
	return fmt.Sprintf("%02d:%02d", b.Hour, m)
}

// visibleSlots returns how many slots fit on screen.
func (dp DailyPlanner) visibleSlots() int {
	h := dp.height - 14
	if h < 8 {
		h = 8
	}
	return h
}

// blockColor returns the appropriate color for a block based on its type and
// priority.
func blockColor(bt blockType, priority int) lipgloss.Color {
	switch bt {
	case blockEvent:
		return lavender
	case blockBreak:
		return overlay0
	case blockFocus:
		return green
	case blockTask:
		return taskPriorityColor(priority)
	default:
		return text
	}
}

// taskPriorityColor maps task priority to a lipgloss color.
func taskPriorityColor(priority int) lipgloss.Color {
	switch priority {
	case 4:
		return red
	case 3:
		return peach
	case 2:
		return yellow
	case 1:
		return blue
	default:
		return text
	}
}

// blockTypeTag returns the short indicator label for a block type.
func blockTypeTag(bt blockType) string {
	switch bt {
	case blockTask:
		return "[T]"
	case blockEvent:
		return "[E]"
	case blockBreak:
		return "[B]"
	case blockFocus:
		return "[F]"
	default:
		return ""
	}
}

// blockTypeName returns the human-readable name for a block type.
func blockTypeName(bt blockType) string {
	switch bt {
	case blockTask:
		return "task"
	case blockEvent:
		return "event"
	case blockBreak:
		return "break"
	case blockFocus:
		return "focus"
	default:
		return ""
	}
}

// parseBlockType converts a string name to a blockType.
func parseBlockType(s string) blockType {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "task":
		return blockTask
	case "event":
		return blockEvent
	case "break":
		return blockBreak
	case "focus":
		return blockFocus
	default:
		return blockTask
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles key messages while the daily planner is active.
func (dp DailyPlanner) Update(msg tea.Msg) (DailyPlanner, tea.Cmd) {
	if !dp.active {
		return dp, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// ---- Add-block input mode ----
		if dp.adding {
			return dp.updateAdding(msg)
		}

		// ---- Normal mode, dispatch by panel ----
		switch dp.focus {
		case panelSchedule:
			return dp.updateSchedule(msg)
		case panelUnscheduled:
			return dp.updateUnscheduled(msg)
		case panelHabits:
			return dp.updateHabits(msg)
		}
	}

	return dp, nil
}

// updateAdding handles keystrokes while the user is typing a new block name.
func (dp DailyPlanner) updateAdding(msg tea.KeyMsg) (DailyPlanner, tea.Cmd) {
	switch msg.String() {
	case "esc":
		dp.adding = false
		dp.addBuf = ""
	case "enter":
		if strings.TrimSpace(dp.addBuf) != "" {
			dp.placeBlock(dp.cursor, strings.TrimSpace(dp.addBuf), dp.addType, dp.addSlots)
			dp.modified = true
			dp.recountProgress()
		}
		dp.adding = false
		dp.addBuf = ""
	case "backspace":
		if len(dp.addBuf) > 0 {
			dp.addBuf = TrimLastRune(dp.addBuf)
		}
	case "1":
		dp.addType = blockTask
	case "2":
		dp.addType = blockEvent
	case "3":
		dp.addType = blockBreak
	case "4":
		dp.addType = blockFocus
	case "-":
		if dp.addSlots > 1 {
			dp.addSlots--
		}
	case "=", "+":
		if dp.addSlots < 6 {
			dp.addSlots++
		}
	default:
		if len(msg.String()) == 1 || msg.String() == " " {
			dp.addBuf += msg.String()
		}
	}
	return dp, nil
}

// updateSchedule handles keystrokes on the schedule panel.
func (dp DailyPlanner) updateSchedule(msg tea.KeyMsg) (DailyPlanner, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if dp.moving {
			dp.moving = false
			dp.moveFrom = -1
			return dp, nil
		}
		dp.saveIfModified()
		dp.active = false
		return dp, nil

	case "tab":
		dp.focus = panelUnscheduled
	case "shift+tab":
		dp.focus = panelHabits

	case "j", "down":
		if dp.cursor < len(dp.blocks)-1 {
			dp.cursor++
			dp.adjustScroll()
		}
	case "k", "up":
		if dp.cursor > 0 {
			dp.cursor--
			dp.adjustScroll()
		}
	case "g":
		dp.cursor = 0
		dp.scroll = 0
	case "G":
		if len(dp.blocks) > 0 {
			dp.cursor = len(dp.blocks) - 1
		}
		dp.adjustScroll()

	case "a":
		dp.adding = true
		dp.addBuf = ""
		dp.addType = blockTask
		dp.addSlots = 2 // default 1 hour

	case "d":
		dp.deleteBlock(dp.cursor)
		dp.modified = true
		dp.recountProgress()

	case " ":
		dp.toggleDone(dp.cursor)
		dp.modified = true
		dp.recountProgress()

	case "f":
		if dp.cursor >= 0 && dp.cursor < len(dp.blocks) {
			b := dp.blocks[dp.cursor]
			if b.TaskType != blockEmpty {
				dp.focusTask = b.TaskText
				dp.focusDuration = dp.spanDuration(dp.cursor)
				dp.hasFocusResult = true
				dp.saveIfModified()
				dp.active = false
				return dp, nil
			}
		}

	case "m":
		if dp.moving {
			// Place the block at the new location
			dp.moveBlock(dp.moveFrom, dp.cursor)
			dp.moving = false
			dp.moveFrom = -1
			dp.modified = true
			dp.recountProgress()
		} else {
			if dp.cursor >= 0 && dp.cursor < len(dp.blocks) && dp.blocks[dp.cursor].TaskType != blockEmpty {
				dp.moving = true
				dp.moveFrom = dp.cursor
			}
		}

	case "enter":
		if dp.moving {
			dp.moveBlock(dp.moveFrom, dp.cursor)
			dp.moving = false
			dp.moveFrom = -1
			dp.modified = true
			dp.recountProgress()
		} else {
			// Try to assign first unscheduled task to this slot
			if len(dp.unscheduled) > 0 && dp.cursor >= 0 && dp.cursor < len(dp.blocks) && dp.blocks[dp.cursor].TaskType == blockEmpty {
				task := dp.unscheduled[dp.unschedCursor]
				dp.placeBlock(dp.cursor, task.Text, blockTask, 2)
				dp.blocks[dp.cursor].Priority = task.Priority
				dp.blocks[dp.cursor].SourcePath = task.NotePath
				dp.blocks[dp.cursor].SourceLine = task.LineNum
				if dp.cursor+1 < len(dp.blocks) && dp.blocks[dp.cursor+1].TaskText == task.Text {
					dp.blocks[dp.cursor+1].Priority = task.Priority
					dp.blocks[dp.cursor+1].SourcePath = task.NotePath
					dp.blocks[dp.cursor+1].SourceLine = task.LineNum
				}
				dp.unscheduled = append(dp.unscheduled[:dp.unschedCursor], dp.unscheduled[dp.unschedCursor+1:]...)
				if dp.unschedCursor >= len(dp.unscheduled) && dp.unschedCursor > 0 {
					dp.unschedCursor--
				}
				dp.modified = true
				dp.recountProgress()
			}
		}

	case "[":
		dp.saveIfModified()
		dp.date = dp.date.AddDate(0, 0, -1)
		dp.reloadDay()
	case "]":
		dp.saveIfModified()
		dp.date = dp.date.AddDate(0, 0, 1)
		dp.reloadDay()

	case "s":
		dp.saveToFile()
		dp.modified = false

	case "c":
		summary := dp.buildPlanSummary()
		_ = ClipboardCopy(summary)

	case "S":
		dp.exportPlanAsMarkdown()
	}

	return dp, nil
}

// buildPlanSummary formats the daily plan as shareable text for clipboard.
func (dp *DailyPlanner) buildPlanSummary() string {
	var b strings.Builder
	dateStr := dp.date.Format("Monday, January 2, 2006")
	b.WriteString(fmt.Sprintf("Daily Plan — %s\n", dateStr))
	b.WriteString(strings.Repeat("─", 40) + "\n\n")

	// Scheduled blocks (merge consecutive same-task blocks)
	b.WriteString("Schedule:\n")
	hasSchedule := false
	i := 0
	for i < len(dp.blocks) {
		block := dp.blocks[i]
		if block.TaskType == blockEmpty {
			i++
			continue
		}
		hasSchedule = true
		startTime := fmt.Sprintf("%02d:%02d", block.Hour, map[bool]int{false: 0, true: 30}[block.HalfHour])
		// Find span end
		endIdx := i + 1
		for endIdx < len(dp.blocks) && dp.blocks[endIdx].TaskText == block.TaskText && dp.blocks[endIdx].TaskType == block.TaskType {
			endIdx++
		}
		endBlock := dp.blocks[endIdx-1]
		endMin := 0
		if endBlock.HalfHour {
			endMin = 30
		}
		endHour := endBlock.Hour
		endMin += 30
		if endMin >= 60 {
			endHour++
			endMin = 0
		}
		endTime := fmt.Sprintf("%02d:%02d", endHour, endMin)

		typeLabel := ""
		switch block.TaskType {
		case blockEvent:
			typeLabel = " [event]"
		case blockBreak:
			typeLabel = " [break]"
		case blockFocus:
			typeLabel = " [focus]"
		}
		check := "  "
		if block.Done {
			check = "✓ "
		}
		b.WriteString(fmt.Sprintf("  %s%s–%s  %s%s\n", check, startTime, endTime, block.TaskText, typeLabel))
		i = endIdx
	}
	if !hasSchedule {
		b.WriteString("  (no blocks scheduled)\n")
	}

	// Unscheduled tasks
	if len(dp.unscheduled) > 0 {
		b.WriteString("\nTasks:\n")
		for _, t := range dp.unscheduled {
			check := "[ ] "
			if t.Done {
				check = "[x] "
			}
			prio := ""
			switch t.Priority {
			case 3:
				prio = " (!)"
			case 4:
				prio = " (!!)"
			}
			b.WriteString(fmt.Sprintf("  %s%s%s\n", check, t.Text, prio))
		}
	}

	// Habits
	if len(dp.habits) > 0 {
		b.WriteString("\nHabits:\n")
		for _, h := range dp.habits {
			check := "[ ] "
			if h.Done {
				check = "[x] "
			}
			streak := ""
			if h.Streak > 0 {
				streak = fmt.Sprintf(" (%d day streak)", h.Streak)
			}
			b.WriteString(fmt.Sprintf("  %s%s%s\n", check, h.Name, streak))
		}
	}

	// Active goals
	if len(dp.activeGoals) > 0 {
		b.WriteString("\nGoals:\n")
		for _, g := range dp.activeGoals {
			prog := ""
			if len(g.Milestones) > 0 {
				prog = fmt.Sprintf(" (%d%%)", g.Progress())
			}
			b.WriteString(fmt.Sprintf("  %s%s\n", g.Title, prog))
		}
	}

	// Progress
	if dp.totalCount > 0 {
		pct := dp.doneCount * 100 / dp.totalCount
		b.WriteString(fmt.Sprintf("\nProgress: %d/%d blocks done (%d%%)\n", dp.doneCount, dp.totalCount, pct))
	}

	return b.String()
}

// exportPlanAsMarkdown saves the daily plan summary to a markdown file.
func (dp *DailyPlanner) exportPlanAsMarkdown() {
	if dp.vaultRoot == "" {
		return
	}
	dir := filepath.Join(dp.vaultRoot, "Plans")
	_ = os.MkdirAll(dir, 0o755)
	filename := "plan-" + dp.date.Format("2006-01-02") + ".md"
	fp := filepath.Join(dir, filename)

	summary := dp.buildPlanSummary()
	content := "---\ntitle: " + dp.date.Format("Monday, January 2, 2006") + "\ndate: " + dp.date.Format("2006-01-02") + "\ntype: plan\ntags: [plan]\n---\n\n" + summary
	_ = atomicWriteNote(fp, content)
}

// updateUnscheduled handles keystrokes on the unscheduled tasks panel.
func (dp DailyPlanner) updateUnscheduled(msg tea.KeyMsg) (DailyPlanner, tea.Cmd) {
	switch msg.String() {
	case "esc":
		dp.saveIfModified()
		dp.active = false
		return dp, nil

	case "tab":
		dp.focus = panelHabits
	case "shift+tab":
		dp.focus = panelSchedule

	case "j", "down":
		if dp.unschedCursor < len(dp.unscheduled)-1 {
			dp.unschedCursor++
		}
	case "k", "up":
		if dp.unschedCursor > 0 {
			dp.unschedCursor--
		}

	case "enter":
		// Assign selected unscheduled task to the current schedule cursor slot
		if len(dp.unscheduled) > 0 && dp.unschedCursor < len(dp.unscheduled) {
			if dp.cursor >= 0 && dp.cursor < len(dp.blocks) && dp.blocks[dp.cursor].TaskType == blockEmpty {
				task := dp.unscheduled[dp.unschedCursor]
				dp.placeBlock(dp.cursor, task.Text, blockTask, 2)
				dp.blocks[dp.cursor].Priority = task.Priority
				dp.blocks[dp.cursor].SourcePath = task.NotePath
				dp.blocks[dp.cursor].SourceLine = task.LineNum
				if dp.cursor+1 < len(dp.blocks) && dp.blocks[dp.cursor+1].TaskText == task.Text {
					dp.blocks[dp.cursor+1].Priority = task.Priority
					dp.blocks[dp.cursor+1].SourcePath = task.NotePath
					dp.blocks[dp.cursor+1].SourceLine = task.LineNum
				}
				dp.unscheduled = append(dp.unscheduled[:dp.unschedCursor], dp.unscheduled[dp.unschedCursor+1:]...)
				if dp.unschedCursor >= len(dp.unscheduled) && dp.unschedCursor > 0 {
					dp.unschedCursor--
				}
				dp.modified = true
				dp.recountProgress()
				dp.focus = panelSchedule
			}
		}

	case "[":
		dp.saveIfModified()
		dp.date = dp.date.AddDate(0, 0, -1)
		dp.reloadDay()
	case "]":
		dp.saveIfModified()
		dp.date = dp.date.AddDate(0, 0, 1)
		dp.reloadDay()

	case "s":
		dp.saveToFile()
		dp.modified = false
	}

	return dp, nil
}

// updateHabits handles keystrokes on the habits panel.
func (dp DailyPlanner) updateHabits(msg tea.KeyMsg) (DailyPlanner, tea.Cmd) {
	switch msg.String() {
	case "esc":
		dp.saveIfModified()
		dp.active = false
		return dp, nil

	case "tab":
		dp.focus = panelSchedule
	case "shift+tab":
		dp.focus = panelUnscheduled

	case "j", "down":
		if dp.habitCursor < len(dp.habits)-1 {
			dp.habitCursor++
		}
	case "k", "up":
		if dp.habitCursor > 0 {
			dp.habitCursor--
		}

	case " ":
		if dp.habitCursor >= 0 && dp.habitCursor < len(dp.habits) {
			dp.habits[dp.habitCursor].Done = !dp.habits[dp.habitCursor].Done
			dp.modified = true
		}

	case "[":
		dp.saveIfModified()
		dp.date = dp.date.AddDate(0, 0, -1)
		dp.reloadDay()
	case "]":
		dp.saveIfModified()
		dp.date = dp.date.AddDate(0, 0, 1)
		dp.reloadDay()

	case "s":
		dp.saveToFile()
		dp.modified = false
	}

	return dp, nil
}

// ---------------------------------------------------------------------------
// Block manipulation
// ---------------------------------------------------------------------------

// placeBlock writes a block into `slots` consecutive slots starting at idx.
func (dp *DailyPlanner) placeBlock(idx int, text string, bt blockType, slots int) {
	if idx < 0 || idx >= len(dp.blocks) {
		return
	}
	if slots < 1 {
		slots = 1
	}
	c := blockColor(bt, 0)
	for s := 0; s < slots && idx+s < len(dp.blocks); s++ {
		dp.blocks[idx+s].TaskText = text
		dp.blocks[idx+s].TaskType = bt
		dp.blocks[idx+s].Color = c
		dp.blocks[idx+s].Done = false
	}
}

// deleteBlock clears the slot at idx and all contiguous slots with the same
// text (multi-slot blocks).
func (dp *DailyPlanner) deleteBlock(idx int) {
	if idx < 0 || idx >= len(dp.blocks) {
		return
	}
	b := dp.blocks[idx]
	if b.TaskType == blockEmpty {
		return
	}
	text := b.TaskText
	// Find the start of this span
	start := idx
	for start > 0 && dp.blocks[start-1].TaskText == text && dp.blocks[start-1].TaskType != blockEmpty {
		start--
	}
	// Clear forward
	for i := start; i < len(dp.blocks) && dp.blocks[i].TaskText == text && dp.blocks[i].TaskType != blockEmpty; i++ {
		dp.blocks[i].TaskText = ""
		dp.blocks[i].TaskType = blockEmpty
		dp.blocks[i].Done = false
		dp.blocks[i].Color = ""
		dp.blocks[i].Priority = 0
		dp.blocks[i].SourcePath = ""
		dp.blocks[i].SourceLine = 0
	}
}

// toggleDone toggles the done flag on all slots belonging to the same block
// span as idx.
func (dp *DailyPlanner) toggleDone(idx int) {
	if idx < 0 || idx >= len(dp.blocks) {
		return
	}
	b := dp.blocks[idx]
	if b.TaskType == blockEmpty {
		return
	}
	newDone := !b.Done
	text := b.TaskText
	start := idx
	for start > 0 && dp.blocks[start-1].TaskText == text && dp.blocks[start-1].TaskType != blockEmpty {
		start--
	}
	for i := start; i < len(dp.blocks) && dp.blocks[i].TaskText == text && dp.blocks[i].TaskType != blockEmpty; i++ {
		dp.blocks[i].Done = newDone
	}

	// Record completion for bidirectional sync if this block has source tracking
	if b.SourcePath != "" {
		dp.completedTasks = append(dp.completedTasks, TaskCompletion{
			NotePath: b.SourcePath,
			LineNum:  b.SourceLine,
			Text:     text,
			Done:     newDone,
		})
	}
}

// moveBlock moves a multi-slot block from one position to another.
func (dp *DailyPlanner) moveBlock(from, to int) {
	if from < 0 || from >= len(dp.blocks) || to < 0 || to >= len(dp.blocks) {
		return
	}
	if from == to {
		return
	}
	b := dp.blocks[from]
	if b.TaskType == blockEmpty {
		return
	}

	text := b.TaskText
	bt := b.TaskType
	done := b.Done
	c := b.Color
	pri := b.Priority
	srcPath := b.SourcePath
	srcLine := b.SourceLine

	// Count span length
	start := from
	for start > 0 && dp.blocks[start-1].TaskText == text && dp.blocks[start-1].TaskType != blockEmpty {
		start--
	}
	spanLen := 0
	for i := start; i < len(dp.blocks) && dp.blocks[i].TaskText == text && dp.blocks[i].TaskType != blockEmpty; i++ {
		spanLen++
	}
	if spanLen < 1 {
		spanLen = 1
	}

	// Clear old slots
	for i := start; i < start+spanLen && i < len(dp.blocks); i++ {
		dp.blocks[i].TaskText = ""
		dp.blocks[i].TaskType = blockEmpty
		dp.blocks[i].Done = false
		dp.blocks[i].Color = ""
		dp.blocks[i].Priority = 0
		dp.blocks[i].SourcePath = ""
		dp.blocks[i].SourceLine = 0
	}

	// Place at new position
	for s := 0; s < spanLen && to+s < len(dp.blocks); s++ {
		dp.blocks[to+s].TaskText = text
		dp.blocks[to+s].TaskType = bt
		dp.blocks[to+s].Done = done
		dp.blocks[to+s].Color = c
		dp.blocks[to+s].Priority = pri
		dp.blocks[to+s].SourcePath = srcPath
		dp.blocks[to+s].SourceLine = srcLine
	}
}

// spanDuration returns the total minutes for the block span that includes idx.
func (dp *DailyPlanner) spanDuration(idx int) int {
	if idx < 0 || idx >= len(dp.blocks) {
		return 30
	}
	b := dp.blocks[idx]
	if b.TaskType == blockEmpty {
		return 30
	}
	text := b.TaskText
	start := idx
	for start > 0 && dp.blocks[start-1].TaskText == text && dp.blocks[start-1].TaskType != blockEmpty {
		start--
	}
	count := 0
	for i := start; i < len(dp.blocks) && dp.blocks[i].TaskText == text && dp.blocks[i].TaskType != blockEmpty; i++ {
		count++
	}
	if count < 1 {
		count = 1
	}
	return count * 30
}

// adjustScroll ensures the cursor is visible in the scroll window.
func (dp *DailyPlanner) adjustScroll() {
	vis := dp.visibleSlots()
	if dp.cursor < dp.scroll {
		dp.scroll = dp.cursor
	}
	if dp.cursor >= dp.scroll+vis {
		dp.scroll = dp.cursor - vis + 1
	}
	if dp.scroll < 0 {
		dp.scroll = 0
	}
}

// reloadDay reinitialises the blocks and attempts to load from the file for
// the new date.
func (dp *DailyPlanner) reloadDay() {
	dp.initBlocks()
	dp.cursor = 0
	dp.scroll = 0
	dp.unscheduled = nil
	dp.unschedCursor = 0
	dp.habitCursor = 0
	dp.moving = false
	dp.moveFrom = -1
	dp.adding = false
	dp.addBuf = ""
	dp.modified = false
	dp.loadFromFile()
	dp.recountProgress()
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the daily planner overlay.
func (dp DailyPlanner) View() string {
	outerWidth := dp.width * 9 / 10
	if outerWidth > 130 {
		outerWidth = 130
	}
	if outerWidth < 60 {
		outerWidth = 60
	}

	innerWidth := outerWidth - 6 // account for padding + border

	leftWidth := innerWidth * 6 / 10
	rightWidth := innerWidth - leftWidth - 3 // -3 for separator

	// Build left panel (schedule)
	leftContent := dp.viewSchedule(leftWidth)

	// Build right panel (unscheduled + habits)
	rightContent := dp.viewRightPanel(rightWidth)

	// Separator
	sepStyle := lipgloss.NewStyle().Foreground(surface1)

	leftPanel := lipgloss.NewStyle().
		Width(leftWidth).
		Render(leftContent)

	rightPanel := lipgloss.NewStyle().
		Width(rightWidth).
		Render(rightContent)

	separator := sepStyle.Render("│")

	// Join panels side by side
	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftPanel,
		"  "+separator+" ",
		rightPanel,
	)

	// Progress bar
	progressLine := dp.viewProgress(innerWidth)

	// Help line
	helpLine := dp.viewHelp()

	// Title
	titleIcon := lipgloss.NewStyle().Foreground(green).Render(IconCalendarChar)
	dateStr := dp.date.Format("Monday, Jan 2 2006")
	titleText := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" Daily Planner")
	datePart := lipgloss.NewStyle().Foreground(subtext0).Render(" — " + dateStr)
	modifiedMark := ""
	if dp.modified {
		modifiedMark = lipgloss.NewStyle().Foreground(yellow).Render(" [modified]")
	}
	title := "  " + titleIcon + titleText + datePart + modifiedMark
	titleSep := DimStyle.Render("  " + strings.Repeat("─", innerWidth-4))

	var out strings.Builder
	out.WriteString(title)
	out.WriteString("\n")
	out.WriteString(titleSep)
	out.WriteString("\n\n")
	out.WriteString(body)
	out.WriteString("\n\n")
	out.WriteString(progressLine)
	out.WriteString("\n")
	out.WriteString(helpLine)

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(outerWidth).
		Background(mantle)

	return border.Render(out.String())
}

// viewSchedule renders the left panel: the time grid with blocks.
func (dp DailyPlanner) viewSchedule(width int) string {
	var b strings.Builder

	// Section header
	headerIcon := lipgloss.NewStyle().Foreground(mauve).Render(IconCalendarChar)
	headerText := lipgloss.NewStyle().Foreground(blue).Bold(true).Render(" Schedule")
	focusInd := ""
	if dp.focus == panelSchedule {
		focusInd = lipgloss.NewStyle().Foreground(green).Render(" *")
	}
	b.WriteString("  " + headerIcon + headerText + focusInd)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", width-4)))
	b.WriteString("\n")

	vis := dp.visibleSlots()
	end := dp.scroll + vis
	if end > len(dp.blocks) {
		end = len(dp.blocks)
	}

	blockWidth := width - 12 // time label (7) + padding + tag (3)
	if blockWidth < 10 {
		blockWidth = 10
	}

	for i := dp.scroll; i < end; i++ {
		blk := dp.blocks[i]

		// Time label
		timeLabel := dp.slotTime(i)
		timeStyle := lipgloss.NewStyle().Foreground(subtext0)
		if dp.focus == panelSchedule && i == dp.cursor {
			timeStyle = lipgloss.NewStyle().Foreground(mauve).Bold(true)
		}
		timePart := timeStyle.Render(fmt.Sprintf("  %s", timeLabel))

		// Block content
		var blockPart string
		if blk.TaskType == blockEmpty {
			fillChar := "░"
			fillStyle := lipgloss.NewStyle().Foreground(surface0)
			if dp.focus == panelSchedule && i == dp.cursor {
				fillStyle = lipgloss.NewStyle().Foreground(surface1)
			}
			blockPart = fillStyle.Render(strings.Repeat(fillChar, blockWidth))
		} else {
			fillChar := "█"
			c := blk.Color
			if c == "" {
				c = blockColor(blk.TaskType, blk.Priority)
			}

			tag := blockTypeTag(blk.TaskType)

			// Truncate text if needed
			maxText := blockWidth - lipgloss.Width(tag) - 2
			if maxText < 3 {
				maxText = 3
			}
			displayText := TruncateDisplay(blk.TaskText, maxText)

			blockStyle := lipgloss.NewStyle().Foreground(c)
			tagStyle := lipgloss.NewStyle().Foreground(c)

			if blk.Done {
				blockStyle = lipgloss.NewStyle().Foreground(green).Strikethrough(true)
				fillChar = "█"
			}

			// Pad the text portion
			textPart := displayText
			paddingLen := blockWidth - lipgloss.Width(fillChar+fillChar+fillChar+fillChar) - len(textPart) - len(tag) - 1
			if paddingLen < 0 {
				paddingLen = 0
			}

			blockPart = blockStyle.Render(fillChar+fillChar+fillChar+fillChar+" "+textPart) +
				strings.Repeat(" ", paddingLen) +
				tagStyle.Render(tag)
		}

		// Cursor indicator
		cursorMark := "  "
		if dp.focus == panelSchedule && i == dp.cursor {
			cursorChar := ">"
			if dp.moving {
				cursorChar = "+"
			}
			cursorMark = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(cursorChar + " ")
		}

		// Moving source marker
		moveMarker := ""
		if dp.moving && i == dp.moveFrom {
			moveMarker = lipgloss.NewStyle().Foreground(yellow).Render(" <src")
		}

		b.WriteString(cursorMark + timePart + "  " + blockPart + moveMarker)
		b.WriteString("\n")
	}

	// Add-block input at the bottom of schedule
	if dp.adding {
		b.WriteString("\n")
		promptStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
		inputStyle := lipgloss.NewStyle().Foreground(text)
		typeLabel := blockTypeName(dp.addType)
		typeLabelStyled := lipgloss.NewStyle().Foreground(blockColor(dp.addType, 0)).Render("[" + typeLabel + "]")
		durLabels := []string{"30m", "1h", "1.5h", "2h", "2.5h", "3h"}
		durLabel := durLabels[0]
		if dp.addSlots >= 1 && dp.addSlots <= 6 {
			durLabel = durLabels[dp.addSlots-1]
		}
		durStyled := lipgloss.NewStyle().Foreground(peach).Bold(true).Render("(" + durLabel + ")")
		b.WriteString("  " + promptStyle.Render("New block ") + durStyled + promptStyle.Render(": ") + inputStyle.Render(dp.addBuf) + lipgloss.NewStyle().Foreground(mauve).Render("_"))
		b.WriteString("\n")
		b.WriteString("  " + typeLabelStyled + DimStyle.Render("  1:task 2:event 3:break 4:focus  -/+:duration"))
		b.WriteString("\n")
	}

	// Scroll indicators
	if len(dp.blocks) > dp.visibleSlots() {
		indicator := ScrollIndicator(dp.scroll, len(dp.blocks), dp.visibleSlots())
		if indicator != "" {
			b.WriteString(DimStyle.Render("  " + indicator))
			b.WriteString("\n")
		}
	}

	return b.String()
}

// viewRightPanel renders the right panel: unscheduled tasks and habits.
func (dp DailyPlanner) viewRightPanel(width int) string {
	var b strings.Builder

	// --- Unscheduled Tasks ---
	unschedIcon := lipgloss.NewStyle().Foreground(blue).Render(IconOutlineChar)
	unschedTitle := lipgloss.NewStyle().Foreground(blue).Bold(true).Render(" Unscheduled Tasks")
	focusInd := ""
	if dp.focus == panelUnscheduled {
		focusInd = lipgloss.NewStyle().Foreground(green).Render(" *")
	}
	b.WriteString("  " + unschedIcon + unschedTitle + focusInd)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", width-4)))
	b.WriteString("\n")

	if len(dp.unscheduled) == 0 {
		b.WriteString(DimStyle.Render("  No unscheduled tasks"))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  Press 'a' to add one"))
		b.WriteString("\n")
	} else {
		maxVisible := dp.visibleSlots() / 2
		if maxVisible < 4 {
			maxVisible = 4
		}
		startIdx := 0
		if dp.unschedCursor >= maxVisible {
			startIdx = dp.unschedCursor - maxVisible + 1
		}
		endIdx := startIdx + maxVisible
		if endIdx > len(dp.unscheduled) {
			endIdx = len(dp.unscheduled)
		}

		for i := startIdx; i < endIdx; i++ {
			task := dp.unscheduled[i]
			isSelected := dp.focus == panelUnscheduled && i == dp.unschedCursor

			// Cursor marker
			marker := "  "
			if isSelected {
				marker = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("> ")
			}

			// Priority label
			priLabel := ""
			priColor := taskPriorityColor(task.Priority)
			switch task.Priority {
			case 4:
				priLabel = "highest"
			case 3:
				priLabel = "high"
			case 2:
				priLabel = "med"
			case 1:
				priLabel = "low"
			default:
				priLabel = "none"
			}
			priStyled := lipgloss.NewStyle().Foreground(priColor).Render(priLabel)

			// Task text
			maxText := width - 14 - len(priLabel)
			if maxText < 5 {
				maxText = 5
			}
			displayText := TruncateDisplay(task.Text, maxText)

			textStyle := NormalItemStyle
			donePrefix := ""
			if task.Done {
				textStyle = lipgloss.NewStyle().Foreground(green).Strikethrough(true)
				donePrefix = lipgloss.NewStyle().Foreground(green).Render("[x] ")
			}

			// Pad between text and priority
			padding := width - 6 - lipgloss.Width(displayText) - lipgloss.Width(priLabel) - lipgloss.Width(donePrefix)
			if padding < 1 {
				padding = 1
			}

			b.WriteString(marker + donePrefix + textStyle.Render(displayText) + strings.Repeat(" ", padding) + priStyled)
			b.WriteString("\n")
		}

		if startIdx > 0 {
			b.WriteString(DimStyle.Render("  ... more above"))
			b.WriteString("\n")
		}
		if endIdx < len(dp.unscheduled) {
			b.WriteString(DimStyle.Render("  ... more below"))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")

	// --- Habits ---
	habitIcon := lipgloss.NewStyle().Foreground(green).Render(IconBookmarkChar)
	habitTitle := lipgloss.NewStyle().Foreground(green).Bold(true).Render(" Habits")
	habitFocusInd := ""
	if dp.focus == panelHabits {
		habitFocusInd = lipgloss.NewStyle().Foreground(green).Render(" *")
	}
	b.WriteString("  " + habitIcon + habitTitle + habitFocusInd)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", width-4)))
	b.WriteString("\n")

	if len(dp.habits) == 0 {
		b.WriteString(DimStyle.Render("  No habits configured"))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  Open Habits tracker to add some"))
		b.WriteString("\n")
	} else {
		for i, habit := range dp.habits {
			isSelected := dp.focus == panelHabits && i == dp.habitCursor

			marker := "  "
			if isSelected {
				marker = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("> ")
			}

			// Done checkbox
			checkbox := lipgloss.NewStyle().Foreground(surface1).Render("[ ]")
			if habit.Done {
				checkbox = lipgloss.NewStyle().Foreground(green).Render("[x]")
			}

			// Name
			nameStyle := NormalItemStyle
			if habit.Done {
				nameStyle = lipgloss.NewStyle().Foreground(green)
			}

			// Streak visualization
			streakVis := dp.renderStreak(habit.Streak, habit.Done)

			// Streak count
			streakLabel := ""
			if habit.Streak > 0 {
				streakLabel = lipgloss.NewStyle().Foreground(subtext0).Render(fmt.Sprintf(" %dd", habit.Streak))
			}

			maxName := width - 20 - lipgloss.Width(streakVis) - lipgloss.Width(streakLabel)
			if maxName < 5 {
				maxName = 5
			}
			name := TruncateDisplay(habit.Name, maxName)

			padding := width - 6 - lipgloss.Width(name) - lipgloss.Width(streakVis) - lipgloss.Width(streakLabel) - 4
			if padding < 1 {
				padding = 1
			}

			b.WriteString(marker + checkbox + " " + nameStyle.Render(name) + strings.Repeat(" ", padding) + streakVis + streakLabel)
			b.WriteString("\n")
		}
	}

	// --- Active Goals ---
	if len(dp.activeGoals) > 0 {
		b.WriteString("\n")
		goalIcon := lipgloss.NewStyle().Foreground(mauve).Render("◎")
		goalTitle := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" Goals")
		b.WriteString("  " + goalIcon + goalTitle)
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  " + strings.Repeat("─", width-4)))
		b.WriteString("\n")
		for _, g := range dp.activeGoals {
			bullet := lipgloss.NewStyle().Foreground(mauve).Render("  ▸ ")
			name := lipgloss.NewStyle().Foreground(text).Render(TruncateDisplay(g.Title, width-8))
			b.WriteString(bullet + name)
			if g.TargetDate != "" {
				b.WriteString(DimStyle.Render(" (by " + g.TargetDate + ")"))
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

// renderStreak draws a small streak visualization bar.
func (dp DailyPlanner) renderStreak(streak int, done bool) string {
	const maxBars = 5
	filled := streak
	if filled > maxBars {
		filled = maxBars
	}
	empty := maxBars - filled

	filledStyle := lipgloss.NewStyle().Foreground(green)
	emptyStyle := lipgloss.NewStyle().Foreground(surface1)

	return filledStyle.Render(strings.Repeat("█", filled)) +
		emptyStyle.Render(strings.Repeat("░", empty))
}

// viewProgress renders the progress bar at the bottom.
func (dp DailyPlanner) viewProgress(width int) string {
	dp.recountDirect()

	var b strings.Builder
	sepStyle := lipgloss.NewStyle().Foreground(surface1)
	b.WriteString("  " + sepStyle.Render(strings.Repeat("─", 4)) + " ")

	label := fmt.Sprintf("Progress: %d/%d ", dp.doneCount, dp.totalCount)
	labelStyled := lipgloss.NewStyle().Foreground(subtext0).Render(label)
	b.WriteString(labelStyled)

	// Bar
	barWidth := 20
	if barWidth > width/3 {
		barWidth = width / 3
	}
	if barWidth < 8 {
		barWidth = 8
	}

	filled := 0
	if dp.totalCount > 0 {
		filled = dp.doneCount * barWidth / dp.totalCount
		if filled > barWidth {
			filled = barWidth
		}
	}
	empty := barWidth - filled

	filledStyled := lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("█", filled))
	emptyStyled := lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("░", empty))
	b.WriteString(filledStyled + emptyStyled)

	// Percentage
	pct := 0
	if dp.totalCount > 0 {
		pct = dp.doneCount * 100 / dp.totalCount
	}
	pctStyled := lipgloss.NewStyle().Foreground(green).Bold(true).Render(fmt.Sprintf(" %d%%", pct))
	b.WriteString(pctStyled)

	b.WriteString(" " + sepStyle.Render(strings.Repeat("─", 4)))

	return b.String()
}

// recountDirect is a non-mutating recount for use in View.
func (dp *DailyPlanner) recountDirect() {
	dp.doneCount = 0
	dp.totalCount = 0
	seen := make(map[string]bool)
	for _, b := range dp.blocks {
		if b.TaskType == blockEmpty {
			continue
		}
		key := fmt.Sprintf("%d-%v-%s", b.Hour, b.HalfHour, b.TaskText)
		if seen[key] {
			continue
		}
		seen[key] = true
		dp.totalCount++
		if b.Done {
			dp.doneCount++
		}
	}
}

// viewHelp renders the keybinding help bar.
func (dp DailyPlanner) viewHelp() string {
	pairs := []struct{ Key, Desc string }{
		{"j/k", "move"}, {"Tab", "panel"}, {"a", "add"}, {"d", "delete"},
		{"Enter", "assign"}, {"Space", "done"}, {"m", "move block"},
		{"-/+", "resize"}, {"f", "focus"}, {"s", "save"},
		{"[/]", "day"}, {"Esc", "close"},
	}
	return RenderHelpBar(pairs)
}

// ---------------------------------------------------------------------------
// File I/O
// ---------------------------------------------------------------------------

// plannerDir returns the absolute path to the Planner directory in the vault.
func (dp *DailyPlanner) plannerDir() string {
	return filepath.Join(dp.vaultRoot, "Daily Planner")
}

// plannerFilePath returns the path for the current date's planner file.
func (dp *DailyPlanner) plannerFilePath() string {
	return filepath.Join(dp.plannerDir(), dp.date.Format("2006-01-02")+".md")
}

// SaveNow persists the planner to disk unconditionally.  Used by app.go
// after applying an AI schedule so the calendar can load the planner blocks.
func (dp *DailyPlanner) SaveNow() {
	dp.saveToFile()
	dp.modified = false
}

// saveIfModified saves the planner to disk if there are unsaved changes.
func (dp *DailyPlanner) saveIfModified() {
	if dp.modified {
		dp.saveToFile()
		dp.modified = false
	}
}

// saveToFile writes the current schedule to a markdown file in Planner/.
func (dp *DailyPlanner) saveToFile() {
	dir := dp.plannerDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}

	var b strings.Builder

	// Frontmatter
	b.WriteString("---\n")
	b.WriteString("date: " + dp.date.Format("2006-01-02") + "\n")
	b.WriteString("type: planner\n")
	b.WriteString("---\n\n")

	// Schedule section
	b.WriteString("## Schedule\n\n")

	i := 0
	for i < len(dp.blocks) {
		blk := dp.blocks[i]
		if blk.TaskType == blockEmpty {
			i++
			continue
		}

		// Find span end
		text := blk.TaskText
		bt := blk.TaskType
		start := i
		done := blk.Done
		for i < len(dp.blocks) && dp.blocks[i].TaskText == text && dp.blocks[i].TaskType != blockEmpty {
			i++
		}
		endIdx := i

		startTime := dp.slotTime(start)
		endTime := dp.slotTimeEnd(endIdx - 1)
		typeName := blockTypeName(bt)

		line := fmt.Sprintf("- %s-%s | %s | %s", startTime, endTime, text, typeName)
		if done {
			line += " | done"
		}
		b.WriteString(line + "\n")
	}

	// Habits section
	if len(dp.habits) > 0 {
		b.WriteString("\n## Habits\n\n")
		for _, h := range dp.habits {
			marker := " "
			if h.Done {
				marker = "x"
			}
			b.WriteString(fmt.Sprintf("- [%s] %s (streak: %d)\n", marker, h.Name, h.Streak))
		}
	}

	_ = atomicWriteNote(dp.plannerFilePath(), b.String())
}

// slotTimeEnd returns the end time for a slot (i.e. the start of the next
// slot).
func (dp *DailyPlanner) slotTimeEnd(idx int) string {
	if idx < 0 || idx >= len(dp.blocks) {
		return "??:??"
	}
	b := dp.blocks[idx]
	h := b.Hour
	m := 0
	if b.HalfHour {
		m = 30
	}
	// Add 30 minutes
	m += 30
	if m >= 60 {
		m -= 60
		h++
	}
	return fmt.Sprintf("%02d:%02d", h, m)
}

// loadFromFile attempts to parse a saved planner file. Returns true if a file
// was successfully loaded.
func (dp *DailyPlanner) loadFromFile() bool {
	fp := dp.plannerFilePath()
	f, err := os.Open(fp)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	inSchedule := false
	inHabits := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "## Schedule" {
			inSchedule = true
			inHabits = false
			continue
		}
		if trimmed == "## Habits" {
			inSchedule = false
			inHabits = true
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			inSchedule = false
			inHabits = false
			continue
		}

		if inSchedule {
			dp.parseScheduleLine(trimmed)
		}
		if inHabits {
			dp.parseHabitLine(trimmed)
		}
	}

	return true
}

// parseScheduleLine parses a line like:
//
//   - 09:00-10:30 | Fix auth bug | task | done
func (dp *DailyPlanner) parseScheduleLine(line string) {
	if !strings.HasPrefix(line, "- ") {
		return
	}
	line = strings.TrimPrefix(line, "- ")

	parts := strings.Split(line, " | ")
	if len(parts) < 3 {
		return
	}

	// Parse time range
	timeRange := strings.TrimSpace(parts[0])
	timeParts := strings.Split(timeRange, "-")
	if len(timeParts) != 2 {
		return
	}
	startTime := strings.TrimSpace(timeParts[0])
	endTime := strings.TrimSpace(timeParts[1])

	text := strings.TrimSpace(parts[1])
	bt := parseBlockType(parts[2])

	done := false
	if len(parts) >= 4 && strings.TrimSpace(parts[3]) == "done" {
		done = true
	}

	startIdx := dp.timeToSlotIndex(startTime)
	endIdx := dp.timeToSlotIndex(endTime)
	if startIdx < 0 {
		return
	}

	// If endIdx is -1 (e.g. 22:00 out of range), use end of blocks
	if endIdx < 0 {
		endIdx = len(dp.blocks)
	}

	c := blockColor(bt, 0)
	for i := startIdx; i < endIdx && i < len(dp.blocks); i++ {
		dp.blocks[i].TaskText = text
		dp.blocks[i].TaskType = bt
		dp.blocks[i].Done = done
		dp.blocks[i].Color = c
	}
}

// parseHabitLine parses a line like:
//
//   - [x] Exercise (streak: 5)
func (dp *DailyPlanner) parseHabitLine(line string) {
	if !strings.HasPrefix(line, "- [") {
		return
	}

	done := false
	if strings.HasPrefix(line, "- [x]") || strings.HasPrefix(line, "- [X]") {
		done = true
	}

	// Extract name and streak
	rest := line
	if done {
		rest = strings.TrimPrefix(line, "- [x] ")
		if rest == line {
			rest = strings.TrimPrefix(line, "- [X] ")
		}
	} else {
		rest = strings.TrimPrefix(line, "- [ ] ")
	}

	name := rest
	streak := 0

	if idx := strings.Index(rest, "(streak: "); idx >= 0 {
		name = strings.TrimSpace(rest[:idx])
		streakPart := rest[idx+9:]
		if endIdx := strings.Index(streakPart, ")"); endIdx >= 0 {
			_, _ = fmt.Sscanf(streakPart[:endIdx], "%d", &streak)
		}
	}

	// Try to match with existing habits
	for i := range dp.habits {
		if dp.habits[i].Name == name {
			dp.habits[i].Done = done
			if streak > dp.habits[i].Streak {
				dp.habits[i].Streak = streak
			}
			return
		}
	}

	// Add as new habit if not found
	dp.habits = append(dp.habits, PlannerHabit{
		Name:   name,
		Done:   done,
		Streak: streak,
	})
}

// ApplyAISchedule fills the planner grid from AI-generated scheduler slots.
func (dp *DailyPlanner) ApplyAISchedule(slots []schedulerSlot) {
	// Clear existing blocks
	dp.initBlocks()

	for _, slot := range slots {
		timeStr := fmt.Sprintf("%02d:%02d", slot.StartHour, slot.StartMin)
		idx := dp.timeToSlotIndex(timeStr)
		if idx < 0 {
			continue
		}
		durMins := slot.slotMinutes()
		numSlots := durMins / 30
		if numSlots < 1 {
			numSlots = 1
		}

		var bt blockType
		var c lipgloss.Color
		switch slot.Type {
		case "task":
			bt = blockTask
			c = priorityColor(slot.Priority)
		case "event":
			bt = blockEvent
			c = lavender
		case "break", "lunch":
			bt = blockBreak
			c = teal
		default:
			bt = blockTask
			c = text
		}

		// Try to match slot text to an unscheduled task for source tracking
		var srcPath string
		var srcLine int
		for _, ut := range dp.unscheduled {
			if ut.Text == slot.Task || strings.Contains(ut.Text, slot.Task) || strings.Contains(slot.Task, ut.Text) {
				srcPath = ut.NotePath
				srcLine = ut.LineNum
				break
			}
		}

		for s := 0; s < numSlots && idx+s < len(dp.blocks); s++ {
			dp.blocks[idx+s].TaskText = slot.Task
			dp.blocks[idx+s].TaskType = bt
			dp.blocks[idx+s].Color = c
			dp.blocks[idx+s].Priority = slot.Priority
			dp.blocks[idx+s].SourcePath = srcPath
			dp.blocks[idx+s].SourceLine = srcLine
		}
	}

	dp.recountProgress()
	dp.modified = true
}

// AddEventToFile appends an event block to an existing planner file for the
// given date.  If no planner file exists for that date, one is created with
// frontmatter and a schedule section.  The event is placed at the next
// available hour (after the last existing block, or 09:00 by default) and
// given a 1-hour duration.
func AddEventToPlannerFile(vaultRoot, dateStr, text string) {
	plannerDir := filepath.Join(vaultRoot, "Daily Planner")
	if err := os.MkdirAll(plannerDir, 0755); err != nil {
		return
	}

	fp := filepath.Join(plannerDir, dateStr+".md")

	// Determine the next available start time by scanning existing blocks.
	nextHour := 9
	nextMin := 0

	if data, err := os.ReadFile(fp); err == nil {
		inSchedule := false
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "## Schedule" {
				inSchedule = true
				continue
			}
			if strings.HasPrefix(trimmed, "## ") {
				inSchedule = false
				continue
			}
			if !inSchedule || !strings.HasPrefix(trimmed, "- ") {
				continue
			}
			// Parse end time from "- HH:MM-HH:MM | ..."
			entry := strings.TrimPrefix(trimmed, "- ")
			parts := strings.Split(entry, " | ")
			if len(parts) < 2 {
				continue
			}
			timeRange := strings.TrimSpace(parts[0])
			timeParts := strings.Split(timeRange, "-")
			if len(timeParts) != 2 {
				continue
			}
			var h, m int
			if _, err := fmt.Sscanf(strings.TrimSpace(timeParts[1]), "%d:%d", &h, &m); err == nil {
				if h > nextHour || (h == nextHour && m > nextMin) {
					nextHour = h
					nextMin = m
				}
			}
		}
	}

	startTime := fmt.Sprintf("%02d:%02d", nextHour, nextMin)
	endHour := nextHour + 1
	endMin := nextMin
	if endHour > 22 {
		endHour = 22
		endMin = 0
	}
	endTime := fmt.Sprintf("%02d:%02d", endHour, endMin)

	newLine := fmt.Sprintf("- %s-%s | %s | event\n", startTime, endTime, text)

	// If the file does not exist, create it with frontmatter and schedule header.
	if _, err := os.Stat(fp); os.IsNotExist(err) {
		var content strings.Builder
		content.WriteString("---\n")
		content.WriteString("date: " + dateStr + "\n")
		content.WriteString("type: planner\n")
		content.WriteString("---\n\n")
		content.WriteString("## Schedule\n\n")
		content.WriteString(newLine)
		_ = atomicWriteNote(fp, content.String())
		return
	}

	// File exists — append into the ## Schedule section.
	data, err := os.ReadFile(fp)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")

	// Find the end of the schedule section to insert.
	insertIdx := -1
	inSchedule := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Schedule" {
			inSchedule = true
			continue
		}
		if inSchedule {
			if strings.HasPrefix(trimmed, "## ") || (trimmed == "" && i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "## ")) {
				insertIdx = i
				break
			}
		}
	}

	if insertIdx < 0 {
		// Append at end if no other section found
		insertIdx = len(lines)
	}

	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:insertIdx]...)
	newLines = append(newLines, strings.TrimRight(newLine, "\n"))
	newLines = append(newLines, lines[insertIdx:]...)
	_ = atomicWriteNote(fp, strings.Join(newLines, "\n"))
}
// UI configuration updated.
