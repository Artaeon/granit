package tui

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

type planMyDayResultMsg struct {
	response string
	err      error
}

type planMyDayTickMsg struct{}

func planMyDayTickCmd() tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
		return planMyDayTickMsg{}
	})
}

// planMyDayGatherMsg signals that the gathering animation is done.
type planMyDayGatherMsg struct{}

func planMyDayGatherCmd() tea.Cmd {
	return tea.Tick(1200*time.Millisecond, func(time.Time) tea.Msg {
		return planMyDayGatherMsg{}
	})
}

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

// daySlot is a single time block in the AI-generated day plan.
type daySlot struct {
	Start    string // "09:00"
	End      string // "09:30"
	Task     string
	Type     string // "deep-work", "admin", "meeting", "break", "habit", "review"
	Priority int
	Project  string // which project this belongs to
}

// ---------------------------------------------------------------------------
// PlanMyDay overlay
// ---------------------------------------------------------------------------

// PlanMyDay orchestrates the full "plan my day" flow in a single overlay.
type PlanMyDay struct {
	active bool
	width  int
	height int
	phase  int // 0=gathering, 1=planning, 2=result, 3=applied

	// Gathered data
	tasks    []Task
	events   []PlannerEvent
	habits   []habitEntry
	projects []Project
	goals    []Goal

	// Yesterday's carry-forward tasks
	yesterdayTasks []string

	// AI result
	schedule   []daySlot
	focusOrder []string // prioritized task names for the day
	topGoal    string   // AI-suggested #1 goal for today
	advice     string   // personalized AI advice

	// Config
	ai AIConfig

	scroll    int
	vaultRoot string

	// Consumed-once results
	shouldApply     bool
	appliedSchedule []daySlot
	appliedGoal     string
	appliedFocus    []string
	appliedAdvice   string

	// Clocked sessions for display
	clockedSessions []clockPlanSlot

	// Streaming
	streaming    bool
	streamBuf    strings.Builder
	streamCh     <-chan tea.Msg
	streamCancel context.CancelFunc

	// Animation
	loadingTick  int
	spinnerTick  int
	loadingStart time.Time
}

// NewPlanMyDay creates a new PlanMyDay overlay.
func NewPlanMyDay() PlanMyDay {
	return PlanMyDay{}
}

// IsActive reports whether the overlay is currently displayed.
func (p PlanMyDay) IsActive() bool { return p.active }

// SetSize updates the available terminal dimensions.
func (p *PlanMyDay) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// Open activates the overlay with all gathered data and AI config, then
// starts the gathering animation before auto-advancing to the AI call.
func (p *PlanMyDay) Open(
	vaultRoot string,
	tasks []Task,
	events []PlannerEvent,
	habits []habitEntry,
	projects []Project,
	yesterdayTasks []string,
	cfg AIConfig,
) tea.Cmd {
	p.active = true
	p.vaultRoot = vaultRoot
	p.phase = 0
	p.scroll = 0
	p.loadingTick = 0
	p.spinnerTick = 0
	p.loadingStart = time.Now()
	p.shouldApply = false
	p.appliedSchedule = nil
	p.appliedGoal = ""
	p.appliedFocus = nil
	p.appliedAdvice = ""

	// Copy data
	p.tasks = tasks
	p.events = events
	p.habits = habits
	p.projects = projects
	p.yesterdayTasks = yesterdayTasks

	// AI result reset
	p.schedule = nil
	p.focusOrder = nil
	p.topGoal = ""
	p.advice = ""

	// AI config
	p.ai = cfg
	if p.ai.Provider == "" {
		p.ai.Provider = "local"
	}
	if p.ai.OllamaURL == "" {
		p.ai.OllamaURL = "http://localhost:11434"
	}
	if p.ai.Model == "" {
		p.ai.Model = "qwen2.5:0.5b"
	}

	// Start gathering animation, auto-advance after delay
	return tea.Batch(planMyDayGatherCmd(), planMyDayTickCmd())
}

// Close deactivates the overlay.
func (p *PlanMyDay) Close() { p.active = false }

// SetClockedSessions provides today's clocked sessions for display in the planner.
func (p *PlanMyDay) SetClockedSessions(sessions []clockPlanSlot) {
	p.clockedSessions = sessions
}

// GetAppliedPlan returns the schedule to write to the daily note and resets the flag.
func (p *PlanMyDay) GetAppliedPlan() ([]daySlot, string, []string, string, bool) {
	if !p.shouldApply {
		return nil, "", nil, "", false
	}
	sched := p.appliedSchedule
	goal := p.appliedGoal
	focus := p.appliedFocus
	advice := p.appliedAdvice
	p.shouldApply = false
	p.appliedSchedule = nil
	p.appliedGoal = ""
	p.appliedFocus = nil
	p.appliedAdvice = ""
	return sched, goal, focus, advice, true
}

// ---------------------------------------------------------------------------
// Prompt builder
// ---------------------------------------------------------------------------

func (p PlanMyDay) buildPrompt() string {
	if p.ai.IsSmallModel() {
		return p.buildSmallPrompt()
	}
	return p.buildFullPrompt()
}

