package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

type aiPlannerResultMsg struct {
	response string
	err      error
}

type aiPlannerTickMsg struct{}

func aiPlannerTickCmd() tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
		return aiPlannerTickMsg{}
	})
}

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

type plannerState int

const (
	plannerInput      plannerState = iota
	plannerGenerating
	plannerReview
	plannerDone
)

// ---------------------------------------------------------------------------
// AIProjectPlanner overlay
// ---------------------------------------------------------------------------

// AIProjectPlanner takes a project idea/description and uses AI to break it
// down into phases, milestones, and tasks automatically.
type AIProjectPlanner struct {
	active        bool
	width, height int
	state         plannerState

	// Input
	nameInput  string
	descInput  string
	inputFocus int // 0=name, 1=desc

	// AI config
	ai        AIConfig
	vaultRoot string
	vaultTitles []string

	// Existing context for AI
	existingProjects []Project
	existingGoals    []Goal
	overdueCount     int
	dueTodayCount    int
	totalActiveTasks int

	// Generated
	generatedPlan string
	parsedProject Project
	parsedTasks   []string
	err           string

	// Review
	reviewScroll int

	// Animation
	spinnerTick int
}

// NewAIProjectPlanner creates a new AI project planner overlay.
func NewAIProjectPlanner() AIProjectPlanner {
	return AIProjectPlanner{}
}

// IsActive reports whether the overlay is currently displayed.
func (ap AIProjectPlanner) IsActive() bool { return ap.active }

// SetSize updates the available terminal dimensions.
func (ap *AIProjectPlanner) SetSize(w, h int) {
	ap.width = w
	ap.height = h
}

// Open activates the overlay.
func (ap *AIProjectPlanner) Open(vaultRoot string, vaultTitles []string,
	cfg AIConfig, projects []Project, goals []Goal) {
	ap.active = true
	ap.state = plannerInput
	ap.nameInput = ""
	ap.descInput = ""
	ap.inputFocus = 0
	ap.vaultRoot = vaultRoot
	ap.vaultTitles = vaultTitles
	ap.existingProjects = projects
	ap.existingGoals = goals
	// Calculate current workload for AI context
	ap.overdueCount = 0
	ap.dueTodayCount = 0
	ap.totalActiveTasks = 0
	for _, p := range projects {
		if p.Status == "" || p.Status == "active" {
			ap.totalActiveTasks += p.TasksTotal - p.TasksDone
		}
	}
	ap.generatedPlan = ""
	ap.parsedProject = Project{}
	ap.parsedTasks = nil
	ap.err = ""
	ap.reviewScroll = 0
	ap.spinnerTick = 0

	ap.ai = cfg
	if ap.ai.OllamaURL == "" {
		ap.ai.OllamaURL = "http://localhost:11434"
	}
	if ap.ai.NousURL == "" {
		ap.ai.NousURL = "http://localhost:3333"
	}
}

// Close deactivates the overlay.
func (ap *AIProjectPlanner) Close() { ap.active = false }

// GetCreatedProject returns the project and tasks after the user accepts.
func (ap *AIProjectPlanner) GetCreatedProject() (Project, []string, bool) {
	if ap.state != plannerDone {
		return Project{}, nil, false
	}
	return ap.parsedProject, ap.parsedTasks, true
}

// ---------------------------------------------------------------------------
// Prompt builder
// ---------------------------------------------------------------------------

