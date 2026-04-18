package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestVault creates a temp vault root with the given files (path→content)
// and returns its absolute path. Keeps tests terse.
func newTestVault(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	return root
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// ---------------------------------------------------------------------------
// validTimeRange
// ---------------------------------------------------------------------------

func TestValidTimeRange(t *testing.T) {
	cases := []struct {
		start, end string
		want       bool
	}{
		{"09:00", "10:00", true},
		{"09:00", "09:00", false}, // zero-length
		{"10:00", "09:00", false}, // end before start
		{"09:0", "10:00", false},  // bad format
		{"09:00", "24:01", false}, // past midnight
		{"", "", false},
	}
	for _, tc := range cases {
		got := validTimeRange(tc.start, tc.end)
		if got != tc.want {
			t.Errorf("validTimeRange(%q,%q) = %v, want %v", tc.start, tc.end, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// SetTaskSchedule — the end-to-end contract
// ---------------------------------------------------------------------------

func TestSetTaskSchedule_WritesBothMarkerAndPlannerBlock(t *testing.T) {
	root := newTestVault(t, map[string]string{
		"projects/work.md": "# Work\n\n- [ ] Deploy v2\n- [ ] Write docs\n",
	})
	ref := ScheduleRef{NotePath: "projects/work.md", LineNum: 3, Text: "Deploy v2"}

	if err := SetTaskSchedule(root, "2026-04-18", ref, "09:00", "10:00", "task"); err != nil {
		t.Fatalf("SetTaskSchedule: %v", err)
	}

	// Source marker written
	src := readFile(t, filepath.Join(root, "projects/work.md"))
	if !strings.Contains(src, "⏰ 09:00-10:00") {
		t.Errorf("source file missing ⏰ marker:\n%s", src)
	}

	// Planner block written
	plan := readFile(t, filepath.Join(root, "Planner", "2026-04-18.md"))
	if !strings.Contains(plan, "- 09:00-10:00 | Deploy v2 | task") {
		t.Errorf("planner missing block:\n%s", plan)
	}
}

func TestSetTaskSchedule_Reschedule_DoesNotDuplicate(t *testing.T) {
	root := newTestVault(t, map[string]string{
		"Tasks.md": "# Tasks\n\n- [ ] Meeting\n",
	})
	ref := ScheduleRef{NotePath: "Tasks.md", LineNum: 3, Text: "Meeting"}

	if err := SetTaskSchedule(root, "2026-04-18", ref, "09:00", "10:00", "task"); err != nil {
		t.Fatalf("first schedule: %v", err)
	}
	if err := SetTaskSchedule(root, "2026-04-18", ref, "14:00", "15:00", "task"); err != nil {
		t.Fatalf("re-schedule: %v", err)
	}

	plan := readFile(t, filepath.Join(root, "Planner", "2026-04-18.md"))
	if strings.Count(plan, "| Meeting |") != 1 {
		t.Errorf("expected 1 Meeting block, got:\n%s", plan)
	}
	if !strings.Contains(plan, "- 14:00-15:00 | Meeting | task") {
		t.Errorf("expected re-scheduled block at 14:00, got:\n%s", plan)
	}

	// Source marker should also be the new time, not the old one.
	src := readFile(t, filepath.Join(root, "Tasks.md"))
	if !strings.Contains(src, "⏰ 14:00-15:00") {
		t.Errorf("source marker not updated:\n%s", src)
	}
	if strings.Contains(src, "⏰ 09:00-10:00") {
		t.Errorf("stale old marker still present:\n%s", src)
	}
}

func TestSetTaskSchedule_SortsBlocksByStart(t *testing.T) {
	root := newTestVault(t, map[string]string{
		"Tasks.md": "# Tasks\n\n- [ ] A\n- [ ] B\n- [ ] C\n",
	})
	// Insert out of time order.
	_ = SetTaskSchedule(root, "2026-04-18",
		ScheduleRef{NotePath: "Tasks.md", LineNum: 5, Text: "C"}, "15:00", "16:00", "task")
	_ = SetTaskSchedule(root, "2026-04-18",
		ScheduleRef{NotePath: "Tasks.md", LineNum: 3, Text: "A"}, "09:00", "10:00", "task")
	_ = SetTaskSchedule(root, "2026-04-18",
		ScheduleRef{NotePath: "Tasks.md", LineNum: 4, Text: "B"}, "11:00", "12:00", "task")

	plan := readFile(t, filepath.Join(root, "Planner", "2026-04-18.md"))
	iA := strings.Index(plan, "| A |")
	iB := strings.Index(plan, "| B |")
	iC := strings.Index(plan, "| C |")
	if iA < 0 || iB < 0 || iC < 0 {
		t.Fatalf("missing blocks:\n%s", plan)
	}
	if !(iA < iB && iB < iC) {
		t.Errorf("blocks not sorted by start time:\n%s", plan)
	}
}

func TestSetTaskSchedule_RejectsInvalidRange(t *testing.T) {
	root := t.TempDir()
	err := SetTaskSchedule(root, "2026-04-18",
		ScheduleRef{Text: "x"}, "10:00", "09:00", "task")
	if err == nil {
		t.Error("expected error for end-before-start, got nil")
	}
}

// ---------------------------------------------------------------------------
// ClearTaskSchedule
// ---------------------------------------------------------------------------

func TestClearTaskSchedule_RemovesBoth(t *testing.T) {
	root := newTestVault(t, map[string]string{
		"Tasks.md": "# Tasks\n\n- [ ] Meeting\n",
	})
	ref := ScheduleRef{NotePath: "Tasks.md", LineNum: 3, Text: "Meeting"}

	if err := SetTaskSchedule(root, "2026-04-18", ref, "09:00", "10:00", "task"); err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if err := ClearTaskSchedule(root, "2026-04-18", ref); err != nil {
		t.Fatalf("clear: %v", err)
	}

	src := readFile(t, filepath.Join(root, "Tasks.md"))
	if strings.Contains(src, "⏰") {
		t.Errorf("⏰ marker not removed:\n%s", src)
	}
	plan := readFile(t, filepath.Join(root, "Planner", "2026-04-18.md"))
	if strings.Contains(plan, "Meeting") {
		t.Errorf("planner block not removed:\n%s", plan)
	}
}

func TestClearTaskSchedule_IsIdempotent(t *testing.T) {
	root := newTestVault(t, map[string]string{
		"Tasks.md": "# Tasks\n\n- [ ] Meeting\n",
	})
	ref := ScheduleRef{NotePath: "Tasks.md", LineNum: 3, Text: "Meeting"}
	// Clearing an unscheduled task must not error.
	if err := ClearTaskSchedule(root, "2026-04-18", ref); err != nil {
		t.Errorf("clear on unscheduled task: %v", err)
	}
}

// ---------------------------------------------------------------------------
// upsert/remove planner-block primitives
// ---------------------------------------------------------------------------

func TestUpsertPlannerBlock_PreservesOtherSections(t *testing.T) {
	root := newTestVault(t, map[string]string{
		"Planner/2026-04-18.md": "---\ndate: 2026-04-18\n---\n\n## Focus\n- Top goal: Ship\n- Review PR\n\n## Schedule\n- 09:00-10:00 | Old | task\n",
	})

	err := UpsertPlannerBlock(root, "2026-04-18", ScheduleRef{Text: "New"}, PlannerBlock{
		Date: "2026-04-18", StartTime: "11:00", EndTime: "12:00", Text: "New", BlockType: "task",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	content := readFile(t, filepath.Join(root, "Planner", "2026-04-18.md"))
	if !strings.Contains(content, "## Focus") {
		t.Errorf("focus section lost:\n%s", content)
	}
	if !strings.Contains(content, "- Top goal: Ship") {
		t.Errorf("focus content lost:\n%s", content)
	}
	if !strings.Contains(content, "- 09:00-10:00 | Old | task") {
		t.Errorf("old block lost:\n%s", content)
	}
	if !strings.Contains(content, "- 11:00-12:00 | New | task") {
		t.Errorf("new block missing:\n%s", content)
	}
}

func TestReadPlannerScheduleBlocks_SkipsMalformedLines(t *testing.T) {
	root := newTestVault(t, map[string]string{
		"Planner/2026-04-18.md": "## Schedule\n- 09:00-10:00 | Good | task\n- malformed line\n- 11:00 | no range | task\n- 13:00-14:00 | Another | break\n",
	})
	blocks := readPlannerScheduleBlocks(root, "2026-04-18")
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2: %+v", len(blocks), blocks)
	}
	if blocks[0].Text != "Good" || blocks[1].Text != "Another" {
		t.Errorf("unexpected blocks: %+v", blocks)
	}
}

// ---------------------------------------------------------------------------
// writeTaskScheduleMarker — precise vs. text fallback
// ---------------------------------------------------------------------------

func TestWriteTaskScheduleMarker_PreciseRef_TargetsExactLine(t *testing.T) {
	// Two tasks with identical text on different lines — precise ref must
	// pick the right one.
	root := newTestVault(t, map[string]string{
		"notes.md": "- [ ] Dup\n- [ ] Dup\n",
	})
	ref := ScheduleRef{NotePath: "notes.md", LineNum: 2, Text: "Dup"}
	if err := writeTaskScheduleMarker(root, ref, "09:00", "10:00"); err != nil {
		t.Fatalf("%v", err)
	}
	content := readFile(t, filepath.Join(root, "notes.md"))
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("unexpected file:\n%s", content)
	}
	if strings.Contains(lines[0], "⏰") {
		t.Errorf("line 1 was modified but shouldn't have been: %q", lines[0])
	}
	if !strings.Contains(lines[1], "⏰ 09:00-10:00") {
		t.Errorf("line 2 missing marker: %q", lines[1])
	}
}

// ---------------------------------------------------------------------------
// SourceRef round-trip — precise match when task text collides
// ---------------------------------------------------------------------------

func TestSetTaskSchedule_RoundTripsSourceRef(t *testing.T) {
	root := newTestVault(t, map[string]string{
		"projects/work.md": "- [ ] Deploy\n",
	})
	ref := ScheduleRef{NotePath: "projects/work.md", LineNum: 1, Text: "Deploy"}
	if err := SetTaskSchedule(root, "2026-04-18", ref, "09:00", "10:00", "task"); err != nil {
		t.Fatal(err)
	}
	plan := readFile(t, filepath.Join(root, "Planner", "2026-04-18.md"))
	if !strings.Contains(plan, "@projects/work.md:1") {
		t.Errorf("planner did not round-trip source ref:\n%s", plan)
	}
	// Re-parse and confirm the block carries the ref back.
	blocks := readPlannerScheduleBlocks(root, "2026-04-18")
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	if blocks[0].SourceRef.NotePath != "projects/work.md" || blocks[0].SourceRef.LineNum != 1 {
		t.Errorf("parsed ref mismatch: %+v", blocks[0].SourceRef)
	}
}

func TestSetTaskSchedule_PreciseMatch_WhenTextIsDuplicated(t *testing.T) {
	// Two notes both contain a task with identical text. Scheduling each one
	// must produce two distinct planner blocks — text matching alone would
	// collapse them.
	root := newTestVault(t, map[string]string{
		"projects/a.md": "- [ ] Review\n",
		"projects/b.md": "- [ ] Review\n",
	})
	refA := ScheduleRef{NotePath: "projects/a.md", LineNum: 1, Text: "Review"}
	refB := ScheduleRef{NotePath: "projects/b.md", LineNum: 1, Text: "Review"}
	if err := SetTaskSchedule(root, "2026-04-18", refA, "09:00", "10:00", "task"); err != nil {
		t.Fatal(err)
	}
	if err := SetTaskSchedule(root, "2026-04-18", refB, "14:00", "15:00", "task"); err != nil {
		t.Fatal(err)
	}
	blocks := readPlannerScheduleBlocks(root, "2026-04-18")
	if len(blocks) != 2 {
		plan := readFile(t, filepath.Join(root, "Planner", "2026-04-18.md"))
		t.Fatalf("got %d blocks, want 2:\n%s", len(blocks), plan)
	}
}

func TestParseScheduleBlockLine_BackCompatWithoutRef(t *testing.T) {
	// Existing planner files written before SourceRef must still parse.
	b, ok := parseScheduleBlockLine("- 09:00-10:00 | Task | task", "2026-04-18")
	if !ok {
		t.Fatal("parse failed")
	}
	if b.SourceRef.hasLocation() {
		t.Errorf("expected empty SourceRef, got %+v", b.SourceRef)
	}
	if b.Done {
		t.Errorf("expected Done=false")
	}

	// done flag still works
	b, _ = parseScheduleBlockLine("- 09:00-10:00 | Task | task | done", "2026-04-18")
	if !b.Done {
		t.Error("done flag lost")
	}

	// done + ref in either order
	b, _ = parseScheduleBlockLine("- 09:00-10:00 | Task | task | done | @notes.md:5", "2026-04-18")
	if !b.Done || b.SourceRef.NotePath != "notes.md" || b.SourceRef.LineNum != 5 {
		t.Errorf("done+ref combo: %+v", b)
	}
	b, _ = parseScheduleBlockLine("- 09:00-10:00 | Task | task | @notes.md:5 | done", "2026-04-18")
	if !b.Done || b.SourceRef.NotePath != "notes.md" || b.SourceRef.LineNum != 5 {
		t.Errorf("ref+done combo: %+v", b)
	}
}

// ---------------------------------------------------------------------------
// CurrentPlannerBlock — powers Pomodoro's "start for current block" flow
// ---------------------------------------------------------------------------

func TestCurrentPlannerBlock_FindsOverlappingBlock(t *testing.T) {
	root := newTestVault(t, map[string]string{
		"Planner/2026-04-18.md": "## Schedule\n- 09:00-10:00 | Morning task | task\n- 11:00-12:00 | Deep work | deep-work\n",
	})
	// 11:30 → should match the second block.
	got := CurrentPlannerBlock(root, "2026-04-18", 11*60+30)
	if got == nil {
		t.Fatal("expected a block, got nil")
	}
	if got.Text != "Deep work" {
		t.Errorf("expected Deep work, got %q", got.Text)
	}
}

func TestCurrentPlannerBlock_NilBetweenBlocks(t *testing.T) {
	root := newTestVault(t, map[string]string{
		"Planner/2026-04-18.md": "## Schedule\n- 09:00-10:00 | A | task\n- 11:00-12:00 | B | task\n",
	})
	// 10:30 falls in the gap.
	if got := CurrentPlannerBlock(root, "2026-04-18", 10*60+30); got != nil {
		t.Errorf("expected nil in gap, got %+v", got)
	}
}

func TestCurrentPlannerBlock_EndIsExclusive(t *testing.T) {
	// Exactly-at-end must not match — a 09:00-10:00 block is over at 10:00.
	root := newTestVault(t, map[string]string{
		"Planner/2026-04-18.md": "## Schedule\n- 09:00-10:00 | A | task\n",
	})
	if got := CurrentPlannerBlock(root, "2026-04-18", 10*60); got != nil {
		t.Errorf("end of range should be exclusive, got %+v", got)
	}
}

func TestWriteTaskScheduleMarker_StaleRef_DoesNotError(t *testing.T) {
	root := newTestVault(t, map[string]string{
		"notes.md": "- [ ] Only\n",
	})
	// Line num beyond file length — should be a no-op, not an error.
	ref := ScheduleRef{NotePath: "notes.md", LineNum: 99, Text: "Only"}
	if err := writeTaskScheduleMarker(root, ref, "09:00", "10:00"); err != nil {
		t.Errorf("stale ref should be no-op, got: %v", err)
	}
}
