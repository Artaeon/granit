package tui

import (
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
// AI Messages
// ---------------------------------------------------------------------------

type pmAIInsightMsg struct {
	insight string
	err     error
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	pmPhaseList      = 0
	pmPhaseDashboard = 1
	pmPhaseEdit      = 2
)

// projectCategories is the built-in set of project categories.
var projectCategories = []string{
	"development",
	"social-media",
	"personal",
	"business",
	"writing",
	"research",
	"health",
	"finance",
	"other",
}

// projectStatuses is the ordered set of valid status values.
var projectStatuses = []string{
	"active",
	"paused",
	"completed",
	"archived",
}

// projectColorNames are the colour choices available for a project accent.
var projectColorNames = []string{
	"blue", "green", "mauve", "peach", "red", "yellow",
	"pink", "lavender", "teal", "sapphire", "flamingo",
}

// projectPriorityLabels maps priority values to display labels.
var projectPriorityLabels = []string{
	"none", "low", "medium", "high", "highest",
}

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

// ProjectMilestone represents a sub-step within a project goal.
type ProjectMilestone struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// ProjectGoal represents a high-level goal within a project.
type ProjectGoal struct {
	Title      string             `json:"title"`
	Done       bool               `json:"done"`
	Milestones []ProjectMilestone `json:"milestones"`
}

// Project represents a single tracked project in the vault.
type Project struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Folder      string        `json:"folder"`
	Tags        []string      `json:"tags"`
	Status      string        `json:"status"`
	Color       string        `json:"color"`
	CreatedAt   string        `json:"created_at"`
	Notes       []string      `json:"notes"`
	TaskFilter  string        `json:"task_filter"`
	Category    string        `json:"category"`
	Goals       []ProjectGoal `json:"goals"`
	NextAction  string        `json:"next_action"`
	Priority    int           `json:"priority"`
	DueDate     string        `json:"due_date"`
	TimeSpent   int           `json:"time_spent"`

	// Computed fields (not stored in JSON)
	TasksDone  int `json:"-"` // completed tasks matched to this project
	TasksTotal int `json:"-"` // total tasks matched to this project
}

// Progress returns 0.0..1.0 representing project completion.
// Priority: goals with milestones > goals without milestones > tasks.
func (p Project) Progress() float64 {
	// Check for goals with milestones first.
	totalMilestones := 0
	doneMilestones := 0
	for _, g := range p.Goals {
		totalMilestones += len(g.Milestones)
		for _, m := range g.Milestones {
			if m.Done {
				doneMilestones++
			}
		}
	}
	if totalMilestones > 0 {
		return float64(doneMilestones) / float64(totalMilestones)
	}

	// Goals without milestones.
	if len(p.Goals) > 0 {
		doneGoals := 0
		for _, g := range p.Goals {
			if g.Done {
				doneGoals++
			}
		}
		return float64(doneGoals) / float64(len(p.Goals))
	}

	// No goals — fall back to task progress if available.
	if p.TasksTotal > 0 {
		return float64(p.TasksDone) / float64(p.TasksTotal)
	}
	return 0.0
}

// TaskProgress returns 0.0..1.0 representing task completion for this project.
func (p Project) TaskProgress() float64 {
	if p.TasksTotal == 0 {
		return 0.0
	}
	return float64(p.TasksDone) / float64(p.TasksTotal)
}

// ComputeTaskCounts populates TasksDone and TasksTotal from a list of tasks.
func (p *Project) ComputeTaskCounts(tasks []Task) {
	p.TasksDone = 0
	p.TasksTotal = 0
	for _, t := range tasks {
		if t.Project != p.Name {
			continue
		}
		p.TasksTotal++
		if t.Done {
			p.TasksDone++
		}
	}
}

// projectTask is a parsed checkbox task relevant to a project.
type projectTask struct {
	Text    string
	Done    bool
	Source  string
	LineNum int // 1-based line number in source file
}

// projectNote is a note file discovered in the project folder.
type projectNote struct {
	Name    string
	Path    string
	ModTime time.Time
}

// ---------------------------------------------------------------------------
// ProjectMode overlay
// ---------------------------------------------------------------------------

// ProjectMode is an overlay that provides project management capabilities
// inside the Granit TUI. It supports listing, dashboarding, and editing
// projects stored in <vault>/.granit/projects.json.
type ProjectMode struct {
	OverlayBase
	vaultRoot string

	// Data
	projects []Project

	// Navigation
	phase        int // pmPhaseList, pmPhaseDashboard, pmPhaseEdit
	cursor       int
	scroll       int
	dashSection  int // 0=notes, 1=tasks, 2=stats
	dashScroll   int
	categoryIdx  int // -1 = all, 0..len-1 = filter by category
	selectedProj int // index of the project shown in dashboard

	// Dashboard data (populated on open)
	dashNotes []projectNote
	dashTasks []projectTask

	// Goal management mode (inside dashboard)
	goalMode     bool // true when in goal management sub-mode
	goalCursor   int  // cursor within goals list
	goalExpanded int  // index of expanded goal (-1 = none)
	milestoneCur int  // cursor within milestones of expanded goal

	// Dashboard input mode for next-action / goal / milestone entry
	dashInput     bool   // true when typing a single-line input
	dashInputKind string // "next_action", "goal", "milestone"
	dashInputBuf  string

	// Edit form state
	editIdx        int // -1 = new project, >= 0 = editing existing
	editField      int // 0..9 (see editFields slice)
	editName       string
	editDesc       string
	editFolder     string
	editCategory   int
	editTags       string
	editColor      int
	editStatus     int
	editPriority   int
	editDueDate    string
	editNextAction string

	// AI integration
	ai          AIConfig
	aiPending   bool
	aiInsight   string
	showInsight bool

	// Consumed-once outputs
	selectedNote string
	hasNote      bool
	action       CommandAction
	fileChanged  bool // set when a task is toggled on disk
}

// NewProjectMode creates a new inactive ProjectMode overlay.
func NewProjectMode() ProjectMode {
	return ProjectMode{
		categoryIdx:  -1,
		editIdx:      -1,
		goalExpanded: -1,
	}
}

// Open activates the overlay and loads projects from disk.
func (pm *ProjectMode) Open(vaultRoot string) {
	pm.Activate()
	pm.vaultRoot = vaultRoot
	pm.phase = pmPhaseList
	pm.cursor = 0
	pm.scroll = 0
	pm.categoryIdx = -1
	pm.selectedNote = ""
	pm.hasNote = false
	pm.action = CmdNone
	pm.goalMode = false
	pm.goalCursor = 0
	pm.goalExpanded = -1
	pm.milestoneCur = 0
	pm.dashInput = false
	pm.loadProjects()
}

// GetProjects returns the current list of projects.
func (pm *ProjectMode) GetProjects() []Project {
	return pm.projects
}

// Refresh re-loads projects from disk WITHOUT resetting UI
// state (cursor / scroll / expanded dashboard). Called from
// refreshComponents whenever the vault changes so the project
// list reflects external edits + the dashboard's task count
// updates after a TaskManager toggle. scanProjectTasks already
// reads tasks live, so this only needs to reload project
// metadata; the dashboard's dashTasks slice is recomputed by
// the existing render path.
//
// Skips entirely if not active so we don't pay the disk cost
// for a closed surface.
func (pm *ProjectMode) Refresh(vaultRoot string) {
	if !pm.active {
		return
	}
	pm.vaultRoot = vaultRoot
	pm.loadProjects()
	// If we're sitting on the dashboard for a specific project,
	// re-scan its tasks so the progress bar reflects fresh state.
	if pm.phase == pmPhaseDashboard && pm.cursor < len(pm.projects) {
		pm.dashTasks = pm.scanProjectTasks(pm.projects[pm.cursor])
	}
}

// GetSelectedNote returns the note path the user chose and resets the flag.
func (pm *ProjectMode) GetSelectedNote() (string, bool) {
	if !pm.hasNote {
		return "", false
	}
	path := pm.selectedNote
	pm.selectedNote = ""
	pm.hasNote = false
	return path, true
}

// GetAction returns a pending action and clears it.
func (pm *ProjectMode) GetAction() (CommandAction, bool) {
	if pm.action != CmdNone {
		a := pm.action
		pm.action = CmdNone
		return a, true
	}
	return CmdNone, false
}

// WasFileChanged returns true once after a task was toggled on disk.
func (pm *ProjectMode) WasFileChanged() bool {
	if pm.fileChanged {
		pm.fileChanged = false
		return true
	}
	return false
}

// toggleTask toggles the done state of a task in the dashboard and writes
// the change back to the source file on disk.
func (pm *ProjectMode) toggleTask(idx int) {
	if idx < 0 || idx >= len(pm.dashTasks) {
		return
	}
	task := &pm.dashTasks[idx]
	absPath := filepath.Join(pm.vaultRoot, task.Source)
	data, err := os.ReadFile(absPath)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	if task.LineNum < 1 || task.LineNum > len(lines) {
		return
	}
	line := lines[task.LineNum-1]
	if task.Done {
		line = strings.Replace(line, "[x]", "[ ]", 1)
		line = strings.Replace(line, "[X]", "[ ]", 1)
	} else {
		line = strings.Replace(line, "[ ]", "[x]", 1)
	}
	lines[task.LineNum-1] = line
	if err := atomicWriteNote(absPath, strings.Join(lines, "\n")); err != nil {
		return
	}
	task.Done = !task.Done
	pm.fileChanged = true
}

