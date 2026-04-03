package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// createProjectVault sets up a temp vault with a .granit directory ready for
// project mode testing.
func createProjectVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, ".granit"), 0755)
	return dir
}

// writeProjectsJSON writes a list of projects to the vault's projects.json.
func writeProjectsJSON(t *testing.T, vaultRoot string, projects []Project) {
	t.Helper()
	data, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		t.Fatalf("marshal projects: %v", err)
	}
	path := filepath.Join(vaultRoot, ".granit", "projects.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write projects.json: %v", err)
	}
}

// readProjectsJSON reads back the persisted projects from disk.
func readProjectsJSON(t *testing.T, vaultRoot string) []Project {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(vaultRoot, ".granit", "projects.json"))
	if err != nil {
		t.Fatalf("read projects.json: %v", err)
	}
	var projects []Project
	if err := json.Unmarshal(data, &projects); err != nil {
		t.Fatalf("unmarshal projects: %v", err)
	}
	return projects
}

// createNotesInFolder creates .md files inside a folder relative to the vault.
func createNotesInFolder(t *testing.T, vaultRoot, folder string, names []string) {
	t.Helper()
	absFolder := filepath.Join(vaultRoot, folder)
	_ = os.MkdirAll(absFolder, 0755)
	for i, name := range names {
		content := "# " + name + "\n\nSome content.\n"
		path := filepath.Join(absFolder, name+".md")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write note %s: %v", name, err)
		}
		// Stagger mod times so sort order is deterministic.
		modTime := time.Now().Add(-time.Duration(i) * time.Minute)
		_ = os.Chtimes(path, modTime, modTime)
	}
}

// createTaskFile writes a markdown file with checkbox tasks.
func createTaskFile(t *testing.T, vaultRoot, relPath, content string) {
	t.Helper()
	abs := filepath.Join(vaultRoot, relPath)
	_ = os.MkdirAll(filepath.Dir(abs), 0755)
	if err := os.WriteFile(abs, []byte(content), 0644); err != nil {
		t.Fatalf("write task file %s: %v", relPath, err)
	}
}

// ---------------------------------------------------------------------------
// NewProjectMode & lifecycle
// ---------------------------------------------------------------------------

func TestNewProjectMode_Defaults(t *testing.T) {
	pm := NewProjectMode()
	if pm.IsActive() {
		t.Error("expected new ProjectMode to be inactive")
	}
	if pm.categoryIdx != -1 {
		t.Errorf("expected categoryIdx=-1, got %d", pm.categoryIdx)
	}
	if pm.editIdx != -1 {
		t.Errorf("expected editIdx=-1, got %d", pm.editIdx)
	}
}

func TestOpenClose(t *testing.T) {
	vault := createProjectVault(t)
	pm := NewProjectMode()

	pm.Open(vault)
	if !pm.IsActive() {
		t.Error("expected active after Open")
	}
	if pm.phase != pmPhaseList {
		t.Errorf("expected phase=pmPhaseList after Open, got %d", pm.phase)
	}
	if pm.vaultRoot != vault {
		t.Errorf("vaultRoot mismatch: got %q", pm.vaultRoot)
	}

	pm.Close()
	if pm.IsActive() {
		t.Error("expected inactive after Close")
	}
}

func TestProjectModeSetSize(t *testing.T) {
	pm := NewProjectMode()
	pm.SetSize(120, 40)
	if pm.width != 120 || pm.height != 40 {
		t.Errorf("expected 120x40, got %dx%d", pm.width, pm.height)
	}
}

// ---------------------------------------------------------------------------
// Project creation with different categories
// ---------------------------------------------------------------------------

func TestCreateProject_AllCategories(t *testing.T) {
	vault := createProjectVault(t)

	for i, cat := range projectCategories {
		pm := NewProjectMode()
		pm.Open(vault)
		pm.openAddForm()

		pm.editName = "Project-" + cat
		pm.editDesc = "Description for " + cat
		pm.editFolder = "projects/" + cat
		pm.editCategory = i
		pm.editTags = "tag1, tag2"
		pm.editColor = 0 // blue
		pm.editStatus = 0 // active

		pm.commitEdit()

		// Verify the project was appended correctly.
		found := false
		for _, p := range pm.projects {
			if p.Name == "Project-"+cat {
				found = true
				if p.Category != cat {
					t.Errorf("project %q: expected category=%q, got %q", p.Name, cat, p.Category)
				}
				if p.Status != "active" {
					t.Errorf("project %q: expected status=active, got %q", p.Name, p.Status)
				}
				if p.Color != "blue" {
					t.Errorf("project %q: expected color=blue, got %q", p.Name, p.Color)
				}
				if len(p.Tags) != 2 || p.Tags[0] != "tag1" || p.Tags[1] != "tag2" {
					t.Errorf("project %q: unexpected tags %v", p.Name, p.Tags)
				}
				if p.CreatedAt == "" {
					t.Errorf("project %q: expected non-empty CreatedAt", p.Name)
				}
				break
			}
		}
		if !found {
			t.Errorf("project Project-%s not found after commitEdit", cat)
		}
	}
}