func (ap AIProjectPlanner) buildPrompt() string {
	var b strings.Builder

	b.WriteString("You are a project planning assistant. Break down the following project idea into a structured plan.\n\n")
	b.WriteString(fmt.Sprintf("PROJECT NAME: %s\n", ap.nameInput))
	b.WriteString(fmt.Sprintf("DESCRIPTION: %s\n\n", ap.descInput))

	// Current workload context
	activeProjects := 0
	for _, p := range ap.existingProjects {
		if p.Status == "" || p.Status == "active" {
			activeProjects++
		}
	}
	if activeProjects > 0 || ap.totalActiveTasks > 0 {
		b.WriteString(fmt.Sprintf("Current workload: %d active projects, ~%d open tasks across all projects.\n", activeProjects, ap.totalActiveTasks))
		b.WriteString("Keep this in mind when setting realistic timelines — don't overload the user.\n\n")
	}

	// Small models get a tighter context budget.
	maxProjects := 10
	maxGoals := 10
	maxVaultTitles := 30
	if ap.ai.IsSmallModel() {
		maxProjects = 5
		maxGoals = 5
		maxVaultTitles = 10
	}

	if len(ap.existingProjects) > 0 {
		b.WriteString("Existing projects (avoid duplicates, build on these where relevant):\n")
		limit := len(ap.existingProjects)
		if limit > maxProjects {
			limit = maxProjects
		}
		for _, p := range ap.existingProjects[:limit] {
			status := p.Status
			if status == "" {
				status = "active"
			}
			b.WriteString(fmt.Sprintf("- %s [%s] %s\n", p.Name, status, p.Category))
		}
		b.WriteString("\n")
	}

	if len(ap.existingGoals) > 0 {
		b.WriteString("Existing goals (link to these where relevant):\n")
		limit := len(ap.existingGoals)
		if limit > maxGoals {
			limit = maxGoals
		}
		for _, g := range ap.existingGoals[:limit] {
			b.WriteString(fmt.Sprintf("- %s [%s] %s\n", g.Title, string(g.Status), g.Category))
		}
		b.WriteString("\n")
	}

	if len(ap.vaultTitles) > 0 {
		b.WriteString("Existing notes in the vault (for context on user's interests):\n")
		limit := len(ap.vaultTitles)
		if limit > maxVaultTitles {
			limit = maxVaultTitles
		}
		for _, t := range ap.vaultTitles[:limit] {
			b.WriteString(fmt.Sprintf("- %s\n", t))
		}
		b.WriteString("\n")
	}

	b.WriteString(`Respond in EXACTLY this format (no markdown code blocks, no extra text before or after):

PROJECT: [project name]
CATEGORY: [one of: development, social-media, personal, business, writing, research, health, finance, other]
PHASES:
  Phase 1: [name] (due: YYYY-MM-DD)
    - [ ] milestone 1
    - [ ] milestone 2
  Phase 2: [name] (due: YYYY-MM-DD)
    - [ ] milestone 1
    - [ ] milestone 2
  Phase 3: [name] (due: YYYY-MM-DD)
    - [ ] milestone 1
TASKS:
  - [ ] task description here #tag
  - [ ] another task #tag
FOLDER: [suggested-folder-name]
TAGS: tag1, tag2, tag3

Rules:
- Create 2-5 phases with realistic due dates starting from today
- Each phase should have 2-4 milestones
- Create 5-15 concrete, actionable tasks
- Suggest a kebab-case folder name (e.g., "my-web-app")
- Suggest 2-5 relevant tags (no # prefix in TAGS line)
- Tasks can optionally include priority markers and due dates
- Keep everything practical and actionable
`)

	return b.String()
}

// ---------------------------------------------------------------------------
// AI HTTP calls
// ---------------------------------------------------------------------------

func aiPlannerClaude(prompt string) tea.Cmd {
	return func() tea.Msg {
		claudePath := findClaude()
		if claudePath == "" {
			return aiPlannerResultMsg{err: fmt.Errorf("claude CLI not found - install Claude Code first")}
		}

		cmd := exec.Command(claudePath,
			"-p", prompt,
			"--output-format", "text",
		)
		cmd.Env = append(cmd.Environ(), "CLAUDECODE=")

		output, err := cmd.CombinedOutput()
		if err != nil {
			return aiPlannerResultMsg{err: fmt.Errorf("claude CLI error: %w\n%s", err, string(output))}
		}
		return aiPlannerResultMsg{response: string(output)}
	}
}

// ---------------------------------------------------------------------------
// Response parser
// ---------------------------------------------------------------------------

