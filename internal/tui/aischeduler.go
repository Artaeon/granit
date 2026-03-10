package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Input types — passed from app.go
// ---------------------------------------------------------------------------

// SchedulerInput holds all data needed by the AI scheduler.
type SchedulerInput struct {
	Tasks       []SchedulerTask
	Events      []SchedulerEvent
	Preferences SchedulerPrefs
}

// SchedulerTask represents a single task to be scheduled.
type SchedulerTask struct {
	Text      string
	Priority  int    // 0=none, 1=low, 2=med, 3=high, 4=highest
	DueDate   string // YYYY-MM-DD or empty
	Estimated int    // estimated minutes (0 = unknown)
	Done      bool
}

// SchedulerEvent represents a fixed calendar event.
type SchedulerEvent struct {
	Title    string
	Time     string // "HH:MM"
	Duration int    // minutes
}

// SchedulerPrefs holds user scheduling preferences.
type SchedulerPrefs struct {
	WorkStart     int // hour (default 8)
	WorkEnd       int // hour (default 18)
	LunchStart    int // hour (default 12)
	LunchDuration int // minutes (default 60)
	FocusBlockMin int // minutes (default 25)
	BreakEvery    int // minutes (default 90, take break every N min)
}

// ---------------------------------------------------------------------------
// Schedule slot
// ---------------------------------------------------------------------------

type schedulerSlot struct {
	StartHour int
	StartMin  int
	EndHour   int
	EndMin    int
	Task      string
	Type      string // "task", "event", "break", "lunch", "focus"
	Priority  int
}

// slotMinutes returns the duration of a slot in minutes.
func (s schedulerSlot) slotMinutes() int {
	return (s.EndHour*60 + s.EndMin) - (s.StartHour*60 + s.StartMin)
}

// ---------------------------------------------------------------------------
// Async AI message
// ---------------------------------------------------------------------------

type aiSchedulerResultMsg struct {
	response string
	err      error
}

type aiSchedulerTickMsg struct{}

// ---------------------------------------------------------------------------
// AIScheduler component
// ---------------------------------------------------------------------------

// AIScheduler is an AI-powered scheduling overlay that analyzes tasks,
// deadlines, priorities, and estimated effort to suggest an optimal daily
// schedule.
type AIScheduler struct {
	active    bool
	width     int
	height    int
	vaultRoot string

	phase int // 0=setup, 1=generating, 2=result, 3=applied

	// Input
	tasks  []SchedulerTask
	events []SchedulerEvent
	prefs  SchedulerPrefs

	// AI config
	aiProvider  string
	ollamaURL   string
	ollamaModel string
	openaiKey   string
	openaiModel string

	// Setup UI
	setupField  int    // 0=workStart, 1=workEnd, 2=lunchStart, 3=focusBlock, 4=breakEvery
	taskCursor  int    // selected task index
	estimating  bool   // editing estimated time for a task
	estimateBuf string // buffer for estimate input

	// Generated schedule
	schedule []schedulerSlot

	// Result
	resultScroll int

	// Output for daily planner
	applied bool

	statusMsg  string
	generating bool
	spinner    int
}

// NewAIScheduler creates a new AIScheduler with default preferences.
func NewAIScheduler() AIScheduler {
	return AIScheduler{
		prefs: SchedulerPrefs{
			WorkStart:     8,
			WorkEnd:       18,
			LunchStart:    12,
			LunchDuration: 60,
			FocusBlockMin: 25,
			BreakEvery:    90,
		},
	}
}

// ---------------------------------------------------------------------------
// Overlay interface
// ---------------------------------------------------------------------------

func (as AIScheduler) IsActive() bool { return as.active }

func (as *AIScheduler) SetSize(w, h int) {
	as.width = w
	as.height = h
}

// Open initialises the scheduler overlay with tasks, events, and AI config.
func (as *AIScheduler) Open(vaultRoot string, tasks []SchedulerTask, events []SchedulerEvent,
	aiProvider, ollamaURL, ollamaModel, openaiKey, openaiModel string) {

	as.active = true
	as.vaultRoot = vaultRoot
	as.phase = 0
	as.setupField = 0
	as.taskCursor = 0
	as.estimating = false
	as.estimateBuf = ""
	as.schedule = nil
	as.resultScroll = 0
	as.applied = false
	as.statusMsg = ""
	as.generating = false
	as.spinner = 0

	// Copy tasks (only incomplete ones)
	as.tasks = nil
	for _, t := range tasks {
		if !t.Done {
			as.tasks = append(as.tasks, t)
		}
	}
	as.events = events

	// AI config
	as.aiProvider = aiProvider
	if as.aiProvider == "" {
		as.aiProvider = "local"
	}
	as.ollamaURL = ollamaURL
	if as.ollamaURL == "" {
		as.ollamaURL = "http://localhost:11434"
	}
	as.ollamaModel = ollamaModel
	if as.ollamaModel == "" {
		as.ollamaModel = "qwen2.5:0.5b"
	}
	as.openaiKey = openaiKey
	as.openaiModel = openaiModel
	if as.openaiModel == "" {
		as.openaiModel = "gpt-4o-mini"
	}
}

