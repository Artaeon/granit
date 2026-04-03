package tui

import (
	"encoding/json"
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
// Data types
// ---------------------------------------------------------------------------

// GoalStatus represents the lifecycle state of a goal.
type GoalStatus string

const (
	GoalStatusActive    GoalStatus = "active"
	GoalStatusCompleted GoalStatus = "completed"
	GoalStatusArchived  GoalStatus = "archived"
	GoalStatusPaused    GoalStatus = "paused"
)

// GoalMilestone is a sub-step within a goal.
type GoalMilestone struct {
	Text      string `json:"text"`
	Done      bool   `json:"done"`
	DueDate   string `json:"due_date,omitempty"` // YYYY-MM-DD
	CompletedAt string `json:"completed_at,omitempty"`
}

// Goal is a standalone goal independent of projects or habits.
type Goal struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Status      GoalStatus      `json:"status"`
	Category    string          `json:"category,omitempty"` // e.g. "Career", "Health", "Learning"
	Color       string          `json:"color,omitempty"`  // "red","blue","green","yellow","mauve","pink","teal"
	Tags        []string        `json:"tags,omitempty"`
	TargetDate  string          `json:"target_date,omitempty"` // YYYY-MM-DD
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
	CompletedAt string          `json:"completed_at,omitempty"`
	Project     string          `json:"project,omitempty"` // linked project name
	Milestones      []GoalMilestone `json:"milestones"`
	Notes           string          `json:"notes,omitempty"`
	ReviewFrequency string          `json:"review_frequency,omitempty"` // "weekly", "monthly", "quarterly"
	LastReviewed    string          `json:"last_reviewed,omitempty"`    // YYYY-MM-DD
	ReviewLog       []GoalReview    `json:"review_log,omitempty"`
}

// goalColorMap returns the theme color for a goal color name.
func goalColorMap(name string) lipgloss.Color {
	switch name {
	case "red":
		return red
	case "blue":
		return blue
	case "green":
		return green
	case "yellow":
		return yellow
	case "mauve":
		return mauve
	case "pink":
		return pink
	case "teal":
		return teal
	default:
		return blue
	}
}

// GoalReview records a periodic check-in on a goal.
type GoalReview struct {
	Date     string `json:"date"`
	Note     string `json:"note"`
	Progress int    `json:"progress"` // snapshot at time of review
}

// Progress returns milestone completion percentage (0-100).
func (g Goal) Progress() int {
	if len(g.Milestones) == 0 {
		if g.Status == GoalStatusCompleted {
			return 100
		}
		return 0
	}
	done := 0
	for _, m := range g.Milestones {
		if m.Done {
			done++
		}
	}
	return done * 100 / len(g.Milestones)
}

// DoneCount returns the number of completed milestones.
func (g Goal) DoneCount() int {
	done := 0
	for _, m := range g.Milestones {
		if m.Done {
			done++
		}
	}
	return done
}

// IsOverdue returns true if the goal has a target date in the past and is not completed.
func (g Goal) IsOverdue() bool {
	if g.TargetDate == "" || g.Status == GoalStatusCompleted || g.Status == GoalStatusArchived {
		return false
	}
	target, err := time.Parse("2006-01-02", g.TargetDate)
	if err != nil {
		return false
	}
	return time.Now().After(target)
}

