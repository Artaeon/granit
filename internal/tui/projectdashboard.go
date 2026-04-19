package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/vault"
)

// ---------------------------------------------------------------------------
// ProjectDashboard — cross-project overview overlay
// ---------------------------------------------------------------------------

// ProjectDashboard shows ALL projects at a glance with progress, blockers,
// next actions, and deadlines grouped by status.
type ProjectDashboard struct {
	OverlayBase
	vaultRoot string
	projects  []Project
	allTasks  []Task
	cursor    int
	scroll    int

	// Computed groups
	activeProjects    []Project
	pausedProjects    []Project
	completedProjects []Project

	// Consumed-once outputs
	selectedProject string
}

// NewProjectDashboard creates a new inactive ProjectDashboard.
func NewProjectDashboard() ProjectDashboard {
	return ProjectDashboard{}
}

// SelectedProject returns the chosen project name (consumed once).
func (pd *ProjectDashboard) SelectedProject() string {
	if pd.selectedProject != "" {
		p := pd.selectedProject
		pd.selectedProject = ""
		return p
	}
	return ""
}

// ---------------------------------------------------------------------------
// Data loading
// ---------------------------------------------------------------------------

// Open activates the overlay and loads all project/task data.
func (pd *ProjectDashboard) Open(vaultRoot string, v *vault.Vault) {
	pd.active = true
	pd.vaultRoot = vaultRoot
	pd.cursor = 0
	pd.scroll = 0
	pd.selectedProject = ""

	// Load projects from .granit/projects.json.
	pd.projects = LoadProjects(vaultRoot)

	// Parse all tasks from vault notes.
	pd.allTasks = ParseAllTasks(v.Notes)

	// Match tasks to projects and compute counts.
	MatchTasksToProjects(pd.allTasks, pd.projects)
	for i := range pd.projects {
		pd.projects[i].ComputeTaskCounts(pd.allTasks)
	}

	// Group by status.
	pd.groupProjects()
}

// groupProjects splits projects into active/paused/completed and sorts them.
func (pd *ProjectDashboard) groupProjects() {
	pd.activeProjects = nil
	pd.pausedProjects = nil
	pd.completedProjects = nil

	now := time.Now()

	for _, p := range pd.projects {
		switch p.Status {
		case "active":
			pd.activeProjects = append(pd.activeProjects, p)
		case "paused":
			pd.pausedProjects = append(pd.pausedProjects, p)
		case "completed":
			pd.completedProjects = append(pd.completedProjects, p)
		// archived projects are not shown
		}
	}

	// Sort active projects: those with deadlines come first (nearest first),
	// then those without deadlines.
	sort.Slice(pd.activeProjects, func(i, j int) bool {
		di := pd.activeProjects[i].DueDate
		dj := pd.activeProjects[j].DueDate
		if di != "" && dj != "" {
			return di < dj
		}
		if di != "" {
			return true
		}
		if dj != "" {
			return false
		}
		// Both no deadline — sort by priority descending.
		return pd.activeProjects[i].Priority > pd.activeProjects[j].Priority
	})

	// Sort completed by completion (newest first — use CreatedAt as proxy).
	sort.Slice(pd.completedProjects, func(i, j int) bool {
		return pd.completedProjects[i].CreatedAt > pd.completedProjects[j].CreatedAt
	})

	_ = now
}

// totalItems returns the count of all displayable lines for scrolling.
func (pd *ProjectDashboard) totalItems() int {
	return len(pd.activeProjects) + len(pd.pausedProjects) + len(pd.completedProjects)
}

// projectAtCursor returns the project at the current cursor position.
func (pd *ProjectDashboard) projectAtCursor() *Project {
	idx := pd.cursor
	if idx < len(pd.activeProjects) {
		return &pd.activeProjects[idx]
	}
	idx -= len(pd.activeProjects)
	if idx < len(pd.pausedProjects) {
		return &pd.pausedProjects[idx]
	}
	idx -= len(pd.pausedProjects)
	if idx < len(pd.completedProjects) {
		return &pd.completedProjects[idx]
	}
	return nil
}

// ---------------------------------------------------------------------------
// Health assessment
// ---------------------------------------------------------------------------

// projectHealth returns "green", "yellow", or "red" for a project.
func (pd *ProjectDashboard) projectHealth(p Project) string {
	now := time.Now()

	// Count overdue tasks for this project.
	overdue := pd.overdueCount(p)

	// Check deadline proximity.
	if p.DueDate != "" {
		if due, err := time.Parse("2006-01-02", p.DueDate); err == nil {
			daysLeft := int(math.Ceil(due.Sub(now).Hours() / 24))
			if daysLeft < 0 || overdue > 2 {
				return "red"
			}
			if daysLeft <= 7 {
				return "red"
			}
			// Check if time vs progress is concerning.
			if p.CreatedAt != "" {
				if created, err2 := time.Parse("2006-01-02", p.CreatedAt[:10]); err2 == nil {
					totalDays := due.Sub(created).Hours() / 24
					elapsedDays := now.Sub(created).Hours() / 24
					if totalDays > 0 {
						timePct := elapsedDays / totalDays
						progress := p.Progress()
						if timePct > 0.5 && progress < 0.5 {
							return "yellow"
						}
					}
				}
			}
		}
	}

	if overdue > 0 {
		return "yellow"
	}

	return "green"
}