// buildSmallPrompt creates a compact prompt that fits within a small model's
// ~1500 token input budget. Only the most relevant data is included.
func (p PlanMyDay) buildSmallPrompt() string {
	var b strings.Builder

	now := time.Now()
	ct := now.Format("15:04")

	b.WriteString(fmt.Sprintf("Plan a schedule from %s until 18:00. Today: %s %s.\n\n",
		ct, now.Format("2006-01-02"), now.Weekday().String()))

	// Only tasks due today/overdue + high priority — max 10
	today := now.Format("2006-01-02")
	b.WriteString("Tasks:\n")
	count := 0
	for _, t := range p.tasks {
		if t.Done {
			continue
		}
		urgent := t.DueDate != "" && t.DueDate <= today
		highPri := t.Priority >= 3
		if !urgent && !highPri {
			continue
		}
		b.WriteString(fmt.Sprintf("- %s (P%d", t.Text, t.Priority))
		if t.DueDate != "" {
			b.WriteString(", " + t.DueDate)
		}
		b.WriteString(")\n")
		count++
		if count >= 10 {
			break
		}
	}
	// Fill remaining slots with other tasks
	if count < 10 {
		for _, t := range p.tasks {
			if t.Done || (t.DueDate != "" && t.DueDate <= today) || t.Priority >= 3 {
				continue
			}
			b.WriteString(fmt.Sprintf("- %s (P%d)\n", t.Text, t.Priority))
			count++
			if count >= 10 {
				break
			}
		}
	}
	if count == 0 {
		b.WriteString("(none)\n")
	}

	// Calendar events (max 5)
	if len(p.events) > 0 {
		b.WriteString("\nEvents:\n")
		for i, ev := range p.events {
			if i >= 5 {
				break
			}
			timeStr := ev.Time
			if timeStr == "" {
				timeStr = "all-day"
			}
			b.WriteString(fmt.Sprintf("- %s %s\n", ev.Title, timeStr))
		}
	}

	b.WriteString(`
Respond EXACTLY:

TOP_GOAL: one sentence

SCHEDULE:
HH:MM-HH:MM | Task | type | project
(types: deep-work, admin, meeting, break, habit)

FOCUS_ORDER:
1. Task
2. Task
3. Task

ADVICE: one sentence
`)

	return b.String()
}

// buildFullPrompt creates the full prompt for larger models.
func (p PlanMyDay) buildFullPrompt() string {
	var b strings.Builder

	now := time.Now()
	currentTime := now.Format("15:04")
	b.WriteString(fmt.Sprintf("You are a productivity coach planning someone's day. Today is %s, %s. The current time is %s.\n\n",
		now.Format("2006-01-02"), now.Weekday().String(), currentTime))

	b.WriteString("## Context\n\n")

	// Active projects
	if len(p.projects) > 0 {
		b.WriteString("### Active Projects (by priority):\n")
		for _, proj := range p.projects {
			if proj.Status != "active" {
				continue
			}
			b.WriteString(fmt.Sprintf("- %s [%s]: %s\n", proj.Name, proj.Category, proj.Description))
		}
		b.WriteString("\n")
	}

	// Tasks due today or overdue
	today := now.Format("2006-01-02")
	b.WriteString("### Tasks Due Today or Overdue:\n")
	todayCount := 0
	for _, t := range p.tasks {
		if t.Done {
			continue
		}
		if t.DueDate == today || (t.DueDate != "" && t.DueDate <= today) {
			priLabel := priorityLabel(t.Priority)
			line := fmt.Sprintf("- %s (priority: %s", t.Text, priLabel)
			if t.DueDate != "" {
				line += ", due: " + t.DueDate
			}
			line += ")"
			b.WriteString(line + "\n")
			todayCount++
		}
	}
	if todayCount == 0 {
		b.WriteString("(none)\n")
	}
	b.WriteString("\n")

	// Upcoming tasks (next 3 days)
	threeDays := now.AddDate(0, 0, 3).Format("2006-01-02")
	b.WriteString("### Upcoming Tasks (next 3 days):\n")
	upcomingCount := 0
	for _, t := range p.tasks {
		if t.Done {
			continue
		}
		if t.DueDate > today && t.DueDate <= threeDays {
			b.WriteString(fmt.Sprintf("- %s (priority: %s, due: %s)\n",
				t.Text, priorityLabel(t.Priority), t.DueDate))
			upcomingCount++
		}
	}
	if upcomingCount == 0 {
		b.WriteString("(none)\n")
	}
	b.WriteString("\n")

	// Remaining incomplete tasks (no specific due date or future)
	b.WriteString("### Other Incomplete Tasks:\n")
	otherCount := 0
	for _, t := range p.tasks {
		if t.Done {
			continue
		}
		if t.DueDate == "" || t.DueDate > threeDays {
			b.WriteString(fmt.Sprintf("- %s (priority: %s)\n", t.Text, priorityLabel(t.Priority)))
			otherCount++
			if otherCount >= 10 {
				break
			}
		}
	}
	if otherCount == 0 {
		b.WriteString("(none)\n")
	}
	b.WriteString("\n")

	// Calendar events
	b.WriteString("### Calendar Events:\n")
	if len(p.events) > 0 {
		for _, ev := range p.events {
			dur := ev.Duration
			if dur <= 0 {
				dur = 60
			}
			timeStr := ev.Time
			if timeStr == "" {
				timeStr = "all-day"
			}
			b.WriteString(fmt.Sprintf("- %s at %s (%d min)\n", ev.Title, timeStr, dur))
		}
	} else {
		b.WriteString("(none)\n")
	}
	b.WriteString("\n")

	// Active goals
	if len(p.goals) > 0 {
		b.WriteString("### Active Goals:\n")
		for _, g := range p.goals {
			prog := ""
			if len(g.Milestones) > 0 {
				prog = fmt.Sprintf(" (%d%%)", g.Progress())
			}
			overdue := ""
			if g.IsOverdue() {
				overdue = " OVERDUE"
			}
			review := ""
			if g.IsDueForReview() {
				review = " [review due]"
			}
			b.WriteString(fmt.Sprintf("- %s%s%s%s\n", g.Title, prog, overdue, review))
		}
		b.WriteString("\n")
	}

	// Habits
	b.WriteString("### Habits to Complete:\n")
	if len(p.habits) > 0 {
		for _, h := range p.habits {
			b.WriteString(fmt.Sprintf("- %s (current streak: %d days)\n", h.Name, h.Streak))
		}
	} else {
		b.WriteString("(none)\n")
	}
	b.WriteString("\n")

	// Yesterday's unfinished
	if len(p.yesterdayTasks) > 0 {
		b.WriteString("### Yesterday's Unfinished:\n")
		for _, t := range p.yesterdayTasks {
			b.WriteString(t + "\n")
		}
		b.WriteString("\n")
	}

	// Instructions
	b.WriteString(fmt.Sprintf(`## Instructions

Create an optimized daily schedule starting from the CURRENT TIME (%s).
Do NOT schedule anything before %s — the day is already in progress.
Only plan for the remaining hours until end of day (~18:00).

Consider:
1. Start with most important/urgent tasks (eat the frog)
2. Group similar tasks together (batching)
3. Schedule deep work first, admin tasks later
4. Include breaks every 90 minutes
5. Leave buffer time for unexpected things
6. Ensure habits get done

Respond in this EXACT format:

TOP_GOAL: {one sentence - the #1 thing to accomplish today}

SCHEDULE:
HH:MM-HH:MM | Task description | type | project
HH:MM-HH:MM | Task description | type | project
...

FOCUS_ORDER:
1. Task name
2. Task name
3. Task name

ADVICE: {2-3 sentences of personalized productivity advice for today}
`, currentTime, currentTime))

	return b.String()
}

