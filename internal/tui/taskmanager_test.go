package tui

import (
	"testing"

	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/vault"
)

// ---------------------------------------------------------------------------
// MatchTasksToProjects
// ---------------------------------------------------------------------------

func TestMatchTasksToProjects_FolderPrefix(t *testing.T) {
	projects := []Project{
		{Name: "Work", Folder: "projects/work/"},
	}

	tasks := []Task{
		{Text: "fix bug", NotePath: "projects/work/sprint.md"},
		{Text: "buy milk", NotePath: "personal/shopping.md"},
	}

	MatchTasksToProjects(tasks, projects)

	if tasks[0].Project != "Work" {
		t.Errorf("expected task[0].Project = %q, got %q", "Work", tasks[0].Project)
	}
	if tasks[1].Project != "" {
		t.Errorf("expected task[1].Project = %q, got %q", "", tasks[1].Project)
	}
}

func TestMatchTasksToProjects_TaskFilter(t *testing.T) {
	projects := []Project{
		{Name: "Fitness", TaskFilter: "gym"},
	}

	tasks := []Task{
		{Text: "do squats", Tags: []string{"gym", "legs"}},
		{Text: "read book", Tags: []string{"reading"}},
	}

	MatchTasksToProjects(tasks, projects)

	if tasks[0].Project != "Fitness" {
		t.Errorf("expected task[0].Project = %q, got %q", "Fitness", tasks[0].Project)
	}
	if tasks[1].Project != "" {
		t.Errorf("expected task[1].Project = %q, got %q", "", tasks[1].Project)
	}
}

func TestMatchTasksToProjects_ProjectTags(t *testing.T) {
	projects := []Project{
		{Name: "Blog", Tags: []string{"writing", "blog"}},
	}

	tasks := []Task{
		{Text: "draft post", Tags: []string{"blog"}},
		{Text: "cook dinner", Tags: []string{"cooking"}},
	}

	MatchTasksToProjects(tasks, projects)

	if tasks[0].Project != "Blog" {
		t.Errorf("expected task[0].Project = %q, got %q", "Blog", tasks[0].Project)
	}
	if tasks[1].Project != "" {
		t.Errorf("expected task[1].Project = %q, got %q", "", tasks[1].Project)
	}
}

func TestMatchTasksToProjects_NoMatch(t *testing.T) {
	projects := []Project{
		{Name: "Alpha", Folder: "alpha/", TaskFilter: "alpha-task", Tags: []string{"alpha-tag"}},
	}

	tasks := []Task{
		{Text: "unrelated task", NotePath: "beta/notes.md", Tags: []string{"beta"}},
	}

	MatchTasksToProjects(tasks, projects)

	if tasks[0].Project != "" {
		t.Errorf("expected empty project, got %q", tasks[0].Project)
	}
}

func TestMatchTasksToProjects_TaskFilterCaseInsensitive(t *testing.T) {
	projects := []Project{
		{Name: "Dev", TaskFilter: "DEV"},
	}

	tasks := []Task{
		{Text: "deploy", Tags: []string{"dev"}},
	}

	MatchTasksToProjects(tasks, projects)

	if tasks[0].Project != "Dev" {
		t.Errorf("expected task.Project = %q, got %q", "Dev", tasks[0].Project)
	}
}

// ---------------------------------------------------------------------------
// Recurring task parsing
// ---------------------------------------------------------------------------

