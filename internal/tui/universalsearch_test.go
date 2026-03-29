package tui

import (
	"testing"

	"github.com/artaeon/granit/internal/vault"
)

func TestUsFuzzyMatch(t *testing.T) {
	tests := []struct {
		str, pat string
		want     bool
	}{
		{"hello world", "hw", true},
		{"hello world", "hwo", true},
		{"hello world", "xyz", false},
		{"Tasks", "tsk", true},
		{"Goals", "gol", true},
		{"", "a", false},
		{"anything", "", true},
	}
	for _, tc := range tests {
		got := usFuzzyMatch(tc.str, tc.pat)
		if got != tc.want {
			t.Errorf("usFuzzyMatch(%q, %q) = %v, want %v", tc.str, tc.pat, got, tc.want)
		}
	}
}

func TestUsFuzzyScore(t *testing.T) {
	// Exact start match should score highest
	s1 := usFuzzyScore("Tasks", "task")
	s2 := usFuzzyScore("My Tasks", "task")
	if s1 <= s2 {
		t.Errorf("start match should score higher: %d vs %d", s1, s2)
	}

	// No match should return 0
	s3 := usFuzzyScore("hello", "xyz")
	if s3 != 0 {
		t.Errorf("no match should return 0, got %d", s3)
	}
}

func TestUniversalSearchNotes(t *testing.T) {
	us := &UniversalSearch{
		notes: map[string]*vault.Note{
			"Projects/alpha.md": {RelPath: "Projects/alpha.md", Content: "# Alpha Project\nSome content here"},
			"Daily/2026-01-01.md": {RelPath: "Daily/2026-01-01.md", Content: "# Daily Note\nTasks for today"},
		},
		tasks:  nil,
		goals:  nil,
		habits: nil,
	}
	us.query = "alpha"
	us.search()
	if len(us.results) == 0 {
		t.Error("should find note matching 'alpha'")
	}
	if us.results[0].Type != usResultNote {
		t.Errorf("first result should be note, got type %d", us.results[0].Type)
	}
}

func TestUniversalSearchTasks(t *testing.T) {
	us := &UniversalSearch{
		notes: nil,
		tasks: []Task{
			{Text: "fix the login bug", NotePath: "Tasks.md", LineNum: 5},
			{Text: "write documentation", NotePath: "Tasks.md", LineNum: 10},
		},
		goals:  nil,
		habits: nil,
	}
	us.query = "login"
	us.search()
	if len(us.results) == 0 {
		t.Error("should find task matching 'login'")
	}
	if us.results[0].Type != usResultTask {
		t.Errorf("first result should be task, got type %d", us.results[0].Type)
	}
}

func TestUniversalSearchGoals(t *testing.T) {
	us := &UniversalSearch{
		notes: nil,
		tasks: nil,
		goals: []Goal{
			{ID: "G001", Title: "Learn Rust", Status: GoalStatusActive, TargetDate: "2026-12-31"},
		},
		habits: nil,
	}
	us.query = "rust"
	us.search()
	if len(us.results) == 0 {
		t.Error("should find goal matching 'rust'")
	}
	if us.results[0].Type != usResultGoal {
		t.Errorf("first result should be goal, got type %d", us.results[0].Type)
	}
}

func TestUniversalSearchEmpty(t *testing.T) {
	us := &UniversalSearch{}
	us.query = ""
	us.search()
	if len(us.results) != 0 {
		t.Error("empty query should return no results")
	}
}
