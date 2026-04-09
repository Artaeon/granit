package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// hasWorkTime returns true if enough work hours remain today for a full schedule.
// Tests that assert on lunch, breaks, habits, etc. can only pass during work hours.
func hasWorkTime() bool {
	now := time.Now()
	return now.Hour()*60+now.Minute() < 16*60 // before 16:00
}

// ---------------------------------------------------------------------------
// Initialization
// ---------------------------------------------------------------------------

func TestPlanMyDayNew(t *testing.T) {
	p := NewPlanMyDay()
	if p.active {
		t.Error("expected new PlanMyDay to be inactive")
	}
	if p.phase != 0 {
		t.Errorf("expected phase 0, got %d", p.phase)
	}
	if p.scroll != 0 {
		t.Errorf("expected scroll 0, got %d", p.scroll)
	}
	if len(p.schedule) != 0 {
		t.Errorf("expected empty schedule, got %d", len(p.schedule))
	}
	if len(p.focusOrder) != 0 {
		t.Errorf("expected empty focusOrder, got %d", len(p.focusOrder))
	}
	if p.topGoal != "" {
		t.Errorf("expected empty topGoal, got %q", p.topGoal)
	}
}

// ---------------------------------------------------------------------------
// Open / Close / IsActive
// ---------------------------------------------------------------------------

func TestPlanMyDayOpenCloseIsActive(t *testing.T) {
	p := NewPlanMyDay()
	if p.IsActive() {
		t.Error("expected IsActive false before Open")
	}

	cmd := p.Open("/tmp/vault", nil, nil, nil, nil, nil, AIConfig{Provider: "local"})
	if !p.IsActive() {
		t.Error("expected IsActive true after Open")
	}
	if cmd == nil {
		t.Error("expected non-nil cmd from Open (tick + gather commands)")
	}
	if p.phase != 0 {
		t.Errorf("expected phase 0 after Open, got %d", p.phase)
	}

	p.Close()
	if p.IsActive() {
		t.Error("expected IsActive false after Close")
	}
}

func TestPlanMyDayOpenResetsState(t *testing.T) {
	p := NewPlanMyDay()
	p.shouldApply = true
	p.appliedSchedule = []daySlot{{Task: "old"}}
	p.appliedGoal = "old goal"
	p.appliedFocus = []string{"old"}
	p.appliedAdvice = "old advice"
	p.schedule = []daySlot{{Task: "leftover"}}
	p.focusOrder = []string{"leftover"}
	p.topGoal = "leftover"
	p.advice = "leftover"
	p.scroll = 10
	p.loadingTick = 5

	p.Open("/tmp", nil, nil, nil, nil, nil, AIConfig{})

	if p.shouldApply {
		t.Error("Open should reset shouldApply")
	}
	if p.appliedSchedule != nil {
		t.Error("Open should reset appliedSchedule")
	}
	if p.appliedGoal != "" {
		t.Error("Open should reset appliedGoal")
	}
	if p.appliedFocus != nil {
		t.Error("Open should reset appliedFocus")
	}
	if p.appliedAdvice != "" {
		t.Error("Open should reset appliedAdvice")
	}
	if p.schedule != nil {
		t.Error("Open should reset schedule")
	}
	if p.focusOrder != nil {
		t.Error("Open should reset focusOrder")
	}
	if p.topGoal != "" {
		t.Error("Open should reset topGoal")
	}
	if p.scroll != 0 {
		t.Error("Open should reset scroll")
	}
	if p.loadingTick != 0 {
		t.Error("Open should reset loadingTick")
	}
}

func TestPlanMyDayOpenDefaults(t *testing.T) {
	p := NewPlanMyDay()
	p.Open("/tmp", nil, nil, nil, nil, nil, AIConfig{})

	if p.ai.Provider != "local" {
		t.Errorf("expected ai.Provider 'local', got %q", p.ai.Provider)
	}
	if p.ai.OllamaURL != "http://localhost:11434" {
		t.Errorf("expected default OllamaURL, got %q", p.ai.OllamaURL)
	}
	if p.ai.Model != "qwen2.5:0.5b" {
		t.Errorf("expected default Model, got %q", p.ai.Model)
	}
}

// ---------------------------------------------------------------------------
// SetSize
// ---------------------------------------------------------------------------

func TestPlanMyDaySetSize(t *testing.T) {
	p := NewPlanMyDay()
	p.SetSize(120, 50)
	if p.width != 120 {
		t.Errorf("expected width 120, got %d", p.width)
	}
	if p.height != 50 {
		t.Errorf("expected height 50, got %d", p.height)
	}
}

// ---------------------------------------------------------------------------
// generateLocalPlan — empty tasks
// ---------------------------------------------------------------------------

