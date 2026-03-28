package tui

import (
	"strings"
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

// ===========================================================================
// suggestPriority (auto-priority heuristic)
// ===========================================================================

func TestSuggestPriority_Overdue(t *testing.T) {
	task := Task{Text: "overdue task", DueDate: "2020-01-01"}
	got := suggestPriority(task, nil)
	// overdue=+2, due<=2d=+1 (it's way overdue), no date penalty doesn't apply => score>=3 => 3
	if got < 3 {
		t.Errorf("overdue task should get high priority, got %d", got)
	}
}

func TestSuggestPriority_NoDueDate(t *testing.T) {
	task := Task{Text: "no date task"}
	got := suggestPriority(task, nil)
	// score: -1 (no date) => 1 (low)
	if got != 1 {
		t.Errorf("no-date task should get priority 1 (low), got %d", got)
	}
}

func TestSuggestPriority_BlocksOthers(t *testing.T) {
	blocker := Task{Text: "build api server", DueDate: "2020-01-01"}
	dependent := Task{Text: "deploy frontend", DependsOn: []string{"build api server"}}
	got := suggestPriority(blocker, []Task{blocker, dependent})
	if got < 3 {
		t.Errorf("blocking task should get high priority, got %d", got)
	}
}

func TestSuggestPriority_HasProject(t *testing.T) {
	task := Task{Text: "project task", Project: "Alpha", DueDate: "2020-01-01"}
	got := suggestPriority(task, nil)
	if got < 3 {
		t.Errorf("project + overdue task should get high priority, got %d", got)
	}
}

// ===========================================================================
// tmIsSnoozed
// ===========================================================================

func TestTmIsSnoozed_Active(t *testing.T) {
	task := Task{SnoozedUntil: "2099-12-31T23:59"}
	if !tmIsSnoozed(task) {
		t.Error("task snoozed until 2099 should be snoozed")
	}
}

func TestTmIsSnoozed_Expired(t *testing.T) {
	task := Task{SnoozedUntil: "2020-01-01T00:00"}
	if tmIsSnoozed(task) {
		t.Error("task snoozed until 2020 should not be snoozed")
	}
}

func TestTmIsSnoozed_Empty(t *testing.T) {
	task := Task{SnoozedUntil: ""}
	if tmIsSnoozed(task) {
		t.Error("task with empty snooze should not be snoozed")
	}
}

func TestTmIsSnoozed_Invalid(t *testing.T) {
	task := Task{SnoozedUntil: "not-a-date"}
	if tmIsSnoozed(task) {
		t.Error("task with invalid snooze should not be snoozed")
	}
}

// ===========================================================================
// Snooze parsing in ParseAllTasks
// ===========================================================================

func TestSnoozeParsing(t *testing.T) {
	notes := map[string]*vault.Note{
		"test.md": {
			RelPath: "test.md",
			Content: "- [ ] fix bug snooze:2099-12-31T14:00",
		},
	}
	tasks := ParseAllTasks(notes)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].SnoozedUntil != "2099-12-31T14:00" {
		t.Errorf("SnoozedUntil = %q, want %q", tasks[0].SnoozedUntil, "2099-12-31T14:00")
	}
}

func TestSnoozeParsing_NoSnooze(t *testing.T) {
	notes := map[string]*vault.Note{
		"test.md": {
			RelPath: "test.md",
			Content: "- [ ] regular task",
		},
	}
	tasks := ParseAllTasks(notes)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].SnoozedUntil != "" {
		t.Errorf("SnoozedUntil should be empty, got %q", tasks[0].SnoozedUntil)
	}
}

// ===========================================================================
// tmCleanText strips snooze
// ===========================================================================

func TestTmCleanText_StripsSnooze(t *testing.T) {
	got := tmCleanText("fix bug snooze:2099-12-31T14:00")
	if got != "fix bug" {
		t.Errorf("tmCleanText should strip snooze, got %q", got)
	}
}

func TestTmCleanText_StripsEstimate(t *testing.T) {
	got := tmCleanText("fix bug ~30m")
	if got != "fix bug" {
		t.Errorf("tmCleanText should strip estimate, got %q", got)
	}
}

// ===========================================================================
// tmFormatMinutes
// ===========================================================================

func TestTmFormatMinutes(t *testing.T) {
	tests := []struct {
		mins int
		want string
	}{
		{15, "15m"},
		{30, "30m"},
		{59, "59m"},
		{60, "1h"},
		{90, "1h30m"},
		{120, "2h"},
		{135, "2h15m"},
	}
	for _, tc := range tests {
		got := tmFormatMinutes(tc.mins)
		if got != tc.want {
			t.Errorf("tmFormatMinutes(%d) = %q, want %q", tc.mins, got, tc.want)
		}
	}
}

