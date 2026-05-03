package goals

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProgress(t *testing.T) {
	cases := []struct {
		name string
		goal Goal
		want int
	}{
		{"no milestones active", Goal{Status: StatusActive}, 0},
		{"no milestones completed", Goal{Status: StatusCompleted}, 100},
		{"all done", Goal{Milestones: []Milestone{{Done: true}, {Done: true}}}, 100},
		{"half done", Goal{Milestones: []Milestone{{Done: true}, {Done: false}}}, 50},
		{"none done", Goal{Milestones: []Milestone{{Done: false}, {Done: false}}}, 0},
		{"one of three", Goal{Milestones: []Milestone{{Done: true}, {Done: false}, {Done: false}}}, 33},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.goal.Progress(); got != c.want {
				t.Errorf("Progress() = %d, want %d", got, c.want)
			}
		})
	}
}

func TestIsOverdue(t *testing.T) {
	cases := []struct {
		name string
		goal Goal
		want bool
	}{
		{"no date", Goal{Status: StatusActive}, false},
		{"future date", Goal{Status: StatusActive, TargetDate: "2099-12-31"}, false},
		{"past date active", Goal{Status: StatusActive, TargetDate: "2020-01-01"}, true},
		{"past date completed", Goal{Status: StatusCompleted, TargetDate: "2020-01-01"}, false},
		{"past date archived", Goal{Status: StatusArchived, TargetDate: "2020-01-01"}, false},
		{"today is not yet overdue", Goal{Status: StatusActive, TargetDate: time.Now().Format("2006-01-02")}, false},
		{"yesterday is overdue", Goal{Status: StatusActive, TargetDate: time.Now().AddDate(0, 0, -1).Format("2006-01-02")}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.goal.IsOverdue(); got != c.want {
				t.Errorf("IsOverdue() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestIsDueForReview(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	cases := []struct {
		name string
		goal Goal
		want bool
	}{
		{"no frequency", Goal{Status: StatusActive, ReviewFrequency: ""}, false},
		{"non-active goal", Goal{Status: StatusCompleted, ReviewFrequency: "weekly", LastReviewed: "2020-01-01"}, false},
		{"never reviewed", Goal{Status: StatusActive, ReviewFrequency: "weekly", LastReviewed: ""}, true},
		{"reviewed today, weekly", Goal{Status: StatusActive, ReviewFrequency: "weekly", LastReviewed: today}, false},
		{"reviewed long ago, weekly", Goal{Status: StatusActive, ReviewFrequency: "weekly", LastReviewed: "2020-01-01"}, true},
		{"reviewed yesterday, monthly", Goal{Status: StatusActive, ReviewFrequency: "monthly", LastReviewed: time.Now().AddDate(0, 0, -1).Format("2006-01-02")}, false},
		{"reviewed 6 months ago, quarterly", Goal{Status: StatusActive, ReviewFrequency: "quarterly", LastReviewed: time.Now().AddDate(0, -6, 0).Format("2006-01-02")}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.goal.IsDueForReview(); got != c.want {
				t.Errorf("IsDueForReview() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestNextReviewDate(t *testing.T) {
	g := Goal{ReviewFrequency: "weekly", LastReviewed: "2026-01-01"}
	if got := g.NextReviewDate(); got != "2026-01-08" {
		t.Errorf("weekly: got %q, want 2026-01-08", got)
	}
	g2 := Goal{ReviewFrequency: "monthly", LastReviewed: "2026-01-01"}
	if got := g2.NextReviewDate(); got != "2026-02-01" {
		t.Errorf("monthly: got %q, want 2026-02-01", got)
	}
	g3 := Goal{ReviewFrequency: "quarterly", LastReviewed: "2026-01-01"}
	if got := g3.NextReviewDate(); got != "2026-04-01" {
		t.Errorf("quarterly: got %q, want 2026-04-01", got)
	}
	// Fallback: no LastReviewed → uses CreatedAt
	g4 := Goal{ReviewFrequency: "weekly", LastReviewed: "", CreatedAt: "2026-01-01"}
	if got := g4.NextReviewDate(); got != "2026-01-08" {
		t.Errorf("created-fallback: got %q, want 2026-01-08", got)
	}
	// No frequency → empty string
	if got := (Goal{}).NextReviewDate(); got != "" {
		t.Errorf("no frequency: got %q, want empty", got)
	}
}

func TestTimeframeLabel(t *testing.T) {
	g := Goal{TargetDate: time.Now().AddDate(0, 0, 5).Format("2006-01-02"), Status: StatusActive}
	if !strings.Contains(g.TimeframeLabel(), "left") {
		t.Errorf("future: got %q", g.TimeframeLabel())
	}
	g2 := Goal{TargetDate: time.Now().AddDate(0, 0, -10).Format("2006-01-02"), Status: StatusActive}
	if !strings.Contains(g2.TimeframeLabel(), "overdue") {
		t.Errorf("past: got %q", g2.TimeframeLabel())
	}
	g3 := Goal{TargetDate: time.Now().Format("2006-01-02"), Status: StatusActive}
	if g3.TimeframeLabel() != "due today" {
		t.Errorf("today: got %q", g3.TimeframeLabel())
	}
}

func TestSaveAndLoadAll(t *testing.T) {
	dir := t.TempDir()
	want := []Goal{
		{ID: "g1", Title: "Ship the parity audit", Status: StatusActive, Notes: "non-trivial"},
		{ID: "g2", Title: "Done already", Status: StatusCompleted, ReviewFrequency: "weekly"},
	}
	if err := SaveAll(dir, want); err != nil {
		t.Fatalf("SaveAll: %v", err)
	}
	got := LoadAll(dir)
	if len(got) != 2 {
		t.Fatalf("LoadAll len=%d, want 2", len(got))
	}
	// Verify Notes field round-trips — this is exactly the schema-truncation
	// fix we are landing (granitmeta.Goal silently dropped Notes on PATCH).
	if got[0].Notes != "non-trivial" {
		t.Errorf("Notes lost on round-trip: got %q", got[0].Notes)
	}
	if got[1].ReviewFrequency != "weekly" {
		t.Errorf("ReviewFrequency lost on round-trip: got %q", got[1].ReviewFrequency)
	}
}

func TestSaveAtomic(t *testing.T) {
	// Regression: SaveAll must use atomic write — no leftover .tmp file
	// in the .granit dir after a successful save.
	dir := t.TempDir()
	if err := SaveAll(dir, []Goal{{ID: "x", Title: "y"}}); err != nil {
		t.Fatalf("SaveAll: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, ".granit"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover tmp file: %s", e.Name())
		}
	}
}

func TestLoadActive(t *testing.T) {
	dir := t.TempDir()
	all := []Goal{
		{ID: "G001", Status: StatusActive},
		{ID: "G002", Status: StatusCompleted},
		{ID: "G003", Status: StatusPaused},
		{ID: "G004", Status: StatusArchived},
		{ID: "G005", Status: StatusActive},
	}
	if err := SaveAll(dir, all); err != nil {
		t.Fatal(err)
	}
	active := LoadActive(dir)
	if len(active) != 2 {
		t.Errorf("len=%d, want 2", len(active))
	}
}

func TestAddMilestone(t *testing.T) {
	dir := t.TempDir()
	if err := SaveAll(dir, []Goal{{ID: "g1", Title: "test"}}); err != nil {
		t.Fatal(err)
	}
	if err := AddMilestone(dir, "g1", "First step", "2026-04-15"); err != nil {
		t.Fatal(err)
	}
	got := LoadAll(dir)
	if len(got) != 1 || len(got[0].Milestones) != 1 {
		t.Fatalf("milestone not added: %+v", got)
	}
	if got[0].Milestones[0].Text != "First step" || got[0].Milestones[0].DueDate != "2026-04-15" {
		t.Errorf("milestone wrong: %+v", got[0].Milestones[0])
	}
}