// DaysRemaining returns days until target date (-1 if no date set).
func (g Goal) DaysRemaining() int {
	if g.TargetDate == "" {
		return -1
	}
	target, err := time.Parse("2006-01-02", g.TargetDate)
	if err != nil {
		return -1
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	return int(target.Sub(today).Hours() / 24)
}

// IsDueForReview returns true if the goal's review period has elapsed.
func (g Goal) IsDueForReview() bool {
	if g.ReviewFrequency == "" || g.Status != GoalStatusActive {
		return false
	}
	if g.LastReviewed == "" {
		return true
	}
	last, err := time.Parse("2006-01-02", g.LastReviewed)
	if err != nil {
		return true
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	switch g.ReviewFrequency {
	case "weekly":
		return today.After(last.AddDate(0, 0, 7))
	case "monthly":
		return today.After(last.AddDate(0, 1, 0))
	case "quarterly":
		return today.After(last.AddDate(0, 3, 0))
	}
	return false
}

// NextReviewDate returns the next scheduled review date as YYYY-MM-DD.
func (g Goal) NextReviewDate() string {
	if g.ReviewFrequency == "" {
		return ""
	}
	base := g.LastReviewed
	if base == "" {
		base = g.CreatedAt
	}
	last, err := time.Parse("2006-01-02", base)
	if err != nil {
		return ""
	}
	switch g.ReviewFrequency {
	case "weekly":
		return last.AddDate(0, 0, 7).Format("2006-01-02")
	case "monthly":
		return last.AddDate(0, 1, 0).Format("2006-01-02")
	case "quarterly":
		return last.AddDate(0, 3, 0).Format("2006-01-02")
	}
	return ""
}

// TimeframeLabel returns a human-readable time remaining label.
func (g Goal) TimeframeLabel() string {
	days := g.DaysRemaining()
	if days < 0 {
		absDays := -days
		if absDays < 30 {
			return fmt.Sprintf("%dd overdue", absDays)
		}
		return fmt.Sprintf("%dmo overdue", absDays/30)
	}
	if days == 0 {
		return "due today"
	}
	if days == 1 {
		return "1d left"
	}
	if days < 14 {
		return fmt.Sprintf("%dd left", days)
	}
	if days < 60 {
		return fmt.Sprintf("%dw left", days/7)
	}
	if days < 365 {
		return fmt.Sprintf("%dmo left", days/30)
	}
	years := days / 365
	rem := (days % 365) / 30
	if rem > 0 {
		return fmt.Sprintf("%dy%dmo left", years, rem)
	}
	return fmt.Sprintf("%dy left", years)
}

// ---------------------------------------------------------------------------
// Input modes
// ---------------------------------------------------------------------------

type goalInputMode int

const (
	goalInputNone goalInputMode = iota
	goalInputTitle              // creating new goal: title
	goalInputDate               // creating new goal: target date
	goalInputCategory           // creating new goal: category
	goalInputMilestone          // adding milestone
	goalInputNotes              // editing notes
	goalInputDescription        // editing description
	goalInputReviewFreq         // setting review frequency
	goalInputReview             // writing review reflection
	goalInputMilestoneDue       // setting milestone due date
	goalInputColor              // setting goal color
	goalInputHelp               // showing help
)

// ---------------------------------------------------------------------------
// Goal views
// ---------------------------------------------------------------------------

type goalViewMode int

const (
	goalViewAll       goalViewMode = iota // all active goals
	goalViewByCategory                    // grouped by category
	goalViewTimeline                      // sorted by deadline
	goalViewCompleted                     // completed/archived
)

// ---------------------------------------------------------------------------
// GoalsMode overlay
// ---------------------------------------------------------------------------

// GoalsMode is the standalone goal management overlay.
type GoalsMode struct {
	active bool
	width  int
	height int

	vaultRoot string
	allTasks  []Task // for linked task stats
	goals     []Goal
	filtered  []Goal // currently visible goals

	view     goalViewMode
	cursor   int
	scroll   int
	input    goalInputMode
	inputBuf string

	// Expanded goal state
	expanded    int    // index in filtered, -1 = none
	expandedID  string // ID of expanded goal (survives rebuildFiltered)
	milestoneCur int

	// Snapshot of target goal for input modes (prevents cursor drift)
	inputGoalID string

	// New goal staging
	newGoalTitle    string
	newGoalDate     string
	newGoalCategory string

	// Confirmation prompt
	confirmMsg    string // "Delete goal X?" — empty means no pending confirm
	confirmAction func() // action to run on 'y'

	statusMsg   string
	fileChanged bool // set when Tasks.md is modified (createTaskFromMilestone)

	// AI config
	ai        AIConfig
	aiPending bool

	// AI review result
	reviewText   string // last AI review text
	reviewGoalID string // goal ID the review belongs to

	// AI coach (holistic goal analysis)
	coachText string
	showCoach bool
}

// NewGoalsMode creates a new goals overlay.
func NewGoalsMode() GoalsMode {
	return GoalsMode{
		expanded: -1,
	}
}

func (gm GoalsMode) IsActive() bool {
	return gm.active
}

func (gm *GoalsMode) SetSize(w, h int) {
	gm.width = w
	gm.height = h
}

func (gm *GoalsMode) Open(vaultRoot string, tasks ...[]Task) {
	gm.active = true
	gm.vaultRoot = vaultRoot
	if len(tasks) > 0 {
		gm.allTasks = tasks[0]
	} else {
		gm.allTasks = nil
	}
	gm.view = goalViewAll
	gm.cursor = 0
	gm.scroll = 0
	gm.input = goalInputNone
	gm.inputBuf = ""
	gm.expanded = -1
	gm.expandedID = ""
	gm.milestoneCur = 0
	gm.statusMsg = ""
	gm.loadGoals()
	gm.rebuildFiltered()
}

func (gm *GoalsMode) Close() {
	gm.active = false
}

// GetGoals returns the current list of goals.
func (gm *GoalsMode) GetGoals() []Goal {
	return gm.goals
}

// ---------------------------------------------------------------------------
// Storage
// ---------------------------------------------------------------------------

func (gm *GoalsMode) goalsPath() string {
	return filepath.Join(gm.vaultRoot, ".granit", "goals.json")
}

func (gm *GoalsMode) loadGoals() {
	gm.goals = nil
	data, err := os.ReadFile(gm.goalsPath())
	if err != nil {
		return
	}
	if err := json.Unmarshal(data, &gm.goals); err != nil {
		// Corrupted JSON — start with empty goals rather than nil
		gm.goals = []Goal{}
	}
}

func (gm *GoalsMode) saveGoals() {
	dir := filepath.Join(gm.vaultRoot, ".granit")
	_ = os.MkdirAll(dir, 0755)
	data, err := json.MarshalIndent(gm.goals, "", "  ")
	if err != nil {
		return // don't write corrupt data
	}
	_ = os.WriteFile(gm.goalsPath(), data, 0644)
}

type gmAIResultMsg struct {
	milestones []GoalMilestone
	goalID     string
	err        error
}

// aiGenerateMilestones sends a goal to the LLM to generate milestones.
func (gm *GoalsMode) aiGenerateMilestones(goal Goal) tea.Cmd {
	prompt := fmt.Sprintf(
		"Create 4-8 specific, measurable milestones for this goal.\n"+
			"Goal: %s\nDescription: %s\nCategory: %s\nTarget date: %s\n\n"+
			"Respond with ONLY a list of milestones, one per line, each starting with '- [ ] '. "+
			"Make each milestone concrete and actionable. Include rough timeframes if the target date is set. No explanations.",
		goal.Title, goal.Description, goal.Category, goal.TargetDate,
	)

	ai := gm.ai
	return func() tea.Msg {
		resp, err := ai.Chat("You are a goal planning assistant. Be concrete and actionable.", prompt)

		if err != nil {
			return gmAIResultMsg{err: err, goalID: goal.ID}
		}

		var milestones []GoalMilestone
		for _, line := range strings.Split(resp, "\n") {
			trimmed := strings.TrimSpace(line)
			text := ""
			if strings.HasPrefix(trimmed, "- [ ] ") {
				text = strings.TrimSpace(trimmed[6:])
			} else if strings.HasPrefix(trimmed, "- ") && !strings.HasPrefix(trimmed, "- [") {
				text = strings.TrimSpace(trimmed[2:])
			}
			if text != "" {
				milestones = append(milestones, GoalMilestone{Text: text})
			}
		}
		return gmAIResultMsg{milestones: milestones, goalID: goal.ID}
	}
}

// gmAIReviewMsg carries an AI review of a goal.
type gmAIReviewMsg struct {
	review string
	err    error
	goalID string
}

// aiReviewGoal sends a goal to the LLM for an honest assessment.
func (gm *GoalsMode) aiReviewGoal(goal Goal) tea.Cmd {
	// Build milestone list
	var msLines []string
	for _, ms := range goal.Milestones {
		check := "[ ]"
		if ms.Done {
			check = "[x]"
		}
		msLines = append(msLines, fmt.Sprintf("  %s %s", check, ms.Text))
	}
	msText := "(none)"
	if len(msLines) > 0 {
		msText = strings.Join(msLines, "\n")
	}
	targetDate := goal.TargetDate
	if targetDate == "" {
		targetDate = "(not set)"
	}

	prompt := fmt.Sprintf(
		"You are DEEPCOVEN, a direct and honest personal assistant.\n"+
			"Review this goal and provide a brief, actionable assessment.\n\n"+
			"GOAL: %s\nDESCRIPTION: %s\nSTATUS: %s\nCREATED: %s\nTARGET DATE: %s\nPROGRESS: %d%%\nMILESTONES:\n%s\n\n"+
			"Give a 3-5 line assessment covering:\n"+
			"1. Progress vs timeline (are they on track?)\n"+
			"2. What's the most important next action?\n"+
			"3. One honest observation (encouragement or reality check)\n\n"+
			"Be direct. No filler.",
		goal.Title, goal.Description, string(goal.Status), goal.CreatedAt, targetDate, goal.Progress(), msText,
	)

	ai := gm.ai
	return func() tea.Msg {
		resp, err := ai.Chat("You are DEEPCOVEN, a direct and honest personal assistant.", prompt)
		return gmAIReviewMsg{review: strings.TrimSpace(resp), err: err, goalID: goal.ID}
	}
}

// gmAIDecomposeMsg carries AI decomposition results for a goal.
type gmAIDecomposeMsg struct {
	milestones []GoalMilestone
	category   string
	targetDate string
	tags       []string
	err        error
	goalID     string
}

// aiDecomposeGoal sends a goal to the LLM for full decomposition.
func (gm *GoalsMode) aiDecomposeGoal(goal Goal) tea.Cmd {
	today := time.Now().Format("2006-01-02")
	desc := goal.Description
	if desc == "" {
		desc = "(none)"
	}
	prompt := fmt.Sprintf(
		"Break down this goal into actionable milestones with realistic dates.\n\n"+
			"GOAL: %s\nDESCRIPTION: %s\nTODAY: %s\n\n"+
			"Respond in this exact format:\n"+
			"CATEGORY: {one of: Career, Health, Learning, Creative, Financial, Social, Personal}\n"+
			"TARGET: {YYYY-MM-DD suggested completion date}\n"+
			"TAGS: {comma-separated tags}\n"+
			"MILESTONES:\n"+
			"- {milestone text} | {YYYY-MM-DD due date}\n"+
			"- {milestone text} | {YYYY-MM-DD due date}\n"+
			"...\n\n"+
			"Generate 4-8 milestones. Space dates realistically.",
		goal.Title, desc, today,
	)

	ai := gm.ai
	return func() tea.Msg {
		resp, err := ai.Chat("You are a goal planning assistant. Be concrete and actionable.", prompt)

		if err != nil {
			return gmAIDecomposeMsg{err: err, goalID: goal.ID}
		}

		var category, targetDate string
		var tags []string
		var milestones []GoalMilestone

		for _, line := range strings.Split(resp, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "CATEGORY:") {
				category = strings.TrimSpace(trimmed[len("CATEGORY:"):])
			} else if strings.HasPrefix(trimmed, "TARGET:") {
				td := strings.TrimSpace(trimmed[len("TARGET:"):])
				// Validate date format
				if _, parseErr := time.Parse("2006-01-02", td); parseErr == nil {
					targetDate = td
				}
			} else if strings.HasPrefix(trimmed, "TAGS:") {
				raw := strings.TrimSpace(trimmed[len("TAGS:"):])
				for _, t := range strings.Split(raw, ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						tags = append(tags, t)
					}
				}
			} else if strings.HasPrefix(trimmed, "- ") {
				// Parse milestone: "- text | YYYY-MM-DD"
				msText := strings.TrimSpace(trimmed[2:])
				dueDate := ""
				if idx := strings.LastIndex(msText, "|"); idx >= 0 {
					datePart := strings.TrimSpace(msText[idx+1:])
					if _, parseErr := time.Parse("2006-01-02", datePart); parseErr == nil {
						dueDate = datePart
					}
					msText = strings.TrimSpace(msText[:idx])
				}
				// Strip checkbox prefix if present
				msText = strings.TrimPrefix(msText, "[ ] ")
				msText = strings.TrimPrefix(msText, "[x] ")
				if msText != "" {
					milestones = append(milestones, GoalMilestone{
						Text:    msText,
						DueDate: dueDate,
					})
				}
			}
		}

		return gmAIDecomposeMsg{
			milestones: milestones,
			category:   category,
			targetDate: targetDate,
			tags:       tags,
			goalID:     goal.ID,
		}
	}
}

// gmAICoachMsg carries a holistic AI analysis of all goals.
type gmAICoachMsg struct {
	analysis string
	err      error
}

// aiGoalCoach sends all goals to the LLM for holistic analysis.
func (gm *GoalsMode) aiGoalCoach() tea.Cmd {
	ai := gm.ai
	goals := make([]Goal, len(gm.goals))
	copy(goals, gm.goals)

	return func() tea.Msg {
		now := time.Now()
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Today: %s\n\n", now.Format("2006-01-02")))

		for _, g := range goals {
			progress := g.Progress()
			doneCount := g.DoneCount()
			totalMs := len(g.Milestones)

			sb.WriteString(fmt.Sprintf("GOAL: %s\n", g.Title))
			sb.WriteString(fmt.Sprintf("  Status: %s | Category: %s | Progress: %d%%\n", string(g.Status), g.Category, progress))
			sb.WriteString(fmt.Sprintf("  Milestones: %d/%d done\n", doneCount, totalMs))
			if g.TargetDate != "" {
				sb.WriteString(fmt.Sprintf("  Target: %s", g.TargetDate))
				if g.IsOverdue() {
					sb.WriteString(" (OVERDUE)")
				}
				sb.WriteString("\n")
			}
			if g.CreatedAt != "" {
				sb.WriteString(fmt.Sprintf("  Created: %s\n", g.CreatedAt))
			}
			if g.LastReviewed != "" {
				sb.WriteString(fmt.Sprintf("  Last reviewed: %s\n", g.LastReviewed))
			}
			sb.WriteString("\n")
		}

		systemPrompt := "You are DEEPCOVEN, a direct and honest personal goal coach.\n\n" +
			"Analyze ALL of the user's goals holistically. Look for:\n" +
			"1. Goals competing for the same time/energy\n" +
			"2. Stalled goals that need attention or should be paused\n" +
			"3. Goals off-track vs their target dates\n" +
			"4. Priority adjustments based on deadline proximity and progress\n" +
			"5. Quick wins that could build momentum\n\n" +
			"Be brutally honest. No filler. Format as:\n" +
			"## Goal Health Report\n" +
			"### On Track\n- {goal}: {1 line status}\n" +
			"### Needs Attention\n- {goal}: {what's wrong and what to do}\n" +
			"### Recommended Priority Order\n1. {goal} — {why first}\n" +
			"### Coach's Note\n{1-2 sentences of honest, actionable advice}"

		resp, err := ai.Chat(systemPrompt, sb.String())
		return gmAICoachMsg{analysis: strings.TrimSpace(resp), err: err}
	}
}

