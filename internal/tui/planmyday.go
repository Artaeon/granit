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

	// If today's plan was already generated (this session or a previous
	// one), load it from Planner/{date}.md and jump straight to the
	// result phase. The user can then review, edit blocks via the
	// calendar, or regenerate from scratch — no need to burn AI tokens
	// on every overlay re-open.
	if p.loadExistingPlan() {
		p.phase = 2
		return planMyDayTickCmd()
	}

	// Start gathering animation, auto-advance after delay
	return tea.Batch(planMyDayGatherCmd(), planMyDayTickCmd())
}

// loadExistingPlan populates p.schedule / p.topGoal / p.focusOrder from
// today's Planner file if a non-empty schedule is on disk. Returns true
// when a plan was loaded. Used by Open to skip the gather→AI dance when
// the user is just re-visiting an already-applied plan.
func (p *PlanMyDay) loadExistingPlan() bool {
	if p.vaultRoot == "" {
		return false
	}
	date := time.Now().Format("2006-01-02")
	blocks := readPlannerScheduleBlocks(p.vaultRoot, date)
	if len(blocks) == 0 {
		return false
	}
	p.schedule = make([]daySlot, 0, len(blocks))
	for _, b := range blocks {
		p.schedule = append(p.schedule, daySlot{
			Start: b.StartTime,
			End:   b.EndTime,
			Task:  b.Text,
			Type:  string(b.BlockType),
		})
	}
	// Pull focus / top goal from the same file. Reuse the directory-scan
	// loader since it already parses ## Focus into DailyFocus.
	if _, focusByDate := loadPlannerBlocks(p.vaultRoot); focusByDate != nil {
		if f, ok := focusByDate[date]; ok {
			p.topGoal = f.TopGoal
			p.focusOrder = f.FocusItems
		}
	}
	return true
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

	b.WriteString(fmt.Sprintf("Plan a schedule from %s until 22:00. Today: %s %s.\n\n",
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
Plan for the remaining hours until 22:00 (include evening habits, review, wind-down).

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

	// Build task list sorted by priority/urgency score
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
		if t.DueDate == today {
			s += 500
		} else if t.DueDate != "" && t.DueDate < today {
			s += 600
		} else if t.DueDate != "" {
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

	// Build ScheduleTask list for the shared generator
	var scheduleTasks []ScheduleTask
	for i, st := range scored {
		if i >= 12 {
			break
		}
		est := st.task.EstimatedMinutes
		if est <= 0 {
			switch {
			case st.task.Priority >= 3:
				est = 90
			case st.task.Priority >= 2:
				est = 60
			default:
				est = 30
			}
		}
		scheduleTasks = append(scheduleTasks, ScheduleTask{
			Text: st.task.Text, Priority: st.task.Priority, Estimate: est,
		})
	}

	// Gather habit names
	var habitNames []string
	for _, h := range p.habits {
		habitNames = append(habitNames, h.Name)
	}

	// Use the shared schedule generator
	p.schedule = GenerateLocalSchedule(ScheduleInput{
		Tasks:    scheduleTasks,
		Events:   p.events,
		Habits:   habitNames,
		Projects: p.projects,
		WorkEnd:  22 * 60,
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
			// Gathering phase — Esc/q to cancel
			if msg.String() == "esc" || msg.String() == "q" {
				p.active = false
			}
			return p, nil

		case 1:
			// Planning phase — Esc/q cancels the in-flight stream and closes.
			if msg.String() == "esc" || msg.String() == "q" {
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
			if msg.String() == "enter" || msg.String() == "esc" || msg.String() == "q" {
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
	case "esc", "q":
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
	case "r", "R":
		// Regenerate: drop the cached plan and re-run the AI flow.
		// Useful when the user reviews a stale plan loaded from disk.
		p.schedule = nil
		p.focusOrder = nil
		p.topGoal = ""
		p.advice = ""
		p.phase = 0
		p.scroll = 0
		p.loadingTick = 0
		p.spinnerTick = 0
		p.loadingStart = time.Now()
		return p, tea.Batch(planMyDayGatherCmd(), planMyDayTickCmd())
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

	// ── Top goal ───────────────────────────────────────────────────────────
	if p.topGoal != "" {
		goalPill := makePill(red, "TARGET")
		goalText := lipgloss.NewStyle().Foreground(text).Bold(true).Render(" " + p.topGoal)
		lines = append(lines, "  "+goalPill+goalText)
		lines = append(lines, "")
	}

	// ── Day summary bar ────────────────────────────────────────────────────
	taskCount := 0
	deepWorkMins := 0
	breakMins := 0
	meetingMins := 0
	for _, s := range p.schedule {
		startMin := slotToMinutes(s.Start)
		endMin := slotToMinutes(s.End)
		dur := endMin - startMin
		if dur < 0 {
			dur = 0
		}
		switch s.Type {
		case "break":
			breakMins += dur
		case "meeting":
			meetingMins += dur
			taskCount++
		default:
			deepWorkMins += dur
			taskCount++
		}
	}
	var summaryParts []string
	summaryParts = append(summaryParts, lipgloss.NewStyle().Foreground(blue).Bold(true).
		Render(fmt.Sprintf("%d items", taskCount)))
	if deepWorkMins > 0 {
		summaryParts = append(summaryParts, lipgloss.NewStyle().Foreground(peach).
			Render(fmt.Sprintf("%dh%02dm work", deepWorkMins/60, deepWorkMins%60)))
	}
	if meetingMins > 0 {
		summaryParts = append(summaryParts, lipgloss.NewStyle().Foreground(lavender).
			Render(fmt.Sprintf("%dh%02dm meetings", meetingMins/60, meetingMins%60)))
	}
	if breakMins > 0 {
		breakLabel := fmt.Sprintf("%dm breaks", breakMins)
		if breakMins >= 60 {
			breakLabel = fmt.Sprintf("%dh%02dm breaks", breakMins/60, breakMins%60)
		}
		summaryParts = append(summaryParts, lipgloss.NewStyle().Foreground(green).
			Render(breakLabel))
	}
	lines = append(lines, "  "+strings.Join(summaryParts, DimStyle.Render(" · ")))
	lines = append(lines, "")

	// ── Clocked sessions ───────────────────────────────────────────────────
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

	// ── Focus order ────────────────────────────────────────────────────────
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

	// ── Advice ─────────────────────────────────────────────────────────────
	if p.advice != "" {
		adviceTitle := lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("  ADVICE")
		lines = append(lines, adviceTitle)
		lines = append(lines, DimStyle.Render("  "+strings.Repeat(ThemeSeparator, innerW-4)))
		wrapped := planWrap(p.advice, innerW-4)
		for _, wl := range strings.Split(wrapped, "\n") {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(text).Italic(true).Render(wl))
		}
		lines = append(lines, "")
	}

	// ── DAILY TIMELINE ─────────────────────────────────────────────────────
	// The crown jewel: a visual half-hour timeline from now until 22:00 showing
	// every scheduled block as colored bars, synced with tasks, events, goals.
	if len(p.schedule) > 0 {
	timelineTitle := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  DAILY TIMELINE")
	lines = append(lines, timelineTitle)
	lines = append(lines, DimStyle.Render("  "+strings.Repeat(ThemeSeparator, innerW-4)))

	now := time.Now()
	nowMins := now.Hour()*60 + now.Minute()
	// Start from current hour (or schedule start), end at 22:00
	timelineStart := (nowMins / 60) * 60 // round down to hour
	if len(p.schedule) > 0 {
		firstSlotMin := slotToMinutes(p.schedule[0].Start)
		if firstSlotMin < timelineStart {
			timelineStart = (firstSlotMin / 60) * 60
		}
	}
	if timelineStart < 6*60 {
		timelineStart = 6 * 60
	}
	timelineEnd := 22 * 60 // 22:00

	contentW := innerW - 12
	if contentW < 20 {
		contentW = 20
	}

	nowLineDrawn := false
	for mins := timelineStart; mins < timelineEnd; mins += 30 {
		hour := mins / 60
		half := mins % 60
		isTopHalf := half == 0

		// Time label
		var timeSt string
		isNowSlot := nowMins >= mins && nowMins < mins+30
		if isTopHalf {
			if isNowSlot {
				timeSt = lipgloss.NewStyle().Foreground(green).Bold(true).
					Render(fmt.Sprintf("  ▸%02d:%02d ", now.Hour(), now.Minute()))
			} else {
				timeSt = DimStyle.Render(fmt.Sprintf("  %02d:00  ", hour))
			}
		} else {
			if isNowSlot {
				timeSt = lipgloss.NewStyle().Foreground(green).Bold(true).
					Render(fmt.Sprintf("  ▸%02d:%02d ", now.Hour(), now.Minute()))
			} else {
				timeSt = DimStyle.Render("     :30  ")
			}
		}

		// Find which schedule slot covers this half-hour
		var activeSlot *daySlot
		overlapCount := 0
		for i := range p.schedule {
			s := &p.schedule[i]
			sMin := slotToMinutes(s.Start)
			eMin := slotToMinutes(s.End)
			if sMin < mins+30 && eMin > mins {
				overlapCount++
				if activeSlot == nil {
					activeSlot = s
				}
			}
		}

		if activeSlot != nil {
			sMin := slotToMinutes(activeSlot.Start)
			isStart := sMin >= mins && sMin < mins+30

			// Pick color by type
			var blockColor lipgloss.Color
			switch activeSlot.Type {
			case "deep-work":
				blockColor = blue
			case "admin":
				blockColor = sapphire
			case "meeting":
				blockColor = lavender
			case "break":
				blockColor = green
			case "habit":
				blockColor = teal
			case "review":
				blockColor = peach
			default:
				blockColor = blue
			}

			inner := contentW - 2
			if inner < 4 {
				inner = 4
			}

			var label string
			if isStart {
				timeRange := activeSlot.Start + "–" + activeSlot.End
				label = timeRange + "  " + activeSlot.Task
				if activeSlot.Project != "" {
					label += "  [" + activeSlot.Project + "]"
				}
				if overlapCount > 1 {
					label = TruncateDisplay(label, inner-4) + fmt.Sprintf(" +%d", overlapCount-1)
				}
			} else {
				// Continuation bar
				label = "▏"
			}

			label = TruncateDisplay(label, inner)
			padLen := inner - lipgloss.Width(label)
			if padLen < 0 {
				padLen = 0
			}
			label += strings.Repeat(" ", padLen)

			blockStyle := lipgloss.NewStyle().Foreground(crust).Background(blockColor)
			if isStart {
				blockStyle = blockStyle.Bold(true)
			}
			lines = append(lines, timeSt+blockStyle.Render(" "+label+" "))
		} else if isTopHalf {
			lines = append(lines, timeSt+DimStyle.Render("┊"))
		} else {
			lines = append(lines, timeSt+DimStyle.Render("·"))
		}

		// Current-time indicator
		if !nowLineDrawn && isNowSlot {
			nowLineDrawn = true
			nowLabel := lipgloss.NewStyle().Foreground(red).Bold(true).
				Render(fmt.Sprintf("  %02d:%02d  ", now.Hour(), now.Minute()))
			nowLine := lipgloss.NewStyle().Foreground(red).Bold(true).
				Render(strings.Repeat("━", contentW))
			lines = append(lines, nowLabel+nowLine)
		}
	}

	// Timeline legend
	legend := "  " +
		lipgloss.NewStyle().Foreground(blue).Render("█") + " work " +
		lipgloss.NewStyle().Foreground(sapphire).Render("█") + " admin " +
		lipgloss.NewStyle().Foreground(lavender).Render("█") + " meeting " +
		lipgloss.NewStyle().Foreground(green).Render("█") + " break " +
		lipgloss.NewStyle().Foreground(teal).Render("█") + " habit " +
		lipgloss.NewStyle().Foreground(peach).Render("█") + " review"
	lines = append(lines, legend)
	lines = append(lines, "")
	} // end if len(p.schedule) > 0

	// ── Project breakdown ──────────────────────────────────────────────────
	projectMins := make(map[string]int)
	for _, s := range p.schedule {
		if s.Project == "" || s.Type == "break" {
			continue
		}
		dur := slotToMinutes(s.End) - slotToMinutes(s.Start)
		if dur > 0 {
			projectMins[s.Project] += dur
		}
	}
	if len(projectMins) > 0 {
		projTitle := lipgloss.NewStyle().Foreground(sapphire).Bold(true).Render("  PROJECTS")
		lines = append(lines, projTitle)
		lines = append(lines, DimStyle.Render("  "+strings.Repeat(ThemeSeparator, innerW-4)))

		// Sort by time descending
		type projEntry struct {
			name string
			mins int
		}
		var projList []projEntry
		for name, mins := range projectMins {
			projList = append(projList, projEntry{name, mins})
		}
		sort.Slice(projList, func(i, j int) bool {
			return projList[i].mins > projList[j].mins
		})
		for _, pe := range projList {
			barW := 10
			maxMins := projList[0].mins
			filled := barW * pe.mins / maxMins
			bar := lipgloss.NewStyle().Foreground(sapphire).Render(strings.Repeat("█", filled)) +
				lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat("░", barW-filled))
			durStr := ""
			if pe.mins >= 60 {
				durStr = fmt.Sprintf("%dh%02dm", pe.mins/60, pe.mins%60)
			} else {
				durStr = fmt.Sprintf("%dm", pe.mins)
			}
			lines = append(lines, "  "+bar+" "+
				lipgloss.NewStyle().Foreground(text).Render(pe.name)+" "+
				DimStyle.Render(durStr))
		}
		lines = append(lines, "")
	}

	// ── Active goals ───────────────────────────────────────────────────────
	if len(p.goals) > 0 {
		goalsTitle := lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("  GOALS")
		lines = append(lines, goalsTitle)
		lines = append(lines, DimStyle.Render("  "+strings.Repeat(ThemeSeparator, innerW-4)))
		for i, g := range p.goals {
			if i >= 5 {
				lines = append(lines, DimStyle.Render(fmt.Sprintf("  +%d more", len(p.goals)-5)))
				break
			}
			prog := g.Progress()
			barW := 6
			filled := barW * prog / 100
			bar := lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("█", filled)) +
				lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat("░", barW-filled))
			progStr := fmt.Sprintf("%3d%%", prog)
			title := TruncateDisplay(g.Title, innerW-20)
			lines = append(lines, "  "+bar+" "+lipgloss.NewStyle().Foreground(overlay0).Render(progStr)+" "+
				lipgloss.NewStyle().Foreground(text).Render(title))
		}
		lines = append(lines, "")
	}

	// ── Footer ─────────────────────────────────────────────────────────────
	lines = append(lines, DimStyle.Render("  "+strings.Repeat(ThemeSeparator, innerW-4)))
	lines = append(lines, DimStyle.Render("  Enter: apply  R: regenerate  j/k: scroll  Esc: close"))

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

// slotToMinutes and FormatDayPlanMarkdown are now in planner_io.go / schedule.go.

// ---------------------------------------------------------------------------
// View: applied phase (phase 3)
// ---------------------------------------------------------------------------

func (p PlanMyDay) viewApplied(width int) string {
	var b strings.Builder
	b.WriteString(p.titleBar(width))
	b.WriteString("\n")

	// Success banner
	successPill := makePill(green, "APPLIED")
	b.WriteString("  " + successPill + lipgloss.NewStyle().Foreground(green).Bold(true).Render(" Day plan written to daily note!"))
	b.WriteString("\n\n")

	// Stats
	taskCount := 0
	totalMins := 0
	for _, s := range p.schedule {
		if s.Type != "break" {
			taskCount++
		}
		dur := slotToMinutes(s.End) - slotToMinutes(s.Start)
		if dur > 0 {
			totalMins += dur
		}
	}
	b.WriteString(lipgloss.NewStyle().Foreground(text).
		Render(fmt.Sprintf("  %d items scheduled", taskCount)))
	if totalMins > 0 {
		b.WriteString(DimStyle.Render(fmt.Sprintf(" · %dh%02dm planned", totalMins/60, totalMins%60)))
	}
	b.WriteString("\n")

	if p.topGoal != "" {
		goalPill := makePill(peach, "GOAL")
		b.WriteString("  " + goalPill + " " + lipgloss.NewStyle().Foreground(text).Render(p.topGoal))
		b.WriteString("\n")
	}

	if len(p.schedule) > 0 {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(teal).Render(
			p.schedule[0].Start+" → "+p.schedule[len(p.schedule)-1].End))
		b.WriteString("\n")
	}

	// Show top 3 focus items
	if len(p.focusOrder) > 0 {
		b.WriteString("\n")
		b.WriteString("  " + lipgloss.NewStyle().Foreground(peach).Bold(true).Render("Focus:") + "\n")
		for i, item := range p.focusOrder {
			if i >= 3 {
				break
			}
			num := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(fmt.Sprintf("  %d. ", i+1))
			b.WriteString(num + lipgloss.NewStyle().Foreground(text).Render(TruncateDisplay(item, width-14)) + "\n")
		}
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

// FormatDayPlanMarkdown is now in schedule.go.