// ---------------------------------------------------------------------------
// Storage
// ---------------------------------------------------------------------------

func (pm *ProjectMode) projectsFilePath() string {
	return projectsStatePath(pm.vaultRoot)
}

func (pm *ProjectMode) loadProjects() {
	pm.projects = LoadProjects(pm.vaultRoot)
}

func (pm *ProjectMode) saveProjects() {
	_ = SaveProjects(pm.vaultRoot, pm.projects)
}

// ---------------------------------------------------------------------------
// AI Insights
// ---------------------------------------------------------------------------

// aiProjectInsight sends the selected project to the LLM for health analysis.
func (pm *ProjectMode) aiProjectInsight() tea.Cmd {
	if pm.selectedProj < 0 || pm.selectedProj >= len(pm.projects) {
		return nil
	}
	ai := pm.ai
	proj := pm.projects[pm.selectedProj]
	// Deep copy goals to avoid sharing slice backing arrays with the main goroutine.
	goalsCopy := make([]ProjectGoal, len(proj.Goals))
	for i, g := range proj.Goals {
		goalsCopy[i] = g
		goalsCopy[i].Milestones = make([]ProjectMilestone, len(g.Milestones))
		copy(goalsCopy[i].Milestones, g.Milestones)
	}
	proj.Goals = goalsCopy

	return func() tea.Msg {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("PROJECT: %s\n", proj.Name))
		sb.WriteString(fmt.Sprintf("Description: %s\n", proj.Description))
		sb.WriteString(fmt.Sprintf("Category: %s | Status: %s | Priority: %d\n", proj.Category, proj.Status, proj.Priority))
		if proj.DueDate != "" {
			sb.WriteString(fmt.Sprintf("Due: %s\n", proj.DueDate))
		}
		sb.WriteString(fmt.Sprintf("Tasks: %d done / %d total\n", proj.TasksDone, proj.TasksTotal))
		if proj.NextAction != "" {
			sb.WriteString(fmt.Sprintf("Next action: %s\n", proj.NextAction))
		}
		sb.WriteString(fmt.Sprintf("Time spent: %d minutes\n", proj.TimeSpent))

		if len(proj.Goals) > 0 {
			sb.WriteString("\nGOALS:\n")
			for _, g := range proj.Goals {
				done := 0
				for _, ms := range g.Milestones {
					if ms.Done {
						done++
					}
				}
				status := "[ ]"
				if g.Done {
					status = "[x]"
				}
				sb.WriteString(fmt.Sprintf("  %s %s (milestones: %d/%d)\n", status, g.Title, done, len(g.Milestones)))
			}
		}

		var systemPrompt string
		if ai.IsSmallModel() {
			systemPrompt = "Analyze this project. Format:\n" +
				"HEALTH STATUS: color — reason\n" +
				"KEY RISKS: bullets\n" +
				"NEXT ACTIONS: 3 items\n" +
				"BLOCKERS: bullets\n" +
				"TIMELINE CHECK: one sentence"
		} else {
			systemPrompt = DeepCovenSystem("project management advisor",
				"Analyze this project and provide:\n"+
					"1. HEALTH STATUS: Green/Yellow/Red with 1-line justification\n"+
					"2. KEY RISKS: What could derail this project (2-3 bullets)\n"+
					"3. NEXT ACTIONS: The 3 most impactful things to do right now\n"+
					"4. BLOCKERS: Anything that appears stalled or blocked\n"+
					"5. TIMELINE CHECK: Is the project on track for its deadline?\n\n"+
					"Be specific. Reference actual tasks and milestones. No filler.")
		}

		resp, err := ai.Chat(systemPrompt, sb.String())
		return pmAIInsightMsg{insight: strings.TrimSpace(resp), err: err}
	}
}

// ---------------------------------------------------------------------------
// Scanning helpers
// ---------------------------------------------------------------------------

// scanProjectFolder scans the project folder for .md files sorted by mod time.
func (pm *ProjectMode) scanProjectFolder(proj Project) []projectNote {
	folder := proj.Folder
	if folder == "" {
		return nil
	}
	absFolder := filepath.Join(pm.vaultRoot, folder)
	entries, err := os.ReadDir(absFolder)
	if err != nil {
		return nil
	}

	var notes []projectNote
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		notes = append(notes, projectNote{
			Name:    strings.TrimSuffix(e.Name(), ".md"),
			Path:    filepath.Join(folder, e.Name()),
			ModTime: info.ModTime(),
		})
	}

	// Sort newest first.
	sort.Slice(notes, func(i, j int) bool {
		return notes[i].ModTime.After(notes[j].ModTime)
	})

	// Limit to 10 most recent.
	if len(notes) > 10 {
		notes = notes[:10]
	}
	return notes
}

// scanProjectTasks scans for tasks matching the project tag in all .md files
// under the vault root.
func (pm *ProjectMode) scanProjectTasks(proj Project) []projectTask {
	filter := proj.TaskFilter
	if filter == "" && len(proj.Tags) > 0 {
		filter = proj.Tags[0]
	}
	if filter == "" {
		return nil
	}

	var tasks []projectTask

	// If filter looks like a folder path, scan just that folder.
	if strings.Contains(filter, "/") || strings.Contains(filter, string(os.PathSeparator)) {
		absDir := filepath.Join(pm.vaultRoot, filter)
		pm.scanTasksInDir(absDir, filter, &tasks)
		return tasks
	}

	// Otherwise treat as a tag; scan the project folder first, then
	// a Tasks.md at vault root.
	if proj.Folder != "" {
		absDir := filepath.Join(pm.vaultRoot, proj.Folder)
		pm.scanTasksInDir(absDir, filter, &tasks)
	}

	// Check vault-root Tasks.md
	pm.scanTasksInFile(tasksFilePath(pm.vaultRoot), filter, &tasks)

	return tasks
}

func (pm *ProjectMode) scanTasksInDir(absDir, filter string, tasks *[]projectTask) {
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		pm.scanTasksInFile(filepath.Join(absDir, e.Name()), filter, tasks)
	}
}

func (pm *ProjectMode) scanTasksInFile(absPath, filter string, tasks *[]projectTask) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return
	}
	relPath, _ := filepath.Rel(pm.vaultRoot, absPath)
	lines := strings.Split(string(data), "\n")
	for lineIdx, line := range lines {
		trimmed := strings.TrimSpace(line)
		var done bool
		var taskText string
		if strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
			done = true
			taskText = strings.TrimSpace(trimmed[6:])
		} else if strings.HasPrefix(trimmed, "- [ ] ") {
			done = false
			taskText = strings.TrimSpace(trimmed[6:])
		} else {
			continue
		}

		// Apply tag filter: task must contain #filter or the filter word.
		tagStr := "#" + filter
		if !strings.Contains(taskText, tagStr) && !strings.Contains(strings.ToLower(taskText), strings.ToLower(filter)) {
			continue
		}

		*tasks = append(*tasks, projectTask{
			Text:    taskText,
			Done:    done,
			Source:  relPath,
			LineNum: lineIdx + 1, // 1-based
		})
	}
}

// ---------------------------------------------------------------------------
// Category colors
// ---------------------------------------------------------------------------

func categoryColor(cat string) lipgloss.Color {
	switch cat {
	case "development":
		return blue
	case "social-media":
		return pink
	case "personal":
		return green
	case "business":
		return peach
	case "writing":
		return lavender
	case "research":
		return sapphire
	case "health":
		return teal
	case "finance":
		return yellow
	default:
		return text
	}
}

func statusColor(status string) lipgloss.Color {
	switch status {
	case "active":
		return green
	case "paused":
		return yellow
	case "completed":
		return blue
	case "archived":
		return overlay0
	default:
		return text
	}
}

func projectAccentColor(name string) lipgloss.Color {
	switch name {
	case "blue":
		return blue
	case "green":
		return green
	case "mauve":
		return mauve
	case "peach":
		return peach
	case "red":
		return red
	case "yellow":
		return yellow
	case "pink":
		return pink
	case "lavender":
		return lavender
	case "teal":
		return teal
	case "sapphire":
		return sapphire
	case "flamingo":
		return flamingo
	default:
		return mauve
	}
}

func statusBadge(status string) string {
	c := statusColor(status)
	icon := "●"
	label := status
	return lipgloss.NewStyle().Foreground(c).Render(icon+" ") +
		lipgloss.NewStyle().Foreground(c).Bold(true).Render(label)
}

// priorityDot returns a colored dot for priority display.
func priorityDot(pri int) string {
	switch pri {
	case 4: // highest
		return lipgloss.NewStyle().Foreground(red).Render("●")
	case 3: // high
		return lipgloss.NewStyle().Foreground(peach).Render("●")
	case 2: // medium
		return lipgloss.NewStyle().Foreground(yellow).Render("●")
	case 1: // low
		return lipgloss.NewStyle().Foreground(blue).Render("●")
	default:
		return lipgloss.NewStyle().Foreground(overlay0).Render("○")
	}
}