// overdueCount returns the number of overdue tasks for a project.
func (pd *ProjectDashboard) overdueCount(p Project) int {
	today := time.Now().Format("2006-01-02")
	count := 0
	for _, t := range pd.allTasks {
		if t.Project != p.Name || t.Done {
			continue
		}
		if t.DueDate != "" && t.DueDate < today {
			count++
		}
	}
	return count
}

// nextActionForProject returns the next undone task text for a project.
func (pd *ProjectDashboard) nextActionForProject(p Project) string {
	// Prefer the project's explicit NextAction field.
	if p.NextAction != "" {
		return p.NextAction
	}
	// Fall back to highest-priority undone task.
	for _, t := range pd.allTasks {
		if t.Project == p.Name && !t.Done {
			return t.Text
		}
	}
	return ""
}

// goalsProgress returns (done, total) for a project's goals.
func (pd *ProjectDashboard) goalsProgress(p Project) (int, int) {
	done := 0
	for _, g := range p.Goals {
		if g.Done {
			done++
		}
	}
	return done, len(p.Goals)
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles keyboard input for the project dashboard overlay.
func (pd ProjectDashboard) Update(msg tea.Msg) (ProjectDashboard, tea.Cmd) {
	if !pd.active {
		return pd, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "esc", "q":
			pd.active = false
			return pd, nil

		case "up", "k":
			if pd.cursor > 0 {
				pd.cursor--
			}

		case "down", "j":
			max := pd.totalItems() - 1
			if max < 0 {
				max = 0
			}
			if pd.cursor < max {
				pd.cursor++
			}

		case "enter":
			if proj := pd.projectAtCursor(); proj != nil {
				pd.selectedProject = proj.Name
				pd.active = false
			}
			return pd, nil
		}
	}

	return pd, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the project dashboard overlay.
func (pd ProjectDashboard) View() string {
	width := pd.width * 3 / 4
	if width < 65 {
		width = 65
	}
	if width > 110 {
		width = 110
	}
	innerW := width - 6

	var b strings.Builder

	// Title
	title := lipgloss.NewStyle().Foreground(mauve).Bold(true).
		Render("  " + IconFolderChar + " PROJECT DASHBOARD")
	b.WriteString(title)

	// Summary stats
	completedCount := len(pd.completedProjects)
	totalCount := len(pd.activeProjects) + len(pd.pausedProjects) + len(pd.completedProjects)
	summaryStr := fmt.Sprintf("Completed: %d/%d", completedCount, totalCount)
	summaryW := lipgloss.Width(summaryStr) + lipgloss.Width(title)
	gap := innerW - summaryW
	if gap < 2 {
		gap = 2
	}
	b.WriteString(strings.Repeat(" ", gap))
	b.WriteString(DimStyle.Render(summaryStr))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n\n")

	// Track which line we're on for cursor highlighting.
	lineIdx := 0

	// Calculate visible window.
	maxVisible := pd.height - 12
	if maxVisible < 10 {
		maxVisible = 10
	}

	// Adjust scroll to keep cursor visible.
	if pd.cursor < pd.scroll {
		pd.scroll = pd.cursor
	}
	if pd.cursor >= pd.scroll+maxVisible {
		pd.scroll = pd.cursor - maxVisible + 1
	}

	var lines []string

	// ── Active Projects ──
	if len(pd.activeProjects) > 0 {
		header := lipgloss.NewStyle().Foreground(lavender).Bold(true).
			Render(fmt.Sprintf("  \u25b6 Active Projects (%d)", len(pd.activeProjects)))
		lines = append(lines, header)
		lines = append(lines, "  "+DimStyle.Render(strings.Repeat("\u2550", innerW-4)))

		for i, p := range pd.activeProjects {
			card := pd.renderProjectCard(p, innerW, lineIdx == pd.cursor)
			lines = append(lines, strings.Split(card, "\n")...)
			if i < len(pd.activeProjects)-1 {
				lines = append(lines, "")
			}
			lineIdx++
		}
		lines = append(lines, "")
	}

	// ── Paused Projects ──
	if len(pd.pausedProjects) > 0 {
		lines = append(lines, "  "+DimStyle.Render(strings.Repeat("\u2500", 4)+" Paused ("+fmt.Sprintf("%d", len(pd.pausedProjects))+") "+strings.Repeat("\u2500", 4)))
		for _, p := range pd.pausedProjects {
			selected := lineIdx == pd.cursor
			line := pd.renderPausedProject(p, innerW, selected)
			lines = append(lines, line)
			lineIdx++
		}
		lines = append(lines, "")
	}

	// ── Completed Projects ──
	if len(pd.completedProjects) > 0 {
		lines = append(lines, "  "+DimStyle.Render(strings.Repeat("\u2500", 4)+" Completed ("+fmt.Sprintf("%d", len(pd.completedProjects))+") "+strings.Repeat("\u2500", 4)))
		for _, p := range pd.completedProjects {
			selected := lineIdx == pd.cursor
			line := pd.renderCompletedProject(p, innerW, selected)
			lines = append(lines, line)
			lineIdx++
		}
	}

	// Empty state
	if len(lines) == 0 {
		lines = append(lines, DimStyle.Render("  No projects yet. Create one from Projects (Cmd Palette)."))
	}

	// Apply scrolling.
	for _, l := range lines {
		b.WriteString(l)
		b.WriteString("\n")
	}

	// Bottom stats bar
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")

	totalTasks := 0
	doneTasks := 0
	for _, t := range pd.allTasks {
		if t.Project != "" {
			totalTasks++
			if t.Done {
				doneTasks++
			}
		}
	}
	completionRate := 0
	if totalTasks > 0 {
		completionRate = doneTasks * 100 / totalTasks
	}
	stats := fmt.Sprintf("  Active: %d  |  Tasks: %d/%d (%d%% done)",
		len(pd.activeProjects), doneTasks, totalTasks, completionRate)
	b.WriteString(DimStyle.Render(stats))
	b.WriteString("\n\n")

	// Help bar
	b.WriteString(pd.renderHelp())

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// renderProjectCard renders a full active project card.
func (pd *ProjectDashboard) renderProjectCard(p Project, innerW int, selected bool) string {
	var b strings.Builder

	health := pd.projectHealth(p)
	healthDot := pd.healthDot(health)

	// Name + due date line
	accent := projectAccentColor(p.Color)
	nameStyle := lipgloss.NewStyle().Foreground(accent).Bold(true)
	name := p.Name
	maxNameW := innerW - 30
	if maxNameW < 20 {
		maxNameW = 20
	}
	if r := []rune(name); len(r) > maxNameW {
		name = string(r[:maxNameW-3]) + "..."
	}

	dueStr := ""
	if p.DueDate != "" {
		dueLabel := pd.formatDueDate(p.DueDate)
		dueColor := pd.dueDateColor(p.DueDate)
		dueStr = "  " + DimStyle.Render("Due: ") +
			lipgloss.NewStyle().Foreground(dueColor).Render(dueLabel)
	} else {
		dueStr = "  " + DimStyle.Render("No deadline")
	}

	nameLine := "  " + healthDot + " " + nameStyle.Render(name) + dueStr
	if selected {
		nameLine = lipgloss.NewStyle().Background(surface0).Width(innerW).Render(nameLine)
	}
	b.WriteString(nameLine)
	b.WriteString("\n")

	// Progress bar + task/goal counts
	progress := p.Progress()
	pctInt := int(progress * 100)
	bar := pd.progressBar(progress, 14)
	pctStr := lipgloss.NewStyle().Foreground(peach).Bold(true).Render(fmt.Sprintf("%d%%", pctInt))

	taskStr := ""
	if p.TasksTotal > 0 {
		taskStr = fmt.Sprintf("  Tasks: %d/%d", p.TasksDone, p.TasksTotal)
	}

	goalStr := ""
	goalsDone, goalsTotal := pd.goalsProgress(p)
	if goalsTotal > 0 {
		goalStr = fmt.Sprintf("  Goals: %d/%d", goalsDone, goalsTotal)
	}

	progressLine := "     " + bar + " " + pctStr +
		DimStyle.Render(taskStr) + DimStyle.Render(goalStr)
	b.WriteString(progressLine)
	b.WriteString("\n")

	// Next action
	nextAct := pd.nextActionForProject(p)
	if nextAct != "" {
		maxActW := innerW - 12
		if maxActW < 20 {
			maxActW = 20
		}
		if r := []rune(nextAct); len(r) > maxActW {
			nextAct = string(r[:maxActW-3]) + "..."
		}
		// Clean up task markdown prefixes.
		nextAct = strings.TrimPrefix(nextAct, "- [ ] ")
		nextAct = strings.TrimPrefix(nextAct, "- [x] ")
		b.WriteString("     " + DimStyle.Render("Next: ") +
			lipgloss.NewStyle().Foreground(text).Render(nextAct))
		b.WriteString("\n")
	}

	// Warnings
	overdue := pd.overdueCount(p)
	if overdue > 0 {
		warning := fmt.Sprintf("     \u26a0 %d overdue task", overdue)
		if overdue > 1 {
			warning += "s"
		}
		b.WriteString(lipgloss.NewStyle().Foreground(yellow).Render(warning))
		b.WriteString("\n")
	}

	if p.DueDate != "" {
		if due, err := time.Parse("2006-01-02", p.DueDate); err == nil {
			daysLeft := int(math.Ceil(time.Until(due).Hours() / 24))
			if daysLeft >= 0 && daysLeft <= 7 && daysLeft > 0 {
				urgency := fmt.Sprintf("     \u26a0 Due in %d day", daysLeft)
				if daysLeft > 1 {
					urgency += "s"
				}
				urgency += "!"
				b.WriteString(lipgloss.NewStyle().Foreground(red).Render(urgency))
				b.WriteString("\n")
			} else if daysLeft < 0 {
				b.WriteString(lipgloss.NewStyle().Foreground(red).Bold(true).Render("     \u26a0 OVERDUE!"))
				b.WriteString("\n")
			}
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// renderPausedProject renders a compact paused project line.
func (pd *ProjectDashboard) renderPausedProject(p Project, innerW int, selected bool) string {
	icon := lipgloss.NewStyle().Foreground(yellow).Render("\u23f8")
	name := p.Name
	if r := []rune(name); len(r) > 30 {
		name = string(r[:27]) + "..."
	}
	nameStyle := lipgloss.NewStyle().Foreground(overlay1)
	line := "  " + icon + " " + nameStyle.Render(name) + "  " + DimStyle.Render("Paused")
	if selected {
		line = lipgloss.NewStyle().Background(surface0).Width(innerW).Render(line)
	}
	return line
}

// renderCompletedProject renders a compact completed project line.
func (pd *ProjectDashboard) renderCompletedProject(p Project, innerW int, selected bool) string {
	icon := lipgloss.NewStyle().Foreground(green).Render("\u2713")
	name := p.Name
	if r := []rune(name); len(r) > 30 {
		name = string(r[:27]) + "..."
	}
	nameStyle := lipgloss.NewStyle().Foreground(overlay1).Strikethrough(true)
	dateStr := ""
	if p.CreatedAt != "" {
		if t, err := time.Parse("2006-01-02", p.CreatedAt[:10]); err == nil {
			dateStr = "  " + DimStyle.Render("Completed "+t.Format("Jan 2"))
		}
	}
	line := "  " + icon + " " + nameStyle.Render(name) + dateStr
	if selected {
		line = lipgloss.NewStyle().Background(surface0).Width(innerW).Render(line)
	}
	return line
}

// ---------------------------------------------------------------------------
// View helpers
// ---------------------------------------------------------------------------

// healthDot returns a colored status dot based on project health.
func (pd *ProjectDashboard) healthDot(health string) string {
	switch health {
	case "red":
		return lipgloss.NewStyle().Foreground(red).Render("\u25cf")
	case "yellow":
		return lipgloss.NewStyle().Foreground(yellow).Render("\u25cf")
	default:
		return lipgloss.NewStyle().Foreground(green).Render("\u25cf")
	}
}

// progressBar renders a colored progress bar of the given width.
func (pd *ProjectDashboard) progressBar(progress float64, barWidth int) string {
	filled := int(float64(barWidth) * progress)
	if filled > barWidth {
		filled = barWidth
	}
	empty := barWidth - filled
	return lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("\u2588", filled)) +
		lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("\u2591", empty))
}

// formatDueDate returns a human-readable due date label.
func (pd *ProjectDashboard) formatDueDate(dueDate string) string {
	today := time.Now().Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	switch {
	case dueDate < today:
		return "overdue"
	case dueDate == today:
		return "today"
	case dueDate == tomorrow:
		return "tomorrow"
	default:
		if t, err := time.Parse("2006-01-02", dueDate); err == nil {
			return t.Format("Jan 2")
		}
		return dueDate
	}
}

// dueDateColor returns a color for due date urgency.
func (pd *ProjectDashboard) dueDateColor(dueDate string) lipgloss.Color {
	today := time.Now().Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	switch {
	case dueDate < today:
		return red
	case dueDate == today:
		return peach
	case dueDate == tomorrow:
		return yellow
	default:
		return subtext0
	}
}

// renderHelp renders the footer help bar.
func (pd ProjectDashboard) renderHelp() string {
	keys := []struct {
		key  string
		desc string
	}{
		{"j/k", "scroll"},
		{"Enter", "open project"},
		{"Esc", "close"},
	}

	var parts []string
	for _, k := range keys {
		kk := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render(k.key)
		dd := DimStyle.Render(":" + k.desc)
		parts = append(parts, kk+dd)
	}
	return "  " + strings.Join(parts, "  ")
}