func TestPlanMyDayLocalPlanEmpty(t *testing.T) {
	p := NewPlanMyDay()
	p.tasks = nil
	p.events = nil
	p.habits = nil
	p.projects = nil

	p.generateLocalPlan()

	if len(p.schedule) == 0 {
		t.Fatal("expected at least one schedule slot")
	}
	if hasWorkTime() {
		hasLunch := false
		hasReview := false
		for _, s := range p.schedule {
			if s.Task == "Lunch" {
				hasLunch = true
			}
			if s.Task == "Daily review" {
				hasReview = true
			}
		}
		if !hasLunch {
			t.Error("expected lunch block in empty schedule")
		}
		if !hasReview {
			t.Error("expected daily review block in empty schedule")
		}
		if p.topGoal == "" {
			t.Error("expected topGoal to be set even with no tasks")
		}
		if p.advice == "" {
			t.Error("expected advice to be set even with no tasks")
		}
	}
}

// ---------------------------------------------------------------------------
// generateLocalPlan — single high-priority task
// ---------------------------------------------------------------------------

func TestPlanMyDayLocalPlanSingleTask(t *testing.T) {
	if !hasWorkTime() {
		t.Skip("test requires work hours (before 16:00)")
	}
	p := NewPlanMyDay()
	p.tasks = []Task{
		{Text: "Important work", Done: false, Priority: 4},
	}

	p.generateLocalPlan()

	found := false
	for _, s := range p.schedule {
		if s.Task == "Important work" {
			found = true
			if s.Priority != 4 {
				t.Errorf("expected priority 4, got %d", s.Priority)
			}
			break
		}
	}
	if !found {
		t.Error("expected 'Important work' in schedule")
	}
	if p.topGoal != "Important work" {
		t.Errorf("expected topGoal 'Important work', got %q", p.topGoal)
	}
	if len(p.focusOrder) != 1 {
		t.Errorf("expected 1 item in focusOrder, got %d", len(p.focusOrder))
	}
}

// ---------------------------------------------------------------------------
// generateLocalPlan — tasks sorted by priority score
// ---------------------------------------------------------------------------

func TestPlanMyDayLocalPlanSortedByPriority(t *testing.T) {
	if !hasWorkTime() {
		t.Skip("test requires work hours (before 16:00)")
	}
	p := NewPlanMyDay()
	today := time.Now().Format("2006-01-02")
	p.tasks = []Task{
		{Text: "low task", Done: false, Priority: 1},
		{Text: "overdue task", Done: false, Priority: 2, DueDate: time.Now().AddDate(0, 0, -1).Format("2006-01-02")},
		{Text: "high task", Done: false, Priority: 4},
		{Text: "today task", Done: false, Priority: 3, DueDate: today},
	}

	p.generateLocalPlan()

	if p.topGoal != "overdue task" && p.topGoal != "today task" {
		t.Errorf("expected topGoal to be overdue or today task, got %q", p.topGoal)
	}

	// focusOrder should have up to 5 tasks
	if len(p.focusOrder) != 4 {
		t.Errorf("expected 4 items in focusOrder, got %d", len(p.focusOrder))
	}
}

// ---------------------------------------------------------------------------
// generateLocalPlan — done tasks excluded
// ---------------------------------------------------------------------------

func TestPlanMyDayLocalPlanExcludesDone(t *testing.T) {
	p := NewPlanMyDay()
	p.tasks = []Task{
		{Text: "done task", Done: true, Priority: 4},
		{Text: "pending task", Done: false, Priority: 1},
	}

	p.generateLocalPlan()

	for _, s := range p.schedule {
		if s.Task == "done task" {
			t.Error("done task should not appear in schedule")
		}
	}
	if p.topGoal == "done task" {
		t.Error("done task should not be topGoal")
	}
}

// ---------------------------------------------------------------------------
// generateLocalPlan — calendar events respected
// ---------------------------------------------------------------------------

func TestPlanMyDayLocalPlanCalendarEvents(t *testing.T) {
	if !hasWorkTime() {
		t.Skip("test requires work hours (before 16:00)")
	}
	p := NewPlanMyDay()
	p.events = []PlannerEvent{
		{Title: "Team standup", Time: "09:00", Duration: 30},
	}
	p.tasks = []Task{
		{Text: "Write code", Done: false, Priority: 3},
	}

	p.generateLocalPlan()

	// Both should appear in schedule
	hasEvent := false
	hasTask := false
	for _, s := range p.schedule {
		if s.Task == "Team standup" {
			hasEvent = true
			if s.Type != "meeting" {
				t.Errorf("expected event type 'meeting', got %q", s.Type)
			}
		}
		if s.Task == "Write code" {
			hasTask = true
		}
	}
	if !hasEvent {
		t.Error("expected calendar event in schedule")
	}
	if !hasTask {
		t.Error("expected task in schedule")
	}

	// Task should not overlap with the 09:00-09:30 event slot
	for _, s := range p.schedule {
		if s.Task == "Write code" {
			// Parse the start time
			if s.Start == "09:00" || (s.Start > "09:00" && s.Start < "09:30") {
				t.Error("task should not overlap with calendar event")
			}
		}
	}
}