// staleWarning returns a warning string for goals that need attention.
func staleWarning(g Goal) string {
	if g.Status != GoalStatusActive {
		return ""
	}
	created, err := time.Parse("2006-01-02", g.CreatedAt)
	if err != nil {
		return ""
	}
	now := time.Now()
	daysSinceCreated := int(now.Sub(created).Hours() / 24)

	if g.IsOverdue() {
		return "Overdue \u2014 review timeline or archive"
	}
	if g.IsDueForReview() {
		return "Review due \u2014 press R"
	}
	if daysSinceCreated > 14 && len(g.Milestones) == 0 {
		return "No milestones yet \u2014 press D to break this down"
	}
	if daysSinceCreated > 21 && g.Progress() == 0 {
		return "No progress in 3 weeks \u2014 still relevant?"
	}
	return ""
}

func (gm *GoalsMode) nextID() string {
	max := 0
	for _, g := range gm.goals {
		if len(g.ID) > 1 && g.ID[0] == 'G' {
			n := 0
			_, _ = fmt.Sscanf(g.ID[1:], "%d", &n)
			if n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("G%03d", max+1)
}

// ---------------------------------------------------------------------------
// Filtering
// ---------------------------------------------------------------------------

func (gm *GoalsMode) rebuildFiltered() {
	gm.filtered = nil
	switch gm.view {
	case goalViewAll:
		for _, g := range gm.goals {
			if g.Status == GoalStatusActive || g.Status == GoalStatusPaused {
				gm.filtered = append(gm.filtered, g)
			}
		}
	case goalViewByCategory:
		for _, g := range gm.goals {
			if g.Status == GoalStatusActive || g.Status == GoalStatusPaused {
				gm.filtered = append(gm.filtered, g)
			}
		}
		sort.SliceStable(gm.filtered, func(i, j int) bool {
			return gm.filtered[i].Category < gm.filtered[j].Category
		})
	case goalViewTimeline:
		for _, g := range gm.goals {
			if g.Status == GoalStatusActive || g.Status == GoalStatusPaused {
				gm.filtered = append(gm.filtered, g)
			}
		}
		sort.SliceStable(gm.filtered, func(i, j int) bool {
			if gm.filtered[i].TargetDate == "" {
				return false
			}
			if gm.filtered[j].TargetDate == "" {
				return true
			}
			return gm.filtered[i].TargetDate < gm.filtered[j].TargetDate
		})
	case goalViewCompleted:
		for _, g := range gm.goals {
			if g.Status == GoalStatusCompleted || g.Status == GoalStatusArchived {
				gm.filtered = append(gm.filtered, g)
			}
		}
	}
	gm.restoreExpanded()
}

// restoreExpanded re-finds the expanded goal by ID after a rebuild.
func (gm *GoalsMode) restoreExpanded() {
	if gm.expandedID == "" {
		gm.expanded = -1
		return
	}
	for i, g := range gm.filtered {
		if g.ID == gm.expandedID {
			gm.expanded = i
			return
		}
	}
	// Goal no longer in this view (e.g. completed/archived)
	gm.expanded = -1
	gm.expandedID = ""
	gm.milestoneCur = 0
}

func (gm *GoalsMode) ensureVisible() {
	maxVisible := gm.visibleHeight()
	if gm.cursor < gm.scroll {
		gm.scroll = gm.cursor
	}
	if gm.cursor >= gm.scroll+maxVisible {
		gm.scroll = gm.cursor - maxVisible + 1
	}
}

// createTaskFromMilestone writes a new task to Tasks.md linked to the goal.
func (gm *GoalsMode) createTaskFromMilestone(goal Goal, ms GoalMilestone) {
	tasksPath := filepath.Join(gm.vaultRoot, "Tasks.md")
	taskLine := fmt.Sprintf("\n- [ ] %s goal:%s\n", ms.Text, goal.ID)

	existing, err := os.ReadFile(tasksPath)
	if err != nil {
		existing = []byte("# Tasks\n")
	}
	_ = os.WriteFile(tasksPath, append(existing, []byte(taskLine)...), 0644)
	gm.statusMsg = "Task created: " + ms.Text
	gm.fileChanged = true
}

// WasFileChanged returns true if the overlay modified vault files, then resets the flag.
func (gm *GoalsMode) WasFileChanged() bool {
	if gm.fileChanged {
		gm.fileChanged = false
		return true
	}
	return false
}

// linkedTaskStats returns done/total counts for tasks linked to a goal.
func (gm *GoalsMode) linkedTaskStats(goalID string) (done, total int) {
	for _, t := range gm.allTasks {
		if t.GoalID == goalID {
			total++
			if t.Done {
				done++
			}
		}
	}
	return
}

func (gm *GoalsMode) setReviewFrequency(freq string) {
	if gm.inputGoalID == "" {
		return
	}
	idx := gm.findGoalIndex(gm.inputGoalID)
	if idx < 0 {
		return
	}
	now := time.Now().Format("2006-01-02")
	gm.goals[idx].ReviewFrequency = freq
	if freq != "" && gm.goals[idx].LastReviewed == "" {
		gm.goals[idx].LastReviewed = now
	}
	if freq == "" {
		gm.goals[idx].LastReviewed = ""
	}
	gm.goals[idx].UpdatedAt = now
	gm.saveGoals()
	gm.rebuildFiltered()
	if freq == "" {
		gm.statusMsg = "Reviews removed"
	} else {
		gm.statusMsg = "Review set: " + freq
	}
}

func (gm *GoalsMode) submitReview(note string) {
	if gm.inputGoalID == "" {
		return
	}
	idx := gm.findGoalIndex(gm.inputGoalID)
	if idx < 0 {
		return
	}
	now := time.Now().Format("2006-01-02")
	review := GoalReview{
		Date:     now,
		Note:     note,
		Progress: gm.goals[idx].Progress(),
	}
	gm.goals[idx].ReviewLog = append(gm.goals[idx].ReviewLog, review)
	gm.goals[idx].LastReviewed = now
	gm.goals[idx].UpdatedAt = now
	gm.saveGoals()
	gm.rebuildFiltered()
	gm.statusMsg = "Review logged"
}

func (gm *GoalsMode) deleteGoal() {
	if gm.cursor >= len(gm.filtered) {
		return
	}
	goal := gm.filtered[gm.cursor]
	idx := gm.findGoalIndex(goal.ID)
	if idx < 0 {
		return
	}
	gm.goals = append(gm.goals[:idx], gm.goals[idx+1:]...)
	gm.saveGoals()
	gm.rebuildFiltered()
	if gm.cursor >= len(gm.filtered) && gm.cursor > 0 {
		gm.cursor--
	}
	gm.statusMsg = "Goal deleted"
}

func (gm *GoalsMode) uniqueCategories() []string {
	cats := make(map[string]bool)
	for _, g := range gm.goals {
		if g.Category != "" {
			cats[g.Category] = true
		}
	}
	result := make([]string, 0, len(cats))
	for c := range cats {
		result = append(result, c)
	}
	sort.Strings(result)
	return result
}

// ---------------------------------------------------------------------------
// Goal operations
// ---------------------------------------------------------------------------

func (gm *GoalsMode) findGoalIndex(id string) int {
	for i, g := range gm.goals {
		if g.ID == id {
			return i
		}
	}
	return -1
}

func (gm *GoalsMode) addGoal(title, targetDate, category string) {
	now := time.Now().Format("2006-01-02")
	g := Goal{
		ID:         gm.nextID(),
		Title:      title,
		Status:     GoalStatusActive,
		Category:   category,
		TargetDate: targetDate,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	gm.goals = append(gm.goals, g)
	gm.saveGoals()
	gm.rebuildFiltered()
	gm.statusMsg = "Goal created: " + title
}

func (gm *GoalsMode) toggleComplete() {
	if gm.cursor >= len(gm.filtered) {
		return
	}
	goal := gm.filtered[gm.cursor]
	idx := gm.findGoalIndex(goal.ID)
	if idx < 0 {
		return
	}
	now := time.Now().Format("2006-01-02")
	if gm.goals[idx].Status == GoalStatusCompleted {
		gm.goals[idx].Status = GoalStatusActive
		gm.goals[idx].CompletedAt = ""
		gm.statusMsg = "Goal reactivated"
	} else {
		gm.goals[idx].Status = GoalStatusCompleted
		gm.goals[idx].CompletedAt = now
		gm.statusMsg = "Goal completed!"
	}
	gm.goals[idx].UpdatedAt = now
	gm.saveGoals()
	gm.rebuildFiltered()
	if gm.cursor >= len(gm.filtered) && gm.cursor > 0 {
		gm.cursor--
	}
}

func (gm *GoalsMode) archiveGoal() {
	if gm.cursor >= len(gm.filtered) {
		return
	}
	goal := gm.filtered[gm.cursor]
	idx := gm.findGoalIndex(goal.ID)
	if idx < 0 {
		return
	}
	gm.goals[idx].Status = GoalStatusArchived
	gm.goals[idx].UpdatedAt = time.Now().Format("2006-01-02")
	gm.saveGoals()
	gm.rebuildFiltered()
	if gm.cursor >= len(gm.filtered) && gm.cursor > 0 {
		gm.cursor--
	}
	gm.statusMsg = "Goal archived"
}

func (gm *GoalsMode) togglePause() {
	if gm.cursor >= len(gm.filtered) {
		return
	}
	goal := gm.filtered[gm.cursor]
	idx := gm.findGoalIndex(goal.ID)
	if idx < 0 {
		return
	}
	now := time.Now().Format("2006-01-02")
	if gm.goals[idx].Status == GoalStatusPaused {
		gm.goals[idx].Status = GoalStatusActive
		gm.statusMsg = "Goal resumed"
	} else {
		gm.goals[idx].Status = GoalStatusPaused
		gm.statusMsg = "Goal paused"
	}
	gm.goals[idx].UpdatedAt = now
	gm.saveGoals()
	gm.rebuildFiltered()
}

func (gm *GoalsMode) addMilestone(text string) {
	if gm.expanded < 0 || gm.expanded >= len(gm.filtered) {
		return
	}
	goal := gm.filtered[gm.expanded]
	idx := gm.findGoalIndex(goal.ID)
	if idx < 0 {
		return
	}
	gm.goals[idx].Milestones = append(gm.goals[idx].Milestones, GoalMilestone{
		Text: text,
	})
	gm.goals[idx].UpdatedAt = time.Now().Format("2006-01-02")
	gm.saveGoals()
	gm.rebuildFiltered()
	gm.statusMsg = "Milestone added"
}

func (gm *GoalsMode) toggleMilestone() {
	if gm.expanded < 0 || gm.expanded >= len(gm.filtered) {
		return
	}
	goal := gm.filtered[gm.expanded]
	idx := gm.findGoalIndex(goal.ID)
	if idx < 0 {
		return
	}
	if gm.milestoneCur < 0 || gm.milestoneCur >= len(gm.goals[idx].Milestones) {
		return
	}
	ms := &gm.goals[idx].Milestones[gm.milestoneCur]
	ms.Done = !ms.Done
	if ms.Done {
		ms.CompletedAt = time.Now().Format("2006-01-02")
	} else {
		ms.CompletedAt = ""
	}
	gm.goals[idx].UpdatedAt = time.Now().Format("2006-01-02")

	// Auto-complete goal if all milestones done
	allDone := true
	for _, m := range gm.goals[idx].Milestones {
		if !m.Done {
			allDone = false
			break
		}
	}
	if allDone && len(gm.goals[idx].Milestones) > 0 {
		gm.goals[idx].Status = GoalStatusCompleted
		gm.goals[idx].CompletedAt = time.Now().Format("2006-01-02")
		gm.statusMsg = "All milestones done — goal completed!"
		// Goal leaves active view, clear expanded state
		gm.expanded = -1
		gm.expandedID = ""
		gm.milestoneCur = 0
	}

	gm.saveGoals()
	gm.rebuildFiltered()
}

func (gm *GoalsMode) deleteMilestone() {
	if gm.expanded < 0 || gm.expanded >= len(gm.filtered) {
		return
	}
	goal := gm.filtered[gm.expanded]
	idx := gm.findGoalIndex(goal.ID)
	if idx < 0 {
		return
	}
	if gm.milestoneCur < 0 || gm.milestoneCur >= len(gm.goals[idx].Milestones) {
		return
	}
	gm.goals[idx].Milestones = append(
		gm.goals[idx].Milestones[:gm.milestoneCur],
		gm.goals[idx].Milestones[gm.milestoneCur+1:]...,
	)
	if gm.milestoneCur >= len(gm.goals[idx].Milestones) && gm.milestoneCur > 0 {
		gm.milestoneCur--
	}
	gm.goals[idx].UpdatedAt = time.Now().Format("2006-01-02")
	gm.saveGoals()
	gm.rebuildFiltered()
	gm.statusMsg = "Milestone deleted"
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (gm GoalsMode) Update(msg tea.Msg) (GoalsMode, tea.Cmd) {
	if !gm.active {
		return gm, nil
	}
	switch msg := msg.(type) {
	case gmAIResultMsg:
		gm.aiPending = false
		if msg.err != nil {
			gm.statusMsg = "AI error: " + msg.err.Error()
		} else if len(msg.milestones) > 0 {
			for i := range gm.goals {
				if gm.goals[i].ID == msg.goalID {
					gm.goals[i].Milestones = append(gm.goals[i].Milestones, msg.milestones...)
					gm.saveGoals()
					break
				}
			}
			gm.statusMsg = fmt.Sprintf("Added %d milestones", len(msg.milestones))
		} else {
			gm.statusMsg = "AI returned no milestones"
		}
		return gm, nil
	case gmAIReviewMsg:
		gm.aiPending = false
		if msg.err != nil {
			gm.statusMsg = "AI error: " + msg.err.Error()
		} else {
			gm.reviewText = msg.review
			gm.reviewGoalID = msg.goalID
			gm.statusMsg = "AI review ready"
		}
		return gm, nil
	case gmAIDecomposeMsg:
		gm.aiPending = false
		if msg.err != nil {
			gm.statusMsg = "AI error: " + msg.err.Error()
		} else {
			for i := range gm.goals {
				if gm.goals[i].ID == msg.goalID {
					if len(msg.milestones) > 0 {
						gm.goals[i].Milestones = append(gm.goals[i].Milestones, msg.milestones...)
					}
					if msg.category != "" && gm.goals[i].Category == "" {
						gm.goals[i].Category = msg.category
					}
					if msg.targetDate != "" && gm.goals[i].TargetDate == "" {
						gm.goals[i].TargetDate = msg.targetDate
					}
					if len(msg.tags) > 0 && len(gm.goals[i].Tags) == 0 {
						gm.goals[i].Tags = msg.tags
					}
					gm.goals[i].UpdatedAt = time.Now().Format("2006-01-02")
					gm.saveGoals()
					break
				}
			}
			parts := []string{}
			if len(msg.milestones) > 0 {
				parts = append(parts, fmt.Sprintf("%d milestones", len(msg.milestones)))
			}
			if msg.category != "" {
				parts = append(parts, "category")
			}
			if msg.targetDate != "" {
				parts = append(parts, "target date")
			}
			if len(msg.tags) > 0 {
				parts = append(parts, "tags")
			}
			if len(parts) > 0 {
				gm.statusMsg = "AI decomposed: " + strings.Join(parts, ", ")
			} else {
				gm.statusMsg = "AI returned no decomposition"
			}
			gm.rebuildFiltered()
		}
		return gm, nil
	case gmAICoachMsg:
		gm.aiPending = false
		if msg.err != nil {
			gm.statusMsg = "AI error: " + msg.err.Error()
		} else {
			gm.coachText = msg.analysis
			gm.showCoach = true
			gm.statusMsg = "Goal coach ready"
		}
		return gm, nil
	case tea.KeyMsg:
		key := msg.String()
		if gm.input != goalInputNone {
			return gm.updateInput(key)
		}
		return gm.updateNormal(key)
	}
	return gm, nil
}

func (gm GoalsMode) updateNormal(key string) (GoalsMode, tea.Cmd) {
	gm.statusMsg = "" // clear on any keypress

	// Handle pending confirmation
	if gm.confirmMsg != "" {
		switch key {
		case "y", "Y":
			if gm.confirmAction != nil {
				gm.confirmAction()
			}
		}
		gm.confirmMsg = ""
		gm.confirmAction = nil
		return gm, nil
	}

	switch key {
	case "esc", "q":
		if gm.showCoach {
			gm.showCoach = false
			gm.coachText = ""
		} else if gm.expanded >= 0 {
			gm.expanded = -1
			gm.expandedID = ""
			gm.milestoneCur = 0
		} else {
			gm.Close()
		}

	// Navigation
	case "j", "down":
		if gm.expanded >= 0 && gm.expanded < len(gm.filtered) {
			idx := gm.findGoalIndex(gm.filtered[gm.expanded].ID)
			if idx >= 0 && gm.milestoneCur < len(gm.goals[idx].Milestones)-1 {
				gm.milestoneCur++
			}
		} else if gm.cursor < len(gm.filtered)-1 {
			gm.cursor++
			gm.ensureVisible()
		}
	case "k", "up":
		if gm.expanded >= 0 {
			if gm.milestoneCur > 0 {
				gm.milestoneCur--
			}
		} else if gm.cursor > 0 {
			gm.cursor--
			gm.ensureVisible()
		}

	// Views
	case "tab":
		gm.view = (gm.view + 1) % 4
		gm.cursor = 0
		gm.expanded = -1
		gm.expandedID = ""
		gm.rebuildFiltered()
	case "1":
		gm.view = goalViewAll
		gm.cursor = 0
		gm.expanded = -1
		gm.expandedID = ""
		gm.rebuildFiltered()
	case "2":
		gm.view = goalViewByCategory
		gm.cursor = 0
		gm.expanded = -1
		gm.expandedID = ""
		gm.rebuildFiltered()
	case "3":
		gm.view = goalViewTimeline
		gm.cursor = 0
		gm.expanded = -1
		gm.expandedID = ""
		gm.rebuildFiltered()
	case "4":
		gm.view = goalViewCompleted
		gm.cursor = 0
		gm.expanded = -1
		gm.expandedID = ""
		gm.rebuildFiltered()

	// Expand / collapse
	case "enter":
		if gm.expanded >= 0 {
			gm.toggleMilestone()
		} else if gm.cursor < len(gm.filtered) {
			gm.expanded = gm.cursor
			gm.expandedID = gm.filtered[gm.cursor].ID
			gm.milestoneCur = 0
		}

	// Complete goal
	case "x":
		if gm.expanded < 0 {
			gm.toggleComplete()
		} else {
			gm.toggleMilestone()
		}

	// New goal
	case "a":
		gm.input = goalInputTitle
		gm.inputBuf = ""
		gm.newGoalTitle = ""
		gm.newGoalDate = ""
		gm.newGoalCategory = ""

	// Add milestone
	case "m":
		if gm.expanded >= 0 || gm.cursor < len(gm.filtered) {
			if gm.expanded < 0 {
				gm.expanded = gm.cursor
				gm.expandedID = gm.filtered[gm.cursor].ID
			}
			gm.input = goalInputMilestone
			gm.inputBuf = ""
		}

	// Archive (with confirmation)
	case "A":
		if gm.cursor < len(gm.filtered) {
			goal := gm.filtered[gm.cursor]
			gm.confirmMsg = fmt.Sprintf("Archive goal \"%s\"? (y/n)", goal.Title)
			gm.confirmAction = func() { gm.archiveGoal() }
		}

	// Delete goal (with confirmation) / AI decompose (when expanded, no milestones)
	case "D":
		if gm.expanded >= 0 && gm.expanded < len(gm.filtered) && !gm.aiPending && gm.ai.Provider != "local" && gm.ai.Provider != "" {
			goal := gm.filtered[gm.expanded]
			idx := gm.findGoalIndex(goal.ID)
			if idx >= 0 && len(gm.goals[idx].Milestones) == 0 {
				gm.aiPending = true
				gm.statusMsg = "AI decomposing goal..."
				return gm, gm.aiDecomposeGoal(gm.goals[idx])
			}
		} else if gm.expanded < 0 && gm.cursor < len(gm.filtered) {
			goal := gm.filtered[gm.cursor]
			gm.confirmMsg = fmt.Sprintf("Permanently delete \"%s\"? (y/n)", goal.Title)
			gm.confirmAction = func() { gm.deleteGoal() }
		}

	// Pause / resume
	case "p":
		gm.togglePause()

	// Delete milestone (when expanded)
	case "d":
		if gm.expanded >= 0 {
			gm.deleteMilestone()
		}

	// Set milestone due date (when expanded)
	case "!":
		if gm.expanded >= 0 {
			gm.input = goalInputMilestoneDue
			gm.inputBuf = ""
		}

	// Edit title
	case "e":
		if gm.expanded < 0 && gm.cursor < len(gm.filtered) {
			goal := gm.filtered[gm.cursor]
			gm.input = goalInputTitle
			gm.inputBuf = goal.Title
			// Reuse title input but mark it as edit mode
			gm.newGoalTitle = "__EDIT__"
		}

	// Edit description
	case "E":
		if gm.cursor < len(gm.filtered) {
			goal := gm.filtered[gm.cursor]
			gm.input = goalInputDescription
			gm.inputBuf = goal.Description
		}

	// Edit notes
	case "n":
		if gm.cursor < len(gm.filtered) {
			goal := gm.filtered[gm.cursor]
			gm.input = goalInputNotes
			gm.inputBuf = goal.Notes
		}

	// AI goal coach (holistic analysis)
	case "I":
		if !gm.aiPending && gm.ai.Provider != "local" && gm.ai.Provider != "" && len(gm.goals) > 0 {
			gm.aiPending = true
			gm.showCoach = false
			gm.statusMsg = "AI analyzing all goals..."
			return gm, gm.aiGoalCoach()
		}

	// AI generate milestones
	case "G":
		if gm.cursor < len(gm.filtered) && !gm.aiPending && gm.ai.Provider != "local" && gm.ai.Provider != "" {
			goal := gm.filtered[gm.cursor]
			gm.aiPending = true
			gm.statusMsg = "AI generating milestones..."
			return gm, gm.aiGenerateMilestones(goal)
		}

	// Set goal color
	case "C":
		if gm.cursor < len(gm.filtered) {
			gm.inputGoalID = gm.filtered[gm.cursor].ID
			gm.input = goalInputColor
		}

	// Reorder milestones
	case "J":
		if gm.expanded >= 0 && gm.expanded < len(gm.filtered) {
			goal := gm.filtered[gm.expanded]
			idx := gm.findGoalIndex(goal.ID)
			if idx >= 0 && gm.milestoneCur < len(gm.goals[idx].Milestones)-1 {
				ms := gm.goals[idx].Milestones
				ms[gm.milestoneCur], ms[gm.milestoneCur+1] = ms[gm.milestoneCur+1], ms[gm.milestoneCur]
				gm.milestoneCur++
				gm.goals[idx].UpdatedAt = time.Now().Format("2006-01-02")
				gm.saveGoals()
				gm.rebuildFiltered()
			}
		}
	case "K":
		if gm.expanded >= 0 && gm.expanded < len(gm.filtered) {
			goal := gm.filtered[gm.expanded]
			idx := gm.findGoalIndex(goal.ID)
			if idx >= 0 && gm.milestoneCur > 0 {
				ms := gm.goals[idx].Milestones
				ms[gm.milestoneCur], ms[gm.milestoneCur-1] = ms[gm.milestoneCur-1], ms[gm.milestoneCur]
				gm.milestoneCur--
				gm.goals[idx].UpdatedAt = time.Now().Format("2006-01-02")
				gm.saveGoals()
				gm.rebuildFiltered()
			}
		}

	// Create task from milestone
	case "t":
		if gm.expanded >= 0 && gm.expanded < len(gm.filtered) {
			goal := gm.filtered[gm.expanded]
			idx := gm.findGoalIndex(goal.ID)
			if idx >= 0 && gm.milestoneCur >= 0 && gm.milestoneCur < len(gm.goals[idx].Milestones) {
				ms := gm.goals[idx].Milestones[gm.milestoneCur]
				gm.createTaskFromMilestone(gm.goals[idx], ms)
			}
		}

	// Set review frequency / write review
	case "r":
		if gm.cursor < len(gm.filtered) {
			gm.inputGoalID = gm.filtered[gm.cursor].ID
			if gm.expanded >= 0 {
				gm.inputGoalID = gm.filtered[gm.expanded].ID
				gm.input = goalInputReview
				gm.inputBuf = ""
			} else {
				gm.input = goalInputReviewFreq
			}
		}

	// AI review goal (when expanded)
	case "R":
		if gm.expanded >= 0 && gm.expanded < len(gm.filtered) && !gm.aiPending && gm.ai.Provider != "local" && gm.ai.Provider != "" {
			goal := gm.filtered[gm.expanded]
			idx := gm.findGoalIndex(goal.ID)
			if idx >= 0 {
				gm.aiPending = true
				gm.statusMsg = "AI reviewing goal..."
				return gm, gm.aiReviewGoal(gm.goals[idx])
			}
		}

	// Help
	case "?":
		gm.input = goalInputHelp
	}

	return gm, nil
}

func (gm GoalsMode) updateInput(key string) (GoalsMode, tea.Cmd) {
	switch gm.input {
	case goalInputHelp:
		gm.input = goalInputNone
		return gm, nil

	case goalInputTitle:
		switch key {
		case "esc":
			gm.input = goalInputNone
			gm.newGoalTitle = ""
		case "enter":
			title := strings.TrimSpace(gm.inputBuf)
			if title == "" {
				break
			}
			if gm.newGoalTitle == "__EDIT__" {
				// Edit existing goal title
				if gm.cursor < len(gm.filtered) {
					goal := gm.filtered[gm.cursor]
					idx := gm.findGoalIndex(goal.ID)
					if idx >= 0 {
						gm.goals[idx].Title = title
						gm.goals[idx].UpdatedAt = time.Now().Format("2006-01-02")
						gm.saveGoals()
						gm.rebuildFiltered()
						gm.statusMsg = "Title updated"
					}
				}
				gm.input = goalInputNone
				gm.inputBuf = ""
				gm.newGoalTitle = ""
			} else {
				// New goal flow: title → date → category
				gm.newGoalTitle = title
				gm.inputBuf = ""
				gm.input = goalInputDate
			}
		case "backspace":
			if len(gm.inputBuf) > 0 {
				gm.inputBuf = gm.inputBuf[:len(gm.inputBuf)-1]
			}
		default:
			if len(key) == 1 || key == " " {
				gm.inputBuf += key
			}
		}

	case goalInputDate:
		now := time.Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		switch key {
		case "esc":
			gm.input = goalInputNone
		case "enter", "0": // skip / no date
			gm.newGoalDate = ""
			gm.inputBuf = ""
			gm.input = goalInputCategory
		case "1": // 1 month
			gm.newGoalDate = today.AddDate(0, 1, 0).Format("2006-01-02")
			gm.inputBuf = ""
			gm.input = goalInputCategory
		case "2": // 3 months
			gm.newGoalDate = today.AddDate(0, 3, 0).Format("2006-01-02")
			gm.inputBuf = ""
			gm.input = goalInputCategory
		case "3": // 6 months
			gm.newGoalDate = today.AddDate(0, 6, 0).Format("2006-01-02")
			gm.inputBuf = ""
			gm.input = goalInputCategory
		case "4": // 1 year
			gm.newGoalDate = today.AddDate(1, 0, 0).Format("2006-01-02")
			gm.inputBuf = ""
			gm.input = goalInputCategory
		case "5": // 2 years
			gm.newGoalDate = today.AddDate(2, 0, 0).Format("2006-01-02")
			gm.inputBuf = ""
			gm.input = goalInputCategory
		case "6": // 3 years
			gm.newGoalDate = today.AddDate(3, 0, 0).Format("2006-01-02")
			gm.inputBuf = ""
			gm.input = goalInputCategory
		case "7": // 5 years
			gm.newGoalDate = today.AddDate(5, 0, 0).Format("2006-01-02")
			gm.inputBuf = ""
			gm.input = goalInputCategory
		}

	case goalInputCategory:
		cats := gm.uniqueCategories()
		switch key {
		case "esc":
			gm.input = goalInputNone
		case "enter":
			gm.newGoalCategory = strings.TrimSpace(gm.inputBuf)
			gm.addGoal(gm.newGoalTitle, gm.newGoalDate, gm.newGoalCategory)
			gm.input = goalInputNone
			gm.inputBuf = ""
		case "backspace":
			if len(gm.inputBuf) > 0 {
				gm.inputBuf = gm.inputBuf[:len(gm.inputBuf)-1]
			}
		default:
			// Number keys select existing categories
			if len(key) == 1 && key[0] >= '1' && key[0] <= '9' && gm.inputBuf == "" {
				idx := int(key[0] - '1')
				if idx < len(cats) {
					gm.newGoalCategory = cats[idx]
					gm.addGoal(gm.newGoalTitle, gm.newGoalDate, gm.newGoalCategory)
					gm.input = goalInputNone
					gm.inputBuf = ""
					break
				}
			}
			if len(key) == 1 || key == " " {
				gm.inputBuf += key
			}
		}

	case goalInputMilestone:
		switch key {
		case "esc":
			gm.input = goalInputNone
		case "enter":
			text := strings.TrimSpace(gm.inputBuf)
			if text != "" {
				gm.addMilestone(text)
			}
			gm.input = goalInputNone
			gm.inputBuf = ""
		case "backspace":
			if len(gm.inputBuf) > 0 {
				gm.inputBuf = gm.inputBuf[:len(gm.inputBuf)-1]
			}
		default:
			if len(key) == 1 || key == " " {
				gm.inputBuf += key
			}
		}

	case goalInputColor:
		colors := []string{"blue", "red", "green", "yellow", "mauve", "pink", "teal"}
		switch key {
		case "esc":
			gm.input = goalInputNone
		case "1", "2", "3", "4", "5", "6", "7":
			idx := int(key[0] - '1')
			if idx < len(colors) && gm.inputGoalID != "" {
				gi := gm.findGoalIndex(gm.inputGoalID)
				if gi >= 0 {
					gm.goals[gi].Color = colors[idx]
					gm.goals[gi].UpdatedAt = time.Now().Format("2006-01-02")
					gm.saveGoals()
					gm.rebuildFiltered()
					gm.statusMsg = "Color: " + colors[idx]
				}
			}
			gm.input = goalInputNone
		case "0":
			if gm.inputGoalID != "" {
				gi := gm.findGoalIndex(gm.inputGoalID)
				if gi >= 0 {
					gm.goals[gi].Color = ""
					gm.goals[gi].UpdatedAt = time.Now().Format("2006-01-02")
					gm.saveGoals()
					gm.rebuildFiltered()
					gm.statusMsg = "Color reset"
				}
			}
			gm.input = goalInputNone
		}

	case goalInputMilestoneDue:
		now := time.Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		var dueDate string
		switch key {
		case "esc":
			gm.input = goalInputNone
			return gm, nil
		case "1":
			dueDate = today.AddDate(0, 0, 7).Format("2006-01-02")
		case "2":
			dueDate = today.AddDate(0, 0, 14).Format("2006-01-02")
		case "3":
			dueDate = today.AddDate(0, 1, 0).Format("2006-01-02")
		case "4":
			dueDate = today.AddDate(0, 3, 0).Format("2006-01-02")
		case "0":
			dueDate = ""
		default:
			return gm, nil
		}
		if gm.expanded >= 0 && gm.expanded < len(gm.filtered) {
			goal := gm.filtered[gm.expanded]
			idx := gm.findGoalIndex(goal.ID)
			if idx >= 0 && gm.milestoneCur >= 0 && gm.milestoneCur < len(gm.goals[idx].Milestones) {
				gm.goals[idx].Milestones[gm.milestoneCur].DueDate = dueDate
				gm.goals[idx].UpdatedAt = time.Now().Format("2006-01-02")
				gm.saveGoals()
				gm.rebuildFiltered()
				if dueDate == "" {
					gm.statusMsg = "Milestone date cleared"
				} else {
					gm.statusMsg = "Milestone due: " + dueDate
				}
			}
		}
		gm.input = goalInputNone

	case goalInputReviewFreq:
		switch key {
		case "esc":
			gm.input = goalInputNone
		case "1": // weekly
			gm.setReviewFrequency("weekly")
			gm.input = goalInputNone
		case "2": // monthly
			gm.setReviewFrequency("monthly")
			gm.input = goalInputNone
		case "3": // quarterly
			gm.setReviewFrequency("quarterly")
			gm.input = goalInputNone
		case "0": // remove
			gm.setReviewFrequency("")
			gm.input = goalInputNone
		}

	case goalInputReview:
		switch key {
		case "esc":
			gm.input = goalInputNone
			gm.inputBuf = ""
		case "enter":
			gm.submitReview(strings.TrimSpace(gm.inputBuf))
			gm.input = goalInputNone
			gm.inputBuf = ""
		case "backspace":
			if len(gm.inputBuf) > 0 {
				gm.inputBuf = gm.inputBuf[:len(gm.inputBuf)-1]
			}
		default:
			if len(key) == 1 || key == " " {
				gm.inputBuf += key
			}
		}

	case goalInputDescription:
		switch key {
		case "esc":
			gm.input = goalInputNone
		case "enter":
			if gm.cursor < len(gm.filtered) {
				goal := gm.filtered[gm.cursor]
				idx := gm.findGoalIndex(goal.ID)
				if idx >= 0 {
					gm.goals[idx].Description = strings.TrimSpace(gm.inputBuf)
					gm.goals[idx].UpdatedAt = time.Now().Format("2006-01-02")
					gm.saveGoals()
					gm.rebuildFiltered()
					gm.statusMsg = "Description saved"
				}
			}
			gm.input = goalInputNone
			gm.inputBuf = ""
		case "backspace":
			if len(gm.inputBuf) > 0 {
				gm.inputBuf = gm.inputBuf[:len(gm.inputBuf)-1]
			}
		default:
			if len(key) == 1 || key == " " {
				gm.inputBuf += key
			}
		}

	case goalInputNotes:
		switch key {
		case "esc":
			gm.input = goalInputNone
		case "enter":
			if gm.cursor < len(gm.filtered) {
				goal := gm.filtered[gm.cursor]
				idx := gm.findGoalIndex(goal.ID)
				if idx >= 0 {
					gm.goals[idx].Notes = strings.TrimSpace(gm.inputBuf)
					gm.goals[idx].UpdatedAt = time.Now().Format("2006-01-02")
					gm.saveGoals()
					gm.rebuildFiltered()
					gm.statusMsg = "Notes saved"
				}
			}
			gm.input = goalInputNone
			gm.inputBuf = ""
		case "backspace":
			if len(gm.inputBuf) > 0 {
				gm.inputBuf = gm.inputBuf[:len(gm.inputBuf)-1]
			}
		default:
			if len(key) == 1 || key == " " {
				gm.inputBuf += key
			}
		}
	}

	return gm, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (gm *GoalsMode) visibleHeight() int {
	h := gm.height - 14 // title + tabs + stats + input + help + border/padding
	if h < 5 {
		h = 5
	}
	return h
}

func (gm *GoalsMode) View() string {
	if !gm.active {
		return ""
	}

	width := gm.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}
	innerW := width - 8 // account for border + padding

	var b strings.Builder

	// Title bar
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render(IconBookmarkChar+" Goals") + "\n")

	// Tabs
	gm.renderTabs(&b, innerW)
	b.WriteString("\n")

	// Stats summary
	gm.renderStats(&b, innerW)
	b.WriteString("\n")

	// View content
	if gm.showCoach {
		gm.renderCoach(&b, innerW)
	} else if gm.input == goalInputHelp {
		gm.renderHelp(&b, innerW)
	} else if gm.input != goalInputNone {
		gm.renderInput(&b, innerW)
	} else {
		gm.renderGoals(&b, innerW)
	}

	// Confirmation prompt
	if gm.confirmMsg != "" {
		b.WriteString("\n")
		b.WriteString("  " + lipgloss.NewStyle().Foreground(red).Bold(true).Render(gm.confirmMsg))
	}

	// Status message
	if gm.statusMsg != "" {
		b.WriteString("\n")
		b.WriteString("  " + lipgloss.NewStyle().Foreground(green).Render(gm.statusMsg))
	}

	// Help bar
	b.WriteString("\n")
	gm.renderHelpBar(&b, innerW)

	// Bordered overlay (matches task manager style)
	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (gm *GoalsMode) renderTabs(b *strings.Builder, w int) {
	tabs := []string{"Active", "By Category", "Timeline", "Completed"}
	var parts []string
	for i, name := range tabs {
		if goalViewMode(i) == gm.view {
			parts = append(parts, lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("["+name+"]"))
		} else {
			parts = append(parts, DimStyle.Render(" "+name+" "))
		}
	}
	b.WriteString("  " + strings.Join(parts, " "))
}

func (gm *GoalsMode) renderStats(b *strings.Builder, w int) {
	active, paused, completed, overdue := 0, 0, 0, 0
	totalProgress := 0
	for _, g := range gm.goals {
		switch g.Status {
		case GoalStatusActive:
			active++
			totalProgress += g.Progress()
		case GoalStatusPaused:
			paused++
		case GoalStatusCompleted:
			completed++
		}
		if g.IsOverdue() {
			overdue++
		}
	}
	avgProgress := 0
	if active > 0 {
		avgProgress = totalProgress / active
	}

	ds := DimStyle
	ns := lipgloss.NewStyle().Foreground(lavender).Bold(true)

	var parts []string
	parts = append(parts, ns.Render(fmt.Sprintf("%d", active))+ds.Render(" active"))
	if paused > 0 {
		parts = append(parts, ns.Render(fmt.Sprintf("%d", paused))+ds.Render(" paused"))
	}
	parts = append(parts, ns.Render(fmt.Sprintf("%d", completed))+ds.Render(" done"))
	if active > 0 {
		parts = append(parts, ns.Render(fmt.Sprintf("%d%%", avgProgress))+ds.Render(" avg"))
	}
	if overdue > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(red).Bold(true).Render(fmt.Sprintf("%d overdue", overdue)))
	}
	reviewsDue := 0
	for _, g := range gm.goals {
		if g.IsDueForReview() {
			reviewsDue++
		}
	}
	if reviewsDue > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(yellow).Bold(true).Render(fmt.Sprintf("%d review due", reviewsDue)))
	}
	b.WriteString("  " + strings.Join(parts, "  "))
}