func (ap *AIProjectPlanner) parseAIResponse(response string) {
	ap.generatedPlan = response
	ap.parsedTasks = nil

	proj := Project{
		Status:    "active",
		CreatedAt: time.Now().Format("2006-01-02"),
		Color:     "blue",
		Priority:  2, // medium
	}

	lines := strings.Split(response, "\n")
	var currentPhase *ProjectGoal
	inPhases := false
	inTasks := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// PROJECT:
		if strings.HasPrefix(trimmed, "PROJECT:") {
			proj.Name = strings.TrimSpace(strings.TrimPrefix(trimmed, "PROJECT:"))
			inPhases = false
			inTasks = false
			continue
		}

		// CATEGORY:
		if strings.HasPrefix(trimmed, "CATEGORY:") {
			cat := strings.TrimSpace(strings.ToLower(strings.TrimPrefix(trimmed, "CATEGORY:")))
			// Validate category
			valid := false
			for _, c := range projectCategories {
				if c == cat {
					valid = true
					break
				}
			}
			if valid {
				proj.Category = cat
			} else {
				proj.Category = "other"
			}
			inPhases = false
			inTasks = false
			continue
		}

		// PHASES:
		if trimmed == "PHASES:" {
			inPhases = true
			inTasks = false
			continue
		}

		// TASKS:
		if trimmed == "TASKS:" {
			inPhases = false
			inTasks = true
			continue
		}

		// FOLDER:
		if strings.HasPrefix(trimmed, "FOLDER:") {
			proj.Folder = strings.TrimSpace(strings.TrimPrefix(trimmed, "FOLDER:"))
			inPhases = false
			inTasks = false
			continue
		}

		// TAGS:
		if strings.HasPrefix(trimmed, "TAGS:") {
			tagStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "TAGS:"))
			for _, t := range strings.Split(tagStr, ",") {
				t = strings.TrimSpace(t)
				t = strings.TrimPrefix(t, "#")
				if t != "" {
					proj.Tags = append(proj.Tags, t)
				}
			}
			inPhases = false
			inTasks = false
			continue
		}

		// Parse phase headers: "Phase N: name (due: YYYY-MM-DD)"
		if inPhases && strings.Contains(trimmed, "Phase") && strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "- [") {
			// Extract phase name
			idx := strings.Index(trimmed, ":")
			if idx >= 0 {
				rest := strings.TrimSpace(trimmed[idx+1:])
				// Remove due date
				name := rest
				if duIdx := strings.Index(rest, "(due:"); duIdx >= 0 {
					name = strings.TrimSpace(rest[:duIdx])
					// Extract due date for the phase — store as first milestone description
					duePart := rest[duIdx+5:]
					duePart = strings.TrimSuffix(strings.TrimSpace(duePart), ")")
					_ = duePart // We'll use the phase due date as DueDate if it's the last phase
					if proj.DueDate == "" || duePart > proj.DueDate {
						proj.DueDate = strings.TrimSpace(duePart)
					}
				}
				goal := ProjectGoal{Title: name}
				proj.Goals = append(proj.Goals, goal)
				currentPhase = &proj.Goals[len(proj.Goals)-1]
			}
			continue
		}

		// Parse milestones within phases: "- [ ] milestone text"
		if inPhases && currentPhase != nil && strings.HasPrefix(trimmed, "- [") {
			text := trimmed
			done := false
			if strings.HasPrefix(text, "- [x]") || strings.HasPrefix(text, "- [X]") {
				done = true
				text = strings.TrimSpace(text[5:])
			} else if strings.HasPrefix(text, "- [ ]") {
				text = strings.TrimSpace(text[5:])
			} else {
				text = strings.TrimSpace(text[2:])
			}
			currentPhase.Milestones = append(currentPhase.Milestones, ProjectMilestone{
				Text: text,
				Done: done,
			})
			// Update the actual slice element
			proj.Goals[len(proj.Goals)-1] = *currentPhase
			continue
		}

		// Parse tasks: "- [ ] task text #tag"
		if inTasks && strings.HasPrefix(trimmed, "- [") {
			text := trimmed
			if strings.HasPrefix(text, "- [x]") || strings.HasPrefix(text, "- [X]") {
				text = strings.TrimSpace(text[5:])
			} else if strings.HasPrefix(text, "- [ ]") {
				text = strings.TrimSpace(text[5:])
			} else {
				text = strings.TrimSpace(text[2:])
			}
			ap.parsedTasks = append(ap.parsedTasks, text)
			continue
		}
	}

	// Fallback: use user input if AI didn't provide
	if proj.Name == "" {
		proj.Name = ap.nameInput
	}
	if proj.Description == "" {
		proj.Description = ap.descInput
	}
	if proj.Category == "" {
		proj.Category = "other"
	}
	if proj.Folder == "" {
		proj.Folder = strings.ToLower(strings.ReplaceAll(proj.Name, " ", "-"))
	}

	ap.parsedProject = proj
}