// ===========================================================================
// Eisenhower quadrants
// ===========================================================================

func TestEisenhowerQuadrants(t *testing.T) {
	tm := &TaskManager{
		allTasks: []Task{
			{Text: "urgent important", Priority: 4, DueDate: "2020-01-01", Done: false}, // Q1: overdue + high prio
			{Text: "important", Priority: 3, DueDate: "2099-12-31", Done: false},         // Q2: high prio + far date
			{Text: "urgent low", Priority: 1, DueDate: "2020-01-01", Done: false},         // Q3: overdue + low prio
			{Text: "neither", Priority: 1, DueDate: "2099-12-31", Done: false},            // Q4: low prio + far date
			{Text: "done", Priority: 4, DueDate: "2020-01-01", Done: true},                // excluded
			{Text: "snoozed", Priority: 4, DueDate: "2020-01-01", SnoozedUntil: "2099-12-31T23:59"}, // excluded
		},
	}
	q := tm.eisenhowerQuadrants()
	if len(q[0]) != 1 || q[0][0].Text != "urgent important" {
		t.Errorf("Q1 (DO) should have 'urgent important', got %v", q[0])
	}
	if len(q[1]) != 1 || q[1][0].Text != "important" {
		t.Errorf("Q2 (SCHEDULE) should have 'important', got %v", q[1])
	}
	if len(q[2]) != 1 || q[2][0].Text != "urgent low" {
		t.Errorf("Q3 (DELEGATE) should have 'urgent low', got %v", q[2])
	}
	if len(q[3]) != 1 || q[3][0].Text != "neither" {
		t.Errorf("Q4 (ELIMINATE) should have 'neither', got %v", q[3])
	}
}

func TestEisenhowerQuadrants_Empty(t *testing.T) {
	tm := &TaskManager{allTasks: nil}
	q := tm.eisenhowerQuadrants()
	for i := 0; i < 4; i++ {
		if len(q[i]) != 0 {
			t.Errorf("Q%d should be empty, got %d", i+1, len(q[i]))
		}
	}
}

// ===========================================================================
// Quick-add inline syntax parsing
// ===========================================================================

func TestParseInlineTaskSyntax_Date(t *testing.T) {
	clean, markers := parseInlineTaskSyntax("review PR @tomorrow #work")
	// After removing @tomorrow, there may be extra spaces; clean trims
	if clean != "review PR  #work" && clean != "review PR #work" {
		t.Errorf("clean = %q, want date removed", clean)
	}
	if markers == "" {
		t.Error("markers should contain date")
	}
}

func TestParseInlineTaskSyntax_Priority(t *testing.T) {
	clean, markers := parseInlineTaskSyntax("deploy !high")
	if clean != "deploy" {
		t.Errorf("clean = %q, want %q", clean, "deploy")
	}
	if markers == "" {
		t.Error("markers should contain priority icon")
	}
}

func TestParseInlineTaskSyntax_DateAndPriority(t *testing.T) {
	clean, markers := parseInlineTaskSyntax("task @today !low ~1h")
	// After removing @today and !low, remaining is "task   ~1h" trimmed to "task   ~1h"
	if !strings.Contains(clean, "task") || !strings.Contains(clean, "~1h") {
		t.Errorf("clean = %q, should contain 'task' and '~1h'", clean)
	}
	if markers == "" {
		t.Error("markers should contain date and priority")
	}
}

func TestParseInlineTaskSyntax_NoMarkers(t *testing.T) {
	clean, markers := parseInlineTaskSyntax("plain task #tag")
	if clean != "plain task #tag" {
		t.Errorf("clean = %q, want %q", clean, "plain task #tag")
	}
	if markers != "" {
		t.Errorf("markers should be empty, got %q", markers)
	}
}

func TestResolveRelativeDate_Tomorrow(t *testing.T) {
	got := resolveRelativeDate("tomorrow")
	if got == "" || got == "tomorrow" {
		t.Errorf("resolveRelativeDate('tomorrow') should return YYYY-MM-DD, got %q", got)
	}
	// Should be a valid date format
	if len(got) != 10 || got[4] != '-' || got[7] != '-' {
		t.Errorf("resolveRelativeDate('tomorrow') should return YYYY-MM-DD format, got %q", got)
	}
}