func (gm *GoalsMode) renderGoals(b *strings.Builder, w int) {
	if len(gm.filtered) == 0 {
		msg := "No goals yet. Press 'a' to create one."
		switch gm.view {
		case goalViewCompleted:
			msg = "No completed goals yet."
		case goalViewByCategory:
			msg = "No active goals. Press 'a' to create one."
		case goalViewTimeline:
			msg = "No goals with deadlines. Press 'a' to create one."
		}
		b.WriteString("\n  " + DimStyle.Render(msg))
		return
	}

	maxH := gm.visibleHeight()

	lastCategory := ""
	lineCount := 0
	startIdx := gm.scroll
	if startIdx >= len(gm.filtered) {
		startIdx = 0
		gm.scroll = 0
	}

	if startIdx > 0 {
		b.WriteString("  " + DimStyle.Render(fmt.Sprintf("... %d above", startIdx)) + "\n")
		lineCount++
	}

	for i := startIdx; i < len(gm.filtered); i++ {
		goal := gm.filtered[i]
		if lineCount >= maxH {
			remaining := len(gm.filtered) - i
			if remaining > 0 {
				b.WriteString("\n  " + DimStyle.Render(fmt.Sprintf("+%d more (j to scroll)", remaining)))
			}
			break
		}

		// Category header in category view
		if gm.view == goalViewByCategory && goal.Category != lastCategory {
			catLabel := goal.Category
			if catLabel == "" {
				catLabel = "Uncategorized"
			}
			b.WriteString("\n  " + lipgloss.NewStyle().Foreground(yellow).Bold(true).Render(catLabel) + "\n")
			lastCategory = goal.Category
			lineCount += 2
		}

		// Cursor
		isCursor := i == gm.cursor
		prefix := "  "
		if isCursor && gm.expanded < 0 {
			prefix = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("> ")
		}
		if gm.expanded == i {
			prefix = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("\u25BC ")
		}

		// Status icon
		var statusIcon string
		switch goal.Status {
		case GoalStatusCompleted:
			statusIcon = lipgloss.NewStyle().Foreground(green).Bold(true).Render("\u2713 ")
		case GoalStatusPaused:
			statusIcon = lipgloss.NewStyle().Foreground(yellow).Render("\u23F8 ")
		case GoalStatusActive:
			if goal.IsOverdue() {
				statusIcon = lipgloss.NewStyle().Foreground(red).Bold(true).Render("! ")
			} else {
				goalCol := blue
				if goal.Color != "" {
					goalCol = goalColorMap(goal.Color)
				}
				statusIcon = lipgloss.NewStyle().Foreground(goalCol).Render("\u25CB ")
			}
		default:
			statusIcon = DimStyle.Render("\u25CB ")
		}

		// Progress
		prog := goal.Progress()
		barWidth := 10
		filled := prog * barWidth / 100
		if filled > barWidth {
			filled = barWidth
		}
		// Bar color: green when on track, yellow <50%, red if overdue
		barColor := green
		if goal.Color != "" {
			barColor = goalColorMap(goal.Color)
		}
		if goal.IsOverdue() {
			barColor = red
		} else if prog < 50 && goal.TargetDate != "" && goal.DaysRemaining() < 30 {
			barColor = yellow
		}
		bar := lipgloss.NewStyle().Foreground(barColor).Render(strings.Repeat("\u2588", filled)) +
			DimStyle.Render(strings.Repeat("\u2591", barWidth-filled))

		// Milestone count inline
		msCount := ""
		if len(goal.Milestones) > 0 {
			msCount = DimStyle.Render(fmt.Sprintf(" %d/%d", goal.DoneCount(), len(goal.Milestones)))
		}

		// Title styling
		titleStyle := lipgloss.NewStyle().Foreground(text)
		if isCursor && gm.expanded < 0 {
			titleStyle = titleStyle.Bold(true)
		}
		if goal.Status == GoalStatusCompleted {
			titleStyle = titleStyle.Strikethrough(true).Foreground(overlay0)
		}
		if goal.Status == GoalStatusPaused {
			titleStyle = titleStyle.Foreground(overlay1)
		}

		title := TruncateDisplay(goal.Title, w-45)

		// Timeframe badge (human-readable)
		timeBadge := ""
		if goal.TargetDate != "" {
			label := goal.TimeframeLabel()
			timeColor := overlay0
			days := goal.DaysRemaining()
			if days < 0 {
				timeColor = red
			} else if days <= 14 {
				timeColor = yellow
			} else if days <= 90 {
				timeColor = lavender
			}
			timeBadge = " " + lipgloss.NewStyle().Foreground(timeColor).Render(label)
		}

		// Category badge
		catBadge := ""
		if goal.Category != "" && gm.view != goalViewByCategory {
			catBadge = " " + lipgloss.NewStyle().Foreground(sapphire).Render("["+goal.Category+"]")
		}

		// Review indicator
		reviewBadge := ""
		if goal.IsDueForReview() {
			reviewBadge = " " + lipgloss.NewStyle().Foreground(red).Bold(true).Render("[review due]")
		} else if goal.ReviewFrequency != "" {
			reviewBadge = " " + DimStyle.Render("["+goal.ReviewFrequency+"]")
		}

		line := prefix + statusIcon + titleStyle.Render(title) + " " + bar + msCount + timeBadge + reviewBadge + catBadge
		b.WriteString(line + "\n")
		lineCount++

		// Stale warning (shown below goal title in list view)
		if gm.expanded != i {
			if warn := staleWarning(goal); warn != "" {
				b.WriteString("      " + lipgloss.NewStyle().Foreground(yellow).Faint(true).Render(warn) + "\n")
				lineCount++
			}
		}

		// Expanded detail
		if gm.expanded == i {
			// Read milestone data from goals (not filtered copy)
			idx := gm.findGoalIndex(goal.ID)
			if idx < 0 {
				continue
			}
			goalData := gm.goals[idx]

			if goalData.Description != "" {
				b.WriteString("      " + lipgloss.NewStyle().Foreground(overlay1).Italic(true).Render(TruncateDisplay(goalData.Description, w-10)) + "\n")
				lineCount++
			}
			if goalData.Notes != "" {
				b.WriteString("      " + DimStyle.Render("\U0001F4DD "+TruncateDisplay(goalData.Notes, w-10)) + "\n")
				lineCount++
			}
			if goalData.CreatedAt != "" {
				meta := "Created: " + goalData.CreatedAt
				if goalData.ReviewFrequency != "" {
					meta += "  Review: " + goalData.ReviewFrequency
					if goalData.LastReviewed != "" {
						meta += " (last: " + goalData.LastReviewed + ")"
					}
				}
				b.WriteString("      " + DimStyle.Render(meta) + "\n")
				lineCount++
			}
			// Progress chart from review log
			if len(goalData.ReviewLog) >= 2 {
				chartWidth := 20
				if len(goalData.ReviewLog) < chartWidth {
					chartWidth = len(goalData.ReviewLog)
				}
				start := len(goalData.ReviewLog) - chartWidth
				bars := []rune("▁▂▃▄▅▆▇█")
				var chartParts []string
				for _, rev := range goalData.ReviewLog[start:] {
					idx := rev.Progress * (len(bars) - 1) / 100
					if idx < 0 {
						idx = 0
					}
					if idx >= len(bars) {
						idx = len(bars) - 1
					}
					chartParts = append(chartParts, lipgloss.NewStyle().Foreground(green).Render(string(bars[idx])))
				}
				b.WriteString("      " + DimStyle.Render("Progress: ") + strings.Join(chartParts, "") +
					DimStyle.Render(fmt.Sprintf(" %d%%", goalData.Progress())) + "\n")
				lineCount++
			}
			// Linked tasks
			if taskDone, taskTotal := gm.linkedTaskStats(goalData.ID); taskTotal > 0 {
				taskColor := green
				if taskDone < taskTotal {
					taskColor = lavender
				}
				b.WriteString("      " + lipgloss.NewStyle().Foreground(taskColor).Render(fmt.Sprintf("Tasks: %d/%d done", taskDone, taskTotal)) + "\n")
				lineCount++
			}
			// Recent reviews (last 3)
			if len(goalData.ReviewLog) > 0 {
				start := len(goalData.ReviewLog) - 3
				if start < 0 {
					start = 0
				}
				for _, rev := range goalData.ReviewLog[start:] {
					revLine := fmt.Sprintf("      %s %d%% ", rev.Date, rev.Progress)
					if rev.Note != "" {
						revLine += DimStyle.Render(TruncateDisplay(rev.Note, w-30))
					}
					b.WriteString(lipgloss.NewStyle().Foreground(lavender).Render(revLine) + "\n")
					lineCount++
				}
			}
			if len(goalData.Milestones) == 0 {
				b.WriteString("      " + DimStyle.Render("No milestones yet. Press 'm' to add one.") + "\n")
				lineCount++
			}
			for mi, ms := range goalData.Milestones {
				mPrefix := "      "
				if mi == gm.milestoneCur {
					mPrefix = "    " + lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("> ")
				}
				check := DimStyle.Render("[ ] ")
				mStyle := lipgloss.NewStyle().Foreground(text)
				if ms.Done {
					check = lipgloss.NewStyle().Foreground(green).Render("[x] ")
					mStyle = mStyle.Strikethrough(true).Foreground(overlay0)
				}
				dueLbl := ""
				if ms.DueDate != "" {
					dColor := overlay0
					if d, err := time.Parse("2006-01-02", ms.DueDate); err == nil {
						if time.Now().After(d) && !ms.Done {
							dColor = red
						} else if time.Now().AddDate(0, 0, 7).After(d) && !ms.Done {
							dColor = yellow
						}
					}
					dueLbl = " " + lipgloss.NewStyle().Foreground(dColor).Render(ms.DueDate)
				}
				b.WriteString(mPrefix + check + mStyle.Render(ms.Text) + dueLbl + "\n")
				lineCount++
			}
			// AI review text (shown below milestones)
			if gm.reviewText != "" && gm.reviewGoalID == goalData.ID {
				reviewStyle := lipgloss.NewStyle().Foreground(lavender).Italic(true)
				b.WriteString("\n")
				lineCount++
				b.WriteString("      " + lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("AI Review:") + "\n")
				lineCount++
				for _, rl := range strings.Split(gm.reviewText, "\n") {
					rl = strings.TrimSpace(rl)
					if rl != "" {
						b.WriteString("      " + reviewStyle.Render(TruncateDisplay(rl, w-10)) + "\n")
						lineCount++
					}
				}
			}
		}
	}
}

