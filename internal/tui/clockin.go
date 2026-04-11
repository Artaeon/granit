package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// clockInTickMsg is sent every second to update the clock-in timer display.
type clockInTickMsg struct{}

// clockInReminderMsg is sent when a reminder fires.
type clockInReminderMsg struct {
	Text string
}

// clockInSession mirrors the CLI clockSession type for JSON compatibility.
type clockInSession struct {
	Start   string `json:"start"`
	End     string `json:"end,omitempty"`
	Project string `json:"project,omitempty"`
}

// clockInData mirrors the CLI clockData type for JSON compatibility.
type clockInData struct {
	Active   *clockInSession  `json:"active,omitempty"`
	Sessions []clockInSession `json:"sessions"`
}

// clockInReminder mirrors the CLI reminder type for JSON compatibility.
type clockInReminder struct {
	Text    string `json:"text"`
	Time    string `json:"time"`    // "HH:MM"
	Repeat  string `json:"repeat"`  // "daily", "weekdays", "once"
	Enabled bool   `json:"enabled"`
}

// ClockIn manages the clock-in timer and reminders within the TUI.
type ClockIn struct {
	vaultPath string
	active    bool   // whether currently clocked in
	startTime time.Time
	project   string
	elapsed   time.Duration

	// Reminders
	reminders     []clockInReminder
	lastCheckMin  string // "HH:MM" of last reminder check to avoid duplicates
	firedToday    map[string]bool

	// Today's completed sessions (for display)
	todaySessions []clockInSession
	todayTotal    time.Duration
}

// NewClockIn creates a new ClockIn component.
func NewClockIn(vaultPath string) ClockIn {
	c := ClockIn{
		vaultPath:  vaultPath,
		firedToday: make(map[string]bool),
	}
	c.loadState()
	c.loadReminders()
	c.loadTodaySessions()
	return c
}

// IsActive returns whether currently clocked in.
func (c *ClockIn) IsActive() bool {
	return c.active
}

// Project returns the current project name.
func (c *ClockIn) Project() string {
	return c.project
}

// Elapsed returns the elapsed time of the current session.
func (c *ClockIn) Elapsed() time.Duration {
	return c.elapsed
}

// TodaySessions returns completed sessions for today.
func (c *ClockIn) TodaySessions() []clockInSession {
	return c.todaySessions
}

// TodayTotal returns total work time today (including active).
func (c *ClockIn) TodayTotal() time.Duration {
	total := c.todayTotal
	if c.active {
		total += c.elapsed
	}
	return total
}

// StatusString returns a short status for the status bar.
func (c *ClockIn) StatusString() string {
	if !c.active {
		return ""
	}
	h := int(c.elapsed.Hours())
	m := int(c.elapsed.Minutes()) % 60
	s := int(c.elapsed.Seconds()) % 60

	base := fmt.Sprintf("⏱ %d:%02d:%02d", h, m, s)
	if c.project != "" {
		label := TruncateDisplay(c.project, 20)
		base += " · " + label
	}
	return base
}

// ClockInCmd starts a clock-in session and begins ticking.
func (c *ClockIn) ClockInCmd(project string) tea.Cmd {
	if c.active {
		return nil
	}
	c.active = true
	c.startTime = time.Now()
	c.project = project
	c.elapsed = 0
	c.saveState()
	return c.tick()
}

// ClockOutCmd stops the clock-in session and saves it.
func (c *ClockIn) ClockOutCmd() tea.Cmd {
	if !c.active {
		return nil
	}

	end := time.Now()
	session := clockInSession{
		Start:   c.startTime.Format(time.RFC3339),
		End:     end.Format(time.RFC3339),
		Project: c.project,
	}

	// Save to clock.json
	data := c.loadClockData()
	data.Active = nil
	data.Sessions = append(data.Sessions, session)
	c.saveClockData(data)

	// Save to vault timetracking note
	c.saveSessionNote(c.startTime, end, c.project, c.elapsed)

	c.active = false
	c.project = ""
	c.elapsed = 0
	c.loadTodaySessions()

	return nil
}

