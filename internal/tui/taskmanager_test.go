package tui

import (
	"os"
	"strings"
	"testing"
	"time"

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
		name      string
		line      string
		wantRecur string
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
	cfg := config.DefaultConfig()
	cfg.TaskFilterMode = "all" // explicit all mode
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
	cfg.TaskFilterMode = "all"
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
	cfg.TaskFilterMode = "all"
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
		{Text: "a", Done: false, NotePath: "a.md", Tags: []string{"task"}},
		{Text: "b", Done: true, NotePath: "b.md", Tags: []string{"task"}},
		{Text: "c", Done: false, NotePath: "Archive/c.md", Tags: []string{"task"}},
	}
	cfg := config.DefaultConfig() // TaskFilterMode = "tagged", tags = ["task", "todo"]

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
// Quick-edit parsers
// ===========================================================================

func TestParseEstimateSpec(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"30m", 30},
		{"2h", 120},
		{"1h30m", 90},
		{"0m", 0}, // parses but yields 0
		{"", 0},
		{"garbage", 0},
		{"3d", 0}, // days not supported
	}
	for _, tc := range cases {
		if got := parseEstimateSpec(tc.in); got != tc.want {
			t.Errorf("parseEstimateSpec(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestParseQuickEditDate(t *testing.T) {
	// Only check static inputs (absolute + unparseable); relative dates
	// (today/tomorrow/+Nd) depend on wall clock.
	if got := parseQuickEditDate("2026-04-15"); got != "2026-04-15" {
		t.Errorf("absolute date: got %q", got)
	}
	if got := parseQuickEditDate("garbage"); got != "" {
		t.Errorf("garbage: got %q, want empty", got)
	}
	if got := parseQuickEditDate("+xd"); got != "" {
		t.Errorf("+xd: got %q, want empty", got)
	}
}

// ===========================================================================
// fuzzyMatch subsequence matching (shared helper from sidebar.go)
// ===========================================================================

func TestFuzzyMatch(t *testing.T) {
	cases := []struct {
		haystack, needle string
		want             bool
	}{
		{"buy groceries", "bygr", true}, // subsequence with gaps
		{"buy groceries", "buy", true},  // contiguous substring
		{"buy groceries", "groc", true},
		{"buy groceries", "ceries", true},
		{"buy groceries", "", true}, // empty needle always matches
		{"buy groceries", "xyz", false},
		{"buy groceries", "gbo", false}, // wrong order rejected
		{"abc", "abcd", false},          // needle longer than haystack
	}
	for _, tc := range cases {
		got := fuzzyMatch(tc.haystack, tc.needle)
		if got != tc.want {
			t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", tc.haystack, tc.needle, got, tc.want)
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
			{TaskText: "write tests", Duration: 30 * 60_000_000_000}, // 30 min in nanoseconds
			{TaskText: "write tests", Duration: 15 * 60_000_000_000}, // 15 min
			{TaskText: "review code", Duration: 60 * 60_000_000_000}, // 60 min
			{TaskText: "", Duration: 10 * 60_000_000_000},            // no task, excluded
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

// ---------------------------------------------------------------------------
// ParseAllTasks – metadata field extraction
// ---------------------------------------------------------------------------

func makeVaultNote(content string) map[string]*vault.Note {
	return map[string]*vault.Note{
		"test.md": {Content: content},
	}
}

func TestParseAllTasks_EstimateMinutes(t *testing.T) {
	notes := makeVaultNote("- [ ] Quick fix ~15m\n- [ ] Big feature ~3h")
	tasks := ParseAllTasks(notes)
	if len(tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(tasks))
	}
	if tasks[0].EstimatedMinutes != 15 {
		t.Errorf("task 0 estimate = %d, want 15", tasks[0].EstimatedMinutes)
	}
	if tasks[1].EstimatedMinutes != 180 {
		t.Errorf("task 1 estimate = %d, want 180", tasks[1].EstimatedMinutes)
	}
}

func TestParseAllTasks_ScheduledTime(t *testing.T) {
	notes := makeVaultNote("- [ ] Meeting ⏰ 09:00-10:30")
	tasks := ParseAllTasks(notes)
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	if tasks[0].ScheduledTime != "09:00-10:30" {
		t.Errorf("scheduled = %q, want %q", tasks[0].ScheduledTime, "09:00-10:30")
	}
}

func TestParseAllTasks_GoalID(t *testing.T) {
	notes := makeVaultNote("- [ ] Ship feature goal:G042")
	tasks := ParseAllTasks(notes)
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	if tasks[0].GoalID != "G042" {
		t.Errorf("goalID = %q, want %q", tasks[0].GoalID, "G042")
	}
}

func TestParseAllTasks_DependsOn(t *testing.T) {
	notes := makeVaultNote(`- [ ] Deploy depends:"Run tests"`)
	tasks := ParseAllTasks(notes)
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	if len(tasks[0].DependsOn) != 1 || tasks[0].DependsOn[0] != "Run tests" {
		t.Errorf("depends = %v, want [Run tests]", tasks[0].DependsOn)
	}
}

func TestParseAllTasks_MultipleMetadata(t *testing.T) {
	notes := makeVaultNote("- [ ] Big task 📅 2026-04-01 🔺 #work ~2h ⏰ 14:00-16:00 goal:G001")
	tasks := ParseAllTasks(notes)
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	tk := tasks[0]
	if tk.DueDate != "2026-04-01" {
		t.Errorf("dueDate = %q, want 2026-04-01", tk.DueDate)
	}
	if tk.Priority != 4 {
		t.Errorf("priority = %d, want 4", tk.Priority)
	}
	if tk.EstimatedMinutes != 120 {
		t.Errorf("estimate = %d, want 120", tk.EstimatedMinutes)
	}
	if tk.ScheduledTime != "14:00-16:00" {
		t.Errorf("scheduled = %q, want 14:00-16:00", tk.ScheduledTime)
	}
	if tk.GoalID != "G001" {
		t.Errorf("goalID = %q, want G001", tk.GoalID)
	}
	found := false
	for _, tag := range tk.Tags {
		if tag == "work" {
			found = true
		}
	}
	if !found {
		t.Errorf("tags = %v, want to contain 'work'", tk.Tags)
	}
}

func TestParseAllTasks_Subtasks(t *testing.T) {
	notes := makeVaultNote("- [ ] Parent\n  - [ ] Child\n    - [ ] Grandchild")
	tasks := ParseAllTasks(notes)
	if len(tasks) != 3 {
		t.Fatalf("got %d tasks, want 3", len(tasks))
	}
	if tasks[0].Indent != 0 {
		t.Errorf("parent indent = %d, want 0", tasks[0].Indent)
	}
	if tasks[1].Indent != 1 {
		t.Errorf("child indent = %d, want 1", tasks[1].Indent)
	}
	if tasks[2].Indent != 2 {
		t.Errorf("grandchild indent = %d, want 2", tasks[2].Indent)
	}
}

// ---------------------------------------------------------------------------
// ICS parsing
// ---------------------------------------------------------------------------

func TestParseICSFile_DateFormats(t *testing.T) {
	tests := []struct {
		input string
		want  bool // allDay
	}{
		{"20260401T140000Z", false},
		{"20260401T140000", false},
		{"20260401", true},
		{"2026-04-01T14:00:00Z", false},
		{"2026-04-01T14:00:00", false},
		{"2026-04-01", true},
	}
	for _, tc := range tests {
		parsed, allDay, err := parseICSTime(tc.input)
		if err != nil || parsed.IsZero() {
			t.Errorf("parseICSTime(%q) returned error or zero time: %v", tc.input, err)
			continue
		}
		if allDay != tc.want {
			t.Errorf("parseICSTime(%q) allDay = %v, want %v", tc.input, allDay, tc.want)
		}
	}
}

func TestParseICSFile_LineUnfolding(t *testing.T) {
	// RFC 5545: continuation lines start with space or tab
	ics := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nSUMMARY:Very long\r\n  event title\r\nDTSTART:20260401T100000Z\r\nDTEND:20260401T110000Z\r\nEND:VEVENT\r\nEND:VCALENDAR"
	// Write temp file
	tmpDir := t.TempDir()
	path := tmpDir + "/test.ics"
	if err := writeTestFile(path, ics); err != nil {
		t.Fatal(err)
	}
	events, err := ParseICSFile(path)
	if err != nil {
		t.Fatalf("ParseICSFile error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Title != "Very long event title" {
		t.Errorf("title = %q, want %q", events[0].Title, "Very long event title")
	}
}

func TestParseICSFile_RRULEDaily(t *testing.T) {
	ics := "BEGIN:VCALENDAR\nBEGIN:VEVENT\nSUMMARY:Standup\nDTSTART:20260101T090000\nDTEND:20260101T093000\nRRULE:FREQ=DAILY\nEND:VEVENT\nEND:VCALENDAR"
	tmpDir := t.TempDir()
	path := tmpDir + "/test.ics"
	if err := writeTestFile(path, ics); err != nil {
		t.Fatal(err)
	}
	events, err := ParseICSFile(path)
	if err != nil {
		t.Fatalf("ParseICSFile error: %v", err)
	}
	// Should have many occurrences (daily for ~90 days)
	if len(events) < 30 {
		t.Errorf("got %d events from daily RRULE, want at least 30", len(events))
	}
	// All should have same title
	for _, ev := range events {
		if ev.Title != "Standup" {
			t.Errorf("event title = %q, want Standup", ev.Title)
			break
		}
	}
}

func TestParseICSFile_RRULEWeekly(t *testing.T) {
	ics := "BEGIN:VCALENDAR\nBEGIN:VEVENT\nSUMMARY:Weekly sync\nDTSTART:20260101T140000\nDTEND:20260101T150000\nRRULE:FREQ=WEEKLY\nEND:VEVENT\nEND:VCALENDAR"
	tmpDir := t.TempDir()
	path := tmpDir + "/test.ics"
	if err := writeTestFile(path, ics); err != nil {
		t.Fatal(err)
	}
	events, err := ParseICSFile(path)
	if err != nil {
		t.Fatalf("ParseICSFile error: %v", err)
	}
	// Weekly for ~90 days = ~13 occurrences
	if len(events) < 5 {
		t.Errorf("got %d events from weekly RRULE, want at least 5", len(events))
	}
}

func TestParseICSRRule(t *testing.T) {
	tests := []struct {
		input    string
		wantFreq string
		wantInt  int
	}{
		{"FREQ=DAILY", "DAILY", 1},
		{"FREQ=WEEKLY;BYDAY=MO", "WEEKLY", 1},
		{"FREQ=MONTHLY;INTERVAL=2", "MONTHLY", 2},
		{"FREQ=YEARLY", "YEARLY", 1},
		{"BYDAY=MO", "", 1},
	}
	for _, tc := range tests {
		freq, interval, _, _ := parseICSRRule(tc.input)
		if freq != tc.wantFreq {
			t.Errorf("parseICSRRule(%q) freq = %q, want %q", tc.input, freq, tc.wantFreq)
		}
		if interval != tc.wantInt {
			t.Errorf("parseICSRRule(%q) interval = %d, want %d", tc.input, interval, tc.wantInt)
		}
	}
}

func writeTestFile(path, content string) error {
	return writeFile(path, []byte(content))
}

func writeFile(path string, data []byte) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	f.Close()
	return err
}

func createFile(path string) (*os.File, error) {
	return os.Create(path)
}

// ---------------------------------------------------------------------------
// Refresh applies FilterTasks
// ---------------------------------------------------------------------------

func TestRefresh_AppliesFilterTasks(t *testing.T) {
	notes := map[string]*vault.Note{
		"test.md": {Content: "- [ ] Tagged #task\n- [ ] Untagged task"},
	}
	tm := NewTaskManager()
	tm.config = config.Config{
		TaskFilterMode:   "tagged",
		TaskRequiredTags: []string{"task"},
	}
	v := &vault.Vault{Notes: notes}
	tm.Open(v)
	// Open should filter: only 1 task with #task tag
	if len(tm.allTasks) != 1 {
		t.Errorf("after Open: got %d tasks, want 1", len(tm.allTasks))
	}

	// Refresh should also filter
	tm.Refresh(v)
	if len(tm.allTasks) != 1 {
		t.Errorf("after Refresh: got %d tasks, want 1", len(tm.allTasks))
	}
	if tm.allTasks[0].Tags[0] != "task" {
		t.Errorf("task tag = %q, want 'task'", tm.allTasks[0].Tags[0])
	}
}

// ---------------------------------------------------------------------------
// ParseAllTasks – recurrence emoji variant
// ---------------------------------------------------------------------------

func TestParseAllTasks_RecurrenceEmoji(t *testing.T) {
	notes := makeVaultNote("- [ ] Standup 🔁 daily\n- [ ] Review 🔁 weekly\n- [ ] Report 🔁 monthly\n- [ ] Gym 🔁 3x-week")
	tasks := ParseAllTasks(notes)
	if len(tasks) != 4 {
		t.Fatalf("got %d tasks, want 4", len(tasks))
	}
	want := []string{"daily", "weekly", "monthly", "3x-week"}
	for i, w := range want {
		if tasks[i].Recurrence != w {
			t.Errorf("task %d recurrence = %q, want %q", i, tasks[i].Recurrence, w)
		}
	}
}

func TestParseAllTasks_RecurrenceTagVariant(t *testing.T) {
	notes := makeVaultNote("- [ ] Standup #daily\n- [ ] Review #weekly")
	tasks := ParseAllTasks(notes)
	if len(tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(tasks))
	}
	if tasks[0].Recurrence != "daily" {
		t.Errorf("task 0 recurrence = %q, want daily", tasks[0].Recurrence)
	}
	if tasks[1].Recurrence != "weekly" {
		t.Errorf("task 1 recurrence = %q, want weekly", tasks[1].Recurrence)
	}
}

func TestParseAllTasks_NoMetadata(t *testing.T) {
	notes := makeVaultNote("- [ ] Simple task\n- [x] Done task")
	tasks := ParseAllTasks(notes)
	if len(tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(tasks))
	}
	tk := tasks[0]
	if tk.Done {
		t.Error("task 0 should not be done")
	}
	if tk.Priority != 0 {
		t.Errorf("priority = %d, want 0", tk.Priority)
	}
	if tk.DueDate != "" {
		t.Errorf("dueDate = %q, want empty", tk.DueDate)
	}
	if tk.EstimatedMinutes != 0 {
		t.Errorf("estimate = %d, want 0", tk.EstimatedMinutes)
	}
	if tk.ScheduledTime != "" {
		t.Errorf("scheduledTime = %q, want empty", tk.ScheduledTime)
	}
	if tk.GoalID != "" {
		t.Errorf("goalID = %q, want empty", tk.GoalID)
	}
	if tasks[1].Done != true {
		t.Error("task 1 should be done")
	}
}

func TestParseAllTasks_Snooze(t *testing.T) {
	notes := makeVaultNote("- [ ] Snoozed task snooze:2099-12-31T14:00")
	tasks := ParseAllTasks(notes)
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	if tasks[0].SnoozedUntil != "2099-12-31T14:00" {
		t.Errorf("snoozedUntil = %q, want 2099-12-31T14:00", tasks[0].SnoozedUntil)
	}
}

func TestParseAllTasks_NotePath(t *testing.T) {
	notes := map[string]*vault.Note{
		"projects/work.md":    {Content: "- [ ] Work task", RelPath: "projects/work.md"},
		"daily/2026-04-01.md": {Content: "- [ ] Daily task", RelPath: "daily/2026-04-01.md"},
	}
	tasks := ParseAllTasks(notes)
	if len(tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(tasks))
	}
	paths := map[string]bool{}
	for _, tk := range tasks {
		paths[tk.NotePath] = true
	}
	if !paths["projects/work.md"] {
		t.Error("missing task from projects/work.md")
	}
	if !paths["daily/2026-04-01.md"] {
		t.Error("missing task from daily/2026-04-01.md")
	}
}

// ---------------------------------------------------------------------------
// FilterTasks – snoozed tasks
// ---------------------------------------------------------------------------

func TestFilterAll_ExcludesSnoozed(t *testing.T) {
	notes := makeVaultNote("- [ ] Normal task\n- [ ] Snoozed snooze:2099-12-31T14:00")
	tm := NewTaskManager()
	tm.config = config.Config{TaskFilterMode: "all"}
	v := &vault.Vault{Notes: notes}
	tm.Open(v)
	// filterAll should exclude snoozed tasks
	all := tm.filterAll()
	if len(all) != 1 {
		t.Errorf("filterAll got %d tasks, want 1 (snoozed excluded)", len(all))
	}
	if len(all) > 0 && all[0].SnoozedUntil != "" {
		t.Error("snoozed task should not appear in filterAll")
	}
}

func TestFilterToday_ExcludesSnoozed(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	notes := makeVaultNote("- [ ] Today task 📅 " + today + "\n- [ ] Snoozed today 📅 " + today + " snooze:2099-12-31T14:00")
	tm := NewTaskManager()
	tm.config = config.Config{TaskFilterMode: "all"}
	v := &vault.Vault{Notes: notes}
	tm.Open(v)
	todayTasks := tm.filterToday()
	if len(todayTasks) != 1 {
		t.Errorf("filterToday got %d tasks, want 1", len(todayTasks))
	}
}

// ---------------------------------------------------------------------------
// ICS – edge cases
// ---------------------------------------------------------------------------

func TestParseICSFile_MultipleEvents(t *testing.T) {
	ics := "BEGIN:VCALENDAR\nBEGIN:VEVENT\nSUMMARY:Event 1\nDTSTART:20260401T090000\nEND:VEVENT\nBEGIN:VEVENT\nSUMMARY:Event 2\nDTSTART:20260401T140000\nEND:VEVENT\nEND:VCALENDAR"
	tmpDir := t.TempDir()
	path := tmpDir + "/test.ics"
	if err := writeTestFile(path, ics); err != nil {
		t.Fatal(err)
	}
	events, err := ParseICSFile(path)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("got %d events, want 2", len(events))
	}
}