// ---------------------------------------------------------------------------
// generateLocalPlan — breaks inserted
// ---------------------------------------------------------------------------

func TestPlanMyDayLocalPlanBreaks(t *testing.T) {
	p := NewPlanMyDay()
	// Add enough tasks to trigger break insertion
	for i := 0; i < 8; i++ {
		p.tasks = append(p.tasks, Task{
			Text:     "Task " + string(rune('A'+i)),
			Done:     false,
			Priority: 3,
		})
	}

	p.generateLocalPlan()

	// Schedule should always produce at least one slot
	if len(p.schedule) == 0 {
		t.Error("expected at least one schedule slot")
	}

	// If enough working time remains (>2h), breaks should be inserted
	now := time.Now()
	workEnd := 18 * 60
	remaining := workEnd - (now.Hour()*60 + now.Minute())
	if remaining > 120 {
		breakCount := 0
		for _, s := range p.schedule {
			if s.Type == "break" && s.Task == "Break" {
				breakCount++
			}
		}
		if breakCount == 0 {
			t.Error("expected at least one break with >2h remaining")
		}
	}
}

// ---------------------------------------------------------------------------
// generateLocalPlan — focusOrder limited to 5
// ---------------------------------------------------------------------------

func TestPlanMyDayLocalPlanFocusOrderMax5(t *testing.T) {
	p := NewPlanMyDay()
	for i := 0; i < 10; i++ {
		p.tasks = append(p.tasks, Task{
			Text:     "Task " + string(rune('A'+i)),
			Done:     false,
			Priority: 2,
		})
	}

	p.generateLocalPlan()

	if len(p.focusOrder) > 5 {
		t.Errorf("expected focusOrder max 5, got %d", len(p.focusOrder))
	}
}

// ---------------------------------------------------------------------------
// generateLocalPlan — habits scheduled
// ---------------------------------------------------------------------------

func TestPlanMyDayLocalPlanHabits(t *testing.T) {
	if !hasWorkTime() {
		t.Skip("test requires work hours (before 16:00)")
	}
	p := NewPlanMyDay()
	p.habits = []habitEntry{
		{Name: "Meditate"},
		{Name: "Exercise"},
	}

	p.generateLocalPlan()

	habitSlot := false
	for _, s := range p.schedule {
		if s.Type == "habit" {
			habitSlot = true
			if !strings.Contains(s.Task, "Meditate") || !strings.Contains(s.Task, "Exercise") {
				t.Errorf("expected habit slot to include habit names, got %q", s.Task)
			}
		}
	}
	if !habitSlot {
		t.Error("expected a habit slot in schedule")
	}
}

// ---------------------------------------------------------------------------
// generateLocalPlan — schedule sorted by start time
// ---------------------------------------------------------------------------

func TestPlanMyDayLocalPlanScheduleSorted(t *testing.T) {
	p := NewPlanMyDay()
	p.tasks = []Task{
		{Text: "Task A", Done: false, Priority: 3},
		{Text: "Task B", Done: false, Priority: 2},
	}
	p.events = []PlannerEvent{
		{Title: "Meeting", Time: "14:00", Duration: 60},
	}

	p.generateLocalPlan()

	for i := 1; i < len(p.schedule); i++ {
		if p.schedule[i].Start < p.schedule[i-1].Start {
			t.Errorf("schedule not sorted: %q before %q", p.schedule[i-1].Start, p.schedule[i].Start)
		}
	}
}

// ---------------------------------------------------------------------------
// generateLocalPlan — advice varies by task count
// ---------------------------------------------------------------------------

