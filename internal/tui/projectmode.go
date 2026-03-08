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

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

// Project represents a single tracked project in the vault.
type Project struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Folder      string   `json:"folder"`
	Tags        []string `json:"tags"`
	Status      string   `json:"status"`
	Color       string   `json:"color"`
	CreatedAt   string   `json:"created_at"`
	Notes       []string `json:"notes"`
	TaskFilter  string   `json:"task_filter"`
	Category    string   `json:"category"`
}

// projectTask is a parsed checkbox task relevant to a project.
type projectTask struct {
	Text   string
	Done   bool
	Source string
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
	active    bool
	width     int
	height    int
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

	// Edit form state
	editIdx       int // -1 = new project, >= 0 = editing existing
	editField     int // 0=name, 1=desc, 2=folder, 3=category, 4=tags, 5=color, 6=status
	editName      string
	editDesc      string
	editFolder    string
	editCategory  int
	editTags      string
	editColor     int
	editStatus    int

	// Consumed-once outputs
	selectedNote string
	hasNote      bool
	action       CommandAction
}

// NewProjectMode creates a new inactive ProjectMode overlay.
func NewProjectMode() ProjectMode {
	return ProjectMode{
		categoryIdx: -1,
		editIdx:     -1,
	}
}

// IsActive reports whether the project mode overlay is currently displayed.
func (pm ProjectMode) IsActive() bool {
	return pm.active
}

// SetSize updates the available terminal dimensions.
func (pm *ProjectMode) SetSize(w, h int) {
	pm.width = w
	pm.height = h
}

// Open activates the overlay and loads projects from disk.
func (pm *ProjectMode) Open(vaultRoot string) {
	pm.active = true
	pm.vaultRoot = vaultRoot
	pm.phase = pmPhaseList
	pm.cursor = 0
	pm.scroll = 0
	pm.categoryIdx = -1
	pm.selectedNote = ""
	pm.hasNote = false
	pm.action = CmdNone
	pm.loadProjects()
}

// Close deactivates the overlay.
func (pm *ProjectMode) Close() {
	pm.active = false
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

// ---------------------------------------------------------------------------
// Storage
// ---------------------------------------------------------------------------

func (pm *ProjectMode) projectsFilePath() string {
	return filepath.Join(pm.vaultRoot, ".granit", "projects.json")
}

func (pm *ProjectMode) loadProjects() {
	pm.projects = nil
	data, err := os.ReadFile(pm.projectsFilePath())
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &pm.projects)
}

