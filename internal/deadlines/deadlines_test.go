package deadlines

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveAndLoadAll_RoundTripsEveryField(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)
	want := []Deadline{
		{
			ID:          "01J0000000000000000000BAR1",
			Title:       "Bar exam",
			Date:        "2026-09-15",
			Description: "make-or-break",
			GoalID:      "g1",
			ProjectName: "Law",
			TaskIDs:     []string{"t1", "t2"},
			Importance:  string(ImportanceCritical),
			Status:      string(StatusActive),
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:         "01J0000000000000000000FIN1",
			Title:      "File 2025 taxes",
			Date:       "2026-04-15",
			Importance: string(ImportanceHigh),
			Status:     string(StatusMet),
			CreatedAt:  now,
			UpdatedAt:  now,
		},
	}
	if err := SaveAll(dir, want); err != nil {
		t.Fatalf("SaveAll: %v", err)
	}
	got := LoadAll(dir)
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	// Round-trip every linkage field — the schema-truncation regression
	// the goals package fought is exactly the kind of bug we don't
	// want to ship for deadlines either.
	if got[0].GoalID != "g1" || got[0].ProjectName != "Law" {
		t.Errorf("goal/project lost: %+v", got[0])
	}
	if len(got[0].TaskIDs) != 2 || got[0].TaskIDs[0] != "t1" || got[0].TaskIDs[1] != "t2" {
		t.Errorf("task_ids lost: %+v", got[0].TaskIDs)
	}
	if got[0].Description != "make-or-break" {
		t.Errorf("description lost: %q", got[0].Description)
	}
}

func TestSaveAtomic_NoLeftoverTmp(t *testing.T) {
	dir := t.TempDir()
	if err := SaveAll(dir, []Deadline{{ID: "x", Title: "y", Date: "2026-12-31", Importance: "normal", Status: "active"}}); err != nil {
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

func TestSortForDisplay_GroupsByStatusThenDateAsc(t *testing.T) {
	in := []Deadline{
		{ID: "a", Date: "2026-12-31", Status: string(StatusMet)},
		{ID: "b", Date: "2026-06-01", Status: string(StatusActive)},
		{ID: "c", Date: "2026-01-15", Status: string(StatusMissed)},
		{ID: "d", Date: "2026-03-01", Status: string(StatusActive)},
		{ID: "e", Date: "2025-01-01", Status: string(StatusCancelled)},
	}
	got := SortForDisplay(in)
	// Active comes first, sorted by date asc: d (Mar) before b (Jun).
	// Then missed: c.
	// Then met: a.
	// Then cancelled: e.
	wantIDs := []string{"d", "b", "c", "a", "e"}
	for i, id := range wantIDs {
		if got[i].ID != id {
			t.Errorf("position %d: got %q, want %q (full: %v)", i, got[i].ID, id, ids(got))
		}
	}
	// Caller's slice must be untouched — the function returns a copy.
	if in[0].ID != "a" {
		t.Errorf("input mutated: in[0].ID=%q", in[0].ID)
	}
}

func ids(ds []Deadline) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.ID
	}
	return out
}

func TestValidateDate(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"2026-09-15", true},
		{"2026-9-15", false},  // strict YYYY-MM-DD only
		{"2026-09-15T00:00:00", false},
		{"", false},
		{"not a date", false},
		{"2026-13-01", false}, // out-of-range month
	}
	for _, c := range cases {
		if got := ValidateDate(c.in); got != c.want {
			t.Errorf("ValidateDate(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestNormalizeImportance(t *testing.T) {
	cases := map[string]string{
		"critical":   "critical",
		"CRITICAL":   "critical",
		" high ":     "high",
		"":           "normal",
		"unknown":    "normal",
		"normal":     "normal",
	}
	for in, want := range cases {
		if got := NormalizeImportance(in); got != want {
			t.Errorf("NormalizeImportance(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCountdownAndIsOverdue(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	if d := (Deadline{Date: today, Status: "active"}); d.Countdown() != "today" {
		t.Errorf("today: got %q", d.Countdown())
	}
	if d := (Deadline{Date: tomorrow, Status: "active"}); d.Countdown() != "tomorrow" {
		t.Errorf("tomorrow: got %q", d.Countdown())
	}
	if d := (Deadline{Date: yesterday, Status: "active"}); d.Countdown() != "yesterday" {
		t.Errorf("yesterday: got %q", d.Countdown())
	}

	// Overdue: past date AND status=active.
	if !(Deadline{Date: yesterday, Status: "active"}).IsOverdue() {
		t.Error("yesterday/active should be overdue")
	}
	// Cancelled deadlines are never overdue even if past.
	if (Deadline{Date: yesterday, Status: "cancelled"}).IsOverdue() {
		t.Error("cancelled should never be overdue")
	}
	// Met deadlines never overdue.
	if (Deadline{Date: yesterday, Status: "met"}).IsOverdue() {
		t.Error("met should never be overdue")
	}
}

func TestLoadAll_MissingFileReturnsNil(t *testing.T) {
	dir := t.TempDir()
	if got := LoadAll(dir); got != nil {
		t.Errorf("missing file: got %v, want nil", got)
	}
}

func TestLoadAll_CorruptFileReturnsNil(t *testing.T) {
	dir := t.TempDir()
	gdir := filepath.Join(dir, ".granit")
	if err := os.MkdirAll(gdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gdir, "deadlines.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadAll(dir); got != nil {
		t.Errorf("corrupt file: got %v, want nil", got)
	}
}