// projectPriorityLabel returns a colored label for a priority value.
func projectPriorityLabel(pri int) string {
	if pri < 0 || pri >= len(projectPriorityLabels) {
		pri = 0
	}
	label := strings.ToUpper(projectPriorityLabels[pri])
	switch pri {
	case 4:
		return lipgloss.NewStyle().Foreground(red).Bold(true).Render(label)
	case 3:
		return lipgloss.NewStyle().Foreground(peach).Bold(true).Render(label)
	case 2:
		return lipgloss.NewStyle().Foreground(yellow).Bold(true).Render(label)
	case 1:
		return lipgloss.NewStyle().Foreground(blue).Bold(true).Render(label)
	default:
		return lipgloss.NewStyle().Foreground(overlay0).Render(label)
	}
}

// formatTimeSpent returns a human-readable duration string.
func formatTimeSpent(minutes int) string {
	if minutes <= 0 {
		return "0m"
	}
	h := minutes / 60
	m := minutes % 60
	if h == 0 {
		return fmt.Sprintf("%dm", m)
	}
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

// ---------------------------------------------------------------------------
// Filtering & Sorting
// ---------------------------------------------------------------------------

func (pm *ProjectMode) filteredProjects() []int {
	var indices []int
	for i, p := range pm.projects {
		if p.Status == "archived" && pm.categoryIdx != -1 {
			// When filtering by category, skip archived unless specifically selected.
			continue
		}
		if pm.categoryIdx == -1 {
			indices = append(indices, i)
		} else if pm.categoryIdx < len(projectCategories) && p.Category == projectCategories[pm.categoryIdx] {
			indices = append(indices, i)
		}
	}

	// Sort: active projects by priority (desc), then due date (asc).
	// Paused/completed/archived go to the bottom.
	sort.SliceStable(indices, func(a, b int) bool {
		pa := pm.projects[indices[a]]
		pb := pm.projects[indices[b]]

		aActive := pa.Status == "active"
		bActive := pb.Status == "active"

		// Active projects first.
		if aActive != bActive {
			return aActive
		}

		// Within same activity group, sort by priority (highest first).
		if pa.Priority != pb.Priority {
			return pa.Priority > pb.Priority
		}

		// Then by due date (soonest first, empty last).
		if pa.DueDate != pb.DueDate {
			if pa.DueDate == "" {
				return false
			}
			if pb.DueDate == "" {
				return true
			}
			return pa.DueDate < pb.DueDate
		}

		return false
	})

	return indices
}

// progressWithTasks computes project progress, using task data as fallback.
func progressWithTasks(proj Project, tasks []projectTask) float64 {
	prog := proj.Progress()
	if prog > 0 || len(proj.Goals) > 0 {
		return prog
	}
	// Fallback to tasks.
	if len(tasks) == 0 {
		return 0
	}
	done := 0
	for _, t := range tasks {
		if t.Done {
			done++
		}
	}
	return float64(done) / float64(len(tasks))
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles keyboard input for the project mode overlay.
func (pm ProjectMode) Update(msg tea.Msg) (ProjectMode, tea.Cmd) {
	if !pm.active {
		return pm, nil
	}

	switch msg := msg.(type) {
	case pmAIInsightMsg:
		pm.aiPending = false
		if msg.err != nil {
			pm.aiInsight = "AI error: " + msg.err.Error()
		} else {
			pm.aiInsight = msg.insight
		}
		pm.showInsight = true
		return pm, nil
	case tea.KeyMsg:
		switch pm.phase {
		case pmPhaseList:
			return pm.updateList(msg)
		case pmPhaseDashboard:
			return pm.updateDashboard(msg)
		case pmPhaseEdit:
			return pm.updateEdit(msg)
		}
	}
	return pm, nil
}

func (pm ProjectMode) updateList(msg tea.KeyMsg) (ProjectMode, tea.Cmd) {
	filtered := pm.filteredProjects()
	switch msg.String() {
	case "esc":
		pm.active = false
	case "up", "k":
		if pm.cursor > 0 {
			pm.cursor--
			if pm.cursor < pm.scroll {
				pm.scroll = pm.cursor
			}
		}
	case "down", "j":
		if pm.cursor < len(filtered)-1 {
			pm.cursor++
			visH := pm.listVisibleHeight()
			if pm.cursor >= pm.scroll+visH {
				pm.scroll = pm.cursor - visH + 1
			}
		}
	case "enter":
		if len(filtered) > 0 && pm.cursor < len(filtered) {
			pm.selectedProj = filtered[pm.cursor]
			pm.openDashboard()
		}
	case "a":
		pm.openAddForm()
	case "e":
		if len(filtered) > 0 && pm.cursor < len(filtered) {
			pm.openEditForm(filtered[pm.cursor])
		}
	case "d":
		if len(filtered) > 0 && pm.cursor < len(filtered) {
			idx := filtered[pm.cursor]
			pm.projects[idx].Status = "archived"
			pm.saveProjects()
			// Clamp cursor after archiving shrinks the filtered list.
			newFiltered := pm.filteredProjects()
			if pm.cursor >= len(newFiltered) {
				pm.cursor = max(0, len(newFiltered)-1)
			}
		}
	case "tab":
		pm.categoryIdx++
		if pm.categoryIdx >= len(projectCategories) {
			pm.categoryIdx = -1
		}
		pm.cursor = 0
		pm.scroll = 0
	case " ":
		if len(filtered) > 0 && pm.cursor < len(filtered) {
			idx := filtered[pm.cursor]
			pm.cycleStatus(idx)
			pm.saveProjects()
			// Clamp cursor after status change may alter filtered list.
			newFiltered := pm.filteredProjects()
			if pm.cursor >= len(newFiltered) {
				pm.cursor = max(0, len(newFiltered)-1)
			}
			pm.scroll = 0
		}
	}
	return pm, nil
}

func (pm *ProjectMode) cycleStatus(idx int) {
	cur := pm.projects[idx].Status
	for i, s := range projectStatuses {
		if s == cur {
			next := (i + 1) % len(projectStatuses)
			pm.projects[idx].Status = projectStatuses[next]
			return
		}
	}
	pm.projects[idx].Status = "active"
}

func (pm ProjectMode) updateDashboard(msg tea.KeyMsg) (ProjectMode, tea.Cmd) {
	if pm.selectedProj < 0 || pm.selectedProj >= len(pm.projects) {
		pm.phase = pmPhaseList
		return pm, nil
	}

	key := msg.String()

	// Handle input mode first.
	if pm.dashInput {
		return pm.updateDashInput(msg)
	}

	// Handle goal mode.
	if pm.goalMode {
		return pm.updateGoalMode(msg)
	}

	switch key {
	case "esc":
		if pm.showInsight {
			pm.showInsight = false
			pm.aiInsight = ""
			return pm, nil
		}
		pm.phase = pmPhaseList
		pm.dashSection = 0
		pm.dashScroll = 0
		pm.goalMode = false
		pm.goalExpanded = -1
	case "tab":
		pm.dashSection = (pm.dashSection + 1) % 3
		pm.dashScroll = 0
	case "up", "k":
		if pm.dashScroll > 0 {
			pm.dashScroll--
		}
	case "down", "j":
		// Clamp scroll to the current section's item count.
		var sectionLen int
		switch pm.dashSection {
		case 0:
			sectionLen = len(pm.dashNotes)
		case 1:
			sectionLen = len(pm.dashTasks)
		default:
			sectionLen = 0
		}
		if pm.dashScroll < sectionLen-1 {
			pm.dashScroll++
		}
	case "o":
		// Open selected note from the notes section.
		if pm.dashSection == 0 && len(pm.dashNotes) == 0 {
			return pm, nil
		}
		if pm.dashSection == 0 && len(pm.dashNotes) > 0 {
			noteIdx := pm.dashScroll
			if noteIdx >= len(pm.dashNotes) {
				noteIdx = len(pm.dashNotes) - 1
			}
			pm.selectedNote = pm.dashNotes[noteIdx].Path
			pm.hasNote = true
			pm.active = false
		}
	case "t":
		pm.action = CmdTaskManager
		pm.active = false
	case "n":
		// Set next action (input mode).
		pm.dashInput = true
		pm.dashInputKind = "next_action"
		pm.dashInputBuf = pm.projects[pm.selectedProj].NextAction
	case "N":
		// Create new note in project folder. Don't switch to it on
		// failure — leaving the user on a "loaded" path that isn't on
		// disk would silently lose their first edit.
		proj := pm.projects[pm.selectedProj]
		if proj.Folder != "" {
			newPath := filepath.Join(proj.Folder, fmt.Sprintf("Untitled %s.md", time.Now().Format("2006-01-02 15-04")))
			absPath := filepath.Join(pm.vaultRoot, newPath)
			dir := filepath.Dir(absPath)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				break
			}
			if err := atomicWriteNote(absPath, "# New Note\n\n"); err != nil {
				break
			}
			pm.selectedNote = newPath
			pm.hasNote = true
			pm.active = false
		}
	case "g":
		// Enter goal management mode.
		pm.goalMode = true
		pm.goalCursor = 0
		pm.goalExpanded = -1
		pm.milestoneCur = 0
	case "p":
		// Cycle priority: none -> low -> medium -> high -> highest -> none.
		proj := &pm.projects[pm.selectedProj]
		proj.Priority = (proj.Priority + 1) % len(projectPriorityLabels)
		pm.saveProjects()
	case " ", "x":
		// Toggle task completion in the tasks section.
		if pm.dashSection == 1 && len(pm.dashTasks) > 0 {
			idx := pm.dashScroll
			if idx >= len(pm.dashTasks) {
				idx = len(pm.dashTasks) - 1
			}
			pm.toggleTask(idx)
		}
	case "I":
		// AI project insights.
		if !pm.aiPending && pm.ai.Provider != "local" && pm.ai.Provider != "" {
			pm.aiPending = true
			pm.showInsight = false
			return pm, pm.aiProjectInsight()
		}
	}
	return pm, nil
}

func (pm ProjectMode) updateDashInput(msg tea.KeyMsg) (ProjectMode, tea.Cmd) {
	if pm.selectedProj < 0 || pm.selectedProj >= len(pm.projects) {
		pm.phase = pmPhaseList
		return pm, nil
	}

	key := msg.String()

	switch key {
	case "esc":
		pm.dashInput = false
		pm.dashInputBuf = ""
		pm.dashInputKind = ""
	case "enter":
		val := strings.TrimSpace(pm.dashInputBuf)
		switch pm.dashInputKind {
		case "next_action":
			pm.projects[pm.selectedProj].NextAction = val
			pm.saveProjects()
		case "goal":
			if val != "" {
				pm.projects[pm.selectedProj].Goals = append(pm.projects[pm.selectedProj].Goals, ProjectGoal{
					Title: val,
				})
				pm.saveProjects()
			}
		case "milestone":
			if val != "" && pm.goalExpanded >= 0 && pm.goalExpanded < len(pm.projects[pm.selectedProj].Goals) {
				pm.projects[pm.selectedProj].Goals[pm.goalExpanded].Milestones = append(
					pm.projects[pm.selectedProj].Goals[pm.goalExpanded].Milestones,
					ProjectMilestone{Text: val},
				)
				pm.saveProjects()
			}
		}
		pm.dashInput = false
		pm.dashInputBuf = ""
		pm.dashInputKind = ""
	case "backspace":
		if len(pm.dashInputBuf) > 0 {
			pm.dashInputBuf = TrimLastRune(pm.dashInputBuf)
		}
	default:
		if len(key) == 1 {
			pm.dashInputBuf += key
		} else if key == "space" {
			pm.dashInputBuf += " "
		}
	}
	return pm, nil
}

func (pm ProjectMode) updateGoalMode(msg tea.KeyMsg) (ProjectMode, tea.Cmd) {
	if pm.selectedProj < 0 || pm.selectedProj >= len(pm.projects) {
		pm.phase = pmPhaseList
		return pm, nil
	}

	key := msg.String()
	proj := &pm.projects[pm.selectedProj]

	switch key {
	case "esc":
		if pm.goalExpanded >= 0 {
			pm.goalExpanded = -1
			pm.milestoneCur = 0
		} else {
			pm.goalMode = false
		}
	case "up", "k":
		if pm.goalExpanded >= 0 {
			if pm.milestoneCur > 0 {
				pm.milestoneCur--
			}
		} else {
			if pm.goalCursor > 0 {
				pm.goalCursor--
			}
		}
	case "down", "j":
		if pm.goalExpanded >= 0 {
			if pm.goalExpanded < len(proj.Goals) {
				g := proj.Goals[pm.goalExpanded]
				if pm.milestoneCur < len(g.Milestones)-1 {
					pm.milestoneCur++
				}
			}
		} else {
			if pm.goalCursor < len(proj.Goals)-1 {
				pm.goalCursor++
			}
		}
	case " ", "enter":
		if pm.goalExpanded >= 0 {
			// Toggle milestone.
			if pm.goalExpanded < len(proj.Goals) {
				g := &proj.Goals[pm.goalExpanded]
				if pm.milestoneCur >= 0 && pm.milestoneCur < len(g.Milestones) {
					g.Milestones[pm.milestoneCur].Done = !g.Milestones[pm.milestoneCur].Done
					pm.saveProjects()
				}
			}
		} else {
			// Toggle goal done or expand.
			if pm.goalCursor >= 0 && pm.goalCursor < len(proj.Goals) {
				g := &proj.Goals[pm.goalCursor]
				if len(g.Milestones) > 0 {
					// Expand to show milestones.
					pm.goalExpanded = pm.goalCursor
					pm.milestoneCur = 0
				} else {
					// Toggle done.
					g.Done = !g.Done
					pm.saveProjects()
				}
			}
		}
	case "a":
		// Add new goal.
		pm.dashInput = true
		pm.dashInputKind = "goal"
		pm.dashInputBuf = ""
	case "m":
		// Add milestone to selected goal.
		if pm.goalExpanded >= 0 {
			pm.dashInput = true
			pm.dashInputKind = "milestone"
			pm.dashInputBuf = ""
		} else if pm.goalCursor >= 0 && pm.goalCursor < len(proj.Goals) {
			pm.goalExpanded = pm.goalCursor
			pm.milestoneCur = 0
			pm.dashInput = true
			pm.dashInputKind = "milestone"
			pm.dashInputBuf = ""
		}
	case "d":
		// Delete goal or milestone.
		if pm.goalExpanded >= 0 && pm.goalExpanded < len(proj.Goals) {
			g := &proj.Goals[pm.goalExpanded]
			if len(g.Milestones) > 0 && pm.milestoneCur >= 0 && pm.milestoneCur < len(g.Milestones) {
				g.Milestones = append(g.Milestones[:pm.milestoneCur], g.Milestones[pm.milestoneCur+1:]...)
				if pm.milestoneCur >= len(g.Milestones) && pm.milestoneCur > 0 {
					pm.milestoneCur--
				}
				pm.saveProjects()
			}
		} else if pm.goalCursor >= 0 && pm.goalCursor < len(proj.Goals) {
			proj.Goals = append(proj.Goals[:pm.goalCursor], proj.Goals[pm.goalCursor+1:]...)
			if len(proj.Goals) == 0 {
				pm.goalCursor = 0
			} else if pm.goalCursor >= len(proj.Goals) {
				pm.goalCursor = len(proj.Goals) - 1
			}
			pm.saveProjects()
		}
	}
	return pm, nil
}

// editFieldCount is the total number of edit form fields.
const editFieldCount = 10

func (pm ProjectMode) updateEdit(msg tea.KeyMsg) (ProjectMode, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		pm.phase = pmPhaseList
	case "tab":
		pm.editField = (pm.editField + 1) % editFieldCount
	case "shift+tab":
		pm.editField--
		if pm.editField < 0 {
			pm.editField = editFieldCount - 1
		}
	case "enter":
		pm.commitEdit()
		pm.phase = pmPhaseList
	case "left":
		switch pm.editField {
		case 3: // category
			pm.editCategory--
			if pm.editCategory < 0 {
				pm.editCategory = len(projectCategories) - 1
			}
		case 5: // color
			pm.editColor--
			if pm.editColor < 0 {
				pm.editColor = len(projectColorNames) - 1
			}
		case 6: // status
			pm.editStatus--
			if pm.editStatus < 0 {
				pm.editStatus = len(projectStatuses) - 1
			}
		case 7: // priority
			pm.editPriority--
			if pm.editPriority < 0 {
				pm.editPriority = len(projectPriorityLabels) - 1
			}
		}
	case "right":
		switch pm.editField {
		case 3: // category
			pm.editCategory = (pm.editCategory + 1) % len(projectCategories)
		case 5: // color
			pm.editColor = (pm.editColor + 1) % len(projectColorNames)
		case 6: // status
			pm.editStatus = (pm.editStatus + 1) % len(projectStatuses)
		case 7: // priority
			pm.editPriority = (pm.editPriority + 1) % len(projectPriorityLabels)
		}
	case "backspace":
		pm.editBackspace()
	default:
		if len(key) == 1 {
			pm.editInsertChar(key)
		}
	}
	return pm, nil
}

