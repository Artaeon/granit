package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Shared schedule generation — used by both MorningRoutine and PlanMyDay
// to build a time-blocked daily schedule from tasks, events, and habits.
// ---------------------------------------------------------------------------

// ScheduleInput holds all the data needed to generate a daily schedule.
type ScheduleInput struct {
	// Tasks to schedule (already filtered/selected by the caller)
	Tasks []ScheduleTask

	// Calendar events for today (meetings — placed first as immovable blocks)
	Events []PlannerEvent

	// Habit names to schedule as a single block
	Habits []string

	// Projects for matching tasks to project names
	Projects []Project

	// Existing planner blocks already scheduled (marked as occupied)
	ExistingBlocks []PlannerBlock

	// WorkEnd is the last minute of the day to schedule into (default 22*60)
	WorkEnd int
}

// ScheduleTask represents a task to be scheduled.
type ScheduleTask struct {
	Text     string
	Priority int
	Estimate int // minutes; 0 = auto-estimate from priority
}

// GenerateLocalSchedule builds a time-blocked schedule from the given input.
// Returns a sorted slice of daySlot. The algorithm:
//  1. Mark existing blocks and calendar events as occupied
//  2. Place lunch break (12:00–13:00) if not already occupied
//  3. Place tasks in priority order with breaks every 90 minutes
//  4. Place habits block
//  5. Place end-of-day review
func GenerateLocalSchedule(input ScheduleInput) []daySlot {
	now := time.Now()
	currentMin := now.Hour()*60 + now.Minute()
	workStart := ((currentMin + 14) / 15) * 15 // round to next 15m
	if workStart < 8*60 {
		workStart = 8 * 60
	}
	workEnd := input.WorkEnd
	if workEnd <= 0 {
		workEnd = 22 * 60
	}
	if workStart >= workEnd {
		// Invoked after the configured end-of-day (e.g. running Plan My Day
		// at 22:30 with WorkEnd=22:00): widen the window by one slot so we
		// still emit the daily review instead of returning nothing at all.
		workEnd = workStart + 15
		if workEnd > 24*60 {
			return nil
		}
	}

	type timeRange struct{ start, end int }
	var occupied []timeRange
	var schedule []daySlot

	isOccupied := func(start, end int) bool {
		for _, o := range occupied {
			if start < o.end && end > o.start {
				return true
			}
		}
		return false
	}

	findSlot := func(duration int) (int, bool) {
		for pos := workStart; pos+duration <= workEnd; pos += 15 {
			if !isOccupied(pos, pos+duration) {
				return pos, true
			}
		}
		return 0, false
	}

	// 0. Mark existing planner blocks as occupied
	for _, pb := range input.ExistingBlocks {
		var sH, sM, eH, eM int
		_, _ = fmt.Sscanf(pb.StartTime, "%d:%d", &sH, &sM)
		_, _ = fmt.Sscanf(pb.EndTime, "%d:%d", &eH, &eM)
		start := sH*60 + sM
		end := eH*60 + eM
		if end > start {
			occupied = append(occupied, timeRange{start, end})
		}
	}

	// 1. Place calendar events first (immovable meetings)
	for _, ev := range input.Events {
		if ev.Time == "" {
			continue
		}
		h, m := 0, 0
		_, _ = fmt.Sscanf(ev.Time, "%d:%d", &h, &m)
		start := h*60 + m
		dur := ev.Duration
		if dur <= 0 {
			dur = 60
		}
		end := start + dur
		occupied = append(occupied, timeRange{start, end})
		schedule = append(schedule, daySlot{
			Start: fmtTimeSlot(start), End: fmtTimeSlot(end),
			Task: ev.Title, Type: "meeting",
		})
	}

	// 2. Place lunch break (12:00–13:00) if not already occupied
	lunchStart := 12 * 60
	lunchEnd := 13 * 60
	if lunchEnd > workStart && !isOccupied(lunchStart, lunchEnd) {
		occupied = append(occupied, timeRange{lunchStart, lunchEnd})
		schedule = append(schedule, daySlot{
			Start: "12:00", End: "13:00", Task: "Lunch", Type: "break",
		})
	}

	// 3. Sort tasks by priority and schedule them
	sorted := make([]ScheduleTask, len(input.Tasks))
	copy(sorted, input.Tasks)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority > sorted[j].Priority
	})

	workMinsSinceBreak := 0
	for _, st := range sorted {
		// Break every 90 min of work
		if workMinsSinceBreak >= 90 {
			breakStart, found := findSlot(15)
			if found {
				occupied = append(occupied, timeRange{breakStart, breakStart + 15})
				schedule = append(schedule, daySlot{
					Start: fmtTimeSlot(breakStart), End: fmtTimeSlot(breakStart + 15),
					Task: "Break", Type: "break",
				})
				workMinsSinceBreak = 0
			}
		}

		dur := st.Estimate
		if dur <= 0 {
			// Auto-estimate from priority
			switch {
			case st.Priority >= 3:
				dur = 90
			case st.Priority >= 2:
				dur = 60
			default:
				dur = 30
			}
		}

		taskStart, found := findSlot(dur)
		if !found {
			dur = 30 // try a shorter slot
			taskStart, found = findSlot(dur)
			if !found {
				continue
			}
		}
		occupied = append(occupied, timeRange{taskStart, taskStart + dur})

		slotType := "deep-work"
		if st.Priority <= 1 {
			slotType = "admin"
		}

		// Match project
		project := ""
		for _, proj := range input.Projects {
			if proj.TaskFilter != "" && strings.Contains(strings.ToLower(st.Text), strings.ToLower(proj.TaskFilter)) {
				project = proj.Name
				break
			}
			for _, tag := range proj.Tags {
				if strings.Contains(strings.ToLower(st.Text), strings.ToLower(tag)) {
					project = proj.Name
					break
				}
			}
		}

		schedule = append(schedule, daySlot{
			Start: fmtTimeSlot(taskStart), End: fmtTimeSlot(taskStart + dur),
			Task: st.Text, Type: slotType, Priority: st.Priority, Project: project,
		})
		workMinsSinceBreak += dur
	}

	// 4. Place habits block
	if len(input.Habits) > 0 {
		habitStart, found := findSlot(30)
		if found {
			occupied = append(occupied, timeRange{habitStart, habitStart + 30})
			schedule = append(schedule, daySlot{
				Start: fmtTimeSlot(habitStart), End: fmtTimeSlot(habitStart + 30),
				Task: "Habits: " + strings.Join(input.Habits, ", "), Type: "habit",
			})
		}
	}

	// 5. End-of-day review
	reviewStart, found := findSlot(15)
	if found {
		schedule = append(schedule, daySlot{
			Start: fmtTimeSlot(reviewStart), End: fmtTimeSlot(reviewStart + 15),
			Task: "Daily review", Type: "review",
		})
	}

	// Sort by start time
	sort.Slice(schedule, func(i, j int) bool {
		return schedule[i].Start < schedule[j].Start
	})

	return schedule
}

// FormatDayPlanMarkdown generates the markdown content for the daily note.
func FormatDayPlanMarkdown(schedule []daySlot, topGoal string, focusOrder []string, advice string) string {
	var b strings.Builder

	b.WriteString("## Day Plan\n\n")

	if topGoal != "" {
		b.WriteString("**Today's Goal:** " + topGoal + "\n\n")
	}

	b.WriteString("### Schedule\n\n")
	b.WriteString("| Time | Task | Type |\n")
	b.WriteString("|------|------|------|\n")
	for _, slot := range schedule {
		b.WriteString(fmt.Sprintf("| %s-%s | %s | %s |\n", slot.Start, slot.End, slot.Task, slot.Type))
	}
	b.WriteString("\n")

	if len(focusOrder) > 0 {
		b.WriteString("### Focus Order\n\n")
		for i, item := range focusOrder {
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, item))
		}
		b.WriteString("\n")
	}

	if advice != "" {
		b.WriteString("### Advice\n\n")
		b.WriteString("> " + advice + "\n\n")
	}

	return b.String()
}