func (gm *GoalsMode) renderInput(b *strings.Builder, w int) {
	promptStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	inputStyle := lipgloss.NewStyle().Foreground(text)
	hintStyle := DimStyle

	switch gm.input {
	case goalInputTitle:
		b.WriteString("\n  " + promptStyle.Render("New Goal") + "\n")
		b.WriteString("  " + promptStyle.Render("Title: ") + inputStyle.Render(gm.inputBuf+"\u2588") + "\n")
		b.WriteString("  " + hintStyle.Render("Enter to continue, Esc to cancel"))
	case goalInputDate:
		b.WriteString("\n  " + promptStyle.Render("New Goal: ") + gm.newGoalTitle + "\n\n")
		b.WriteString("  " + promptStyle.Render("Target date:") + "\n\n")
		ss := lipgloss.NewStyle().Foreground(lavender).Bold(true)
		ds := lipgloss.NewStyle().Foreground(text)
		b.WriteString("  " + ss.Render("1") + ds.Render(" 1 month     ") + ss.Render("2") + ds.Render(" 3 months    ") + ss.Render("3") + ds.Render(" 6 months") + "\n")
		b.WriteString("  " + ss.Render("4") + ds.Render(" 1 year      ") + ss.Render("5") + ds.Render(" 2 years     ") + ss.Render("6") + ds.Render(" 3 years") + "\n")
		b.WriteString("  " + ss.Render("7") + ds.Render(" 5 years     ") + ss.Render("0") + ds.Render(" no deadline") + "\n\n")
		b.WriteString("  " + hintStyle.Render("Pick a timeframe or Esc to cancel"))
	case goalInputCategory:
		dateLbl := gm.newGoalDate
		if dateLbl == "" {
			dateLbl = "no deadline"
		}
		b.WriteString("\n  " + promptStyle.Render("New Goal: ") + gm.newGoalTitle + "  " + hintStyle.Render(dateLbl) + "\n\n")
		cats := gm.uniqueCategories()
		if len(cats) > 0 {
			b.WriteString("  " + promptStyle.Render("Pick a category:") + "\n\n")
			ss := lipgloss.NewStyle().Foreground(lavender).Bold(true)
			ds := lipgloss.NewStyle().Foreground(text)
			for i, c := range cats {
				if i >= 9 {
					break
				}
				b.WriteString("  " + ss.Render(fmt.Sprintf("%d", i+1)) + ds.Render(" "+c) + "\n")
			}
			b.WriteString("\n")
		}
		b.WriteString("  " + promptStyle.Render("Or type new: ") + inputStyle.Render(gm.inputBuf+"\u2588") + "\n")
		b.WriteString("  " + hintStyle.Render("Enter to confirm, empty to skip"))
	case goalInputMilestone:
		b.WriteString("\n  " + promptStyle.Render("Add Milestone: ") + inputStyle.Render(gm.inputBuf+"\u2588") + "\n")
	case goalInputColor:
		b.WriteString("\n  " + promptStyle.Render("Set goal color:") + "\n\n")
		colors := []struct{ name string; color lipgloss.Color }{
			{"blue", blue}, {"red", red}, {"green", green}, {"yellow", yellow},
			{"mauve", mauve}, {"pink", pink}, {"teal", teal},
		}
		for i, c := range colors {
			swatch := lipgloss.NewStyle().Foreground(c.color).Render("\u2588\u2588")
			num := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render(fmt.Sprintf("%d", i+1))
			b.WriteString("  " + num + " " + swatch + " " + c.name + "\n")
		}
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("0") + " reset\n\n")
		b.WriteString("  " + hintStyle.Render("Pick a color or Esc to cancel"))
	case goalInputMilestoneDue:
		b.WriteString("\n  " + promptStyle.Render("Milestone due date:") + "\n\n")
		ss := lipgloss.NewStyle().Foreground(lavender).Bold(true)
		ds := lipgloss.NewStyle().Foreground(text)
		b.WriteString("  " + ss.Render("1") + ds.Render(" 1 week   ") + ss.Render("2") + ds.Render(" 2 weeks") + "\n")
		b.WriteString("  " + ss.Render("3") + ds.Render(" 1 month  ") + ss.Render("4") + ds.Render(" 3 months") + "\n")
		b.WriteString("  " + ss.Render("0") + ds.Render(" clear date") + "\n\n")
		b.WriteString("  " + hintStyle.Render("Pick a timeframe or Esc to cancel"))
	case goalInputReviewFreq:
		b.WriteString("\n  " + promptStyle.Render("Set review frequency:") + "\n\n")
		ss := lipgloss.NewStyle().Foreground(lavender).Bold(true)
		ds := lipgloss.NewStyle().Foreground(text)
		b.WriteString("  " + ss.Render("1") + ds.Render(" weekly   ") + ss.Render("2") + ds.Render(" monthly   ") + ss.Render("3") + ds.Render(" quarterly") + "\n")
		b.WriteString("  " + ss.Render("0") + ds.Render(" remove reviews") + "\n\n")
		b.WriteString("  " + hintStyle.Render("Pick a frequency or Esc to cancel"))
	case goalInputReview:
		b.WriteString("\n  " + promptStyle.Render("Review reflection: ") + inputStyle.Render(gm.inputBuf+"\u2588") + "\n")
		b.WriteString("  " + hintStyle.Render("Enter to save, Esc to cancel"))
	case goalInputDescription:
		b.WriteString("\n  " + promptStyle.Render("Description: ") + inputStyle.Render(gm.inputBuf+"\u2588") + "\n")
	case goalInputNotes:
		b.WriteString("\n  " + promptStyle.Render("Notes: ") + inputStyle.Render(gm.inputBuf+"\u2588") + "\n")
	}
}