func priorityLabel(pri int) string {
	switch pri {
	case 4:
		return "highest"
	case 3:
		return "high"
	case 2:
		return "medium"
	case 1:
		return "low"
	default:
		return "none"
	}
}

// ---------------------------------------------------------------------------
// Response parser
// ---------------------------------------------------------------------------

var daySlotPattern = regexp.MustCompile(`^(\d{1,2}):(\d{2})\s*-\s*(\d{1,2}):(\d{2})\s*\|\s*(.+?)\s*\|\s*([^\|]+?)(?:\s*\|\s*(.+?))?\s*$`)

func (p *PlanMyDay) parseAIResponse(response string) {
	p.schedule = nil
	p.focusOrder = nil
	p.topGoal = ""
	p.advice = ""

	lines := strings.Split(response, "\n")
	section := "" // "schedule", "focus", "advice"

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// TOP_GOAL
		if strings.HasPrefix(trimmed, "TOP_GOAL:") {
			p.topGoal = strings.TrimSpace(strings.TrimPrefix(trimmed, "TOP_GOAL:"))
			continue
		}

		// Section headers
		if trimmed == "SCHEDULE:" {
			section = "schedule"
			continue
		}
		if trimmed == "FOCUS_ORDER:" {
			section = "focus"
			continue
		}
		if strings.HasPrefix(trimmed, "ADVICE:") {
			p.advice = strings.TrimSpace(strings.TrimPrefix(trimmed, "ADVICE:"))
			section = "advice"
			continue
		}

		// Parse based on section
		switch section {
		case "schedule":
			m := daySlotPattern.FindStringSubmatch(trimmed)
			if m == nil {
				continue
			}
			sh, err1 := strconv.Atoi(m[1])
			sm, err2 := strconv.Atoi(m[2])
			eh, err3 := strconv.Atoi(m[3])
			em, err4 := strconv.Atoi(m[4])
			if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
				continue
			}
			// Validate time ranges
			if sh > 23 || sm > 59 || eh > 23 || em > 59 {
				continue
			}
			taskName := strings.TrimSpace(m[5])
			slotType := strings.ToLower(strings.TrimSpace(m[6]))
			project := ""
			if len(m) > 7 {
				project = strings.TrimSpace(m[7])
			}

			// Match priority from original tasks
			pri := 0
			for _, t := range p.tasks {
				if strings.EqualFold(t.Text, taskName) || strings.Contains(strings.ToLower(taskName), strings.ToLower(t.Text)) {
					pri = t.Priority
					break
				}
			}

			p.schedule = append(p.schedule, daySlot{
				Start:    fmt.Sprintf("%02d:%02d", sh, sm),
				End:      fmt.Sprintf("%02d:%02d", eh, em),
				Task:     taskName,
				Type:     slotType,
				Priority: pri,
				Project:  project,
			})

		case "focus":
			// Parse "1. Task name" or "- Task name"
			cleaned := strings.TrimLeft(trimmed, "0123456789.- ")
			if cleaned != "" {
				p.focusOrder = append(p.focusOrder, cleaned)
			}

		case "advice":
			// Continuation of advice text
			if trimmed != "" && p.advice != "" {
				p.advice += " " + trimmed
			} else if trimmed != "" {
				p.advice = trimmed
			}
		}
	}

	// Filter out past time slots — keep only slots starting from now onwards.
	// If the AI returned some future slots, drop the past ones.
	// If ALL slots are in the past, keep them (test scenario / edge case).
	now := time.Now()
	currentMin := now.Hour()*60 + now.Minute()
	var futureSlots []daySlot
	for _, slot := range p.schedule {
		sh, sm := 0, 0
		_, _ = fmt.Sscanf(slot.Start, "%d:%d", &sh, &sm)
		if sh*60+sm >= currentMin-15 { // allow 15-min grace period
			futureSlots = append(futureSlots, slot)
		}
	}
	if len(futureSlots) > 0 {
		p.schedule = futureSlots
	}

	// Fall back to local if parsing failed
	if len(p.schedule) == 0 {
		p.generateLocalPlan()
		if p.advice == "" {
			p.advice = "(AI output could not be parsed — showing locally generated schedule)"
		}
	}
}