// saveProjectAndTasks saves the project to projects.json and creates a task
// note inside the project folder.
func (ap *AIProjectPlanner) saveProjectAndTasks() error {
	// Load existing projects
	projFile := filepath.Join(ap.vaultRoot, ".granit", "projects.json")
	var projects []Project
	if data, err := os.ReadFile(projFile); err == nil {
		_ = json.Unmarshal(data, &projects)
	}

	// Append and save
	projects = append(projects, ap.parsedProject)
	dir := filepath.Dir(projFile)
	_ = os.MkdirAll(dir, 0755)
	data, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal projects: %w", err)
	}
	if err := os.WriteFile(projFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write projects: %w", err)
	}

	// Create project folder + tasks note
	folder := ap.parsedProject.Folder
	if folder != "" {
		absFolder := filepath.Join(ap.vaultRoot, folder)
		_ = os.MkdirAll(absFolder, 0755)

		// Write tasks note
		if len(ap.parsedTasks) > 0 {
			var b strings.Builder
			b.WriteString(fmt.Sprintf("# %s - Tasks\n\n", ap.parsedProject.Name))

			// Write tags
			if len(ap.parsedProject.Tags) > 0 {
				for _, t := range ap.parsedProject.Tags {
					b.WriteString(fmt.Sprintf("#%s ", t))
				}
				b.WriteString("\n\n")
			}

			// Write phases as sections
			if len(ap.parsedProject.Goals) > 0 {
				b.WriteString("## Phases\n\n")
				for i, goal := range ap.parsedProject.Goals {
					b.WriteString(fmt.Sprintf("### Phase %d: %s\n\n", i+1, goal.Title))
					for _, ms := range goal.Milestones {
						check := " "
						if ms.Done {
							check = "x"
						}
						b.WriteString(fmt.Sprintf("- [%s] %s\n", check, ms.Text))
					}
					b.WriteString("\n")
				}
			}

			// Write tasks
			b.WriteString("## Tasks\n\n")
			for _, task := range ap.parsedTasks {
				b.WriteString(fmt.Sprintf("- [ ] %s\n", task))
			}

			taskFile := filepath.Join(absFolder, "tasks.md")
			if err := os.WriteFile(taskFile, []byte(b.String()), 0644); err != nil {
				return fmt.Errorf("failed to write tasks note: %w", err)
			}
		}
	}

	// Auto-create a Goal linked to this project
	goalFile := filepath.Join(ap.vaultRoot, ".granit", "goals.json")
	var goals []Goal
	if data, err := os.ReadFile(goalFile); err == nil {
		_ = json.Unmarshal(data, &goals)
	}

	now := time.Now().Format("2006-01-02")
	var milestones []GoalMilestone
	for _, g := range ap.parsedProject.Goals {
		for _, ms := range g.Milestones {
			milestones = append(milestones, GoalMilestone{Text: g.Title + ": " + ms.Text})
		}
	}

	newGoal := Goal{
		ID:          fmt.Sprintf("goal-%d", time.Now().UnixNano()),
		Title:       ap.parsedProject.Name,
		Description: ap.parsedProject.Description,
		Status:      GoalStatusActive,
		Category:    ap.parsedProject.Category,
		Tags:        ap.parsedProject.Tags,
		Project:     ap.parsedProject.Name,
		CreatedAt:   now,
		UpdatedAt:   now,
		Milestones:  milestones,
	}
	if ap.parsedProject.DueDate != "" {
		newGoal.TargetDate = ap.parsedProject.DueDate
	}

	goals = append(goals, newGoal)
	if gdata, err := json.MarshalIndent(goals, "", "  "); err == nil {
		_ = os.WriteFile(goalFile, gdata, 0644)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (ap AIProjectPlanner) Update(msg tea.Msg) (AIProjectPlanner, tea.Cmd) {
	if !ap.active {
		return ap, nil
	}

	switch msg := msg.(type) {
	case aiPlannerTickMsg:
		if ap.state == plannerGenerating {
			ap.spinnerTick++
			return ap, aiPlannerTickCmd()
		}
		return ap, nil

	case aiPlannerResultMsg:
		if ap.state != plannerGenerating {
			return ap, nil
		}
		if msg.err != nil {
			ap.err = msg.err.Error()
			ap.state = plannerInput
			return ap, nil
		}
		ap.parseAIResponse(msg.response)
		ap.state = plannerReview
		ap.reviewScroll = 0
		return ap, nil

	case tea.KeyMsg:
		switch ap.state {
		case plannerInput:
			return ap.updateInput(msg)
		case plannerGenerating:
			if msg.String() == "esc" {
				ap.active = false
			}
			return ap, nil
		case plannerReview:
			return ap.updateReview(msg)
		case plannerDone:
			if msg.String() == "enter" || msg.String() == "esc" {
				ap.active = false
			}
			return ap, nil
		}
	}
	return ap, nil
}

func (ap AIProjectPlanner) updateInput(msg tea.KeyMsg) (AIProjectPlanner, tea.Cmd) {
	switch msg.String() {
	case "esc":
		ap.active = false
		return ap, nil

	case "tab":
		ap.inputFocus = (ap.inputFocus + 1) % 2
		return ap, nil

	case "shift+tab":
		ap.inputFocus = (ap.inputFocus + 1) % 2
		return ap, nil

	case "enter":
		if ap.nameInput == "" {
			ap.err = "Project name is required"
			return ap, nil
		}
		if ap.descInput == "" {
			ap.err = "Project description is required"
			return ap, nil
		}
		ap.err = ""
		return ap.startGeneration()

	case "backspace":
		if ap.inputFocus == 0 && len(ap.nameInput) > 0 {
			ap.nameInput = ap.nameInput[:len(ap.nameInput)-1]
		} else if ap.inputFocus == 1 && len(ap.descInput) > 0 {
			ap.descInput = ap.descInput[:len(ap.descInput)-1]
		}
		return ap, nil

	default:
		char := msg.String()
		if len(char) == 1 && char[0] >= 32 {
			if ap.inputFocus == 0 {
				ap.nameInput += char
			} else {
				ap.descInput += char
			}
		}
		return ap, nil
	}
}

func (ap AIProjectPlanner) startGeneration() (AIProjectPlanner, tea.Cmd) {
	ap.state = plannerGenerating
	ap.spinnerTick = 0
	ap.err = ""

	prompt := ap.buildPrompt()

	if ap.ai.Provider == "claude" {
		return ap, tea.Batch(
			aiPlannerClaude(prompt),
			aiPlannerTickCmd(),
		)
	}

	systemPrompt := "You are a project planning assistant. Always respond in the exact format requested."
	ai := ap.ai
	cmd := func() tea.Msg {
		resp, err := ai.Chat(systemPrompt, prompt)
		return aiPlannerResultMsg{response: resp, err: err}
	}
	return ap, tea.Batch(cmd, aiPlannerTickCmd())
}

func (ap AIProjectPlanner) updateReview(msg tea.KeyMsg) (AIProjectPlanner, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Go back to input
		ap.state = plannerInput
		return ap, nil

	case "enter", "y":
		// Accept — save project and tasks
		if err := ap.saveProjectAndTasks(); err != nil {
			ap.err = err.Error()
			return ap, nil
		}
		ap.state = plannerDone
		return ap, nil

	case "up", "k":
		if ap.reviewScroll > 0 {
			ap.reviewScroll--
		}
		return ap, nil

	case "down", "j":
		ap.reviewScroll++
		return ap, nil

	case "e":
		// Re-generate with same inputs
		return ap.startGeneration()
	}
	return ap, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (ap AIProjectPlanner) View() string {
	width := ap.width * 3 / 5
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}
	innerW := width - 6

	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render(IconBotChar+" AI Project Planner"))
	b.WriteString("\n\n")

	switch ap.state {
	case plannerInput:
		ap.viewInput(&b, innerW)
	case plannerGenerating:
		ap.viewGenerating(&b, innerW)
	case plannerReview:
		ap.viewReview(&b, innerW)
	case plannerDone:
		ap.viewDone(&b, innerW)
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width)

	return border.Render(b.String())
}