// Close deactivates the overlay.
func (as *AIScheduler) Close() { as.active = false }

// GetSchedule returns the applied schedule for the daily planner to consume.
func (as *AIScheduler) GetSchedule() ([]schedulerSlot, bool) {
	if as.applied {
		as.applied = false
		return as.schedule, true
	}
	return nil, false
}

// ---------------------------------------------------------------------------
// AI HTTP calls
// ---------------------------------------------------------------------------

func aiSchedulerOllama(url, model, prompt string) tea.Cmd {
	return func() tea.Msg {
		payload := map[string]interface{}{
			"model":  model,
			"prompt": prompt,
			"stream": false,
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return aiSchedulerResultMsg{err: err}
		}
		client := &http.Client{Timeout: 120 * time.Second}
		resp, err := client.Post(url+"/api/generate", "application/json", bytes.NewReader(body))
		if err != nil {
			return aiSchedulerResultMsg{err: fmt.Errorf("cannot connect to Ollama at %s: %w", url, err)}
		}
		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
		}
		var result struct {
			Response string `json:"response"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return aiSchedulerResultMsg{err: fmt.Errorf("failed to decode Ollama response: %w", err)}
		}
		return aiSchedulerResultMsg{response: result.Response}
	}
}

func aiSchedulerOpenAI(apiKey, model, prompt string) tea.Cmd {
	return func() tea.Msg {
		payload := map[string]interface{}{
			"model": model,
			"messages": []map[string]string{
				{"role": "system", "content": "You are a productivity scheduling assistant."},
				{"role": "user", "content": prompt},
			},
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return aiSchedulerResultMsg{err: err}
		}
		req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
		if err != nil {
			return aiSchedulerResultMsg{err: err}
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return aiSchedulerResultMsg{err: fmt.Errorf("cannot connect to OpenAI: %w", err)}
		}
		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
		}
		var result struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return aiSchedulerResultMsg{err: fmt.Errorf("failed to decode OpenAI response: %w", err)}
		}
		if result.Error != nil {
			return aiSchedulerResultMsg{err: fmt.Errorf("OpenAI error: %s", result.Error.Message)}
		}
		if len(result.Choices) > 0 {
			return aiSchedulerResultMsg{response: result.Choices[0].Message.Content}
		}
		return aiSchedulerResultMsg{err: fmt.Errorf("no response from OpenAI")}
	}
}

func aiSchedulerTickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return aiSchedulerTickMsg{}
	})
}

// ---------------------------------------------------------------------------
// Prompt builder
// ---------------------------------------------------------------------------

func (as AIScheduler) buildSchedulerPrompt() string {
	var b strings.Builder

	b.WriteString("You are a productivity scheduler. Given these tasks and constraints, create an optimal daily schedule.\n\n")

	b.WriteString("Tasks (priority 1-4, 4=highest):\n")
	for _, t := range as.tasks {
		est := t.Estimated
		if est == 0 {
			est = 60
		}
		line := fmt.Sprintf("- %s (priority: %d", t.Text, t.Priority)
		if t.DueDate != "" {
			line += ", due: " + t.DueDate
		}
		line += fmt.Sprintf(", est: %dmin)", est)
		b.WriteString(line + "\n")
	}

	if len(as.events) > 0 {
		b.WriteString("\nFixed events:\n")
		for _, e := range as.events {
			b.WriteString(fmt.Sprintf("- %s at %s (%dmin)\n", e.Title, e.Time, e.Duration))
		}
	}

	b.WriteString("\nConstraints:\n")
	b.WriteString(fmt.Sprintf("- Work hours: %02d:00-%02d:00\n", as.prefs.WorkStart, as.prefs.WorkEnd))
	b.WriteString(fmt.Sprintf("- Lunch: %02d:00-%02d:%02d\n", as.prefs.LunchStart,
		as.prefs.LunchStart+as.prefs.LunchDuration/60, as.prefs.LunchDuration%60))
	b.WriteString(fmt.Sprintf("- Take 10min break every %dmin of work\n", as.prefs.BreakEvery))
	b.WriteString(fmt.Sprintf("- Focus blocks: minimum %d minutes\n", as.prefs.FocusBlockMin))

	b.WriteString("\nOutput format (one per line):\n")
	b.WriteString("HH:MM-HH:MM | Task name | type\n")
	b.WriteString("Example: 08:00-09:30 | Fix auth bug | task\n\n")
	b.WriteString("Types: task, event, break, lunch\n")
	b.WriteString("Prioritize urgent and high-priority tasks in morning hours. Group similar tasks together. End the day with low-priority tasks.\n")

	return b.String()
}

// ---------------------------------------------------------------------------
// Response parser
// ---------------------------------------------------------------------------

var scheduleLinePattern = regexp.MustCompile(`^(\d{1,2}):(\d{2})\s*-\s*(\d{1,2}):(\d{2})\s*\|\s*(.+?)\s*\|\s*(\w+)`)

func (as *AIScheduler) parseAIResponse(response string) {
	as.schedule = nil
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		m := scheduleLinePattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		sh, _ := strconv.Atoi(m[1])
		sm, _ := strconv.Atoi(m[2])
		eh, _ := strconv.Atoi(m[3])
		em, _ := strconv.Atoi(m[4])
		taskName := strings.TrimSpace(m[5])
		slotType := strings.ToLower(strings.TrimSpace(m[6]))

		// Match priority from original tasks
		pri := 0
		for _, t := range as.tasks {
			if strings.EqualFold(t.Text, taskName) {
				pri = t.Priority
				break
			}
		}

		as.schedule = append(as.schedule, schedulerSlot{
			StartHour: sh,
			StartMin:  sm,
			EndHour:   eh,
			EndMin:    em,
			Task:      taskName,
			Type:      slotType,
			Priority:  pri,
		})
	}

	// If parsing failed, fall back to local algorithm
	if len(as.schedule) == 0 {
		as.generateLocalSchedule()
	}
}

// ---------------------------------------------------------------------------
// Local scheduling algorithm (greedy)
// ---------------------------------------------------------------------------

func (as *AIScheduler) generateLocalSchedule() {
	as.schedule = nil

	// Sort tasks: highest priority first, then soonest due date
	sorted := make([]SchedulerTask, len(as.tasks))
	copy(sorted, as.tasks)
	today := time.Now().Format("2006-01-02")

	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority > sorted[j].Priority
		}
		di := sorted[i].DueDate
		dj := sorted[j].DueDate
		if di == "" {
			di = "9999-12-31"
		}
		if dj == "" {
			dj = "9999-12-31"
		}
		return di < dj
	})

	workStart := as.prefs.WorkStart * 60  // in minutes from midnight
	workEnd := as.prefs.WorkEnd * 60      // in minutes from midnight
	lunchStart := as.prefs.LunchStart * 60
	lunchEnd := lunchStart + as.prefs.LunchDuration
	breakEvery := as.prefs.BreakEvery
	if breakEvery <= 0 {
		breakEvery = 90
	}

	// Track occupied slots
	type timeRange struct {
		start int
		end   int
	}
	var occupied []timeRange

	// Place fixed events first
	for _, ev := range as.events {
		parts := strings.Split(ev.Time, ":")
		if len(parts) != 2 {
			continue
		}
		h, _ := strconv.Atoi(parts[0])
		m, _ := strconv.Atoi(parts[1])
		start := h*60 + m
		end := start + ev.Duration
		occupied = append(occupied, timeRange{start, end})
		as.schedule = append(as.schedule, schedulerSlot{
			StartHour: h,
			StartMin:  m,
			EndHour:   end / 60,
			EndMin:    end % 60,
			Task:      ev.Title,
			Type:      "event",
			Priority:  0,
		})
	}

	// Place lunch
	occupied = append(occupied, timeRange{lunchStart, lunchEnd})
	as.schedule = append(as.schedule, schedulerSlot{
		StartHour: lunchStart / 60,
		StartMin:  lunchStart % 60,
		EndHour:   lunchEnd / 60,
		EndMin:    lunchEnd % 60,
		Task:      "Lunch",
		Type:      "lunch",
		Priority:  0,
	})

	// isOccupied checks if a given range overlaps with any occupied slot.
	isOccupied := func(start, end int) bool {
		for _, o := range occupied {
			if start < o.end && end > o.start {
				return true
			}
		}
		return false
	}

	// findSlot finds the earliest available slot of the given duration.
	findSlot := func(duration int) (int, bool) {
		for pos := workStart; pos+duration <= workEnd; pos++ {
			if !isOccupied(pos, pos+duration) {
				return pos, true
			}
		}
		return 0, false
	}

	// Fill tasks with breaks
	workMinsSinceBreak := 0
	_ = today // suppress unused warning if needed

	for _, t := range sorted {
		est := t.Estimated
		if est <= 0 {
			est = 60
		}
		// Enforce minimum focus block
		if est < as.prefs.FocusBlockMin {
			est = as.prefs.FocusBlockMin
		}

		// Check if we need a break before this task
		if workMinsSinceBreak >= breakEvery {
			breakDur := 10
			breakStart, found := findSlot(breakDur)
			if found {
				occupied = append(occupied, timeRange{breakStart, breakStart + breakDur})
				as.schedule = append(as.schedule, schedulerSlot{
					StartHour: breakStart / 60,
					StartMin:  breakStart % 60,
					EndHour:   (breakStart + breakDur) / 60,
					EndMin:    (breakStart + breakDur) % 60,
					Task:      "Break",
					Type:      "break",
					Priority:  0,
				})
				workMinsSinceBreak = 0
			}
		}

		// Add 5 min transition buffer
		transitionDur := 5
		totalNeeded := est + transitionDur

		taskStart, found := findSlot(totalNeeded)
		if !found {
			// Try without transition buffer
			taskStart, found = findSlot(est)
			if !found {
				continue // no space left
			}
			transitionDur = 0
		}

		// Place the task (with transition buffer at the end)
		taskEnd := taskStart + est
		occupied = append(occupied, timeRange{taskStart, taskEnd + transitionDur})
		as.schedule = append(as.schedule, schedulerSlot{
			StartHour: taskStart / 60,
			StartMin:  taskStart % 60,
			EndHour:   taskEnd / 60,
			EndMin:    taskEnd % 60,
			Task:      t.Text,
			Type:      "task",
			Priority:  t.Priority,
		})

		workMinsSinceBreak += est
	}

	// Sort schedule by start time
	sort.SliceStable(as.schedule, func(i, j int) bool {
		si := as.schedule[i].StartHour*60 + as.schedule[i].StartMin
		sj := as.schedule[j].StartHour*60 + as.schedule[j].StartMin
		return si < sj
	})
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (as AIScheduler) Update(msg tea.Msg) (AIScheduler, tea.Cmd) {
	if !as.active {
		return as, nil
	}

	switch msg := msg.(type) {
	case aiSchedulerTickMsg:
		if as.phase == 1 {
			as.spinner++
			return as, aiSchedulerTickCmd()
		}

	case aiSchedulerResultMsg:
		if as.phase != 1 {
			return as, nil
		}
		as.generating = false
		if msg.err != nil {
			// AI failed, fall back to local algorithm
			as.statusMsg = "AI unavailable, using local scheduler: " + msg.err.Error()
			as.generateLocalSchedule()
		} else {
			as.parseAIResponse(msg.response)
			as.statusMsg = ""
		}
		as.phase = 2
		as.resultScroll = 0
		return as, nil

	case tea.KeyMsg:
		switch as.phase {
		case 0:
			return as.updateSetup(msg)
		case 1:
			if msg.String() == "esc" {
				as.active = false
			}
			return as, nil
		case 2:
			return as.updateResult(msg)
		case 3:
			if msg.String() == "enter" || msg.String() == "esc" {
				as.active = false
			}
			return as, nil
		}
	}
	return as, nil
}

// updateSetup handles key events in the setup phase.
func (as AIScheduler) updateSetup(msg tea.KeyMsg) (AIScheduler, tea.Cmd) {
	key := msg.String()

	// If editing an estimate value
	if as.estimating {
		switch key {
		case "esc":
			as.estimating = false
			as.estimateBuf = ""
		case "enter":
			if v, err := strconv.Atoi(as.estimateBuf); err == nil && v > 0 {
				if as.taskCursor >= 0 && as.taskCursor < len(as.tasks) {
					as.tasks[as.taskCursor].Estimated = v
				}
			}
			as.estimating = false
			as.estimateBuf = ""
		case "backspace":
			if len(as.estimateBuf) > 0 {
				as.estimateBuf = as.estimateBuf[:len(as.estimateBuf)-1]
			}
		default:
			if len(key) == 1 && key[0] >= '0' && key[0] <= '9' && len(as.estimateBuf) < 4 {
				as.estimateBuf += key
			}
		}
		return as, nil
	}

	totalFields := 5
	totalItems := totalFields + len(as.tasks) // prefs + task list

	switch key {
	case "esc":
		as.active = false

	case "tab", "down", "j":
		combined := as.combinedCursor()
		combined++
		if combined >= totalItems {
			combined = 0
		}
		as.setCombinedCursor(combined)

	case "shift+tab", "up", "k":
		combined := as.combinedCursor()
		combined--
		if combined < 0 {
			combined = totalItems - 1
		}
		as.setCombinedCursor(combined)

	case "left", "h":
		as.adjustField(-1)

	case "right", "l":
		as.adjustField(1)

	case "e":
		// Edit estimated time for selected task
		if as.combinedCursor() >= totalFields && as.taskCursor >= 0 && as.taskCursor < len(as.tasks) {
			as.estimating = true
			est := as.tasks[as.taskCursor].Estimated
			if est > 0 {
				as.estimateBuf = strconv.Itoa(est)
			} else {
				as.estimateBuf = ""
			}
		}

	case "enter":
		return as.startGeneration()
	}

	return as, nil
}

// combinedCursor returns a unified cursor position across prefs and tasks.
func (as AIScheduler) combinedCursor() int {
	if as.setupField < 5 {
		return as.setupField
	}
	return 5 + as.taskCursor
}

// setCombinedCursor sets the cursor from a unified position.
func (as *AIScheduler) setCombinedCursor(pos int) {
	if pos < 5 {
		as.setupField = pos
		as.taskCursor = -1
	} else {
		as.setupField = 5 // past preference fields
		as.taskCursor = pos - 5
		if as.taskCursor >= len(as.tasks) {
			as.taskCursor = len(as.tasks) - 1
		}
	}
}

// adjustField adjusts the currently selected preference field.
func (as *AIScheduler) adjustField(delta int) {
	switch as.setupField {
	case 0: // workStart
		as.prefs.WorkStart += delta
		if as.prefs.WorkStart < 0 {
			as.prefs.WorkStart = 0
		}
		if as.prefs.WorkStart > 23 {
			as.prefs.WorkStart = 23
		}
		if as.prefs.WorkStart >= as.prefs.WorkEnd {
			as.prefs.WorkEnd = as.prefs.WorkStart + 1
			if as.prefs.WorkEnd > 24 {
				as.prefs.WorkEnd = 24
				as.prefs.WorkStart = 23
			}
		}
	case 1: // workEnd
		as.prefs.WorkEnd += delta
		if as.prefs.WorkEnd < 1 {
			as.prefs.WorkEnd = 1
		}
		if as.prefs.WorkEnd > 24 {
			as.prefs.WorkEnd = 24
		}
		if as.prefs.WorkEnd <= as.prefs.WorkStart {
			as.prefs.WorkStart = as.prefs.WorkEnd - 1
			if as.prefs.WorkStart < 0 {
				as.prefs.WorkStart = 0
				as.prefs.WorkEnd = 1
			}
		}
	case 2: // lunchStart
		as.prefs.LunchStart += delta
		if as.prefs.LunchStart < as.prefs.WorkStart {
			as.prefs.LunchStart = as.prefs.WorkStart
		}
		if as.prefs.LunchStart >= as.prefs.WorkEnd {
			as.prefs.LunchStart = as.prefs.WorkEnd - 1
		}
	case 3: // focusBlockMin
		as.prefs.FocusBlockMin += delta * 5
		if as.prefs.FocusBlockMin < 5 {
			as.prefs.FocusBlockMin = 5
		}
		if as.prefs.FocusBlockMin > 120 {
			as.prefs.FocusBlockMin = 120
		}
	case 4: // breakEvery
		as.prefs.BreakEvery += delta * 10
		if as.prefs.BreakEvery < 20 {
			as.prefs.BreakEvery = 20
		}
		if as.prefs.BreakEvery > 180 {
			as.prefs.BreakEvery = 180
		}
	}
}

// startGeneration kicks off schedule generation.
func (as AIScheduler) startGeneration() (AIScheduler, tea.Cmd) {
	if len(as.tasks) == 0 {
		as.statusMsg = "No tasks to schedule"
		return as, nil
	}

	useAI := as.aiProvider == "ollama" || as.aiProvider == "openai"

	if useAI {
		as.phase = 1
		as.generating = true
		as.spinner = 0
		as.statusMsg = ""

		prompt := as.buildSchedulerPrompt()

		if as.aiProvider == "openai" && as.openaiKey != "" {
			return as, tea.Batch(
				aiSchedulerOpenAI(as.openaiKey, as.openaiModel, prompt),
				aiSchedulerTickCmd(),
			)
		}

		return as, tea.Batch(
			aiSchedulerOllama(as.ollamaURL, as.ollamaModel, prompt),
			aiSchedulerTickCmd(),
		)
	}

	// Local algorithm
	as.generateLocalSchedule()
	as.phase = 2
	as.resultScroll = 0
	as.statusMsg = "Generated with local scheduler"
	return as, nil
}

// updateResult handles key events in the result phase.
func (as AIScheduler) updateResult(msg tea.KeyMsg) (AIScheduler, tea.Cmd) {
	switch msg.String() {
	case "esc":
		as.active = false
	case "up", "k":
		if as.resultScroll > 0 {
			as.resultScroll--
		}
	case "down", "j":
		maxScroll := len(as.schedule) - 1
		if maxScroll < 0 {
			maxScroll = 0
		}
		if as.resultScroll < maxScroll {
			as.resultScroll++
		}
	case "enter":
		as.applied = true
		as.phase = 3
	case "r":
		// Regenerate
		as.phase = 0
		as.resultScroll = 0
		as.schedule = nil
	}
	return as, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (as AIScheduler) View() string {
	if !as.active {
		return ""
	}

	width := as.overlayWidth()

	switch as.phase {
	case 0:
		return as.viewSetup(width)
	case 1:
		return as.viewGenerating(width)
	case 2:
		return as.viewResult(width)
	case 3:
		return as.viewApplied(width)
	}
	return ""
}

func (as AIScheduler) overlayWidth() int {
	w := as.width * 2 / 3
	if w < 54 {
		w = 54
	}
	if w > 90 {
		w = 90
	}
	return w
}


// ---------------------------------------------------------------------------
// View: setup phase
// ---------------------------------------------------------------------------

func (as AIScheduler) viewSetup(width int) string {
	var buf strings.Builder

	// Title
	title := lipgloss.NewStyle().Foreground(mauve).Bold(true).
		Render("  " + IconBotChar + " AI Smart Scheduler")
	buf.WriteString(title)
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render("  " + strings.Repeat(string(ThemeSeparator), width-10)))
	buf.WriteString("\n\n")

	// AI provider indicator
	providerLabel := "Local Algorithm"
	providerColor := overlay1
	switch as.aiProvider {
	case "ollama":
		providerLabel = "Ollama: " + as.ollamaModel
		providerColor = green
	case "openai":
		providerLabel = "OpenAI: " + as.openaiModel
		providerColor = green
	}
	buf.WriteString(lipgloss.NewStyle().Foreground(providerColor).Render("  "+IconBotChar+" "+providerLabel) + "\n\n")

	// Preferences section
	buf.WriteString(lipgloss.NewStyle().Foreground(blue).Bold(true).Render("  Preferences:") + "\n")

	prefFields := []struct {
		label string
		value string
	}{
		{"Work start", fmt.Sprintf("%02d:00", as.prefs.WorkStart)},
		{"Work end", fmt.Sprintf("%02d:00", as.prefs.WorkEnd)},
		{"Lunch", fmt.Sprintf("%02d:00 (%d min)", as.prefs.LunchStart, as.prefs.LunchDuration)},
		{"Focus blocks", fmt.Sprintf("%d min minimum", as.prefs.FocusBlockMin)},
		{"Break every", fmt.Sprintf("%d min", as.prefs.BreakEvery)},
	}

	for i, pf := range prefFields {
		pointer := "    "
		labelStyle := lipgloss.NewStyle().Foreground(text)
		valStyle := lipgloss.NewStyle().Foreground(teal)
		if as.setupField == i {
			pointer = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  > ")
			labelStyle = lipgloss.NewStyle().Foreground(peach).Bold(true)
			valStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
		}
		arrows := ""
		if as.setupField == i {
			arrows = DimStyle.Render("  </>")
		}
		padLabel := fmt.Sprintf("%-16s", pf.label+":")
		buf.WriteString(pointer + labelStyle.Render(padLabel) + valStyle.Render(pf.value) + arrows + "\n")
	}

	buf.WriteString("\n")

	// Tasks section
	buf.WriteString(lipgloss.NewStyle().Foreground(blue).Bold(true).Render("  Tasks to schedule:") + "\n")
	buf.WriteString(DimStyle.Render("  " + strings.Repeat(string(ThemeSeparator), width-10)) + "\n")

	if len(as.tasks) == 0 {
		buf.WriteString(DimStyle.Render("  No pending tasks found.") + "\n")
	} else {
		// Determine how many tasks we can show
		maxVisible := as.height/2 - 14
		if maxVisible < 3 {
			maxVisible = 3
		}
		startIdx := 0
		if as.taskCursor >= maxVisible {
			startIdx = as.taskCursor - maxVisible + 1
		}
		endIdx := startIdx + maxVisible
		if endIdx > len(as.tasks) {
			endIdx = len(as.tasks)
		}

		for i := startIdx; i < endIdx; i++ {
			t := as.tasks[i]

			// Priority indicator
			priStr := schedulerPriorityIcon(t.Priority)

			// Task text (truncate if too long)
			taskText := t.Text
			maxTextLen := width - 36
			if maxTextLen < 10 {
				maxTextLen = 10
			}
			if len(taskText) > maxTextLen {
				taskText = taskText[:maxTextLen-3] + "..."
			}

			// Estimated time
			estStr := "___"
			if t.Estimated > 0 {
				estStr = fmt.Sprintf("%dm", t.Estimated)
			}

			// Due date
			dueStr := ""
			if t.DueDate != "" {
				today := time.Now().Format("2006-01-02")
				if t.DueDate == today {
					dueStr = lipgloss.NewStyle().Foreground(red).Render(" due: today")
				} else {
					// Shorten to "Mar 10" style
					if dt, err := time.Parse("2006-01-02", t.DueDate); err == nil {
						dueStr = DimStyle.Render(" due: " + dt.Format("Jan 2"))
					}
				}
			}

			selected := as.setupField >= 5 && as.taskCursor == i
			pointer := "  "
			if selected {
				pointer = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("> ")
			}

			estRendered := DimStyle.Render(fmt.Sprintf("est: %-4s", estStr))
			if as.estimating && selected {
				estRendered = lipgloss.NewStyle().Foreground(green).Bold(true).
					Render("est: " + as.estimateBuf + "_")
			}

			nameStyle := lipgloss.NewStyle().Foreground(text)
			if selected {
				nameStyle = lipgloss.NewStyle().Foreground(peach).Bold(true)
			}

			buf.WriteString("  " + pointer + priStr + " " + nameStyle.Render(taskText) + "  " + estRendered + dueStr + "\n")
		}

		if endIdx < len(as.tasks) {
			buf.WriteString(DimStyle.Render(fmt.Sprintf("  ... +%d more tasks", len(as.tasks)-endIdx)) + "\n")
		}
	}

	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render("  " + strings.Repeat(string(ThemeSeparator), width-10)) + "\n")
	helpLine := "  Tab: next  </>: adjust  e: edit est.  Enter: generate"
	buf.WriteString(DimStyle.Render(helpLine))

	if as.statusMsg != "" {
		buf.WriteString("\n")
		buf.WriteString(lipgloss.NewStyle().Foreground(yellow).Render("  " + as.statusMsg))
	}

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(buf.String())
}

// ---------------------------------------------------------------------------
// View: generating phase
// ---------------------------------------------------------------------------

func (as AIScheduler) viewGenerating(width int) string {
	var buf strings.Builder

	title := lipgloss.NewStyle().Foreground(mauve).Bold(true).
		Render("  " + IconBotChar + " AI Smart Scheduler")
	buf.WriteString(title)
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render("  " + strings.Repeat(string(ThemeSeparator), width-10)))
	buf.WriteString("\n\n")

	spinFrames := []string{"|", "/", "-", "\\"}
	frame := spinFrames[as.spinner%len(spinFrames)]

	providerLabel := "local algorithm"
	switch as.aiProvider {
	case "ollama":
		providerLabel = "Ollama (" + as.ollamaModel + ")"
	case "openai":
		providerLabel = "OpenAI (" + as.openaiModel + ")"
	}

	buf.WriteString(lipgloss.NewStyle().Foreground(yellow).Bold(true).
		Render(fmt.Sprintf("  %s Generating schedule with %s...", frame, providerLabel)))
	buf.WriteString("\n\n")
	buf.WriteString(DimStyle.Render(fmt.Sprintf("  Scheduling %d tasks", len(as.tasks))))
	if len(as.events) > 0 {
		buf.WriteString(DimStyle.Render(fmt.Sprintf(" around %d events", len(as.events))))
	}
	buf.WriteString("\n\n")
	buf.WriteString(DimStyle.Render("  Esc: cancel"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(buf.String())
}

// ---------------------------------------------------------------------------
// View: result phase
// ---------------------------------------------------------------------------

func (as AIScheduler) viewResult(width int) string {
	var buf strings.Builder

	title := lipgloss.NewStyle().Foreground(mauve).Bold(true).
		Render("  " + IconBotChar + " AI Smart Scheduler")
	buf.WriteString(title)
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render("  " + strings.Repeat(string(ThemeSeparator), width-10)))
	buf.WriteString("\n\n")

	// Show provider used
	if as.statusMsg != "" {
		buf.WriteString(lipgloss.NewStyle().Foreground(yellow).Italic(true).
			Render("  "+as.statusMsg) + "\n\n")
	}

	buf.WriteString(lipgloss.NewStyle().Foreground(blue).Bold(true).
		Render("  Suggested Schedule:") + "\n")
	buf.WriteString(DimStyle.Render("  " + strings.Repeat(string(ThemeSeparator), width-10)) + "\n")

	if len(as.schedule) == 0 {
		buf.WriteString(DimStyle.Render("  No schedule could be generated.") + "\n")
	} else {
		maxVisible := as.height/2 - 10
		if maxVisible < 5 {
			maxVisible = 5
		}
		startIdx := as.resultScroll
		if startIdx > len(as.schedule)-maxVisible {
			startIdx = len(as.schedule) - maxVisible
		}
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx := startIdx + maxVisible
		if endIdx > len(as.schedule) {
			endIdx = len(as.schedule)
		}

		for i := startIdx; i < endIdx; i++ {
			slot := as.schedule[i]
			timeStr := fmt.Sprintf("%02d:%02d-%02d:%02d", slot.StartHour, slot.StartMin, slot.EndHour, slot.EndMin)
			timeStyled := lipgloss.NewStyle().Foreground(teal).Render(timeStr)

			// Pad task name
			taskName := slot.Task
			maxNameLen := width - 40
			if maxNameLen < 8 {
				maxNameLen = 8
			}
			if len(taskName) > maxNameLen {
				taskName = taskName[:maxNameLen-3] + "..."
			}
			paddedName := fmt.Sprintf("%-*s", maxNameLen, taskName)

			// Color by type
			var nameStyled string
			switch slot.Type {
			case "break":
				nameStyled = DimStyle.Render(paddedName)
			case "lunch":
				nameStyled = lipgloss.NewStyle().Foreground(green).Render(paddedName)
			case "event":
				nameStyled = lipgloss.NewStyle().Foreground(lavender).Render(paddedName)
			default:
				nameStyled = lipgloss.NewStyle().Foreground(text).Render(paddedName)
			}

			typeTag := lipgloss.NewStyle().Foreground(surface1).Render("[" + slot.Type + "]")
			priStr := ""
			if slot.Type == "task" {
				priStr = " " + schedulerPriorityIcon(slot.Priority)
			}

			buf.WriteString(fmt.Sprintf("  %s  %s %s%s\n", timeStyled, nameStyled, typeTag, priStr))
		}

		if endIdx < len(as.schedule) {
			buf.WriteString(DimStyle.Render(fmt.Sprintf("  ... +%d more slots (scroll down)", len(as.schedule)-endIdx)) + "\n")
		}
	}

	// Total scheduled time
	totalMins := 0
	taskMins := 0
	for _, s := range as.schedule {
		dur := s.slotMinutes()
		totalMins += dur
		if s.Type == "task" {
			taskMins += dur
		}
	}
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render(fmt.Sprintf("  %d tasks scheduled, %dh%02dm productive time",
		as.countScheduledTasks(), taskMins/60, taskMins%60)) + "\n")

	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render("  " + strings.Repeat(string(ThemeSeparator), width-10)) + "\n")
	buf.WriteString(DimStyle.Render("  Enter: apply to planner  r: regenerate  Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(buf.String())
}

// ---------------------------------------------------------------------------
// View: applied phase
// ---------------------------------------------------------------------------

func (as AIScheduler) viewApplied(width int) string {
	var buf strings.Builder

	title := lipgloss.NewStyle().Foreground(mauve).Bold(true).
		Render("  " + IconBotChar + " AI Smart Scheduler")
	buf.WriteString(title)
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render("  " + strings.Repeat(string(ThemeSeparator), width-10)))
	buf.WriteString("\n\n")

	buf.WriteString(lipgloss.NewStyle().Foreground(green).Bold(true).
		Render("  Schedule applied to daily planner!") + "\n\n")

	taskMins := 0
	for _, s := range as.schedule {
		if s.Type == "task" {
			taskMins += s.slotMinutes()
		}
	}

	buf.WriteString(lipgloss.NewStyle().Foreground(text).
		Render(fmt.Sprintf("  %d tasks scheduled across %dh%02dm",
			as.countScheduledTasks(), taskMins/60, taskMins%60)) + "\n\n")

	if len(as.schedule) > 0 {
		first := as.schedule[0]
		last := as.schedule[len(as.schedule)-1]
		buf.WriteString(DimStyle.Render(fmt.Sprintf("  %02d:%02d - %02d:%02d",
			first.StartHour, first.StartMin, last.EndHour, last.EndMin)) + "\n")
	}

	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render("  Enter/Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(buf.String())
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// schedulerPriorityIcon returns a styled priority indicator.
func schedulerPriorityIcon(priority int) string {
	switch priority {
	case 4:
		return lipgloss.NewStyle().Foreground(red).Render("!!")
	case 3:
		return lipgloss.NewStyle().Foreground(peach).Render("! ")
	case 2:
		return lipgloss.NewStyle().Foreground(yellow).Render("- ")
	case 1:
		return lipgloss.NewStyle().Foreground(blue).Render("  ")
	default:
		return lipgloss.NewStyle().Foreground(text).Render("  ")
	}
}

// countScheduledTasks counts how many "task" type slots are in the schedule.
func (as AIScheduler) countScheduledTasks() int {
	count := 0
	for _, s := range as.schedule {
		if s.Type == "task" {
			count++
		}
	}
	return count
}