func TestPlanMyDayLocalPlanAdvice(t *testing.T) {
	if !hasWorkTime() {
		t.Skip("test requires work hours (before 16:00)")
	}
	tests := []struct {
		name      string
		taskCount int
		contains  string
	}{
		{"no tasks", 0, "no pending tasks"},
		{"few tasks", 2, "Light day"},
		{"moderate tasks", 5, "Balanced"},
		{"many tasks", 10, "competing"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := NewPlanMyDay()
			for i := 0; i < tc.taskCount; i++ {
				p.tasks = append(p.tasks, Task{
					Text:     "task",
					Done:     false,
					Priority: 2,
				})
			}
			p.generateLocalPlan()
			if !strings.Contains(strings.ToLower(p.advice), strings.ToLower(tc.contains)) {
				t.Errorf("expected advice to contain %q, got %q", tc.contains, p.advice)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseAIResponse — valid response
// ---------------------------------------------------------------------------

func TestPlanMyDayParseAIResponseValid(t *testing.T) {
	p := NewPlanMyDay()
	p.tasks = []Task{
		{Text: "Write report", Priority: 3},
	}

	// Use 23:xx times so the time-of-day filter inside parseAIResponse
	// never drops any slots regardless of when this test runs (even at 22:59).
	response := `TOP_GOAL: Complete the quarterly report

SCHEDULE:
23:00-23:10 | Write report | deep-work | Project Alpha
23:10-23:20 | Break | break
23:20-23:35 | Review emails | admin
23:35-23:55 | Lunch | break

FOCUS_ORDER:
1. Write report
2. Review emails

ADVICE: Start with the report while your energy is high. Batch email checking to avoid context switching.`

	p.parseAIResponse(response)

	if p.topGoal != "Complete the quarterly report" {
		t.Errorf("expected topGoal, got %q", p.topGoal)
	}
	if len(p.schedule) != 4 {
		t.Fatalf("expected 4 schedule slots, got %d", len(p.schedule))
	}

	// Check first slot
	slot := p.schedule[0]
	if slot.Start != "23:00" || slot.End != "23:10" {
		t.Errorf("expected 23:00-23:10, got %s-%s", slot.Start, slot.End)
	}
	if slot.Task != "Write report" {
		t.Errorf("expected 'Write report', got %q", slot.Task)
	}
	if slot.Type != "deep-work" {
		t.Errorf("expected type 'deep-work', got %q", slot.Type)
	}
	if slot.Project != "Project Alpha" {
		t.Errorf("expected project 'Project Alpha', got %q", slot.Project)
	}
	// Priority should be matched from tasks
	if slot.Priority != 3 {
		t.Errorf("expected priority 3 from task match, got %d", slot.Priority)
	}

	// Check focus order
	if len(p.focusOrder) != 2 {
		t.Fatalf("expected 2 focus items, got %d", len(p.focusOrder))
	}
	if p.focusOrder[0] != "Write report" {
		t.Errorf("expected focusOrder[0] 'Write report', got %q", p.focusOrder[0])
	}

	// Check advice
	if !strings.Contains(p.advice, "Start with the report") {
		t.Errorf("expected advice to contain 'Start with the report', got %q", p.advice)
	}
}

// ---------------------------------------------------------------------------
// parseAIResponse — malformed lines skipped
// ---------------------------------------------------------------------------

func TestPlanMyDayParseAIResponseMalformed(t *testing.T) {
	p := NewPlanMyDay()

	response := `TOP_GOAL: Do stuff

SCHEDULE:
08:00-09:00 | Valid task | deep-work
This is not a valid schedule line
Another bad line without pipes
09:00-09:30 | Another valid | admin

FOCUS_ORDER:
1. First
2. Second`

	p.parseAIResponse(response)

	if len(p.schedule) != 2 {
		t.Errorf("expected 2 valid schedule slots (malformed skipped), got %d", len(p.schedule))
	}
	if len(p.focusOrder) != 2 {
		t.Errorf("expected 2 focus items, got %d", len(p.focusOrder))
	}
}

// ---------------------------------------------------------------------------
// parseAIResponse — missing sections
// ---------------------------------------------------------------------------

func TestPlanMyDayParseAIResponseMissingSections(t *testing.T) {
	p := NewPlanMyDay()

	// Only TOP_GOAL, no SCHEDULE, FOCUS_ORDER, ADVICE — should fallback to local
	response := `TOP_GOAL: Do something great`

	p.parseAIResponse(response)

	// Since schedule is empty, parseAIResponse falls back to local plan
	if len(p.schedule) == 0 {
		t.Error("expected fallback to produce at least one slot")
	}
	if p.topGoal == "" && hasWorkTime() {
		t.Error("expected topGoal to be set after fallback")
	}
}

// ---------------------------------------------------------------------------
// parseAIResponse — empty response falls back
// ---------------------------------------------------------------------------

func TestPlanMyDayParseAIResponseEmpty(t *testing.T) {
	p := NewPlanMyDay()
	p.parseAIResponse("")

	// Should fallback to local plan
	if len(p.schedule) == 0 {
		t.Error("expected local fallback to produce some schedule")
	}
}

// ---------------------------------------------------------------------------
// Phase transitions
// ---------------------------------------------------------------------------

func TestPlanMyDayPhaseTransitionGatherToLocal(t *testing.T) {
	p := NewPlanMyDay()
	p.Open("/tmp", nil, nil, nil, nil, nil, AIConfig{Provider: "local"})

	// Simulate gather complete
	p2, _ := p.Update(planMyDayGatherMsg{})

	// Local provider: should jump directly to phase 2 (result)
	if p2.phase != 2 {
		t.Errorf("expected phase 2 after gather with local provider, got %d", p2.phase)
	}
}

func TestPlanMyDayPhaseTransitionResultToApplied(t *testing.T) {
	p := NewPlanMyDay()
	p.active = true
	p.phase = 2
	p.schedule = []daySlot{{Task: "test task", Start: "09:00", End: "10:00"}}
	p.topGoal = "test goal"
	p.focusOrder = []string{"focus 1"}
	p.advice = "test advice"

	p2, _ := p.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if p2.phase != 3 {
		t.Errorf("expected phase 3 after Enter in result, got %d", p2.phase)
	}
	if !p2.shouldApply {
		t.Error("expected shouldApply to be true")
	}
}

func TestPlanMyDayPhaseAppliedEnterCloses(t *testing.T) {
	p := NewPlanMyDay()
	p.active = true
	p.phase = 3

	p2, _ := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p2.active {
		t.Error("expected Enter in phase 3 to close overlay")
	}
}

func TestPlanMyDayPhaseAppliedEscCloses(t *testing.T) {
	p := NewPlanMyDay()
	p.active = true
	p.phase = 3

	p2, _ := p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if p2.active {
		t.Error("expected Esc in phase 3 to close overlay")
	}
}

// ---------------------------------------------------------------------------
// Esc closes at any phase
// ---------------------------------------------------------------------------

func TestPlanMyDayEscClosesAllPhases(t *testing.T) {
	for phase := 0; phase <= 3; phase++ {
		t.Run("phase "+string(rune('0'+phase)), func(t *testing.T) {
			p := NewPlanMyDay()
			p.active = true
			p.phase = phase

			p2, _ := p.Update(tea.KeyMsg{Type: tea.KeyEsc})
			if p2.active {
				t.Errorf("expected Esc to close at phase %d", phase)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Scroll in phase 2
// ---------------------------------------------------------------------------

func TestPlanMyDayScrollPhase2(t *testing.T) {
	p := NewPlanMyDay()
	p.active = true
	p.phase = 2

	p2, _ := p.Update(tea.KeyMsg{Type: tea.KeyDown})
	if p2.scroll != 1 {
		t.Errorf("expected scroll 1, got %d", p2.scroll)
	}

	p3, _ := p2.Update(tea.KeyMsg{Type: tea.KeyDown})
	if p3.scroll != 2 {
		t.Errorf("expected scroll 2, got %d", p3.scroll)
	}

	p4, _ := p3.Update(tea.KeyMsg{Type: tea.KeyUp})
	if p4.scroll != 1 {
		t.Errorf("expected scroll 1 after up, got %d", p4.scroll)
	}

	// Can't scroll below 0
	p5 := p4
	p5.scroll = 0
	p6, _ := p5.Update(tea.KeyMsg{Type: tea.KeyUp})
	if p6.scroll != 0 {
		t.Errorf("expected scroll clamped at 0, got %d", p6.scroll)
	}
}

func TestPlanMyDayScrollPhase2BoundedAt200(t *testing.T) {
	p := NewPlanMyDay()
	p.active = true
	p.phase = 2
	p.scroll = 199

	p2, _ := p.Update(tea.KeyMsg{Type: tea.KeyDown})
	if p2.scroll != 200 {
		t.Errorf("expected scroll 200, got %d", p2.scroll)
	}

	p3, _ := p2.Update(tea.KeyMsg{Type: tea.KeyDown})
	if p3.scroll != 200 {
		t.Errorf("expected scroll clamped at 200, got %d", p3.scroll)
	}
}

func TestPlanMyDayScrollPhase2JK(t *testing.T) {
	p := NewPlanMyDay()
	p.active = true
	p.phase = 2

	p2, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if p2.scroll != 1 {
		t.Errorf("expected scroll 1 after j, got %d", p2.scroll)
	}

	p3, _ := p2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if p3.scroll != 0 {
		t.Errorf("expected scroll 0 after k, got %d", p3.scroll)
	}
}

// ---------------------------------------------------------------------------
// GetAppliedPlan — consumed-once
// ---------------------------------------------------------------------------

func TestPlanMyDayGetAppliedPlanConsumedOnce(t *testing.T) {
	p := NewPlanMyDay()

	// Not applied — should return false
	sched, goal, focus, advice, ok := p.GetAppliedPlan()
	if ok {
		t.Error("expected ok=false when not applied")
	}
	if sched != nil || goal != "" || focus != nil || advice != "" {
		t.Error("expected empty returns when not applied")
	}

	// Set applied state
	p.shouldApply = true
	p.appliedSchedule = []daySlot{{Task: "task1", Start: "09:00", End: "10:00"}}
	p.appliedGoal = "test goal"
	p.appliedFocus = []string{"focus1", "focus2"}
	p.appliedAdvice = "test advice"

	// First call should return data
	sched, goal, focus, advice, ok = p.GetAppliedPlan()
	if !ok {
		t.Error("expected ok=true on first call")
	}
	if len(sched) != 1 || sched[0].Task != "task1" {
		t.Error("expected schedule with 'task1'")
	}
	if goal != "test goal" {
		t.Errorf("expected 'test goal', got %q", goal)
	}
	if len(focus) != 2 {
		t.Errorf("expected 2 focus items, got %d", len(focus))
	}
	if advice != "test advice" {
		t.Errorf("expected 'test advice', got %q", advice)
	}

	// Second call should return false (consumed)
	_, _, _, _, ok = p.GetAppliedPlan()
	if ok {
		t.Error("expected ok=false on second call (consumed)")
	}
}

// ---------------------------------------------------------------------------
// Update inactive — noop
// ---------------------------------------------------------------------------

func TestPlanMyDayUpdateInactiveNoop(t *testing.T) {
	p := NewPlanMyDay()
	p2, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p2.active {
		t.Error("expected inactive to stay inactive")
	}
	if cmd != nil {
		t.Error("expected nil cmd when inactive")
	}
}

// ---------------------------------------------------------------------------
// planMyDayResultMsg handling
// ---------------------------------------------------------------------------

func TestPlanMyDayResultMsgSuccess(t *testing.T) {
	p := NewPlanMyDay()
	p.active = true
	p.phase = 1

	response := `TOP_GOAL: Get it done

SCHEDULE:
09:00-10:00 | Work hard | deep-work

FOCUS_ORDER:
1. Work hard

ADVICE: Just do it.`

	p2, _ := p.Update(planMyDayResultMsg{response: response})

	if p2.phase != 2 {
		t.Errorf("expected phase 2, got %d", p2.phase)
	}
	if p2.topGoal != "Get it done" {
		t.Errorf("expected topGoal 'Get it done', got %q", p2.topGoal)
	}
	if len(p2.schedule) != 1 {
		t.Errorf("expected 1 schedule slot, got %d", len(p2.schedule))
	}
}

func TestPlanMyDayResultMsgError(t *testing.T) {
	p := NewPlanMyDay()
	p.active = true
	p.phase = 1

	p2, _ := p.Update(planMyDayResultMsg{err: errTest})

	if p2.phase != 2 {
		t.Errorf("expected phase 2 after error, got %d", p2.phase)
	}
	// Should fall back to local plan
	if len(p2.schedule) == 0 {
		t.Error("expected local fallback to produce schedule")
	}
}

func TestPlanMyDayResultMsgWrongPhase(t *testing.T) {
	p := NewPlanMyDay()
	p.active = true
	p.phase = 0 // Not in planning phase

	p2, _ := p.Update(planMyDayResultMsg{response: "stuff"})

	if p2.phase != 0 {
		t.Errorf("expected phase to stay at 0, got %d", p2.phase)
	}
}

// ---------------------------------------------------------------------------
// planMyDayTickMsg
// ---------------------------------------------------------------------------

func TestPlanMyDayTickMsg(t *testing.T) {
	p := NewPlanMyDay()
	p.active = true
	p.phase = 0
	p.loadingTick = 0

	p2, cmd := p.Update(planMyDayTickMsg{})
	if p2.loadingTick != 1 {
		t.Errorf("expected loadingTick 1, got %d", p2.loadingTick)
	}
	if cmd == nil {
		t.Error("expected tick cmd to be returned during phase 0")
	}

	// Tick in phase 2 should not advance
	p3 := p2
	p3.phase = 2
	p3.loadingTick = 5
	p4, cmd := p3.Update(planMyDayTickMsg{})
	if p4.loadingTick != 5 {
		t.Errorf("expected loadingTick unchanged in phase 2, got %d", p4.loadingTick)
	}
	if cmd != nil {
		t.Error("expected nil cmd in phase 2")
	}
}

// ---------------------------------------------------------------------------
// View smoke tests
// ---------------------------------------------------------------------------

func TestPlanMyDayViewSmoke(t *testing.T) {
	p := NewPlanMyDay()
	p.SetSize(120, 50)

	// Inactive — empty string
	v := p.View()
	if v != "" {
		t.Error("expected empty view when inactive")
	}

	// Phase 0
	p.Open("/tmp", nil, nil, nil, nil, nil, AIConfig{Provider: "local"})
	v = p.View()
	if v == "" {
		t.Error("expected non-empty view for phase 0")
	}

	// Phase 1
	p.phase = 1
	v = p.View()
	if v == "" {
		t.Error("expected non-empty view for phase 1")
	}

	// Phase 2 with data
	p.phase = 2
	p.schedule = []daySlot{
		{Start: "09:00", End: "10:00", Task: "Task", Type: "deep-work", Priority: 3},
	}
	p.topGoal = "Test goal"
	p.focusOrder = []string{"Focus 1"}
	p.advice = "Test advice"
	v = p.View()
	if v == "" {
		t.Error("expected non-empty view for phase 2")
	}

	// Phase 3
	p.phase = 3
	v = p.View()
	if v == "" {
		t.Error("expected non-empty view for phase 3")
	}
}

// ---------------------------------------------------------------------------
// planWrap
// ---------------------------------------------------------------------------

func TestPlanWrap(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxWidth int
		expected string
	}{
		{"empty", "", 40, ""},
		{"single word", "hello", 40, "hello"},
		{"fits in one line", "hello world", 40, "hello world"},
		{"wraps", "hello world foo bar", 11, "hello world\nfoo bar"},
		{"zero width", "hello", 0, "hello"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := planWrap(tc.text, tc.maxWidth)
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FormatDayPlanMarkdown
// ---------------------------------------------------------------------------

func TestFormatDayPlanMarkdown(t *testing.T) {
	schedule := []daySlot{
		{Start: "09:00", End: "10:00", Task: "Write code", Type: "deep-work"},
		{Start: "12:00", End: "13:00", Task: "Lunch", Type: "break"},
	}
	topGoal := "Ship the feature"
	focusOrder := []string{"Write code", "Review PRs"}
	advice := "Stay focused."

	md := FormatDayPlanMarkdown(schedule, topGoal, focusOrder, advice)

	if !strings.Contains(md, "## Day Plan") {
		t.Error("expected '## Day Plan' header")
	}
	if !strings.Contains(md, "**Today's Goal:** Ship the feature") {
		t.Error("expected top goal in markdown")
	}
	if !strings.Contains(md, "| 09:00-10:00 | Write code | deep-work |") {
		t.Error("expected schedule table row")
	}
	if !strings.Contains(md, "1. Write code") {
		t.Error("expected focus order item")
	}
	if !strings.Contains(md, "> Stay focused.") {
		t.Error("expected advice blockquote")
	}
}

func TestFormatDayPlanMarkdownEmpty(t *testing.T) {
	md := FormatDayPlanMarkdown(nil, "", nil, "")
	if !strings.Contains(md, "## Day Plan") {
		t.Error("expected header even with empty data")
	}
	if strings.Contains(md, "**Today's Goal:**") {
		t.Error("expected no goal section with empty goal")
	}
	if strings.Contains(md, "### Focus Order") {
		t.Error("expected no focus section with empty focus")
	}
	if strings.Contains(md, "### Advice") {
		t.Error("expected no advice section with empty advice")
	}
}

// ---------------------------------------------------------------------------
// priorityLabel helper
// ---------------------------------------------------------------------------

func TestPriorityLabel(t *testing.T) {
	tests := []struct {
		pri  int
		want string
	}{
		{4, "highest"},
		{3, "high"},
		{2, "medium"},
		{1, "low"},
		{0, "none"},
		{-1, "none"},
	}

	for _, tc := range tests {
		got := priorityLabel(tc.pri)
		if got != tc.want {
			t.Errorf("priorityLabel(%d): expected %q, got %q", tc.pri, tc.want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// daySlotPriorityIcon — smoke test (should not panic)
// ---------------------------------------------------------------------------

func TestDaySlotPriorityIcon(t *testing.T) {
	for pri := 0; pri <= 4; pri++ {
		icon := daySlotPriorityIcon(pri)
		if icon == "" {
			t.Errorf("expected non-empty icon for priority %d", pri)
		}
	}
}

// ---------------------------------------------------------------------------
// overlayWidth / innerWidth
// ---------------------------------------------------------------------------

func TestPlanMyDayOverlayWidth(t *testing.T) {
	p := NewPlanMyDay()

	// Narrow terminal
	p.SetSize(60, 40)
	w := p.overlayWidth()
	if w < 60 {
		t.Errorf("overlayWidth should be at least 60, got %d", w)
	}

	// Wide terminal
	p.SetSize(200, 40)
	w = p.overlayWidth()
	if w > 100 {
		t.Errorf("overlayWidth should be at most 100, got %d", w)
	}

	// Normal terminal
	p.SetSize(120, 40)
	expected := 120 * 2 / 3
	w = p.overlayWidth()
	if w != expected {
		t.Errorf("expected overlayWidth %d, got %d", expected, w)
	}
}

func TestPlanMyDayInnerWidth(t *testing.T) {
	p := NewPlanMyDay()
	p.SetSize(120, 40)
	ow := p.overlayWidth()
	iw := p.innerWidth()
	if iw != ow-6 {
		t.Errorf("expected innerWidth %d, got %d", ow-6, iw)
	}
}

// ---------------------------------------------------------------------------
// replaceDailySection — deduplication helper
// ---------------------------------------------------------------------------

func TestReplaceDailySectionAppendNew(t *testing.T) {
	existing := "# 2026-04-09\n\nSome content here."
	section := "## Daily Plan\n\n- [ ] Task 1\n"
	result := replaceDailySection(existing, section, "## Daily Plan")
	if !strings.Contains(result, "Some content here.") {
		t.Error("existing content should be preserved")
	}
	if strings.Count(result, "## Daily Plan") != 1 {
		t.Error("section should appear exactly once")
	}
}

func TestReplaceDailySectionReplaceExisting(t *testing.T) {
	existing := "# 2026-04-09\n\n## Daily Plan\n\n- [ ] Old task\n\n## Notes\n\nMy notes."
	section := "## Daily Plan\n\n- [ ] New task\n"
	result := replaceDailySection(existing, section, "## Daily Plan")
	if strings.Contains(result, "Old task") {
		t.Error("old section content should be replaced")
	}
	if !strings.Contains(result, "New task") {
		t.Error("new section content should be present")
	}
	if !strings.Contains(result, "My notes.") {
		t.Error("content after replaced section should be preserved")
	}
	if strings.Count(result, "## Daily Plan") != 1 {
		t.Errorf("section should appear exactly once, got %d", strings.Count(result, "## Daily Plan"))
	}
}

func TestReplaceDailySectionReplaceAtEnd(t *testing.T) {
	existing := "# 2026-04-09\n\n## Daily Plan\n\n- [ ] Old task\n"
	section := "## Daily Plan\n\n- [ ] New task\n"
	result := replaceDailySection(existing, section, "## Daily Plan")
	if strings.Contains(result, "Old task") {
		t.Error("old section content should be replaced")
	}
	if !strings.Contains(result, "New task") {
		t.Error("new section content should be present")
	}
	if strings.Count(result, "## Daily Plan") != 1 {
		t.Errorf("section should appear exactly once, got %d", strings.Count(result, "## Daily Plan"))
	}
}

func TestReplaceDailySectionNoFalsePositive(t *testing.T) {
	// "## Daily Planning" should NOT match "## Daily Plan"
	existing := "# 2026-04-09\n\n## Daily Planning\n\nMy planning notes.\n"
	section := "## Daily Plan\n\n- [ ] Task 1\n"
	result := replaceDailySection(existing, section, "## Daily Plan")
	if !strings.Contains(result, "My planning notes.") {
		t.Error("unrelated section should be preserved")
	}
	if !strings.Contains(result, "## Daily Planning") {
		t.Error("similar-but-different heading should be preserved")
	}
	if strings.Count(result, "## Daily Plan\n") != 1 {
		t.Errorf("new section should appear exactly once")
	}
}

func TestReplaceDailySectionWithDateSuffix(t *testing.T) {
	// Morning routine generates "## Daily Plan — Wednesday, April 9, 2026"
	// but searches for "## Daily Plan". Must match the line with date suffix.
	existing := "# 2026-04-09\n\n## Daily Plan — Wednesday, April 9, 2026\n\n### Tasks\n\n- [ ] Old task\n\n## Notes\n\nSome notes.\n"
	section := "## Daily Plan — Wednesday, April 9, 2026\n\n### Tasks\n\n- [ ] New task\n"
	result := replaceDailySection(existing, section, "## Daily Plan")

	if strings.Count(result, "## Daily Plan") != 1 {
		t.Errorf("expected exactly 1 Daily Plan section, got %d in:\n%s", strings.Count(result, "## Daily Plan"), result)
	}
	if !strings.Contains(result, "New task") {
		t.Error("new task should be present")
	}
	if strings.Contains(result, "Old task") {
		t.Error("old task should be replaced")
	}
	if !strings.Contains(result, "## Notes") {
		t.Error("Notes section after Daily Plan should be preserved")
	}
	if !strings.Contains(result, "Some notes.") {
		t.Error("Notes content should be preserved")
	}
}

func TestReplaceDailySectionDateSuffixNoFalsePositive(t *testing.T) {
	// "## Daily Planning Notes" should NOT be matched by "## Daily Plan"
	existing := "# 2026-04-09\n\n## Daily Planning Notes\n\nPlanning content.\n"
	section := "## Daily Plan — Thursday, April 9, 2026\n\n- [ ] Task\n"
	result := replaceDailySection(existing, section, "## Daily Plan")

	if !strings.Contains(result, "## Daily Planning Notes") {
		t.Error("Daily Planning Notes heading should be preserved")
	}
	if !strings.Contains(result, "Planning content.") {
		t.Error("Planning content should be preserved")
	}
	if !strings.Contains(result, "## Daily Plan —") {
		t.Error("new section should be appended")
	}
}

// errTest is a small sentinel error for testing.
var errTest = &testError{}

type testError struct{}

func (e *testError) Error() string { return "test error" }