func (ap AIProjectPlanner) viewInput(b *strings.Builder, innerW int) {
	labelStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	inputBg := lipgloss.NewStyle().Background(surface0).Foreground(text).Width(innerW - 2).Padding(0, 1)
	activeBg := lipgloss.NewStyle().Background(surface1).Foreground(text).Width(innerW - 2).Padding(0, 1)
	cursor := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("|")
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)
	errStyle := lipgloss.NewStyle().Foreground(red)

	// Name field
	b.WriteString(labelStyle.Render("Project Name"))
	b.WriteString("\n")
	nameDisplay := ap.nameInput
	if ap.inputFocus == 0 {
		nameDisplay += cursor
		b.WriteString(activeBg.Render(nameDisplay))
	} else {
		b.WriteString(inputBg.Render(nameDisplay))
	}
	b.WriteString("\n\n")

	// Description field
	b.WriteString(labelStyle.Render("Description"))
	b.WriteString("\n")
	descDisplay := ap.descInput
	if ap.inputFocus == 1 {
		descDisplay += cursor
		b.WriteString(activeBg.Render(descDisplay))
	} else {
		b.WriteString(inputBg.Render(descDisplay))
	}
	b.WriteString("\n\n")

	// Error
	if ap.err != "" {
		b.WriteString(errStyle.Render(ap.err))
		b.WriteString("\n\n")
	}

	// Hints
	b.WriteString(dimStyle.Render("Tab=switch field  Enter=generate plan  Esc=close"))
}