func (pm *ProjectMode) editBackspace() {
	switch pm.editField {
	case 0:
		if len(pm.editName) > 0 {
			pm.editName = TrimLastRune(pm.editName)
		}
	case 1:
		if len(pm.editDesc) > 0 {
			pm.editDesc = TrimLastRune(pm.editDesc)
		}
	case 2:
		if len(pm.editFolder) > 0 {
			pm.editFolder = TrimLastRune(pm.editFolder)
		}
	case 4:
		if len(pm.editTags) > 0 {
			pm.editTags = TrimLastRune(pm.editTags)
		}
	case 8: // due date
		if len(pm.editDueDate) > 0 {
			pm.editDueDate = TrimLastRune(pm.editDueDate)
		}
	case 9: // next action
		if len(pm.editNextAction) > 0 {
			pm.editNextAction = TrimLastRune(pm.editNextAction)
		}
	}
}

func (pm *ProjectMode) editInsertChar(ch string) {
	switch pm.editField {
	case 0:
		pm.editName += ch
	case 1:
		pm.editDesc += ch
	case 2:
		pm.editFolder += ch
	case 4:
		pm.editTags += ch
	case 8: // due date
		pm.editDueDate += ch
	case 9: // next action
		pm.editNextAction += ch
	}
}

func (pm *ProjectMode) openAddForm() {
	pm.phase = pmPhaseEdit
	pm.editIdx = -1
	pm.editField = 0
	pm.editName = ""
	pm.editDesc = ""
	pm.editFolder = ""
	pm.editCategory = 0
	pm.editTags = ""
	pm.editColor = 0
	pm.editStatus = 0
	pm.editPriority = 0
	pm.editDueDate = ""
	pm.editNextAction = ""
}