// ---------------------------------------------------------------------------
// Local fallback scheduling algorithm
// ---------------------------------------------------------------------------

func (p *PlanMyDay) generateLocalPlan() {
	p.schedule = nil
	p.focusOrder = nil

	// Sort tasks by priority (highest first), then by due date
	type scoredTask struct {
		task  Task
		score int
	}
	today := time.Now().Format("2006-01-02")

	var scored []scoredTask
	for _, t := range p.tasks {
		if t.Done {
			continue
		}
		s := t.Priority * 100
		// Boost for due today or overdue
		if t.DueDate == today {
			s += 500
		} else if t.DueDate != "" && t.DueDate < today {
			s += 600
		} else if t.DueDate != "" {
			// Within 3 days
			threeDays := time.Now().AddDate(0, 0, 3).Format("2006-01-02")
			if t.DueDate <= threeDays {
				s += 200
			}
		}
		scored = append(scored, scoredTask{task: t, score: s})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Start from current time (rounded up to next 15-min boundary), end at 18:00
	now := time.Now()
	currentMin := now.Hour()*60 + now.Minute()
	workStart := ((currentMin + 14) / 15) * 15 // round up to next 15m
	if workStart < 8*60 {
		workStart = 8 * 60 // don't start before 08:00
	}
	workEnd := 18 * 60 // 18:00 in minutes
	if workStart >= workEnd {
		// Day is over — generate a minimal "wind down" schedule
		p.schedule = []daySlot{{
			Start: fmt.Sprintf("%02d:%02d", workStart/60, workStart%60),
			End:   fmt.Sprintf("%02d:%02d", (workStart+30)/60, (workStart+30)%60),
			Task:  "Review today & plan tomorrow",
			Type:  "review",
		}}
		return
	}
	lunchStart := 12 * 60
	lunchEnd := 13 * 60
	if lunchEnd <= workStart {
		// Lunch already passed — no lunch break needed
		lunchStart = workEnd
		lunchEnd = workEnd
	}

	type timeRange struct{ start, end int }
	var occupied []timeRange

	// Place calendar events first
	for _, ev := range p.events {
		if ev.Time == "" {
			continue
		}
		parts := strings.Split(ev.Time, ":")
		if len(parts) != 2 {
			continue
		}
		h, _ := strconv.Atoi(parts[0])
		m, _ := strconv.Atoi(parts[1])
		start := h*60 + m
		dur := ev.Duration
		if dur <= 0 {
			dur = 60
		}
		end := start + dur
		occupied = append(occupied, timeRange{start, end})
		p.schedule = append(p.schedule, daySlot{
			Start:    fmt.Sprintf("%02d:%02d", start/60, start%60),
			End:      fmt.Sprintf("%02d:%02d", end/60, end%60),
			Task:     ev.Title,
			Type:     "meeting",
			Priority: 0,
		})
	}

	// Place lunch
	occupied = append(occupied, timeRange{lunchStart, lunchEnd})
	p.schedule = append(p.schedule, daySlot{
		Start: "12:00",
		End:   "13:00",
		Task:  "Lunch",
		Type:  "break",
	})

	isOccupied := func(start, end int) bool {
		for _, o := range occupied {
			if start < o.end && end > o.start {
				return true
			}
		}
		return false
	}

	findSlot := func(duration int) (int, bool) {
		for pos := workStart; pos+duration <= workEnd; pos += 5 {
			if !isOccupied(pos, pos+duration) {
				return pos, true
			}
		}
		return 0, false
	}

	workMinsSinceBreak := 0

	for i, st := range scored {
		if i >= 12 {
			break // Don't overschedule
		}

		// Break every 90 min
		if workMinsSinceBreak >= 90 {
			breakStart, found := findSlot(15)
			if found {
				occupied = append(occupied, timeRange{breakStart, breakStart + 15})
				p.schedule = append(p.schedule, daySlot{
					Start: fmt.Sprintf("%02d:%02d", breakStart/60, breakStart%60),
					End:   fmt.Sprintf("%02d:%02d", (breakStart+15)/60, (breakStart+15)%60),
					Task:  "Break",
					Type:  "break",
				})
				workMinsSinceBreak = 0
			}
		}

		// Determine slot type and duration
		var dur int
		slotType := "deep-work"
		if i >= len(scored)/2 {
			slotType = "admin"
		}
		// Longer blocks for high priority
		if st.task.Priority >= 3 {
			dur = 90
		} else if st.task.Priority >= 2 {
			dur = 60
		} else {
			dur = 30
		}

		taskStart, found := findSlot(dur)
		if !found {
			// Try a shorter slot
			dur = 30
			taskStart, found = findSlot(dur)
			if !found {
				continue
			}
		}

		occupied = append(occupied, timeRange{taskStart, taskStart + dur})

		// Find project for this task
		project := ""
		for _, proj := range p.projects {
			if proj.Status != "active" {
				continue
			}
			if proj.TaskFilter != "" && strings.Contains(strings.ToLower(st.task.Text), strings.ToLower(proj.TaskFilter)) {
				project = proj.Name
				break
			}
			for _, tag := range proj.Tags {
				if strings.Contains(strings.ToLower(st.task.Text), strings.ToLower(tag)) {
					project = proj.Name
					break
				}
			}
		}

		p.schedule = append(p.schedule, daySlot{
			Start:    fmt.Sprintf("%02d:%02d", taskStart/60, taskStart%60),
			End:      fmt.Sprintf("%02d:%02d", (taskStart+dur)/60, (taskStart+dur)%60),
			Task:     st.task.Text,
			Type:     slotType,
			Priority: st.task.Priority,
			Project:  project,
		})

		workMinsSinceBreak += dur
	}

	// Reserve time for habits at end of day
	if len(p.habits) > 0 {
		habitStart, found := findSlot(30)
		if found {
			occupied = append(occupied, timeRange{habitStart, habitStart + 30})
			habitNames := make([]string, 0, len(p.habits))
			for _, h := range p.habits {
				habitNames = append(habitNames, h.Name)
			}
			p.schedule = append(p.schedule, daySlot{
				Start: fmt.Sprintf("%02d:%02d", habitStart/60, habitStart%60),
				End:   fmt.Sprintf("%02d:%02d", (habitStart+30)/60, (habitStart+30)%60),
				Task:  "Habits: " + strings.Join(habitNames, ", "),
				Type:  "habit",
			})
		}
	}

	// End-of-day review
	reviewStart, found := findSlot(30)
	if found {
		p.schedule = append(p.schedule, daySlot{
			Start: fmt.Sprintf("%02d:%02d", reviewStart/60, reviewStart%60),
			End:   fmt.Sprintf("%02d:%02d", (reviewStart+30)/60, (reviewStart+30)%60),
			Task:  "Daily review",
			Type:  "review",
		})
	}

	// Sort schedule by start time
	sort.SliceStable(p.schedule, func(i, j int) bool {
		return p.schedule[i].Start < p.schedule[j].Start
	})

	// Set top goal = highest priority task
	if len(scored) > 0 {
		p.topGoal = scored[0].task.Text
	} else {
		p.topGoal = "Clear your inbox and plan ahead"
	}

	// Focus order = top 5 by priority
	for i := 0; i < len(scored) && i < 5; i++ {
		p.focusOrder = append(p.focusOrder, scored[i].task.Text)
	}

	// Generic advice
	taskCount := len(scored)
	switch {
	case taskCount == 0:
		p.advice = "You have no pending tasks. Use this time for deep thinking, learning, or working on long-term projects."
	case taskCount <= 3:
		p.advice = "Light day ahead. Focus on quality over quantity. Consider using extra time for skill development or strategic planning."
	case taskCount <= 7:
		p.advice = "Balanced workload today. Start with your highest-priority task before checking messages. Take breaks between deep work sessions."
	default:
		p.advice = fmt.Sprintf("You have %d tasks competing for attention. Focus on the top 3 and defer the rest. Multitasking is the enemy of progress.", taskCount)
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (p PlanMyDay) Update(msg tea.Msg) (PlanMyDay, tea.Cmd) {
	if !p.active {
		return p, nil
	}

	switch msg := msg.(type) {
	case planMyDayGatherMsg:
		if p.phase == 0 {
			// Auto-advance to planning phase
			return p.startPlanning()
		}

	case planMyDayTickMsg:
		if p.phase == 0 || p.phase == 1 {
			p.loadingTick++
			p.spinnerTick++
			return p, planMyDayTickCmd()
		}

	case streamChunkMsg:
		if msg.tag == "planmyday" && p.streaming {
			p.streamBuf.WriteString(msg.text)
			return p, streamCmd(p.streamCh)
		}

	case streamDoneMsg:
		if msg.tag == "planmyday" && p.streaming {
			p.streaming = false
			if p.streamCancel != nil {
				p.streamCancel()
				p.streamCancel = nil
			}
			if msg.err != nil {
				p.generateLocalPlan()
			} else {
				p.parseAIResponse(p.streamBuf.String())
			}
			p.phase = 2
			p.scroll = 0
			return p, nil
		}

	case planMyDayResultMsg:
		// Legacy: used by local fallback if ever reached
		if p.phase != 1 {
			return p, nil
		}
		if msg.err != nil {
			p.generateLocalPlan()
		} else {
			p.parseAIResponse(msg.response)
		}
		p.phase = 2
		p.scroll = 0
		return p, nil

	case tea.KeyMsg:
		switch p.phase {
		case 0:
			// Gathering phase — only Esc to cancel
			if msg.String() == "esc" {
				p.active = false
			}
			return p, nil

		case 1:
			// Planning phase — Esc cancels the in-flight stream and closes.
			if msg.String() == "esc" {
				if p.streamCancel != nil {
					p.streamCancel()
					p.streamCancel = nil
				}
				p.streaming = false
				p.streamCh = nil
				p.active = false
			}
			return p, nil

		case 2:
			return p.updateResult(msg)

		case 3:
			if msg.String() == "enter" || msg.String() == "esc" {
				p.active = false
			}
			return p, nil
		}
	}
	return p, nil
}

func (p PlanMyDay) startPlanning() (PlanMyDay, tea.Cmd) {
	useAI := p.ai.Provider != "" && p.ai.Provider != "local"

	if useAI {
		p.phase = 1
		p.spinnerTick = 0
		p.streaming = true
		p.streamBuf.Reset()

		systemPrompt := "You are a productivity coach that creates optimized daily schedules. Always respond in the exact format requested."
		if p.ai.IsSmallModel() {
			systemPrompt = "Create a daily schedule. Use the exact format requested."
		}
		userPrompt := p.buildPrompt()

		p.streamCh, p.streamCancel = sendToAIStreamingCtx(context.Background(), p.ai, systemPrompt, userPrompt, "planmyday")
		return p, tea.Batch(streamCmd(p.streamCh), planMyDayTickCmd())
	}

	// Local fallback
	p.generateLocalPlan()
	p.phase = 2
	p.scroll = 0
	return p, nil
}

func (p PlanMyDay) updateResult(msg tea.KeyMsg) (PlanMyDay, tea.Cmd) {
	switch msg.String() {
	case "esc":
		p.active = false
	case "up", "k":
		if p.scroll > 0 {
			p.scroll--
		}
	case "down", "j":
		p.scroll++
		// The View function will clamp, but prevent excessive growth
		if p.scroll > 200 {
			p.scroll = 200
		}
	case "enter":
		// Apply to daily note
		p.shouldApply = true
		p.appliedSchedule = make([]daySlot, len(p.schedule))
		copy(p.appliedSchedule, p.schedule)
		p.appliedGoal = p.topGoal
		p.appliedFocus = make([]string, len(p.focusOrder))
		copy(p.appliedFocus, p.focusOrder)
		p.appliedAdvice = p.advice
		p.phase = 3
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (p PlanMyDay) View() string {
	if !p.active {
		return ""
	}

	width := p.overlayWidth()

	switch p.phase {
	case 0:
		return p.viewGathering(width)
	case 1:
		return p.viewPlanning(width)
	case 2:
		return p.viewResult(width)
	case 3:
		return p.viewApplied(width)
	}
	return ""
}

func (p PlanMyDay) overlayWidth() int {
	w := p.width * 2 / 3
	if w < 60 {
		w = 60
	}
	if w > 100 {
		w = 100
	}
	return w
}

func (p PlanMyDay) innerWidth() int {
	return p.overlayWidth() - 6
}

func (p PlanMyDay) titleBar(width int) string {
	now := time.Now()
	dateStr := now.Format("Monday, January 2")
	title := lipgloss.NewStyle().Foreground(mauve).Bold(true).
		Render("  " + IconBotChar + " Plan My Day")
	dateBadge := lipgloss.NewStyle().Foreground(teal).Render(" " + dateStr)
	sep := DimStyle.Render("  " + strings.Repeat(ThemeSeparator, width-10))
	return title + dateBadge + "\n" + sep + "\n"
}

// ---------------------------------------------------------------------------
// View: gathering phase (phase 0)
// ---------------------------------------------------------------------------

func (p PlanMyDay) viewGathering(width int) string {
	var b strings.Builder
	b.WriteString(p.titleBar(width))
	b.WriteString("\n")

	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinner := lipgloss.NewStyle().Foreground(mauve).Render(frames[p.loadingTick%len(frames)])

	elapsed := time.Since(p.loadingStart).Truncate(time.Second)
	b.WriteString("  " + spinner + lipgloss.NewStyle().Foreground(text).Bold(true).Render(fmt.Sprintf(" Gathering your data... (%s)", elapsed)))
	b.WriteString("\n\n")

	// Show what's being collected
	items := []struct {
		icon  string
		label string
		count int
	}{
		{IconCalendarChar, "Tasks", len(p.tasks)},
		{IconCalendarChar, "Calendar events", len(p.events)},
		{IconGraphChar, "Habits", len(p.habits)},
		{IconFolderChar, "Projects", len(p.projects)},
	}

	for i, item := range items {
		check := lipgloss.NewStyle().Foreground(green).Render("[x]")
		if i > p.loadingTick/2 {
			check = lipgloss.NewStyle().Foreground(overlay0).Render("[ ]")
		}
		label := lipgloss.NewStyle().Foreground(text).Render(item.label)
		count := DimStyle.Render(fmt.Sprintf(" (%d)", item.count))
		b.WriteString(fmt.Sprintf("  %s %s %s%s\n", check, item.icon, label, count))
	}

	if len(p.yesterdayTasks) > 0 {
		check := lipgloss.NewStyle().Foreground(green).Render("[x]")
		if 4 > p.loadingTick/2 {
			check = lipgloss.NewStyle().Foreground(overlay0).Render("[ ]")
		}
		b.WriteString(fmt.Sprintf("  %s %s %s\n", check, IconEditChar,
			lipgloss.NewStyle().Foreground(text).Render(fmt.Sprintf("Yesterday's carry-forward (%d)", len(p.yesterdayTasks)))))
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Esc: cancel"))

	return p.wrapBorder(b.String(), width)
}

// ---------------------------------------------------------------------------
// View: planning phase (phase 1)
// ---------------------------------------------------------------------------

func (p PlanMyDay) viewPlanning(width int) string {
	var b strings.Builder
	b.WriteString(p.titleBar(width))
	b.WriteString("\n")

	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinner := lipgloss.NewStyle().Foreground(mauve).Render(frames[p.spinnerTick%len(frames)])

	providerLabel := "local algorithm"
	switch p.ai.Provider {
	case "ollama":
		providerLabel = "Ollama (" + p.ai.Model + ")"
	case "openai":
		providerLabel = "OpenAI (" + p.ai.Model + ")"
	}

	elapsed := time.Since(p.loadingStart).Truncate(time.Second)
	b.WriteString("  " + spinner + lipgloss.NewStyle().Foreground(yellow).Bold(true).
		Render(fmt.Sprintf(" Creating your optimal schedule... (%s)", elapsed)))
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(fmt.Sprintf("  Using %s", providerLabel)))
	b.WriteString("\n")

	// Summary of data
	taskCount := 0
	for _, t := range p.tasks {
		if !t.Done {
			taskCount++
		}
	}
	b.WriteString(DimStyle.Render(fmt.Sprintf("  Analyzing %d tasks, %d events, %d habits",
		taskCount, len(p.events), len(p.habits))))
	b.WriteString("\n\n")

	renderStreamPreview(&b, p.streamBuf.String(), p.height, width)

	b.WriteString(DimStyle.Render("  Esc: cancel"))

	return p.wrapBorder(b.String(), width)
}

// ---------------------------------------------------------------------------
// View: result phase (phase 2)
// ---------------------------------------------------------------------------

func (p PlanMyDay) viewResult(width int) string {
	var lines []string
	innerW := width - 6

	// Title
	lines = append(lines, p.titleBar(width))

	// Top goal
	if p.topGoal != "" {
		goalIcon := lipgloss.NewStyle().Foreground(red).Bold(true).Render("  TARGET ")
		goalText := lipgloss.NewStyle().Foreground(text).Bold(true).Render(p.topGoal)
		lines = append(lines, goalIcon+goalText)
		lines = append(lines, "")
	}

	// Clocked sessions section (if any)
	if len(p.clockedSessions) > 0 {
		clockTitle := lipgloss.NewStyle().Foreground(teal).Bold(true).Render("  CLOCKED TIME")
		lines = append(lines, clockTitle)
		lines = append(lines, DimStyle.Render("  "+strings.Repeat(ThemeSeparator, innerW-4)))

		var totalClocked time.Duration
		for _, cs := range p.clockedSessions {
			timeStr := lipgloss.NewStyle().Foreground(teal).Render(cs.Start + "-" + cs.End)
			projStr := lipgloss.NewStyle().Foreground(sapphire).Render(cs.Project)
			durStr := DimStyle.Render(" (" + cs.FormatDuration() + ")")
			activeTag := ""
			if cs.Active {
				activeTag = lipgloss.NewStyle().Foreground(green).Bold(true).Render(" ● active")
			}
			lines = append(lines, "  "+timeStr+"  "+projStr+durStr+activeTag)
			totalClocked += cs.Duration
		}

		h := int(totalClocked.Hours())
		m := int(totalClocked.Minutes()) % 60
		totalStr := fmt.Sprintf("%dh %02dm", h, m)
		lines = append(lines, DimStyle.Render(fmt.Sprintf("  Total clocked: %s", totalStr)))
		lines = append(lines, "")
	}

	// Schedule section
	schedTitle := lipgloss.NewStyle().Foreground(blue).Bold(true).Render("  SCHEDULE")
	lines = append(lines, schedTitle)
	lines = append(lines, DimStyle.Render("  "+strings.Repeat(ThemeSeparator, innerW-4)))

	for _, slot := range p.schedule {
		timeStr := lipgloss.NewStyle().Foreground(teal).Render(slot.Start + "-" + slot.End)

		// Type icon and color
		var typeIcon string
		var taskStyle lipgloss.Style
		switch slot.Type {
		case "deep-work":
			typeIcon = daySlotPriorityIcon(slot.Priority)
			taskStyle = lipgloss.NewStyle().Foreground(text).Bold(true)
		case "admin":
			typeIcon = lipgloss.NewStyle().Foreground(blue).Render("  ")
			taskStyle = lipgloss.NewStyle().Foreground(text)
		case "meeting":
			typeIcon = lipgloss.NewStyle().Foreground(lavender).Render(IconCalendarChar + " ")
			taskStyle = lipgloss.NewStyle().Foreground(lavender)
		case "break":
			typeIcon = lipgloss.NewStyle().Foreground(green).Render("  ")
			taskStyle = lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		case "habit":
			typeIcon = lipgloss.NewStyle().Foreground(green).Render(IconGraphChar + " ")
			taskStyle = lipgloss.NewStyle().Foreground(green)
		case "review":
			typeIcon = lipgloss.NewStyle().Foreground(peach).Render(IconEditChar + " ")
			taskStyle = lipgloss.NewStyle().Foreground(peach)
		default:
			typeIcon = "  "
			taskStyle = lipgloss.NewStyle().Foreground(text)
		}

		taskName := slot.Task
		maxNameLen := innerW - 28
		if maxNameLen < 10 {
			maxNameLen = 10
		}
		if r := []rune(taskName); len(r) > maxNameLen {
			taskName = string(r[:maxNameLen-3]) + "..."
		}

		typeTag := DimStyle.Render(" [" + slot.Type + "]")
		projTag := ""
		if slot.Project != "" {
			projTag = lipgloss.NewStyle().Foreground(sapphire).Render(" " + slot.Project)
		}

		line := "  " + timeStr + "  " + typeIcon + taskStyle.Render(taskName) + typeTag + projTag
		lines = append(lines, line)
	}

	lines = append(lines, "")

	// Focus order
	if len(p.focusOrder) > 0 {
		focusTitle := lipgloss.NewStyle().Foreground(peach).Bold(true).Render("  FOCUS ORDER")
		lines = append(lines, focusTitle)
		lines = append(lines, DimStyle.Render("  "+strings.Repeat(ThemeSeparator, innerW-4)))
		for i, item := range p.focusOrder {
			num := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(fmt.Sprintf("  %d. ", i+1))
			lines = append(lines, num+lipgloss.NewStyle().Foreground(text).Render(item))
		}
		lines = append(lines, "")
	}

	// Advice
	if p.advice != "" {
		adviceTitle := lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("  ADVICE")
		lines = append(lines, adviceTitle)
		lines = append(lines, DimStyle.Render("  "+strings.Repeat(ThemeSeparator, innerW-4)))
		// Word-wrap the advice text
		wrapped := planWrap(p.advice, innerW-4)
		for _, wl := range strings.Split(wrapped, "\n") {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(text).Italic(true).Render(wl))
		}
		lines = append(lines, "")
	}

	// Footer
	lines = append(lines, DimStyle.Render("  "+strings.Repeat(ThemeSeparator, innerW-4)))
	lines = append(lines, DimStyle.Render("  Enter: apply to daily note  j/k: scroll  Esc: close"))

	// Scrollable output
	visH := p.height - 8
	if visH < 10 {
		visH = 10
	}
	maxScroll := len(lines) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := p.scroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	end := scroll + visH
	if end > len(lines) {
		end = len(lines)
	}

	var b strings.Builder
	for i := scroll; i < end; i++ {
		b.WriteString(lines[i])
		b.WriteString("\n")
	}

	return p.wrapBorder(b.String(), width)
}

// ---------------------------------------------------------------------------
// View: applied phase (phase 3)
// ---------------------------------------------------------------------------

func (p PlanMyDay) viewApplied(width int) string {
	var b strings.Builder
	b.WriteString(p.titleBar(width))
	b.WriteString("\n")

	b.WriteString(lipgloss.NewStyle().Foreground(green).Bold(true).
		Render("  Day plan applied to daily note!"))
	b.WriteString("\n\n")

	taskCount := 0
	for _, s := range p.schedule {
		if s.Type != "break" {
			taskCount++
		}
	}
	b.WriteString(lipgloss.NewStyle().Foreground(text).
		Render(fmt.Sprintf("  %d items scheduled", taskCount)))
	b.WriteString("\n")

	if p.topGoal != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(teal).
			Render(fmt.Sprintf("  Top goal: %s", p.topGoal)))
		b.WriteString("\n")
	}

	if len(p.schedule) > 0 {
		b.WriteString(DimStyle.Render(fmt.Sprintf("  %s - %s",
			p.schedule[0].Start, p.schedule[len(p.schedule)-1].End)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter/Esc: close"))

	return p.wrapBorder(b.String(), width)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (p PlanMyDay) wrapBorder(content string, width int) string {
	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)
	return border.Render(content)
}

func daySlotPriorityIcon(priority int) string {
	switch priority {
	case 4:
		return lipgloss.NewStyle().Foreground(red).Bold(true).Render("!! ")
	case 3:
		return lipgloss.NewStyle().Foreground(peach).Bold(true).Render("!  ")
	case 2:
		return lipgloss.NewStyle().Foreground(yellow).Render("-  ")
	default:
		return "   "
	}
}

// planWrap breaks text into lines of at most maxWidth characters.
func planWrap(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return text
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	var lines []string
	currentLine := words[0]
	for _, word := range words[1:] {
		if len(currentLine)+1+len(word) <= maxWidth {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}
	lines = append(lines, currentLine)
	return strings.Join(lines, "\n")
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