func TestResolveRelativeDate_Weekday(t *testing.T) {
	got := resolveRelativeDate("monday")
	if len(got) != 10 || got[4] != '-' || got[7] != '-' {
		t.Errorf("resolveRelativeDate('monday') should return YYYY-MM-DD format, got %q", got)
	}
}

func TestResolveRelativeDate_ISO(t *testing.T) {
	got := resolveRelativeDate("2026-06-15")
	if got != "2026-06-15" {
		t.Errorf("resolveRelativeDate('2026-06-15') should pass through, got %q", got)
	}
}

func TestResolveRelativeDate_NextWeek(t *testing.T) {
	got := resolveRelativeDate("next week")
	if len(got) != 10 || got[4] != '-' {
		t.Errorf("resolveRelativeDate('next week') should return YYYY-MM-DD, got %q", got)
	}
}

func TestResolveRelativeDate_EndOfMonth(t *testing.T) {
	got := resolveRelativeDate("end of month")
	if len(got) != 10 || got[4] != '-' {
		t.Errorf("resolveRelativeDate('end of month') should return YYYY-MM-DD, got %q", got)
	}
}

func TestResolveRelativeDate_InNDays(t *testing.T) {
	got := resolveRelativeDate("in 3 days")
	if len(got) != 10 || got[4] != '-' {
		t.Errorf("resolveRelativeDate('in 3 days') should return YYYY-MM-DD, got %q", got)
	}
}

func TestResolveRelativeDate_NextFriday(t *testing.T) {
	got := resolveRelativeDate("next friday")
	if len(got) != 10 || got[4] != '-' {
		t.Errorf("resolveRelativeDate('next friday') should return YYYY-MM-DD, got %q", got)
	}
}

// ===========================================================================
// Undo mechanism
// ===========================================================================

func TestUndo_EmptyStack(t *testing.T) {
	tm := &TaskManager{undoStack: nil}
	tm.doUndo()
	if tm.statusMsg != "Nothing to undo" {
		t.Errorf("expected 'Nothing to undo', got %q", tm.statusMsg)
	}
}

// ===========================================================================
// Pinned tasks
// ===========================================================================

func TestPinnedTasks_SortToTop(t *testing.T) {
	tm := &TaskManager{
		allTasks: []Task{
			{Text: "normal", NotePath: "a.md", LineNum: 1},
			{Text: "pinned", NotePath: "b.md", LineNum: 1},
			{Text: "also normal", NotePath: "c.md", LineNum: 1},
		},
		pinnedTasks: map[string]bool{
			"b.md:1": true,
		},
		taskNotes: make(map[string]string),
		collapsed: make(map[string]bool),
		selected:  make(map[string]bool),
	}
	tm.filtered = tm.allTasks

	// Simulate rebuildFiltered pin sorting
	if len(tm.pinnedTasks) > 0 {
		pinned := make([]Task, 0)
		unpinned := make([]Task, 0)
		for _, task := range tm.filtered {
			if tm.pinnedTasks[taskKey(task)] {
				pinned = append(pinned, task)
			} else {
				unpinned = append(unpinned, task)
			}
		}
		tm.filtered = append(pinned, unpinned...)
	}

	if tm.filtered[0].Text != "pinned" {
		t.Errorf("pinned task should be first, got %q", tm.filtered[0].Text)
	}
}

// ===========================================================================
// taskKey
// ===========================================================================

func TestTaskKey(t *testing.T) {
	task := Task{NotePath: "notes/test.md", LineNum: 42}
	got := taskKey(task)
	if got != "notes/test.md:42" {
		t.Errorf("taskKey = %q, want %q", got, "notes/test.md:42")
	}
}

// ===========================================================================
// TimeTracker.TaskTimeMap
// ===========================================================================

func TestTaskTimeMap(t *testing.T) {
	tt := &TimeTracker{
		entries: []timeEntry{
			{TaskText: "write tests", Duration: 30 * 60_000_000_000},  // 30 min in nanoseconds
			{TaskText: "write tests", Duration: 15 * 60_000_000_000},  // 15 min
			{TaskText: "review code", Duration: 60 * 60_000_000_000},  // 60 min
			{TaskText: "", Duration: 10 * 60_000_000_000},             // no task, excluded
		},
	}
	m := tt.TaskTimeMap()
	if m["write tests"] != 45 {
		t.Errorf("write tests = %d minutes, want 45", m["write tests"])
	}
	if m["review code"] != 60 {
		t.Errorf("review code = %d minutes, want 60", m["review code"])
	}
	if _, ok := m[""]; ok {
		t.Error("empty task text should be excluded")
	}
}