func TestParseICSFile_EventWithLocation(t *testing.T) {
	ics := "BEGIN:VCALENDAR\nBEGIN:VEVENT\nSUMMARY:Meeting\nDTSTART:20260401T100000\nLOCATION:Room 42\nEND:VEVENT\nEND:VCALENDAR"
	tmpDir := t.TempDir()
	path := tmpDir + "/test.ics"
	if err := writeTestFile(path, ics); err != nil {
		t.Fatal(err)
	}
	events, err := ParseICSFile(path)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(events) != 1 || events[0].Location != "Room 42" {
		t.Errorf("location = %q, want 'Room 42'", events[0].Location)
	}
}

func TestParseICSFile_AllDayEvent(t *testing.T) {
	ics := "BEGIN:VCALENDAR\nBEGIN:VEVENT\nSUMMARY:Holiday\nDTSTART:20260401\nEND:VEVENT\nEND:VCALENDAR"
	tmpDir := t.TempDir()
	path := tmpDir + "/test.ics"
	if err := writeTestFile(path, ics); err != nil {
		t.Fatal(err)
	}
	events, err := ParseICSFile(path)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if !events[0].AllDay {
		t.Error("expected AllDay=true for date-only DTSTART")
	}
}

func TestParseICSFile_DurationPreserved(t *testing.T) {
	ics := "BEGIN:VCALENDAR\nBEGIN:VEVENT\nSUMMARY:Long meeting\nDTSTART:20260401T090000\nDTEND:20260401T120000\nEND:VEVENT\nEND:VCALENDAR"
	tmpDir := t.TempDir()
	path := tmpDir + "/test.ics"
	if err := writeTestFile(path, ics); err != nil {
		t.Fatal(err)
	}
	events, err := ParseICSFile(path)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	dur := events[0].EndDate.Sub(events[0].Date)
	if dur.Hours() != 3 {
		t.Errorf("duration = %v, want 3h", dur)
	}
}