func (ap AIProjectPlanner) viewGenerating(b *strings.Builder, _ int) {
	spinnerFrames := []string{"|", "/", "-", "\\"}
	frame := spinnerFrames[ap.spinnerTick%len(spinnerFrames)]
	spinStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)

	b.WriteString(spinStyle.Render(frame + " Generating project plan..."))
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("Project: %s", ap.nameInput)))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("Provider: %s", ap.ai.Provider)))
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("Esc=cancel"))
}

func (ap AIProjectPlanner) viewReview(b *strings.Builder, innerW int) {
	labelStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	nameStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)
	tagStyle := lipgloss.NewStyle().Foreground(teal)
	phaseStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)

	proj := ap.parsedProject

	// Project summary
	b.WriteString(nameStyle.Render(proj.Name))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(proj.Description))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("Category: %s  |  Folder: %s  |  Due: %s", proj.Category, proj.Folder, proj.DueDate)))
	b.WriteString("\n")

	// Tags
	if len(proj.Tags) > 0 {
		for _, t := range proj.Tags {
			b.WriteString(tagStyle.Render("#"+t) + " ")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Calculate visible area
	maxVisible := ap.height/2 - 12
	if maxVisible < 8 {
		maxVisible = 8
	}

	var contentLines []string

	// Phases
	if len(proj.Goals) > 0 {
		contentLines = append(contentLines, labelStyle.Render("Phases"))
		for i, goal := range proj.Goals {
			contentLines = append(contentLines, phaseStyle.Render(fmt.Sprintf("  Phase %d: %s", i+1, goal.Title)))
			for _, ms := range goal.Milestones {
				check := " "
				if ms.Done {
					check = "x"
				}
				contentLines = append(contentLines, fmt.Sprintf("    [%s] %s", check, ms.Text))
			}
		}
		contentLines = append(contentLines, "")
	}

	// Tasks
	if len(ap.parsedTasks) > 0 {
		contentLines = append(contentLines, labelStyle.Render(fmt.Sprintf("Tasks (%d)", len(ap.parsedTasks))))
		for _, task := range ap.parsedTasks {
			line := fmt.Sprintf("  [ ] %s", task)
			if len(line) > innerW {
				line = line[:innerW-1] + "…"
			}
			contentLines = append(contentLines, line)
		}
	}

	// Apply scroll
	start := ap.reviewScroll
	if start > len(contentLines) {
		start = len(contentLines)
	}
	end := start + maxVisible
	if end > len(contentLines) {
		end = len(contentLines)
	}
	for _, line := range contentLines[start:end] {
		b.WriteString(line)
		b.WriteString("\n")
	}

	if len(contentLines) > maxVisible {
		b.WriteString(dimStyle.Render(fmt.Sprintf("\n  %d/%d", start+1, len(contentLines))))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Enter/y=accept  e=regenerate  Esc=back  j/k=scroll"))
}

func (ap AIProjectPlanner) viewDone(b *strings.Builder, _ int) {
	successStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)

	b.WriteString(successStyle.Render("Project created successfully!"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("  Name:   %s\n", ap.parsedProject.Name))
	b.WriteString(fmt.Sprintf("  Folder: %s\n", ap.parsedProject.Folder))
	b.WriteString(fmt.Sprintf("  Tasks:  %d\n", len(ap.parsedTasks)))
	b.WriteString(fmt.Sprintf("  Phases: %d\n", len(ap.parsedProject.Goals)))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Enter/Esc=close"))
}