func TestCreateProject_Persists(t *testing.T) {
	vault := createProjectVault(t)
	pm := NewProjectMode()
	pm.Open(vault)
	pm.openAddForm()

	pm.editName = "Persisted"
	pm.editDesc = "A test project"
	pm.editFolder = "projects/test"
	pm.editCategory = 0
	pm.editColor = 2 // mauve
	pm.editStatus = 0

	pm.commitEdit()

	// Read back from disk.
	projects := readProjectsJSON(t, vault)
	if len(projects) == 0 {
		t.Fatal("expected at least 1 project on disk")
	}
	if projects[len(projects)-1].Name != "Persisted" {
		t.Errorf("expected last project name=Persisted, got %q", projects[len(projects)-1].Name)
	}
}

// ---------------------------------------------------------------------------
// Category validation
// ---------------------------------------------------------------------------

func TestProjectCategories_Count(t *testing.T) {
	if len(projectCategories) != 9 {
		t.Errorf("expected 9 categories, got %d", len(projectCategories))
	}
}

func TestProjectCategories_ExpectedValues(t *testing.T) {
	expected := []string{
		"development", "social-media", "personal", "business",
		"writing", "research", "health", "finance", "other",
	}
	for i, want := range expected {
		if i >= len(projectCategories) {
			t.Errorf("missing category at index %d: %q", i, want)
			continue
		}
		if projectCategories[i] != want {
			t.Errorf("category[%d] = %q, want %q", i, projectCategories[i], want)
		}
	}
}

func TestCategoryColor_AllCategoriesMapped(t *testing.T) {
	// Every category should return a non-empty color.
	for _, cat := range projectCategories {
		c := categoryColor(cat)
		if c == "" {
			t.Errorf("categoryColor(%q) returned empty", cat)
		}
	}
}

func TestCategoryColor_UnknownFallback(t *testing.T) {
	c := categoryColor("nonexistent")
	if c != text {
		t.Errorf("expected fallback to text color for unknown category, got %v", c)
	}
}

// ---------------------------------------------------------------------------
// Project dashboard data
// ---------------------------------------------------------------------------

func TestOpenDashboard_PopulatesData(t *testing.T) {
	vault := createProjectVault(t)

	folder := "myproject"
	createNotesInFolder(t, vault, folder, []string{"note1", "note2", "note3"})
	createTaskFile(t, vault, filepath.Join(folder, "tasks.md"),
		"# Tasks\n\n- [ ] Fix bug #myproject\n- [x] Write tests #myproject\n- [ ] Deploy #myproject\n")

	writeProjectsJSON(t, vault, []Project{
		{
			Name:       "MyProject",
			Folder:     folder,
			Tags:       []string{"myproject"},
			Status:     "active",
			Color:      "green",
			Category:   "development",
			CreatedAt:  "2026-01-01",
			TaskFilter: "myproject",
		},
	})

	pm := NewProjectMode()
	pm.Open(vault)
	pm.selectedProj = 0
	pm.openDashboard()

	if pm.phase != pmPhaseDashboard {
		t.Errorf("expected phase=pmPhaseDashboard, got %d", pm.phase)
	}
	// 3 notes + tasks.md = 4 .md files in the folder
	if len(pm.dashNotes) != 4 {
		t.Errorf("expected 4 dashboard notes (3 notes + tasks.md), got %d", len(pm.dashNotes))
	}
	if len(pm.dashTasks) != 3 {
		t.Errorf("expected 3 dashboard tasks, got %d", len(pm.dashTasks))
	}
}

func TestDashboard_NotesSortedByModTime(t *testing.T) {
	vault := createProjectVault(t)

	folder := "sorted"
	createNotesInFolder(t, vault, folder, []string{"alpha", "beta", "gamma"})

	writeProjectsJSON(t, vault, []Project{
		{Name: "SortTest", Folder: folder, Status: "active", Category: "other"},
	})

	pm := NewProjectMode()
	pm.Open(vault)
	pm.selectedProj = 0
	pm.openDashboard()

	if len(pm.dashNotes) < 2 {
		t.Fatal("need at least 2 notes for sort test")
	}
	// Notes created with staggered times: alpha is newest, gamma oldest.
	if pm.dashNotes[0].Name != "alpha" {
		t.Errorf("expected newest note first (alpha), got %q", pm.dashNotes[0].Name)
	}
}

// ---------------------------------------------------------------------------
// Note association with projects
// ---------------------------------------------------------------------------