// Update handles tick messages.
func (c *ClockIn) Update(msg tea.Msg) (ClockIn, tea.Cmd) {
	switch msg.(type) {
	case clockInTickMsg:
		if c.active {
			c.elapsed = time.Since(c.startTime)
		}

		// Check reminders
		reminderCmd := c.checkReminders()

		if c.active {
			if reminderCmd != nil {
				return *c, tea.Batch(c.tick(), reminderCmd)
			}
			return *c, c.tick()
		}
		// Even if not clocked in, keep ticking for reminder checks
		if reminderCmd != nil {
			return *c, tea.Batch(c.tickSlow(), reminderCmd)
		}
		return *c, c.tickSlow()
	}
	return *c, nil
}

// StartTicking begins the tick loop (call on app init).
func (c *ClockIn) StartTicking() tea.Cmd {
	if c.active {
		return c.tick()
	}
	return c.tickSlow()
}

// tick sends a tick every second (when clocked in).
func (c *ClockIn) tick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return clockInTickMsg{}
	})
}

// tickSlow sends a tick every 30 seconds (for reminder checks when not clocked in).
func (c *ClockIn) tickSlow() tea.Cmd {
	return tea.Tick(30*time.Second, func(time.Time) tea.Msg {
		return clockInTickMsg{}
	})
}

// checkReminders returns a Cmd if a reminder should fire right now.
func (c *ClockIn) checkReminders() tea.Cmd {
	now := time.Now()
	currentMin := now.Format("15:04")

	// Only check once per minute
	if currentMin == c.lastCheckMin {
		return nil
	}
	c.lastCheckMin = currentMin

	// Reset fired map at midnight
	if currentMin == "00:00" {
		c.firedToday = make(map[string]bool)
	}

	for i, r := range c.reminders {
		if !r.Enabled {
			continue
		}
		if r.Time != currentMin {
			continue
		}
		// Already fired today?
		key := fmt.Sprintf("%d:%s", i, r.Time)
		if c.firedToday[key] {
			continue
		}
		// Check weekday filter
		if r.Repeat == "weekdays" {
			wd := now.Weekday()
			if wd == time.Saturday || wd == time.Sunday {
				continue
			}
		}

		c.firedToday[key] = true

		// Auto-disable "once" reminders
		if r.Repeat == "once" {
			c.reminders[i].Enabled = false
			c.saveRemindersFile()
		}

		text := r.Text
		return func() tea.Msg {
			return clockInReminderMsg{Text: text}
		}
	}
	return nil
}

// ── Persistence ────────────────────────────────────────────────────

func (c *ClockIn) loadState() {
	data := c.loadClockData()
	if data.Active != nil {
		start, err := time.Parse(time.RFC3339, data.Active.Start)
		if err == nil {
			c.active = true
			c.startTime = start
			c.project = data.Active.Project
			c.elapsed = time.Since(start)
		}
	}
}

func (c *ClockIn) saveState() {
	data := c.loadClockData()
	if c.active {
		data.Active = &clockInSession{
			Start:   c.startTime.Format(time.RFC3339),
			Project: c.project,
		}
	} else {
		data.Active = nil
	}
	c.saveClockData(data)
}

func (c *ClockIn) loadClockData() clockInData {
	var data clockInData
	raw, err := os.ReadFile(filepath.Join(c.vaultPath, ".granit", "clock.json"))
	if err != nil {
		return data
	}
	_ = json.Unmarshal(raw, &data)
	return data
}

func (c *ClockIn) saveClockData(data clockInData) {
	if c.vaultPath == "" {
		return
	}
	dir := filepath.Join(c.vaultPath, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return
	}
	_ = atomicWriteState(filepath.Join(dir, "clock.json"), raw)
}

func (c *ClockIn) loadReminders() {
	raw, err := os.ReadFile(filepath.Join(c.vaultPath, ".granit", "reminders.json"))
	if err != nil {
		return
	}
	_ = json.Unmarshal(raw, &c.reminders)
}