func TestRecurrenceParsing(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		wantRecur  string
	}{
		{
			name:      "emoji daily",
			line:      "- [ ] exercise \U0001F501 daily",
			wantRecur: "daily",
		},
		{
			name:      "emoji weekly",
			line:      "- [ ] review PRs \U0001F501 weekly",
			wantRecur: "weekly",
		},
		{
			name:      "emoji monthly",
			line:      "- [ ] pay rent \U0001F501 monthly",
			wantRecur: "monthly",
		},
		{
			name:      "emoji 3x-week",
			line:      "- [ ] gym \U0001F501 3x-week",
			wantRecur: "3x-week",
		},
		{
			name:      "tag weekly",
			line:      "- [ ] team standup #weekly",
			wantRecur: "weekly",
		},
		{
			name:      "tag monthly",
			line:      "- [ ] expense report #monthly",
			wantRecur: "monthly",
		},
		{
			name:      "tag 3x-week",
			line:      "- [ ] run #3x-week",
			wantRecur: "3x-week",
		},
		{
			name:      "tag daily",
			line:      "- [ ] meditate #daily",
			wantRecur: "daily",
		},
		{
			name:      "no recurrence",
			line:      "- [ ] one-off task #work",
			wantRecur: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			notes := map[string]*vault.Note{
				"test.md": {
					RelPath: "test.md",
					Content: tc.line,
				},
			}
			tasks := ParseAllTasks(notes)
			if len(tasks) != 1 {
				t.Fatalf("expected 1 task, got %d", len(tasks))
			}
			if tasks[0].Recurrence != tc.wantRecur {
				t.Errorf("Recurrence = %q, want %q", tasks[0].Recurrence, tc.wantRecur)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ComputeTaskCounts
// ---------------------------------------------------------------------------

func TestComputeTaskCounts(t *testing.T) {
	tasks := []Task{
		{Text: "a", Done: false, Project: "Alpha"},
		{Text: "b", Done: true, Project: "Alpha"},
		{Text: "c", Done: true, Project: "Alpha"},
		{Text: "d", Done: false, Project: "Beta"},
		{Text: "e", Done: false, Project: ""},
	}

	p := &Project{Name: "Alpha"}
	p.ComputeTaskCounts(tasks)

	if p.TasksTotal != 3 {
		t.Errorf("TasksTotal = %d, want 3", p.TasksTotal)
	}
	if p.TasksDone != 2 {
		t.Errorf("TasksDone = %d, want 2", p.TasksDone)
	}
}

func TestComputeTaskCounts_NoMatch(t *testing.T) {
	tasks := []Task{
		{Text: "x", Done: false, Project: "Other"},
	}

	p := &Project{Name: "Mine"}
	p.ComputeTaskCounts(tasks)

	if p.TasksTotal != 0 {
		t.Errorf("TasksTotal = %d, want 0", p.TasksTotal)
	}
	if p.TasksDone != 0 {
		t.Errorf("TasksDone = %d, want 0", p.TasksDone)
	}
}

func TestComputeTaskCounts_AllDone(t *testing.T) {
	tasks := []Task{
		{Text: "a", Done: true, Project: "Proj"},
		{Text: "b", Done: true, Project: "Proj"},
	}

	p := &Project{Name: "Proj"}
	p.ComputeTaskCounts(tasks)

	if p.TasksTotal != 2 {
		t.Errorf("TasksTotal = %d, want 2", p.TasksTotal)
	}
	if p.TasksDone != 2 {
		t.Errorf("TasksDone = %d, want 2", p.TasksDone)
	}
}

// ---------------------------------------------------------------------------
// uniqueProjects
// ---------------------------------------------------------------------------

func TestUniqueProjects(t *testing.T) {
	tm := &TaskManager{
		allTasks: []Task{
			{Project: "Charlie"},
			{Project: "Alpha"},
			{Project: "Bravo"},
			{Project: "Alpha"}, // duplicate
			{Project: ""},      // should be excluded
			{Project: "Charlie"},
		},
	}

	got := tm.uniqueProjects()
	want := []string{"Alpha", "Bravo", "Charlie"}

	if len(got) != len(want) {
		t.Fatalf("uniqueProjects() returned %d items, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("uniqueProjects()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestUniqueProjects_Empty(t *testing.T) {
	tm := &TaskManager{
		allTasks: []Task{
			{Project: ""},
			{Project: ""},
		},
	}

	got := tm.uniqueProjects()
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// FilterTasks
// ---------------------------------------------------------------------------

func TestFilterTasks_AllMode_NoFilter(t *testing.T) {
	tasks := []Task{
		{Text: "a", NotePath: "notes/a.md"},
		{Text: "b", NotePath: "notes/b.md"},
	}
	cfg := config.DefaultConfig() // TaskFilterMode = "all"
	got := FilterTasks(tasks, cfg)
	if len(got) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(got))
	}
}

func TestFilterTasks_TaggedMode(t *testing.T) {
	tasks := []Task{
		{Text: "tagged task", Tags: []string{"task", "work"}, NotePath: "a.md"},
		{Text: "untagged task", Tags: []string{"note"}, NotePath: "b.md"},
		{Text: "no tags", Tags: nil, NotePath: "c.md"},
	}
	cfg := config.DefaultConfig()
	cfg.TaskFilterMode = "tagged"
	cfg.TaskRequiredTags = []string{"task", "todo"}

	got := FilterTasks(tasks, cfg)
	if len(got) != 1 {
		t.Fatalf("expected 1 task, got %d", len(got))
	}
	if got[0].Text != "tagged task" {
		t.Errorf("expected 'tagged task', got %q", got[0].Text)
	}
}

func TestFilterTasks_TaggedMode_CaseInsensitive(t *testing.T) {
	tasks := []Task{
		{Text: "a", Tags: []string{"TASK"}, NotePath: "a.md"},
	}
	cfg := config.DefaultConfig()
	cfg.TaskFilterMode = "tagged"
	cfg.TaskRequiredTags = []string{"task"}

	got := FilterTasks(tasks, cfg)
	if len(got) != 1 {
		t.Errorf("expected 1 task, got %d", len(got))
	}
}

func TestFilterTasks_ExcludeFolders(t *testing.T) {
	tasks := []Task{
		{Text: "keep", NotePath: "projects/a.md"},
		{Text: "exclude", NotePath: "Archive/old.md"},
		{Text: "also exclude", NotePath: "templates/test.md"},
	}
	cfg := config.DefaultConfig()
	cfg.TaskExcludeFolders = []string{"Archive/", "templates/"}

	got := FilterTasks(tasks, cfg)
	if len(got) != 1 {
		t.Fatalf("expected 1 task, got %d", len(got))
	}
	if got[0].Text != "keep" {
		t.Errorf("expected 'keep', got %q", got[0].Text)
	}
}

func TestFilterTasks_ExcludeDone(t *testing.T) {
	tasks := []Task{
		{Text: "open", Done: false, NotePath: "a.md"},
		{Text: "done", Done: true, NotePath: "b.md"},
	}
	cfg := config.DefaultConfig()
	cfg.TaskExcludeDone = true

	got := FilterTasks(tasks, cfg)
	if len(got) != 1 {
		t.Fatalf("expected 1 task, got %d", len(got))
	}
	if got[0].Text != "open" {
		t.Errorf("expected 'open', got %q", got[0].Text)
	}
}

func TestFilterTasks_Combined(t *testing.T) {
	tasks := []Task{
		{Text: "good", Tags: []string{"task"}, Done: false, NotePath: "work/a.md"},
		{Text: "wrong tag", Tags: []string{"note"}, Done: false, NotePath: "work/b.md"},
		{Text: "archived", Tags: []string{"task"}, Done: false, NotePath: "Archive/c.md"},
		{Text: "completed", Tags: []string{"task"}, Done: true, NotePath: "work/d.md"},
	}
	cfg := config.DefaultConfig()
	cfg.TaskFilterMode = "tagged"
	cfg.TaskRequiredTags = []string{"task"}
	cfg.TaskExcludeFolders = []string{"Archive/"}
	cfg.TaskExcludeDone = true

	got := FilterTasks(tasks, cfg)
	if len(got) != 1 {
		t.Fatalf("expected 1 task, got %d", len(got))
	}
	if got[0].Text != "good" {
		t.Errorf("expected 'good', got %q", got[0].Text)
	}
}

// ---------------------------------------------------------------------------
// FilterTasks — combined filters and empty config
// ---------------------------------------------------------------------------

func TestFilterTasks_CombinedFilters(t *testing.T) {
	tasks := []Task{
		{Text: "pass", Tags: []string{"task"}, Done: false, NotePath: "work/x.md"},
		{Text: "wrong tag", Tags: []string{"note"}, Done: false, NotePath: "work/y.md"},
		{Text: "excluded folder", Tags: []string{"task"}, Done: false, NotePath: "Archive/z.md"},
		{Text: "done task", Tags: []string{"task"}, Done: true, NotePath: "work/w.md"},
		{Text: "template", Tags: []string{"task"}, Done: false, NotePath: "templates/t.md"},
	}
	cfg := config.DefaultConfig()
	cfg.TaskFilterMode = "tagged"
	cfg.TaskRequiredTags = []string{"task"}
	cfg.TaskExcludeFolders = []string{"Archive/", "templates/"}
	cfg.TaskExcludeDone = true

	got := FilterTasks(tasks, cfg)
	if len(got) != 1 {
		t.Fatalf("expected 1 task after combined filters, got %d", len(got))
	}
	if got[0].Text != "pass" {
		t.Errorf("expected 'pass', got %q", got[0].Text)
	}
}

func TestFilterTasks_EmptyConfig(t *testing.T) {
	tasks := []Task{
		{Text: "a", Done: false, NotePath: "a.md"},
		{Text: "b", Done: true, NotePath: "b.md"},
		{Text: "c", Done: false, NotePath: "Archive/c.md"},
	}
	cfg := config.DefaultConfig() // TaskFilterMode = "all", no excludes

	got := FilterTasks(tasks, cfg)
	if len(got) != len(tasks) {
		t.Errorf("expected all %d tasks to pass with default config, got %d", len(tasks), len(got))
	}
}