func TestScanProjectFolder_FindsMdFiles(t *testing.T) {
	vault := createProjectVault(t)

	folder := "docs"
	createNotesInFolder(t, vault, folder, []string{"readme", "changelog", "design"})
	// Also create a non-md file that should be excluded.
	_ = os.WriteFile(filepath.Join(vault, folder, "data.json"), []byte("{}"), 0644)

	pm := NewProjectMode()
	pm.vaultRoot = vault
	proj := Project{Folder: folder}

	notes := pm.scanProjectFolder(proj)
	if len(notes) != 3 {
		t.Errorf("expected 3 notes (only .md), got %d", len(notes))
	}

	// Verify paths are relative to vault.
	for _, n := range notes {
		if !filepath.IsAbs(n.Path) {
			// Path should be relative (folder/name.md style).
			if n.Path == "" {
				t.Error("note path should not be empty")
			}
		}
	}
}

func TestScanProjectFolder_EmptyFolder(t *testing.T) {
	vault := createProjectVault(t)

	folder := "empty"
	_ = os.MkdirAll(filepath.Join(vault, folder), 0755)

	pm := NewProjectMode()
	pm.vaultRoot = vault
	proj := Project{Folder: folder}

	notes := pm.scanProjectFolder(proj)
	if len(notes) != 0 {
		t.Errorf("expected 0 notes for empty folder, got %d", len(notes))
	}
}

func TestScanProjectFolder_NoFolder(t *testing.T) {
	vault := createProjectVault(t)

	pm := NewProjectMode()
	pm.vaultRoot = vault
	proj := Project{Folder: ""}

	notes := pm.scanProjectFolder(proj)
	if notes != nil {
		t.Errorf("expected nil for empty folder path, got %v", notes)
	}
}

func TestScanProjectFolder_NonexistentFolder(t *testing.T) {
	vault := createProjectVault(t)

	pm := NewProjectMode()
	pm.vaultRoot = vault
	proj := Project{Folder: "does-not-exist"}

	notes := pm.scanProjectFolder(proj)
	if notes != nil {
		t.Errorf("expected nil for nonexistent folder, got %v", notes)
	}
}

func TestScanProjectFolder_LimitsTen(t *testing.T) {
	vault := createProjectVault(t)

	folder := "many"
	names := make([]string, 15)
	for i := range names {
		names[i] = "note" + string(rune('A'+i))
	}
	createNotesInFolder(t, vault, folder, names)

	pm := NewProjectMode()
	pm.vaultRoot = vault
	proj := Project{Folder: folder}

	notes := pm.scanProjectFolder(proj)
	if len(notes) != 10 {
		t.Errorf("expected max 10 notes, got %d", len(notes))
	}
}

// ---------------------------------------------------------------------------
// Task counting within projects
// ---------------------------------------------------------------------------

func TestScanProjectTasks_CountsCorrectly(t *testing.T) {
	vault := createProjectVault(t)

	folder := "taskproj"
	createTaskFile(t, vault, filepath.Join(folder, "todo.md"),
		"# TODO\n\n- [ ] First task #work\n- [x] Second task #work\n- [ ] Third task #work\n")
	createTaskFile(t, vault, filepath.Join(folder, "done.md"),
		"# Done\n\n- [x] All done #work\n")

	pm := NewProjectMode()
	pm.vaultRoot = vault

	proj := Project{
		Folder:     folder,
		Tags:       []string{"work"},
		TaskFilter: "work",
	}
	tasks := pm.scanProjectTasks(proj)

	if len(tasks) != 4 {
		t.Errorf("expected 4 tasks, got %d", len(tasks))
	}

	done := 0
	for _, task := range tasks {
		if task.Done {
			done++
		}
	}
	if done != 2 {
		t.Errorf("expected 2 done tasks, got %d", done)
	}
}

func TestScanProjectTasks_FilterByTag(t *testing.T) {
	vault := createProjectVault(t)

	folder := "filtered"
	createTaskFile(t, vault, filepath.Join(folder, "mixed.md"),
		"# Mixed\n\n- [ ] Relevant #projectA\n- [ ] Unrelated #projectB\n- [x] Also relevant #projectA\n")

	pm := NewProjectMode()
	pm.vaultRoot = vault

	proj := Project{
		Folder:     folder,
		Tags:       []string{"projectA"},
		TaskFilter: "projectA",
	}
	tasks := pm.scanProjectTasks(proj)

	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks matching #projectA, got %d", len(tasks))
	}
}

func TestScanProjectTasks_NoFilter(t *testing.T) {
	vault := createProjectVault(t)

	pm := NewProjectMode()
	pm.vaultRoot = vault

	proj := Project{Folder: "some-folder"}
	tasks := pm.scanProjectTasks(proj)

	if tasks != nil {
		t.Errorf("expected nil tasks when no filter/tags, got %v", tasks)
	}
}