func (c *ClockIn) saveRemindersFile() {
	if c.vaultPath == "" {
		return
	}
	dir := filepath.Join(c.vaultPath, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	raw, err := json.MarshalIndent(c.reminders, "", "  ")
	if err != nil {
		return
	}
	_ = atomicWriteState(filepath.Join(dir, "reminders.json"), raw)
}

func (c *ClockIn) loadTodaySessions() {
	data := c.loadClockData()
	today := time.Now().Format("2006-01-02")
	c.todaySessions = nil
	c.todayTotal = 0
	for _, s := range data.Sessions {
		start, err := time.Parse(time.RFC3339, s.Start)
		if err != nil || start.Format("2006-01-02") != today {
			continue
		}
		c.todaySessions = append(c.todaySessions, s)
		if s.End != "" {
			end, err := time.Parse(time.RFC3339, s.End)
			if err == nil {
				c.todayTotal += end.Sub(start)
			}
		}
	}
}

func (c *ClockIn) saveSessionNote(start, end time.Time, project string, elapsed time.Duration) {
	if c.vaultPath == "" {
		return
	}
	dir := filepath.Join(c.vaultPath, "Timetracking")
	_ = os.MkdirAll(dir, 0755)
	dateStr := start.Format("2006-01-02")
	filePath := filepath.Join(dir, dateStr+".md")

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		weekday := start.Weekday().String()
		header := fmt.Sprintf("---\ntitle: Time Log %s\ndate: %s\ntype: timelog\ntags: [timelog]\n---\n\n# Time Log — %s (%s)\n\n| Start | End | Project | Duration |\n|-------|-----|---------|----------|\n",
			dateStr, dateStr, dateStr, weekday)
		_ = os.WriteFile(filePath, []byte(header), 0644)
	}

	dur := elapsed.Truncate(time.Second)
	h := int(dur.Hours())
	m := int(dur.Minutes()) % 60
	durStr := fmt.Sprintf("%dh %02dm", h, m)
	if h == 0 {
		durStr = fmt.Sprintf("%dm", m)
	}

	if project == "" {
		project = "work"
	}
	row := fmt.Sprintf("| %s | %s | %s | %s |\n",
		start.Format("15:04"), end.Format("15:04"), project, durStr)

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(row)
}

// ── PlanMyDay integration ──────────────────────────────────────────

// SessionsForPlan returns today's sessions as daySlot entries for the planner.
func (c *ClockIn) SessionsForPlan() []clockPlanSlot {
	var slots []clockPlanSlot
	today := time.Now().Format("2006-01-02")

	data := c.loadClockData()
	for _, s := range data.Sessions {
		start, err := time.Parse(time.RFC3339, s.Start)
		if err != nil || start.Format("2006-01-02") != today {
			continue
		}
		end, err := time.Parse(time.RFC3339, s.End)
		if err != nil {
			continue
		}
		project := s.Project
		if project == "" {
			project = "work"
		}
		dur := end.Sub(start).Truncate(time.Minute)
		slots = append(slots, clockPlanSlot{
			Start:    start.Format("15:04"),
			End:      end.Format("15:04"),
			Project:  project,
			Duration: dur,
		})
	}

	// Include active session
	if data.Active != nil {
		start, err := time.Parse(time.RFC3339, data.Active.Start)
		if err == nil && start.Format("2006-01-02") == today {
			project := data.Active.Project
			if project == "" {
				project = "work"
			}
			dur := time.Since(start).Truncate(time.Minute)
			slots = append(slots, clockPlanSlot{
				Start:    start.Format("15:04"),
				End:      time.Now().Format("15:04"),
				Project:  project,
				Duration: dur,
				Active:   true,
			})
		}
	}

	return slots
}

// clockPlanSlot represents a clocked session for display in the planner.
type clockPlanSlot struct {
	Start    string
	End      string
	Project  string
	Duration time.Duration
	Active   bool
}

// FormatDuration returns a human-readable duration string.
func (s clockPlanSlot) FormatDuration() string {
	h := int(s.Duration.Hours())
	m := int(s.Duration.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// RemindersForStatus returns active reminders as strings for display.
func (c *ClockIn) RemindersForStatus() []string {
	var active []string
	for _, r := range c.reminders {
		if r.Enabled {
			active = append(active, fmt.Sprintf("%s — %s (%s)", r.Time, r.Text, r.Repeat))
		}
	}
	return active
}

// NextReminder returns the next upcoming reminder text and time, or empty strings.
func (c *ClockIn) NextReminder() (string, string) {
	now := time.Now()
	currentMin := now.Format("15:04")
	var bestTime, bestText string

	for _, r := range c.reminders {
		if !r.Enabled {
			continue
		}
		if r.Time > currentMin {
			if bestTime == "" || r.Time < bestTime {
				bestTime = r.Time
				bestText = r.Text

				// Check weekday filter
				if r.Repeat == "weekdays" {
					wd := now.Weekday()
					if wd == time.Saturday || wd == time.Sunday {
						bestTime = ""
						bestText = ""
					}
				}
			}
		}
	}
	return bestText, bestTime
}