func (pm *ProjectMode) saveProjects() {
	dir := filepath.Dir(pm.projectsFilePath())
	_ = os.MkdirAll(dir, 0755)
	data, err := json.MarshalIndent(pm.projects, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(pm.projectsFilePath(), data, 0644)
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
	tasksFile := filepath.Join(pm.vaultRoot, "Tasks.md")
	pm.scanTasksInFile(tasksFile, filter, &tasks)

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
	for _, line := range lines {
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
			Text:   taskText,
			Done:   done,
			Source: relPath,
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

// ---------------------------------------------------------------------------
// Filtering
// ---------------------------------------------------------------------------

func (pm *ProjectMode) filteredProjects() []int {
	var indices []int
	for i, p := range pm.projects {
		if p.Status == "archived" && pm.categoryIdx != -1 {
			// When filtering by category, skip archived unless specifically selected.
		}
		if pm.categoryIdx == -1 {
			indices = append(indices, i)
		} else if pm.categoryIdx < len(projectCategories) && p.Category == projectCategories[pm.categoryIdx] {
			indices = append(indices, i)
		}
	}
	return indices
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
	switch msg.String() {
	case "esc":
		pm.phase = pmPhaseList
		pm.dashSection = 0
		pm.dashScroll = 0
	case "tab":
		pm.dashSection = (pm.dashSection + 1) % 3
		pm.dashScroll = 0
	case "up", "k":
		if pm.dashScroll > 0 {
			pm.dashScroll--
		}
	case "down", "j":
		pm.dashScroll++
	case "o":
		// Open selected note from the notes section.
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
		// Create new note in project folder.
		proj := pm.projects[pm.selectedProj]
		if proj.Folder != "" {
			newPath := filepath.Join(proj.Folder, fmt.Sprintf("Untitled %s.md", time.Now().Format("2006-01-02 15-04")))
			absPath := filepath.Join(pm.vaultRoot, newPath)
			dir := filepath.Dir(absPath)
			_ = os.MkdirAll(dir, 0755)
			_ = os.WriteFile(absPath, []byte("# New Note\n\n"), 0644)
			pm.selectedNote = newPath
			pm.hasNote = true
			pm.active = false
		}
	}
	return pm, nil
}

func (pm ProjectMode) updateEdit(msg tea.KeyMsg) (ProjectMode, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		pm.phase = pmPhaseList
	case "tab":
		pm.editField = (pm.editField + 1) % 7
	case "shift+tab":
		pm.editField--
		if pm.editField < 0 {
			pm.editField = 6
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
		}
	case "right":
		switch pm.editField {
		case 3: // category
			pm.editCategory = (pm.editCategory + 1) % len(projectCategories)
		case 5: // color
			pm.editColor = (pm.editColor + 1) % len(projectColorNames)
		case 6: // status
			pm.editStatus = (pm.editStatus + 1) % len(projectStatuses)
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
			pm.editName = pm.editName[:len(pm.editName)-1]
		}
	case 1:
		if len(pm.editDesc) > 0 {
			pm.editDesc = pm.editDesc[:len(pm.editDesc)-1]
		}
	case 2:
		if len(pm.editFolder) > 0 {
			pm.editFolder = pm.editFolder[:len(pm.editFolder)-1]
		}
	case 4:
		if len(pm.editTags) > 0 {
			pm.editTags = pm.editTags[:len(pm.editTags)-1]
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
}

func (pm *ProjectMode) openDashboard() {
	pm.phase = pmPhaseDashboard
	pm.dashSection = 0
	pm.dashScroll = 0
	proj := pm.projects[pm.selectedProj]
	pm.dashNotes = pm.scanProjectFolder(proj)
	pm.dashTasks = pm.scanProjectTasks(proj)
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
	h := pm.height - 12
	if h < 3 {
		h = 3
	}
	return h
}

func (pm ProjectMode) overlayWidth() int {
	w := pm.width * 2 / 3
	if w < 60 {
		w = 60
	}
	if w > 100 {
		w = 100
	}
	return w
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

			// Status icon
			stIcon := lipgloss.NewStyle().Foreground(statusColor(proj.Status)).Render("● ")

			// Name
			nameStyle := lipgloss.NewStyle().Foreground(projectAccentColor(proj.Color)).Bold(true)
			name := nameStyle.Render(proj.Name)

			// Category badge
			catBadge := lipgloss.NewStyle().
				Foreground(categoryColor(proj.Category)).
				Render(" [" + proj.Category + "]")

			// Note count
			noteCount := lipgloss.NewStyle().Foreground(subtext0).
				Render(fmt.Sprintf(" %d notes", len(proj.Notes)))

			// Task count (inline scan for display)
			taskCount := ""
			if proj.TaskFilter != "" || len(proj.Tags) > 0 {
				taskCount = lipgloss.NewStyle().Foreground(subtext0).
					Render(fmt.Sprintf(" %s tasks", IconCalendarChar))
			}

			line := "  " + stIcon + name + catBadge + noteCount + taskCount

			if vi == pm.cursor {
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Width(width - 6).
					Render(line))
			} else {
				b.WriteString(line)
			}

			// Description on next line if present
			if proj.Description != "" {
				desc := proj.Description
				maxDesc := width - 12
				if len(desc) > maxDesc {
					desc = desc[:maxDesc-3] + "..."
				}
				b.WriteString("\n    " + DimStyle.Render(desc))
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
	keys := []struct {
		key  string
		desc string
	}{
		{"Enter", "open"},
		{"a", "add"},
		{"e", "edit"},
		{"d", "archive"},
		{"Tab", "filter"},
		{"Space", "status"},
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

// --- Dashboard view ---

func (pm ProjectMode) viewDashboard() string {
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

	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

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

	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(pm.dashHelpBar())

	return pm.wrapBorder(b.String(), width, accent)
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

	title := lipgloss.NewStyle().Foreground(yellow).Bold(true).
		Render(IconCalendarChar + "  Tasks")
	b.WriteString("  " + title)
	b.WriteString(DimStyle.Render(fmt.Sprintf(" (%d)", len(pm.dashTasks))))
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

		taskText := task.Text
		maxLen := width - 16
		if len(taskText) > maxLen {
			taskText = taskText[:maxLen-3] + "..."
		}

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

func (pm ProjectMode) statLine(label, value string, c lipgloss.Color, _ int) string {
	lbl := lipgloss.NewStyle().Foreground(subtext0).Width(16).Render("  " + label)
	val := lipgloss.NewStyle().Foreground(c).Bold(true).Render(value)
	return lbl + val + "\n"
}

func (pm ProjectMode) dashHelpBar() string {
	keys := []struct {
		key  string
		desc string
	}{
		{"o", "open note"},
		{"t", "tasks"},
		{"n", "new note"},
		{"Tab", "section"},
		{"j/k", "scroll"},
		{"Esc", "back"},
	}

	var parts []string
	for _, k := range keys {
		kk := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render(k.key)
		dd := DimStyle.Render(":" + k.desc)
		parts = append(parts, kk+dd)
	}
	return "  " + strings.Join(parts, "  ")
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
	}

	for _, f := range fields {
		active := pm.editField == f.fieldIdx
		labelStyle := DimStyle
		if active {
			labelStyle = lipgloss.NewStyle().Foreground(mauve).Bold(true)
		}
		b.WriteString("  " + labelStyle.Render(f.label) + "\n")

		var valueStr string
		switch f.fieldIdx {
		case 0:
			valueStr = pm.editName
		case 1:
			valueStr = pm.editDesc
		case 2:
			valueStr = pm.editFolder
		case 3:
			valueStr = pm.renderSelector(projectCategories, pm.editCategory, active)
		case 4:
			valueStr = pm.editTags
		case 5:
			valueStr = pm.renderColorSelector(active)
		case 6:
			valueStr = pm.renderSelector(projectStatuses, pm.editStatus, active)
		}

		if f.fieldIdx == 3 || f.fieldIdx == 5 || f.fieldIdx == 6 {
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
				content = DimStyle.Render("(empty)")
			}
			maxW := width - 12
			if len(content) > maxW {
				content = content[:maxW]
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
	keys := []struct {
		key  string
		desc string
	}{
		{"Tab", "next field"},
		{"Enter", "save"},
		{"\u2190/\u2192", "selector"},
		{"Esc", "cancel"},
	}
	var parts []string
	for _, k := range keys {
		kk := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render(k.key)
		dd := DimStyle.Render(":" + k.desc)
		parts = append(parts, kk+dd)
	}
	return "  " + strings.Join(parts, "  ")
}

// ---------------------------------------------------------------------------
// Common rendering
// ---------------------------------------------------------------------------

func (pm ProjectMode) wrapBorder(content string, width int, borderColor lipgloss.Color) string {
	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
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