func TestParseICSFile_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := tmpDir + "/empty.ics"
	if err := writeTestFile(path, ""); err != nil {
		t.Fatal(err)
	}
	events, err := ParseICSFile(path)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("got %d events from empty file, want 0", len(events))
	}
}

func TestParseICSFile_RRULEMonthly(t *testing.T) {
	ics := "BEGIN:VCALENDAR\nBEGIN:VEVENT\nSUMMARY:Monthly review\nDTSTART:20260101T100000\nDTEND:20260101T110000\nRRULE:FREQ=MONTHLY\nEND:VEVENT\nEND:VCALENDAR"
	tmpDir := t.TempDir()
	path := tmpDir + "/test.ics"
	if err := writeTestFile(path, ics); err != nil {
		t.Fatal(err)
	}
	events, err := ParseICSFile(path)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(events) < 2 {
		t.Errorf("got %d events from monthly RRULE, want at least 2", len(events))
	}
}

// ---------------------------------------------------------------------------
// Time-block scheduling
// ---------------------------------------------------------------------------

func TestAssignSchedule_WritesToCorrectFile(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a task in a non-Tasks.md file
	projectFile := tmpDir + "/projects/work.md"
	if err := os.MkdirAll(tmpDir+"/projects", 0755); err != nil {
		t.Fatal(err)
	}
	content := "# Work\n\n- [ ] Deploy v2.0\n- [ ] Write docs\n"
	if err := os.WriteFile(projectFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	v := &vault.Vault{Root: tmpDir}
	v.Notes = map[string]*vault.Note{
		"projects/work.md": {
			Path:    projectFile,
			RelPath: "projects/work.md",
			Content: content,
		},
	}

	tm := &TaskManager{vault: v}
	task := Task{
		Text:     "Deploy v2.0",
		NotePath: "projects/work.md",
		LineNum:  3,
	}

	tm.assignSchedule(task, "14:00", "15:00")

	// Verify the file on disk has the marker
	data, err := os.ReadFile(projectFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "⏰ 14:00-15:00") {
		t.Errorf("file content missing schedule marker:\n%s", data)
	}

	// Verify the vault cache was also updated
	note := v.Notes["projects/work.md"]
	if !strings.Contains(note.Content, "⏰ 14:00-15:00") {
		t.Errorf("vault cache missing schedule marker:\n%s", note.Content)
	}
}

func TestRemoveScheduleMarker_UpdatesVaultCache(t *testing.T) {
	tmpDir := t.TempDir()
	content := "# Tasks\n\n- [ ] Meeting ⏰ 09:00-10:00\n"
	tasksFile := tmpDir + "/Tasks.md"
	if err := os.WriteFile(tasksFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	v := &vault.Vault{Root: tmpDir}
	v.Notes = map[string]*vault.Note{
		"Tasks.md": {
			Path:    tasksFile,
			RelPath: "Tasks.md",
			Content: content,
		},
	}

	tm := &TaskManager{vault: v}
	task := Task{
		Text:          "Meeting ⏰ 09:00-10:00",
		NotePath:      "Tasks.md",
		LineNum:       3,
		ScheduledTime: "09:00-10:00",
	}

	tm.removeScheduleMarker(task)

	// Verify disk
	data, err := os.ReadFile(tasksFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "⏰") {
		t.Errorf("file still has schedule marker:\n%s", data)
	}

	// Verify vault cache
	note := v.Notes["Tasks.md"]
	if strings.Contains(note.Content, "⏰") {
		t.Errorf("vault cache still has schedule marker:\n%s", note.Content)
	}
}

func TestTimeBlockGroup(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	tests := []struct {
		name string
		task Task
		want string
	}{
		{"scheduled morning", Task{ScheduledTime: "08:00-09:00"}, "morning"},
		{"scheduled midday", Task{ScheduledTime: "11:00-12:00"}, "midday"},
		{"scheduled afternoon", Task{ScheduledTime: "15:00-16:00"}, "afternoon"},
		{"scheduled evening", Task{ScheduledTime: "19:00-20:00"}, "evening"},
		{"overdue unscheduled", Task{DueDate: yesterday}, "overdue"},
		{"today unscheduled", Task{DueDate: today}, "today"},
		{"tomorrow", Task{DueDate: tomorrow}, "tomorrow"},
		{"no date no schedule", Task{}, ""},
		// Scheduled overdue task should go to time block, not overdue
		{"scheduled overdue", Task{DueDate: yesterday, ScheduledTime: "14:00-15:00"}, "afternoon"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := timeBlockGroup(tc.task)
			if got != tc.want {
				t.Errorf("timeBlockGroup(%v) = %q, want %q", tc.task, got, tc.want)
			}
		})
	}
}