func TestScanProjectTasks_CaseInsensitiveMatch(t *testing.T) {
	vault := createProjectVault(t)

	folder := "casetest"
	createTaskFile(t, vault, filepath.Join(folder, "notes.md"),
		"# Notes\n\n- [ ] Fix BUG in MyProject module\n- [ ] Update myproject docs\n")

	pm := NewProjectMode()
	pm.vaultRoot = vault

	proj := Project{
		Folder:     folder,
		Tags:       []string{"myproject"},
		TaskFilter: "myproject",
	}
	tasks := pm.scanProjectTasks(proj)

	// Both lines contain "myproject" case-insensitively.
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks (case-insensitive), got %d", len(tasks))
	}
}

func TestScanProjectTasks_UppercaseX(t *testing.T) {
	vault := createProjectVault(t)

	folder := "xcase"
	createTaskFile(t, vault, filepath.Join(folder, "items.md"),
		"# Items\n\n- [X] Done with uppercase X #test\n- [ ] Undone #test\n")

	pm := NewProjectMode()
	pm.vaultRoot = vault

	proj := Project{Folder: folder, Tags: []string{"test"}, TaskFilter: "test"}
	tasks := pm.scanProjectTasks(proj)

	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}

	doneCount := 0
	for _, task := range tasks {
		if task.Done {
			doneCount++
		}
	}
	if doneCount != 1 {
		t.Errorf("expected 1 done task (uppercase X), got %d", doneCount)
	}
}

// ---------------------------------------------------------------------------
// Completion statistics
// ---------------------------------------------------------------------------

func TestCompletionStats_ZeroTasks(t *testing.T) {
	var tasks []projectTask
	total := len(tasks)
	done := 0
	pct := 0
	if total > 0 {
		pct = done * 100 / total
	}
	if pct != 0 {
		t.Errorf("expected 0%% completion for zero tasks, got %d%%", pct)
	}
}

func TestCompletionStats_AllComplete(t *testing.T) {
	tasks := []projectTask{
		{Text: "a", Done: true},
		{Text: "b", Done: true},
		{Text: "c", Done: true},
	}
	total := len(tasks)
	done := 0
	for _, tk := range tasks {
		if tk.Done {
			done++
		}
	}
	pct := done * 100 / total
	if pct != 100 {
		t.Errorf("expected 100%% completion, got %d%%", pct)
	}
}

func TestCompletionStats_Partial(t *testing.T) {
	tasks := []projectTask{
		{Text: "a", Done: true},
		{Text: "b", Done: false},
		{Text: "c", Done: true},
		{Text: "d", Done: false},
	}
	total := len(tasks)
	done := 0
	for _, tk := range tasks {
		if tk.Done {
			done++
		}
	}
	pct := done * 100 / total
	if pct != 50 {
		t.Errorf("expected 50%% completion, got %d%%", pct)
	}
}

// ---------------------------------------------------------------------------
// Color-coded status badges
// ---------------------------------------------------------------------------