func (pm *ProjectMode) openEditForm(projIdx int) {
	proj := pm.projects[projIdx]
	pm.phase = pmPhaseEdit
	pm.editIdx = projIdx
	pm.editField = 0
	pm.editName = proj.Name
	pm.editDesc = proj.Description
	pm.editFolder = proj.Folder
	pm.editTags = strings.Join(proj.Tags, ", ")

	pm.editCategory = 0
	for i, c := range projectCategories {
		if c == proj.Category {
			pm.editCategory = i
			break
		}
	}
	pm.editColor = 0
	for i, c := range projectColorNames {
		if c == proj.Color {
			pm.editColor = i
			break
		}
	}
	pm.editStatus = 0
	for i, s := range projectStatuses {
		if s == proj.Status {
			pm.editStatus = i
			break
		}
	}
	pm.editPriority = proj.Priority
	if pm.editPriority < 0 || pm.editPriority >= len(projectPriorityLabels) {
		pm.editPriority = 0
	}
	pm.editDueDate = proj.DueDate
	pm.editNextAction = proj.NextAction
}

func (pm *ProjectMode) openDashboard() {
	pm.phase = pmPhaseDashboard
	pm.dashSection = 0
	pm.dashScroll = 0
	pm.goalMode = false
	pm.goalCursor = 0
	pm.goalExpanded = -1
	pm.milestoneCur = 0
	pm.dashInput = false
	proj := pm.projects[pm.selectedProj]
	pm.dashNotes = pm.scanProjectFolder(proj)
	pm.dashTasks = pm.scanProjectTasks(proj)

	// Populate TasksDone/TasksTotal from scanned project tasks.
	done := 0
	for _, t := range pm.dashTasks {
		if t.Done {
			done++
		}
	}
	pm.projects[pm.selectedProj].TasksDone = done
	pm.projects[pm.selectedProj].TasksTotal = len(pm.dashTasks)
}

func (pm *ProjectMode) commitEdit() {
	tags := parseTags(pm.editTags)

	cat := "other"
	if pm.editCategory >= 0 && pm.editCategory < len(projectCategories) {
		cat = projectCategories[pm.editCategory]
	}
	col := "blue"
	if pm.editColor >= 0 && pm.editColor < len(projectColorNames) {
		col = projectColorNames[pm.editColor]
	}
	st := "active"
	if pm.editStatus >= 0 && pm.editStatus < len(projectStatuses) {
		st = projectStatuses[pm.editStatus]
	}
	pri := pm.editPriority
	if pri < 0 || pri >= len(projectPriorityLabels) {
		pri = 0
	}

	if pm.editIdx < 0 {
		// New project.
		proj := Project{
			Name:        pm.editName,
			Description: pm.editDesc,
			Folder:      pm.editFolder,
			Tags:        tags,
			Status:      st,
			Color:       col,
			CreatedAt:   time.Now().Format("2006-01-02"),
			Category:    cat,
			Priority:    pri,
			DueDate:     pm.editDueDate,
			NextAction:  pm.editNextAction,
		}
		pm.projects = append(pm.projects, proj)
	} else {
		// Update existing.
		pm.projects[pm.editIdx].Name = pm.editName
		pm.projects[pm.editIdx].Description = pm.editDesc
		pm.projects[pm.editIdx].Folder = pm.editFolder
		pm.projects[pm.editIdx].Tags = tags
		pm.projects[pm.editIdx].Status = st
		pm.projects[pm.editIdx].Color = col
		pm.projects[pm.editIdx].Category = cat
		pm.projects[pm.editIdx].Priority = pri
		pm.projects[pm.editIdx].DueDate = pm.editDueDate
		pm.projects[pm.editIdx].NextAction = pm.editNextAction
	}
	pm.saveProjects()
}

func parseTags(raw string) []string {
	parts := strings.Split(raw, ",")
	var tags []string
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// ---------------------------------------------------------------------------
// View helpers
// ---------------------------------------------------------------------------

func (pm ProjectMode) listVisibleHeight() int {
	// Tab mode reserves the same 12-line chrome (header + footer
	// + status row) but anchors against the full editor pane
	// height, not the overlay-clamped one. Floor at 3 to keep at
	// least one visible row even on a tiny terminal.
	h := pm.height - 12
	if h < 3 {
		h = 3
	}
	return h
}

func (pm ProjectMode) overlayWidth() int {
	if pm.IsTabMode() {
		w := pm.width - 2
		if w < 60 {
			w = 60
		}
		return w
	}
	w := pm.width * 2 / 3
	if w < 60 {
		w = 60
	}
	if w > 100 {
		w = 100
	}
	return w
}

// pmProgressBar renders a small inline progress bar of given width.
func pmProgressBar(progress float64, width int) string {
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	filled := int(progress * float64(width))
	if filled > width {
		filled = width
	}
	empty := width - filled
	bar := lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("█", filled))
	bar += lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("░", empty))
	return bar
}

// pmPadRight pads a string to the given visible width.
func pmPadRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the project mode overlay.
func (pm ProjectMode) View() string {
	switch pm.phase {
	case pmPhaseList:
		return pm.viewList()
	case pmPhaseDashboard:
		return pm.viewDashboard()
	case pmPhaseEdit:
		return pm.viewEdit()
	}
	return ""
}

// --- Project list view ---