func (gm *GoalsMode) renderHelp(b *strings.Builder, w int) {
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true).Width(8)
	descStyle := lipgloss.NewStyle().Foreground(text)
	sectionStyle := lipgloss.NewStyle().Foreground(yellow).Bold(true)

	b.WriteString("\n")
	b.WriteString("  " + titleStyle.Render("Goal Manager Shortcuts") + "\n\n")

	sections := []struct {
		title string
		keys  [][2]string
	}{
		{"Navigation", [][2]string{
			{"j/k", "Move cursor up/down"},
			{"Tab", "Cycle views (Active/Category/Timeline/Completed)"},
			{"1-4", "Jump to specific view"},
			{"Enter", "Expand goal / toggle milestone"},
			{"Esc", "Collapse / close"},
		}},
		{"Goal Actions", [][2]string{
			{"a", "Create new goal (title → date → category)"},
			{"x", "Toggle goal complete / toggle milestone"},
			{"e", "Edit goal title"},
			{"E", "Edit goal description"},
			{"n", "Edit goal notes"},
			{"p", "Pause / resume goal"},
			{"A", "Archive goal"},
			{"D", "Delete goal permanently"},
		}},
		{"Milestones", [][2]string{
			{"m", "Add milestone to current goal"},
			{"t", "Create task from milestone (links to goal)"},
			{"d", "Delete milestone (when expanded)"},
			{"Enter", "Toggle milestone completion"},
		}},
		{"Reviews", [][2]string{
			{"r", "Set review frequency / write review (when expanded)"},
		}},
		{"AI (when expanded)", [][2]string{
			{"R", "AI review goal (honest assessment)"},
			{"D", "AI decompose goal (when no milestones)"},
			{"G", "AI generate milestones"},
		}},
	}

	for _, sec := range sections {
		b.WriteString("  " + sectionStyle.Render(sec.title) + "\n")
		for _, kv := range sec.keys {
			b.WriteString("    " + keyStyle.Render(kv[0]) + descStyle.Render(kv[1]) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("  " + DimStyle.Render("Press any key to close"))
}

func (gm *GoalsMode) renderCoach(b *strings.Builder, w int) {
	headerStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	bodyStyle := lipgloss.NewStyle().Foreground(lavender).Italic(true)
	headingStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)

	b.WriteString(headerStyle.Render("  "+IconBotChar+" AI Goal Coach") + "\n\n")

	for _, line := range strings.Split(gm.coachText, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			b.WriteString("\n")
			continue
		}
		if strings.HasPrefix(trimmed, "##") {
			heading := strings.TrimLeft(trimmed, "# ")
			b.WriteString("  " + headingStyle.Render(heading) + "\n")
		} else {
			b.WriteString("  " + bodyStyle.Render(TruncateDisplay(trimmed, w-6)) + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString("  " + DimStyle.Render("Esc to dismiss"))
}

func (gm *GoalsMode) renderHelpBar(b *strings.Builder, w int) {
	pairs := [][2]string{
		{"j/k", "nav"}, {"a", "add"}, {"m", "milestone"}, {"x", "complete"},
		{"e", "edit"}, {"E", "desc"}, {"n", "notes"}, {"p", "pause"},
		{"t", "task"}, {"r", "review"}, {"A", "archive"},
	}
	// Context-sensitive AI shortcuts
	if gm.expanded >= 0 && gm.expanded < len(gm.filtered) {
		goal := gm.filtered[gm.expanded]
		idx := gm.findGoalIndex(goal.ID)
		if idx >= 0 {
			pairs = append(pairs, [2]string{"R", "AI review"})
			if len(gm.goals[idx].Milestones) == 0 {
				pairs = append(pairs, [2]string{"D", "AI decompose"})
			}
		}
	} else {
		pairs = append(pairs, [2]string{"D", "delete"})
	}
	pairs = append(pairs, [2]string{"I", "AI coach"}, [2]string{"?", "help"}, [2]string{"Tab", "view"}, [2]string{"Esc", "close"})
	var parts []string
	keyStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	for _, p := range pairs {
		parts = append(parts, keyStyle.Render(p[0])+":"+DimStyle.Render(p[1]))
	}
	b.WriteString("  " + strings.Join(parts, " "))
}