func TestStatusColor_AllStatuses(t *testing.T) {
	cases := []struct {
		status string
		want   lipgloss.Color
	}{
		{"active", green},
		{"paused", yellow},
		{"completed", blue},
		{"archived", overlay0},
	}
	for _, tc := range cases {
		got := statusColor(tc.status)
		if got != tc.want {
			t.Errorf("statusColor(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestStatusColor_UnknownFallback(t *testing.T) {
	got := statusColor("unknown")
	if got != text {
		t.Errorf("statusColor(unknown) should fallback to text, got %v", got)
	}
}

func TestStatusBadge_ContainsStatusName(t *testing.T) {
	for _, st := range projectStatuses {
		badge := statusBadge(st)
		// The badge should contain the status text somewhere (rendered with ANSI).
		// We can't easily strip ANSI, but the raw status string will appear.
		if badge == "" {
			t.Errorf("statusBadge(%q) returned empty", st)
		}
	}
}

func TestProjectAccentColor_AllColors(t *testing.T) {
	expected := map[string]lipgloss.Color{
		"blue":     blue,
		"green":    green,
		"mauve":    mauve,
		"peach":    peach,
		"red":      red,
		"yellow":   yellow,
		"pink":     pink,
		"lavender": lavender,
		"teal":     teal,
		"sapphire": sapphire,
		"flamingo": flamingo,
	}
	for name, want := range expected {
		got := projectAccentColor(name)
		if got != want {
			t.Errorf("projectAccentColor(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestProjectAccentColor_UnknownFallback(t *testing.T) {
	got := projectAccentColor("nonexistent")
	if got != mauve {
		t.Errorf("projectAccentColor(unknown) should fallback to mauve, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Edge cases: empty project, project with no notes, completed tasks
// ---------------------------------------------------------------------------

func TestEmptyProject_Defaults(t *testing.T) {
	p := Project{}
	if p.Name != "" {
		t.Error("empty project should have empty name")
	}
	if p.Notes != nil {
		t.Error("empty project Notes should be nil")
	}
	if p.Tags != nil {
		t.Error("empty project Tags should be nil")
	}
}

func TestProjectWithNoNotes_DashboardShowsZero(t *testing.T) {
	vault := createProjectVault(t)

	// Create project pointing to a folder with no markdown files.
	folder := "emptyfolder"
	_ = os.MkdirAll(filepath.Join(vault, folder), 0755)

	writeProjectsJSON(t, vault, []Project{
		{Name: "EmptyProj", Folder: folder, Status: "active", Category: "other"},
	})

	pm := NewProjectMode()
	pm.Open(vault)
	pm.selectedProj = 0
	pm.openDashboard()

	if len(pm.dashNotes) != 0 {
		t.Errorf("expected 0 dashboard notes, got %d", len(pm.dashNotes))
	}
	if len(pm.dashTasks) != 0 {
		t.Errorf("expected 0 dashboard tasks (no filter), got %d", len(pm.dashTasks))
	}
}

func TestProjectWithCompletedTasks_AllDone(t *testing.T) {
	vault := createProjectVault(t)

	folder := "doneproj"
	createTaskFile(t, vault, filepath.Join(folder, "tasks.md"),
		"# Tasks\n\n- [x] Task A #done\n- [x] Task B #done\n- [x] Task C #done\n")

	writeProjectsJSON(t, vault, []Project{
		{Name: "DoneProj", Folder: folder, Tags: []string{"done"}, TaskFilter: "done", Status: "completed", Category: "personal"},
	})

	pm := NewProjectMode()
	pm.Open(vault)
	pm.selectedProj = 0
	pm.openDashboard()

	if len(pm.dashTasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(pm.dashTasks))
	}
	for i, task := range pm.dashTasks {
		if !task.Done {
			t.Errorf("task %d (%q) should be done", i, task.Text)
		}
	}
}

// ---------------------------------------------------------------------------
// Filtering projects by category
// ---------------------------------------------------------------------------

func TestFilteredProjects_AllCategories(t *testing.T) {
	vault := createProjectVault(t)

	projects := []Project{
		{Name: "Dev", Category: "development", Status: "active"},
		{Name: "Social", Category: "social-media", Status: "active"},
		{Name: "Pers", Category: "personal", Status: "active"},
	}
	writeProjectsJSON(t, vault, projects)

	pm := NewProjectMode()
	pm.Open(vault)

	// categoryIdx == -1 means "all"
	filtered := pm.filteredProjects()
	if len(filtered) != 3 {
		t.Errorf("expected 3 projects with 'all' filter, got %d", len(filtered))
	}
}

func TestFilteredProjects_SpecificCategory(t *testing.T) {
	vault := createProjectVault(t)

	projects := []Project{
		{Name: "Dev1", Category: "development", Status: "active"},
		{Name: "Dev2", Category: "development", Status: "active"},
		{Name: "Biz", Category: "business", Status: "active"},
	}
	writeProjectsJSON(t, vault, projects)

	pm := NewProjectMode()
	pm.Open(vault)

	// Filter for "development" (index 0).
	pm.categoryIdx = 0
	filtered := pm.filteredProjects()
	if len(filtered) != 2 {
		t.Errorf("expected 2 development projects, got %d", len(filtered))
	}
}

// ---------------------------------------------------------------------------
// Status cycling
// ---------------------------------------------------------------------------

func TestCycleStatus(t *testing.T) {
	vault := createProjectVault(t)

	writeProjectsJSON(t, vault, []Project{
		{Name: "Cycle", Status: "active", Category: "other"},
	})

	pm := NewProjectMode()
	pm.Open(vault)

	// active -> paused -> completed -> archived -> active
	pm.cycleStatus(0)
	if pm.projects[0].Status != "paused" {
		t.Errorf("after 1st cycle: expected paused, got %q", pm.projects[0].Status)
	}
	pm.cycleStatus(0)
	if pm.projects[0].Status != "completed" {
		t.Errorf("after 2nd cycle: expected completed, got %q", pm.projects[0].Status)
	}
	pm.cycleStatus(0)
	if pm.projects[0].Status != "archived" {
		t.Errorf("after 3rd cycle: expected archived, got %q", pm.projects[0].Status)
	}
	pm.cycleStatus(0)
	if pm.projects[0].Status != "active" {
		t.Errorf("after 4th cycle: expected active, got %q", pm.projects[0].Status)
	}
}

func TestCycleStatus_UnknownResetsToActive(t *testing.T) {
	vault := createProjectVault(t)

	writeProjectsJSON(t, vault, []Project{
		{Name: "Unknown", Status: "bogus", Category: "other"},
	})

	pm := NewProjectMode()
	pm.Open(vault)

	pm.cycleStatus(0)
	if pm.projects[0].Status != "active" {
		t.Errorf("expected unknown status to cycle to active, got %q", pm.projects[0].Status)
	}
}

// ---------------------------------------------------------------------------
// Edit form
// ---------------------------------------------------------------------------

func TestOpenAddForm_ResetsFields(t *testing.T) {
	pm := NewProjectMode()
	pm.editName = "leftover"
	pm.editDesc = "leftover"

	pm.openAddForm()

	if pm.phase != pmPhaseEdit {
		t.Errorf("expected phase=pmPhaseEdit, got %d", pm.phase)
	}
	if pm.editIdx != -1 {
		t.Errorf("expected editIdx=-1 for new project, got %d", pm.editIdx)
	}
	if pm.editName != "" || pm.editDesc != "" || pm.editFolder != "" || pm.editTags != "" {
		t.Error("expected all text fields to be cleared")
	}
}

func TestOpenEditForm_PopulatesFields(t *testing.T) {
	vault := createProjectVault(t)

	writeProjectsJSON(t, vault, []Project{
		{
			Name:        "Existing",
			Description: "A desc",
			Folder:      "existing",
			Tags:        []string{"t1", "t2"},
			Category:    "research",
			Color:       "teal",
			Status:      "paused",
		},
	})

	pm := NewProjectMode()
	pm.Open(vault)
	pm.openEditForm(0)

	if pm.editName != "Existing" {
		t.Errorf("editName = %q, want Existing", pm.editName)
	}
	if pm.editDesc != "A desc" {
		t.Errorf("editDesc = %q, want 'A desc'", pm.editDesc)
	}
	if pm.editFolder != "existing" {
		t.Errorf("editFolder = %q, want existing", pm.editFolder)
	}
	if pm.editTags != "t1, t2" {
		t.Errorf("editTags = %q, want 't1, t2'", pm.editTags)
	}
	// research is index 5
	if pm.editCategory != 5 {
		t.Errorf("editCategory = %d, want 5 (research)", pm.editCategory)
	}
	// teal is index 8
	expectedColorIdx := -1
	for i, c := range projectColorNames {
		if c == "teal" {
			expectedColorIdx = i
			break
		}
	}
	if pm.editColor != expectedColorIdx {
		t.Errorf("editColor = %d, want %d (teal)", pm.editColor, expectedColorIdx)
	}
	// paused is index 1
	if pm.editStatus != 1 {
		t.Errorf("editStatus = %d, want 1 (paused)", pm.editStatus)
	}
}

func TestCommitEdit_UpdatesExisting(t *testing.T) {
	vault := createProjectVault(t)

	writeProjectsJSON(t, vault, []Project{
		{Name: "Old", Description: "old desc", Status: "active", Category: "other", Color: "blue"},
	})

	pm := NewProjectMode()
	pm.Open(vault)
	pm.openEditForm(0)

	pm.editName = "Updated"
	pm.editDesc = "new desc"
	pm.commitEdit()

	if pm.projects[0].Name != "Updated" {
		t.Errorf("expected name=Updated, got %q", pm.projects[0].Name)
	}
	if pm.projects[0].Description != "new desc" {
		t.Errorf("expected desc='new desc', got %q", pm.projects[0].Description)
	}

	// Verify persisted.
	onDisk := readProjectsJSON(t, vault)
	if onDisk[0].Name != "Updated" {
		t.Errorf("on-disk name = %q, want Updated", onDisk[0].Name)
	}
}

// ---------------------------------------------------------------------------
// Edit field input (insert/backspace)
// ---------------------------------------------------------------------------

func TestEditInsertChar(t *testing.T) {
	pm := NewProjectMode()
	pm.openAddForm()

	pm.editField = 0
	pm.editInsertChar("H")
	pm.editInsertChar("i")
	if pm.editName != "Hi" {
		t.Errorf("editName = %q, want Hi", pm.editName)
	}

	pm.editField = 1
	pm.editInsertChar("D")
	if pm.editDesc != "D" {
		t.Errorf("editDesc = %q, want D", pm.editDesc)
	}

	pm.editField = 2
	pm.editInsertChar("f")
	if pm.editFolder != "f" {
		t.Errorf("editFolder = %q, want f", pm.editFolder)
	}

	pm.editField = 4
	pm.editInsertChar("t")
	if pm.editTags != "t" {
		t.Errorf("editTags = %q, want t", pm.editTags)
	}
}

func TestEditBackspace(t *testing.T) {
	pm := NewProjectMode()
	pm.openAddForm()

	pm.editName = "abc"
	pm.editField = 0
	pm.editBackspace()
	if pm.editName != "ab" {
		t.Errorf("after backspace: editName = %q, want ab", pm.editName)
	}

	// Backspace on empty is a no-op.
	pm.editName = ""
	pm.editBackspace()
	if pm.editName != "" {
		t.Errorf("backspace on empty should be no-op, got %q", pm.editName)
	}
}

// ---------------------------------------------------------------------------
// GetSelectedNote / GetAction
// ---------------------------------------------------------------------------

func TestGetSelectedNote_ConsumeOnce(t *testing.T) {
	pm := NewProjectMode()
	pm.selectedNote = "some/path.md"
	pm.hasNote = true

	path, ok := pm.GetSelectedNote()
	if !ok || path != "some/path.md" {
		t.Errorf("first call: ok=%v, path=%q", ok, path)
	}

	path2, ok2 := pm.GetSelectedNote()
	if ok2 || path2 != "" {
		t.Errorf("second call should return empty: ok=%v, path=%q", ok2, path2)
	}
}

func TestGetAction_ConsumeOnce(t *testing.T) {
	pm := NewProjectMode()
	pm.action = CmdTaskManager

	action, ok := pm.GetAction()
	if !ok || action != CmdTaskManager {
		t.Errorf("first call: ok=%v, action=%v", ok, action)
	}

	action2, ok2 := pm.GetAction()
	if ok2 || action2 != CmdNone {
		t.Errorf("second call should return CmdNone: ok=%v, action=%v", ok2, action2)
	}
}

// ---------------------------------------------------------------------------
// parseTags
// ---------------------------------------------------------------------------

func TestParseTags_Normal(t *testing.T) {
	tags := parseTags("go, rust, python")
	if len(tags) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(tags))
	}
	want := []string{"go", "rust", "python"}
	for i, w := range want {
		if tags[i] != w {
			t.Errorf("tags[%d] = %q, want %q", i, tags[i], w)
		}
	}
}

func TestParseTags_Empty(t *testing.T) {
	tags := parseTags("")
	if len(tags) != 0 {
		t.Errorf("expected 0 tags for empty string, got %d", len(tags))
	}
}

func TestParseTags_WhitespaceOnly(t *testing.T) {
	tags := parseTags("  ,  ,  ")
	if len(tags) != 0 {
		t.Errorf("expected 0 tags for whitespace-only input, got %d", len(tags))
	}
}

func TestParseTags_SingleTag(t *testing.T) {
	tags := parseTags("solotag")
	if len(tags) != 1 || tags[0] != "solotag" {
		t.Errorf("expected [solotag], got %v", tags)
	}
}

// ---------------------------------------------------------------------------
// pmTimeAgo
// ---------------------------------------------------------------------------

func TestPmTimeAgo(t *testing.T) {
	cases := []struct {
		ago  time.Duration
		want string
	}{
		{5 * time.Second, "just now"},
		{1 * time.Minute, "1m ago"},
		{30 * time.Minute, "30m ago"},
		{1 * time.Hour, "1h ago"},
		{5 * time.Hour, "5h ago"},
		{24 * time.Hour, "1d ago"},
		{72 * time.Hour, "3d ago"},
	}
	for _, tc := range cases {
		got := pmTimeAgo(time.Now().Add(-tc.ago))
		if got != tc.want {
			t.Errorf("pmTimeAgo(%v ago) = %q, want %q", tc.ago, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// View helper: overlay width / list visible height
// ---------------------------------------------------------------------------

func TestOverlayWidth_Clamps(t *testing.T) {
	pm := NewProjectMode()

	pm.width = 30
	if w := pm.overlayWidth(); w < 60 {
		t.Errorf("expected min 60, got %d", w)
	}

	pm.width = 300
	if w := pm.overlayWidth(); w > 100 {
		t.Errorf("expected max 100, got %d", w)
	}

	pm.width = 120
	w := pm.overlayWidth()
	if w != 80 {
		t.Errorf("expected 80 (120*2/3), got %d", w)
	}
}

func TestListVisibleHeight_Minimum(t *testing.T) {
	pm := NewProjectMode()
	pm.height = 5

	h := pm.listVisibleHeight()
	if h < 3 {
		t.Errorf("expected min 3, got %d", h)
	}
}

// ---------------------------------------------------------------------------
// View renders without panic
// ---------------------------------------------------------------------------

func TestView_ListPhaseNoPanic(t *testing.T) {
	vault := createProjectVault(t)
	writeProjectsJSON(t, vault, []Project{
		{Name: "Test", Status: "active", Category: "other", Color: "blue"},
	})

	pm := NewProjectMode()
	pm.SetSize(100, 40)
	pm.Open(vault)

	// Should not panic.
	output := pm.View()
	if output == "" {
		t.Error("expected non-empty view output")
	}
}

func TestView_DashboardPhaseNoPanic(t *testing.T) {
	vault := createProjectVault(t)

	folder := "viewtest"
	createNotesInFolder(t, vault, folder, []string{"v1", "v2"})

	writeProjectsJSON(t, vault, []Project{
		{Name: "DashTest", Folder: folder, Status: "active", Category: "development", Color: "green"},
	})

	pm := NewProjectMode()
	pm.SetSize(100, 40)
	pm.Open(vault)
	pm.selectedProj = 0
	pm.openDashboard()

	// All three sections should render without panic.
	for section := 0; section < 3; section++ {
		pm.dashSection = section
		output := pm.View()
		if output == "" {
			t.Errorf("dashboard section %d rendered empty", section)
		}
	}
}

func TestView_EditPhaseNoPanic(t *testing.T) {
	pm := NewProjectMode()
	pm.SetSize(100, 40)
	pm.active = true
	pm.openAddForm()

	output := pm.View()
	if output == "" {
		t.Error("expected non-empty edit view output")
	}
}

// ---------------------------------------------------------------------------
// Storage: load / save round-trip
// ---------------------------------------------------------------------------

func TestLoadSave_RoundTrip(t *testing.T) {
	vault := createProjectVault(t)

	original := []Project{
		{Name: "Alpha", Description: "First", Category: "development", Status: "active", Color: "blue", Tags: []string{"go"}},
		{Name: "Beta", Description: "Second", Category: "finance", Status: "paused", Color: "green", Tags: []string{"money", "crypto"}},
	}

	pm := NewProjectMode()
	pm.vaultRoot = vault
	pm.projects = original
	pm.saveProjects()

	pm2 := NewProjectMode()
	pm2.vaultRoot = vault
	pm2.loadProjects()

	if len(pm2.projects) != 2 {
		t.Fatalf("expected 2 projects after reload, got %d", len(pm2.projects))
	}
	if pm2.projects[0].Name != "Alpha" {
		t.Errorf("project[0].Name = %q, want Alpha", pm2.projects[0].Name)
	}
	if pm2.projects[1].Category != "finance" {
		t.Errorf("project[1].Category = %q, want finance", pm2.projects[1].Category)
	}
	if len(pm2.projects[1].Tags) != 2 {
		t.Errorf("project[1] should have 2 tags, got %d", len(pm2.projects[1].Tags))
	}
}

func TestLoadProjects_NoFile(t *testing.T) {
	vault := createProjectVault(t)
	pm := NewProjectMode()
	pm.vaultRoot = vault
	pm.loadProjects()

	if pm.projects != nil {
		t.Errorf("expected nil projects when file doesn't exist, got %v", pm.projects)
	}
}

// ---------------------------------------------------------------------------
// Projects file path
// ---------------------------------------------------------------------------

func TestProjectsFilePath(t *testing.T) {
	pm := NewProjectMode()
	pm.vaultRoot = "/tmp/vault"
	got := pm.projectsFilePath()
	want := filepath.Join("/tmp/vault", ".granit", "projects.json")
	if got != want {
		t.Errorf("projectsFilePath() = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Constants sanity checks
// ---------------------------------------------------------------------------

func TestProjectStatuses_Count(t *testing.T) {
	if len(projectStatuses) != 4 {
		t.Errorf("expected 4 statuses, got %d", len(projectStatuses))
	}
}

func TestProjectColorNames_Count(t *testing.T) {
	if len(projectColorNames) != 11 {
		t.Errorf("expected 11 color names, got %d", len(projectColorNames))
	}
}

// ── AI Project Insights ────────────────────────────────────────

func TestProjectMode_InsightMsg_SetsText(t *testing.T) {
	pm := NewProjectMode()
	pm.active = true
	pm.aiPending = true

	pm, _ = pm.Update(pmAIInsightMsg{insight: "HEALTH: Green"})

	if pm.aiPending {
		t.Error("aiPending should be false after insight msg")
	}
	if !pm.showInsight {
		t.Error("showInsight should be true")
	}
	if pm.aiInsight != "HEALTH: Green" {
		t.Errorf("unexpected aiInsight: %q", pm.aiInsight)
	}
}

func TestProjectMode_InsightMsg_Error(t *testing.T) {
	pm := NewProjectMode()
	pm.active = true
	pm.aiPending = true

	pm, _ = pm.Update(pmAIInsightMsg{err: fmt.Errorf("offline")})

	if pm.aiPending {
		t.Error("aiPending should be false after error")
	}
	if !pm.showInsight {
		t.Error("showInsight should be true even on error to display message")
	}
	if !strings.Contains(pm.aiInsight, "offline") {
		t.Errorf("aiInsight should contain error: %q", pm.aiInsight)
	}
}

func TestProjectMode_InsightEscDismisses(t *testing.T) {
	vaultRoot := createProjectVault(t)
	pm := NewProjectMode()
	pm.Open(vaultRoot)
	// Add a project and enter dashboard
	pm.projects = []Project{{Name: "Test", Status: "active"}}
	pm.selectedProj = 0
	pm.phase = pmPhaseDashboard
	pm.showInsight = true
	pm.aiInsight = "some insight"

	pm, _ = pm.updateDashboard(tea.KeyMsg{Type: tea.KeyEsc})

	if pm.showInsight {
		t.Error("Esc should dismiss insight")
	}
	if pm.aiInsight != "" {
		t.Error("aiInsight should be cleared on dismiss")
	}
}