func (pm ProjectMode) viewList() string {
	width := pm.overlayWidth()
	var b strings.Builder

	// Title
	title := lipgloss.NewStyle().Foreground(mauve).Bold(true).
		Render(IconFolderChar + "  Projects")
	b.WriteString(title)

	// Category filter badge
	if pm.categoryIdx >= 0 && pm.categoryIdx < len(projectCategories) {
		cat := projectCategories[pm.categoryIdx]
		badge := lipgloss.NewStyle().
			Foreground(categoryColor(cat)).
			Render("  [" + cat + "]")
		b.WriteString(badge)
	} else {
		b.WriteString(DimStyle.Render("  [all]"))
	}
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	filtered := pm.filteredProjects()

	if len(filtered) == 0 {
		b.WriteString(DimStyle.Render("  No projects found"))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  Press 'a' to create your first project"))
	} else {
		visH := pm.listVisibleHeight()
		end := pm.scroll + visH
		if end > len(filtered) {
			end = len(filtered)
		}

		for vi := pm.scroll; vi < end; vi++ {
			idx := filtered[vi]
			proj := pm.projects[idx]

			// Priority dot
			pDot := priorityDot(proj.Priority) + " "

			// Name (with max width to leave room for other columns)
			nameMaxW := width - 50
			if nameMaxW < 12 {
				nameMaxW = 12
			}
			displayName := TruncateDisplay(proj.Name, nameMaxW)
			nameStyle := lipgloss.NewStyle().Foreground(projectAccentColor(proj.Color)).Bold(true)
			name := nameStyle.Render(pmPadRight(displayName, nameMaxW))

			// Status badge
			stBadge := lipgloss.NewStyle().Foreground(statusColor(proj.Status)).Render(pmPadRight(proj.Status, 10))

			// Progress bar
			tasks := pm.scanProjectTasks(proj)
			prog := progressWithTasks(proj, tasks)
			pctInt := int(prog * 100)
			bar := pmProgressBar(prog, 10)
			pctStr := lipgloss.NewStyle().Foreground(green).Render(fmt.Sprintf("%3d%%", pctInt))

			// Next action (truncated)
			nextAct := ""
			if proj.NextAction != "" {
				na := proj.NextAction
				maxNA := width - 60 - nameMaxW
				if maxNA < 5 {
					maxNA = 5
				}
				if r := []rune(na); len(r) > maxNA {
					na = string(r[:maxNA-3]) + "..."
				}
				nextAct = DimStyle.Render(" -> ") + lipgloss.NewStyle().Foreground(subtext0).Render(na)
			}

			line := "  " + pDot + name + " " + stBadge + " " + bar + " " + pctStr + nextAct

			if vi == pm.cursor {
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Width(width - 6).
					Render(line))
			} else {
				b.WriteString(line)
			}

			if vi < end-1 {
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(pm.listHelpBar())

	return pm.wrapBorder(b.String(), width, mauve)
}

func (pm ProjectMode) listHelpBar() string {
	return RenderHelpBar([]struct{ Key, Desc string }{
		{"Enter", "open"}, {"a", "add"}, {"e", "edit"},
		{"d", "archive"}, {"Tab", "filter"}, {"Space", "status"}, {"Esc", "close"},
	})
}

// --- Dashboard view ---

func (pm ProjectMode) viewDashboard() string {
	if pm.selectedProj < 0 || pm.selectedProj >= len(pm.projects) {
		return ""
	}

	width := pm.overlayWidth()
	proj := pm.projects[pm.selectedProj]

	var b strings.Builder

	// Project header
	accent := projectAccentColor(proj.Color)
	nameStyle := lipgloss.NewStyle().Foreground(accent).Bold(true)
	b.WriteString(nameStyle.Render(proj.Name))
	b.WriteString("  ")
	b.WriteString(statusBadge(proj.Status))
	b.WriteString("\n")

	if proj.Description != "" {
		b.WriteString(DimStyle.Render(proj.Description))
		b.WriteString("\n")
	}

	// Status line: priority, due date, time spent
	var metaParts []string
	metaParts = append(metaParts, "Status: "+lipgloss.NewStyle().Foreground(statusColor(proj.Status)).Bold(true).Render(proj.Status))
	if proj.Priority > 0 {
		metaParts = append(metaParts, "Priority: "+priorityDot(proj.Priority)+" "+projectPriorityLabel(proj.Priority))
	}
	if proj.DueDate != "" {
		metaParts = append(metaParts, "Due: "+lipgloss.NewStyle().Foreground(teal).Render(proj.DueDate))
	}
	if proj.TimeSpent > 0 {
		metaParts = append(metaParts, "Time: "+lipgloss.NewStyle().Foreground(peach).Render(formatTimeSpent(proj.TimeSpent)))
	}
	b.WriteString(DimStyle.Render("  ") + strings.Join(metaParts, DimStyle.Render("  ")))
	b.WriteString("\n")

	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	// Next Action section
	b.WriteString(pm.viewDashNextAction(width, proj))

	// Goals section
	b.WriteString(pm.viewDashGoals(width, proj))

	// Section tabs
	sections := []string{"Notes", "Tasks", "Stats"}
	var tabs []string
	for i, s := range sections {
		if i == pm.dashSection {
			tabs = append(tabs, lipgloss.NewStyle().
				Foreground(mauve).Bold(true).Underline(true).Render(s))
		} else {
			tabs = append(tabs, DimStyle.Render(s))
		}
	}
	b.WriteString("  " + strings.Join(tabs, "    "))
	b.WriteString("\n\n")

	switch pm.dashSection {
	case 0:
		b.WriteString(pm.viewDashNotes(width))
	case 1:
		b.WriteString(pm.viewDashTasks(width))
	case 2:
		b.WriteString(pm.viewDashStats(width))
	}

	// AI insight panel
	if pm.showInsight && pm.aiInsight != "" {
		b.WriteString("\n")
		headStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
		bodyStyle := lipgloss.NewStyle().Foreground(lavender).Italic(true)
		subhStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
		b.WriteString("  " + headStyle.Render(IconBotChar+" AI Project Insights") + "\n\n")
		for _, line := range strings.Split(pm.aiInsight, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				b.WriteString("\n")
			} else if strings.HasPrefix(trimmed, "##") {
				b.WriteString("  " + subhStyle.Render(strings.TrimLeft(trimmed, "# ")) + "\n")
			} else {
				b.WriteString("  " + bodyStyle.Render(TruncateDisplay(trimmed, width-10)) + "\n")
			}
		}
		b.WriteString("\n  " + DimStyle.Render("Esc to dismiss") + "\n")
	} else if pm.aiPending {
		b.WriteString("\n  " + lipgloss.NewStyle().Foreground(mauve).Render("  AI analyzing project...") + "\n")
	}

	// Input prompt (if active)
	if pm.dashInput {
		b.WriteString("\n")
		var prompt string
		switch pm.dashInputKind {
		case "next_action":
			prompt = "Next action: "
		case "goal":
			prompt = "New goal: "
		case "milestone":
			prompt = "New milestone: "
		}
		b.WriteString("  " + lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(prompt) +
			lipgloss.NewStyle().Foreground(text).Render(pm.dashInputBuf+"_"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(pm.dashHelpBar())

	return pm.wrapBorder(b.String(), width, accent)
}

func (pm ProjectMode) viewDashNextAction(_ int, proj Project) string {
	var b strings.Builder

	sectionTitle := lipgloss.NewStyle().Foreground(peach).Bold(true).Render("  NEXT ACTION")
	b.WriteString(sectionTitle)
	b.WriteString("\n")

	if proj.NextAction != "" {
		box := lipgloss.NewStyle().
			Foreground(text).
			PaddingLeft(1).
			PaddingRight(1).
			Background(surface0)
		b.WriteString("  " + box.Render(proj.NextAction))
	} else {
		b.WriteString("  " + DimStyle.Render("(none set - press 'n' to set)"))
	}
	b.WriteString("\n\n")
	return b.String()
}

func (pm ProjectMode) viewDashGoals(width int, proj Project) string {
	var b strings.Builder

	// Calculate progress
	prog := proj.Progress()
	if len(proj.Goals) == 0 && len(pm.dashTasks) > 0 {
		prog = progressWithTasks(proj, pm.dashTasks)
	}
	pctInt := int(prog * 100)

	sectionTitle := lipgloss.NewStyle().Foreground(blue).Bold(true).Render("  GOALS")
	progStr := DimStyle.Render("Progress: ") +
		lipgloss.NewStyle().Foreground(green).Bold(true).Render(fmt.Sprintf("%d%%", pctInt))

	// Right-align progress
	titleLen := 9 // "  GOALS" approximate visible length
	progVisLen := 16
	padding := width - 6 - titleLen - progVisLen
	if padding < 2 {
		padding = 2
	}
	b.WriteString(sectionTitle + strings.Repeat(" ", padding) + progStr)
	b.WriteString("\n")

	// Progress bar
	barWidth := width - 10
	if barWidth < 10 {
		barWidth = 10
	}
	b.WriteString("  " + pmProgressBar(prog, barWidth))

	// Goal count
	doneGoals := 0
	for _, g := range proj.Goals {
		if g.Done {
			doneGoals++
		}
	}
	if len(proj.Goals) > 0 {
		b.WriteString(" " + DimStyle.Render(fmt.Sprintf("%d/%d goals", doneGoals, len(proj.Goals))))
	}
	b.WriteString("\n")

	// Task progress (secondary indicator)
	if proj.TasksTotal > 0 {
		taskPct := proj.TasksDone * 100 / proj.TasksTotal
		taskProgStr := DimStyle.Render("  Tasks: ") +
			lipgloss.NewStyle().Foreground(yellow).Render(fmt.Sprintf("%d/%d", proj.TasksDone, proj.TasksTotal)) +
			DimStyle.Render(fmt.Sprintf(" (%d%%)", taskPct))
		b.WriteString(taskProgStr)
		b.WriteString("\n")
	}

	// Count milestones for burndown chart
	totalMS := 0
	doneMS := 0
	for _, g := range proj.Goals {
		totalMS += len(g.Milestones)
		for _, m := range g.Milestones {
			if m.Done {
				doneMS++
			}
		}
	}

	if len(proj.Goals) == 0 {
		b.WriteString("  " + DimStyle.Render("No goals yet - press 'g' to manage"))
		b.WriteString("\n")
	} else {
		b.WriteString("\n")
		for gi, g := range proj.Goals {
			// Goal line
			var icon string
			if g.Done {
				icon = lipgloss.NewStyle().Foreground(green).Render("  + ")
			} else {
				// Check if it's "in progress" (has some done milestones)
				hasDone := false
				for _, m := range g.Milestones {
					if m.Done {
						hasDone = true
						break
					}
				}
				if hasDone {
					icon = lipgloss.NewStyle().Foreground(blue).Render("  > ")
				} else {
					icon = lipgloss.NewStyle().Foreground(overlay0).Render("  - ")
				}
			}

			goalStyle := lipgloss.NewStyle().Foreground(text)
			if pm.goalMode && !pm.dashInput && pm.goalExpanded < 0 && pm.goalCursor == gi {
				goalStyle = lipgloss.NewStyle().Foreground(peach).Bold(true)
			}
			if g.Done {
				goalStyle = goalStyle.Strikethrough(true).Foreground(overlay0)
			}
			b.WriteString(icon + goalStyle.Render(g.Title))
			b.WriteString("\n")

			// Milestones
			for mi, m := range g.Milestones {
				var mIcon string
				if m.Done {
					mIcon = lipgloss.NewStyle().Foreground(green).Render("    + ")
				} else {
					mIcon = lipgloss.NewStyle().Foreground(overlay0).Render("    - ")
				}
				msStyle := lipgloss.NewStyle().Foreground(subtext0)
				if pm.goalMode && pm.goalExpanded == gi && pm.milestoneCur == mi {
					msStyle = lipgloss.NewStyle().Foreground(peach).Bold(true)
				}
				if m.Done {
					msStyle = msStyle.Strikethrough(true).Foreground(overlay0)
				}
				b.WriteString(mIcon + msStyle.Render(m.Text))
				b.WriteString("\n")
			}
		}
	}

	// Burndown chart (if project has milestones and dates)
	if totalMS > 0 && proj.CreatedAt != "" && proj.DueDate != "" {
		b.WriteString("\n")
		b.WriteString(pm.renderBurndownChart(proj, totalMS, doneMS, width))
	}

	b.WriteString("\n")
	return b.String()
}

// renderBurndownChart draws an ASCII burndown chart showing ideal vs actual progress.
func (pm ProjectMode) renderBurndownChart(proj Project, totalMS, doneMS, width int) string {
	var out strings.Builder

	created, err1 := time.Parse("2006-01-02", proj.CreatedAt)
	due, err2 := time.Parse("2006-01-02", proj.DueDate)
	if err1 != nil || err2 != nil {
		return ""
	}

	chartW := width - 12
	if chartW < 20 {
		chartW = 20
	}
	if chartW > 40 {
		chartW = 40
	}
	chartH := 8

	totalWeeks := int(due.Sub(created).Hours()/(24*7)) + 1
	if totalWeeks < 2 {
		totalWeeks = 2
	}
	currentWeek := int(time.Since(created).Hours()/(24*7)) + 1
	if currentWeek > totalWeeks {
		currentWeek = totalWeeks
	}

	remaining := totalMS - doneMS

	out.WriteString("  " + lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("Burndown") + "\n")

	// Draw chart rows (top = totalMS, bottom = 0)
	for row := 0; row < chartH; row++ {
		yVal := totalMS - (row * totalMS / (chartH - 1))
		label := fmt.Sprintf("  %3d ", yVal)
		out.WriteString(DimStyle.Render(label) + DimStyle.Render("│"))

		for col := 0; col < chartW; col++ {
			weekAtCol := col * totalWeeks / chartW

			// Ideal burndown line: linear from totalMS to 0
			idealY := totalMS - (weekAtCol * totalMS / totalWeeks)
			// Actual: project from (0, totalMS) to (currentWeek, remaining)
			actualY := totalMS
			if currentWeek > 0 && weekAtCol <= currentWeek {
				actualY = totalMS - ((totalMS - remaining) * weekAtCol / currentWeek)
			}

			idealRow := 0
			if totalMS > 0 {
				idealRow = (totalMS - idealY) * (chartH - 1) / totalMS
			}
			actualRow := 0
			if totalMS > 0 {
				actualRow = (totalMS - actualY) * (chartH - 1) / totalMS
			}

			if row == actualRow && weekAtCol <= currentWeek {
				if remaining > idealY {
					out.WriteString(lipgloss.NewStyle().Foreground(red).Render("*"))
				} else {
					out.WriteString(lipgloss.NewStyle().Foreground(green).Render("*"))
				}
			} else if row == idealRow {
				out.WriteString(DimStyle.Render("·"))
			} else {
				out.WriteString(" ")
			}
		}
		out.WriteString("\n")
	}

	// X-axis
	out.WriteString(DimStyle.Render("       └" + strings.Repeat("─", chartW)))
	out.WriteString("\n")
	endLabel := fmt.Sprintf("wk%d", totalWeeks)
	gap := chartW - 3 - len(endLabel) // 3 = len("wk1")
	if gap < 1 {
		gap = 1
	}
	axisLabel := "        wk1" + strings.Repeat(" ", gap) + endLabel
	out.WriteString(DimStyle.Render(axisLabel))
	out.WriteString("\n")

	// Pace indicator
	idealRemaining := totalMS
	if totalWeeks > 0 {
		idealRemaining = totalMS - (currentWeek * totalMS / totalWeeks)
	}
	if idealRemaining < 0 {
		idealRemaining = 0
	}
	diff := remaining - idealRemaining
	if diff <= 0 {
		out.WriteString("  " + lipgloss.NewStyle().Foreground(green).Bold(true).Render("On track"))
	} else {
		out.WriteString("  " + lipgloss.NewStyle().Foreground(red).Bold(true).
			Render(fmt.Sprintf("Behind by %d milestone", diff)))
		if diff > 1 {
			out.WriteString(lipgloss.NewStyle().Foreground(red).Bold(true).Render("s"))
		}
	}
	out.WriteString("\n")

	return out.String()
}

func (pm ProjectMode) viewDashNotes(width int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().Foreground(blue).Bold(true).
		Render(IconEditChar + "  Recent Notes")
	b.WriteString("  " + title)
	b.WriteString(DimStyle.Render(fmt.Sprintf(" (%d)", len(pm.dashNotes))))
	b.WriteString("\n")

	if len(pm.dashNotes) == 0 {
		b.WriteString("  " + DimStyle.Render("No notes in project folder"))
		b.WriteString("\n")
		return b.String()
	}

	visH := pm.height - 18
	if visH < 3 {
		visH = 3
	}
	start := pm.dashScroll
	if start >= len(pm.dashNotes) {
		start = len(pm.dashNotes) - 1
	}
	if start < 0 {
		start = 0
	}
	end := start + visH
	if end > len(pm.dashNotes) {
		end = len(pm.dashNotes)
	}

	noteIcon := lipgloss.NewStyle().Foreground(blue).Render(IconEditChar + " ")
	for i := start; i < end; i++ {
		note := pm.dashNotes[i]
		ago := pmTimeAgo(note.ModTime)
		agoStr := lipgloss.NewStyle().Foreground(overlay0).Render(ago)

		if i == pm.dashScroll {
			line := "  " + noteIcon + note.Name + "  " + agoStr
			b.WriteString(lipgloss.NewStyle().
				Background(surface0).
				Foreground(text).
				Width(width - 6).
				Render(line))
		} else {
			b.WriteString("  " + noteIcon + lipgloss.NewStyle().Foreground(text).Render(note.Name) + "  " + agoStr)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (pm ProjectMode) viewDashTasks(width int) string {
	var b strings.Builder

	doneTasks := 0
	for _, t := range pm.dashTasks {
		if t.Done {
			doneTasks++
		}
	}
	title := lipgloss.NewStyle().Foreground(yellow).Bold(true).
		Render(IconCalendarChar + "  Tasks")
	b.WriteString("  " + title)
	b.WriteString(DimStyle.Render(fmt.Sprintf(" (%d/%d done)", doneTasks, len(pm.dashTasks))))
	b.WriteString("\n")

	if len(pm.dashTasks) == 0 {
		b.WriteString("  " + DimStyle.Render("No matching tasks found"))
		b.WriteString("\n")
		return b.String()
	}

	visH := pm.height - 18
	if visH < 3 {
		visH = 3
	}
	start := pm.dashScroll
	if start >= len(pm.dashTasks) {
		start = len(pm.dashTasks) - 1
	}
	if start < 0 {
		start = 0
	}
	end := start + visH
	if end > len(pm.dashTasks) {
		end = len(pm.dashTasks)
	}

	for i := start; i < end; i++ {
		task := pm.dashTasks[i]
		var checkbox string
		if task.Done {
			checkbox = lipgloss.NewStyle().Foreground(green).Render("[x] ")
		} else {
			checkbox = lipgloss.NewStyle().Foreground(overlay0).Render("[ ] ")
		}

		taskText := TruncateDisplay(task.Text, width-16)

		textStyle := lipgloss.NewStyle().Foreground(text)
		if task.Done {
			textStyle = textStyle.Strikethrough(true).Foreground(overlay0)
		}

		b.WriteString("  " + checkbox + textStyle.Render(taskText))
		b.WriteString("\n")
	}

	return b.String()
}

func (pm ProjectMode) viewDashStats(width int) string {
	proj := pm.projects[pm.selectedProj]
	var b strings.Builder

	title := lipgloss.NewStyle().Foreground(green).Bold(true).
		Render(IconGraphChar + "  Stats")
	b.WriteString("  " + title + "\n\n")

	// Note count
	noteCount := len(pm.dashNotes)
	b.WriteString(pm.statLine("Notes", fmt.Sprintf("%d", noteCount), blue, width))

	// Task count and completion
	totalTasks := len(pm.dashTasks)
	doneTasks := 0
	for _, t := range pm.dashTasks {
		if t.Done {
			doneTasks++
		}
	}
	b.WriteString(pm.statLine("Tasks", fmt.Sprintf("%d", totalTasks), yellow, width))

	pct := 0
	if totalTasks > 0 {
		pct = doneTasks * 100 / totalTasks
	}
	b.WriteString(pm.statLine("Completion", fmt.Sprintf("%d%%", pct), green, width))

	// Progress bar
	barWidth := width - 14
	if barWidth < 10 {
		barWidth = 10
	}
	filled := barWidth * pct / 100
	bar := lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("█", filled))
	empty := lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("░", barWidth-filled))
	b.WriteString("  " + bar + empty + "\n\n")

	// Priority
	if proj.Priority > 0 && proj.Priority < len(projectPriorityLabels) {
		b.WriteString(pm.statLine("Priority", projectPriorityLabels[proj.Priority], peach, width))
	}

	// Due date
	if proj.DueDate != "" {
		b.WriteString(pm.statLine("Due Date", proj.DueDate, teal, width))
	}

	// Project health indicator
	healthLabel, healthColor := pm.computeHealth(proj)
	healthDot := lipgloss.NewStyle().Foreground(healthColor).Render("●")
	healthText := lipgloss.NewStyle().Foreground(healthColor).Bold(true).Render(healthLabel)
	b.WriteString("  " + healthDot + " " + healthText)

	// Velocity
	if proj.CreatedAt != "" {
		velocity := pm.computeVelocity(proj)
		if velocity > 0 {
			b.WriteString("  " + DimStyle.Render(fmt.Sprintf("%.1f milestones/week", velocity)))
		}
	}
	b.WriteString("\n\n")

	// Time spent
	if proj.TimeSpent > 0 {
		b.WriteString(pm.statLine("Time Spent", formatTimeSpent(proj.TimeSpent), peach, width))
	}

	// Goals progress
	if len(proj.Goals) > 0 {
		doneGoals := 0
		for _, g := range proj.Goals {
			if g.Done {
				doneGoals++
			}
		}
		b.WriteString(pm.statLine("Goals", fmt.Sprintf("%d/%d", doneGoals, len(proj.Goals)), blue, width))
	}

	// Created date
	b.WriteString(pm.statLine("Created", proj.CreatedAt, lavender, width))

	// Category
	b.WriteString(pm.statLine("Category", proj.Category, categoryColor(proj.Category), width))

	// Folder
	if proj.Folder != "" {
		b.WriteString(pm.statLine("Folder", proj.Folder, peach, width))
	}

	// Tags
	if len(proj.Tags) > 0 {
		b.WriteString(pm.statLine("Tags", strings.Join(proj.Tags, ", "), teal, width))
	}

	// Last activity
	if len(pm.dashNotes) > 0 {
		lastMod := pmTimeAgo(pm.dashNotes[0].ModTime)
		b.WriteString(pm.statLine("Last Activity", lastMod, sapphire, width))
	}

	return b.String()
}

func (pm ProjectMode) computeVelocity(proj Project) float64 {
	if proj.CreatedAt == "" {
		return 0
	}
	created, err := time.Parse("2006-01-02", proj.CreatedAt)
	if err != nil {
		return 0
	}
	weeks := time.Since(created).Hours() / (24 * 7)
	if weeks < 1 {
		weeks = 1
	}
	doneMilestones := 0
	for _, g := range proj.Goals {
		for _, m := range g.Milestones {
			if m.Done {
				doneMilestones++
			}
		}
	}
	return float64(doneMilestones) / weeks
}

func (pm ProjectMode) computeHealth(proj Project) (string, lipgloss.Color) {
	// Count overdue tasks
	overdue := 0
	total := len(pm.dashTasks)
	for _, t := range pm.dashTasks {
		if !t.Done && t.Source != "" {
			// Simple heuristic: if task text contains a date that's past
			overdue++ // count undone tasks as potentially at risk
		}
	}

	progress := proj.Progress()

	// Check pace if due date exists
	if proj.DueDate != "" && proj.CreatedAt != "" {
		due, err1 := time.Parse("2006-01-02", proj.DueDate)
		created, err2 := time.Parse("2006-01-02", proj.CreatedAt)
		if err1 == nil && err2 == nil {
			totalDuration := due.Sub(created).Hours()
			elapsed := time.Since(created).Hours()
			if totalDuration > 0 {
				expectedProgress := elapsed / totalDuration
				if expectedProgress > 1 {
					expectedProgress = 1
				}
				if progress < expectedProgress-0.2 {
					return "Behind", red
				}
				if progress < expectedProgress-0.1 {
					return "At Risk", yellow
				}
			}
		}
	}

	// Fallback: check overdue ratio
	if total > 0 {
		overdueRatio := float64(overdue) / float64(total)
		if overdueRatio > 0.5 {
			return "At Risk", yellow
		}
	}

	return "On Track", green
}

func (pm ProjectMode) statLine(label, value string, c lipgloss.Color, _ int) string {
	lbl := lipgloss.NewStyle().Foreground(subtext0).Width(16).Render("  " + label)
	val := lipgloss.NewStyle().Foreground(c).Bold(true).Render(value)
	return lbl + val + "\n"
}

func (pm ProjectMode) dashHelpBar() string {
	if pm.dashInput {
		return RenderHelpBar([]struct{ Key, Desc string }{
			{"Enter", "confirm"}, {"Esc", "cancel"},
		})
	}

	if pm.goalMode {
		if pm.goalExpanded >= 0 {
			return RenderHelpBar([]struct{ Key, Desc string }{
				{"Space", "toggle"}, {"m", "milestone"}, {"d", "delete"},
				{"j/k", "move"}, {"Esc", "back"},
			})
		}
		return RenderHelpBar([]struct{ Key, Desc string }{
			{"Enter", "expand"}, {"a", "add goal"}, {"m", "milestone"},
			{"d", "delete"}, {"j/k", "move"}, {"Esc", "back"},
		})
	}

	return RenderHelpBar([]struct{ Key, Desc string }{
		{"o", "open note"}, {"t", "tasks"}, {"x", "toggle task"}, {"N", "new note"},
		{"g", "goals"}, {"n", "next action"}, {"p", "priority"}, {"I", "AI insights"}, {"Tab", "section"}, {"Esc", "back"},
	})
}

// --- Edit form view ---

func (pm ProjectMode) viewEdit() string {
	width := pm.overlayWidth()
	var b strings.Builder

	titleText := "New Project"
	if pm.editIdx >= 0 {
		titleText = "Edit Project"
	}
	title := lipgloss.NewStyle().Foreground(mauve).Bold(true).
		Render(IconNewChar + "  " + titleText)
	b.WriteString(title + "\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	fields := []struct {
		label    string
		fieldIdx int
	}{
		{"Name", 0},
		{"Description", 1},
		{"Folder", 2},
		{"Category", 3},
		{"Tags", 4},
		{"Color", 5},
		{"Status", 6},
		{"Priority", 7},
		{"Due Date", 8},
		{"Next Action", 9},
	}

	for _, f := range fields {
		active := pm.editField == f.fieldIdx
		labelStyle := DimStyle
		if active {
			labelStyle = lipgloss.NewStyle().Foreground(mauve).Bold(true)
		}
		b.WriteString("  " + labelStyle.Render(f.label) + "\n")

		var valueStr string
		isSelector := false
		switch f.fieldIdx {
		case 0:
			valueStr = pm.editName
		case 1:
			valueStr = pm.editDesc
		case 2:
			valueStr = pm.editFolder
		case 3:
			valueStr = pm.renderSelector(projectCategories, pm.editCategory, active)
			isSelector = true
		case 4:
			valueStr = pm.editTags
		case 5:
			valueStr = pm.renderColorSelector(active)
			isSelector = true
		case 6:
			valueStr = pm.renderSelector(projectStatuses, pm.editStatus, active)
			isSelector = true
		case 7:
			valueStr = pm.renderSelector(projectPriorityLabels, pm.editPriority, active)
			isSelector = true
		case 8:
			valueStr = pm.editDueDate
		case 9:
			valueStr = pm.editNextAction
		}

		if isSelector {
			b.WriteString("    " + valueStr + "\n\n")
		} else {
			cursor := ""
			if active {
				cursor = lipgloss.NewStyle().Foreground(mauve).Render("_")
			}
			inputStyle := lipgloss.NewStyle().Foreground(text)
			if active {
				inputStyle = lipgloss.NewStyle().
					Foreground(text).
					Background(surface0)
			}
			content := valueStr
			if content == "" && !active {
				content = "(empty)"
			}
			content = TruncateDisplay(content, width-12)
			if valueStr == "" && !active {
				content = DimStyle.Render(content)
			}
			b.WriteString("    " + inputStyle.Render(content) + cursor + "\n\n")
		}
	}

	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(pm.editHelpBar())

	return pm.wrapBorder(b.String(), width, mauve)
}

func (pm ProjectMode) renderSelector(options []string, selected int, active bool) string {
	var parts []string
	for i, opt := range options {
		if i == selected {
			s := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("[" + opt + "]")
			parts = append(parts, s)
		} else {
			parts = append(parts, DimStyle.Render(opt))
		}
	}
	hint := ""
	if active {
		hint = lipgloss.NewStyle().Foreground(overlay0).Render("  \u2190/\u2192")
	}
	return strings.Join(parts, "  ") + hint
}

func (pm ProjectMode) renderColorSelector(active bool) string {
	var parts []string
	for i, name := range projectColorNames {
		c := projectAccentColor(name)
		swatch := lipgloss.NewStyle().Foreground(c).Render("●")
		if i == pm.editColor {
			label := lipgloss.NewStyle().Foreground(c).Bold(true).Render(name)
			parts = append(parts, "["+swatch+" "+label+"]")
		} else {
			parts = append(parts, swatch)
		}
	}
	hint := ""
	if active {
		hint = lipgloss.NewStyle().Foreground(overlay0).Render("  \u2190/\u2192")
	}
	return strings.Join(parts, " ") + hint
}

func (pm ProjectMode) editHelpBar() string {
	return RenderHelpBar([]struct{ Key, Desc string }{
		{"Tab", "next field"}, {"Enter", "save"}, {"←/→", "selector"}, {"Esc", "cancel"},
	})
}

// ---------------------------------------------------------------------------
// Common rendering
// ---------------------------------------------------------------------------

func (pm ProjectMode) wrapBorder(content string, width int, borderColor lipgloss.Color) string {
	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(borderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(content)
}

// pmTimeAgo returns a human-readable relative time string for project notes.
func pmTimeAgo(then time.Time) string {
	d := time.Since(then)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}
