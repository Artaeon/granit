package serveapi

import (
	"testing"
	"time"

	"github.com/artaeon/granit/internal/tasks"
)

// recordingScheduler is a planScheduler stub that just remembers every
// call. Used to verify that dry-run mode doesn't write — the real
// TaskStore depends on a vault root + sidecar files, which is too much
// machinery for what we want to assert here.
type recordingScheduler struct {
	scheduleCalls []scheduleCall
	triageCalls   []string
}

type scheduleCall struct {
	id    string
	start time.Time
	dur   time.Duration
}

func (r *recordingScheduler) Schedule(id string, start time.Time, dur time.Duration) error {
	r.scheduleCalls = append(r.scheduleCalls, scheduleCall{id, start, dur})
	return nil
}
func (r *recordingScheduler) Triage(id string, _ tasks.TriageState) error {
	r.triageCalls = append(r.triageCalls, id)
	return nil
}

// parsePlanSection — verifies the regex picks up all three dash variants
// the LLM emits non-deterministically (-, –, —) and only inside the
// `## Plan` section. False positives in body text are the most likely
// regression: a stray "- 09:00–10:00 — coffee" in the user's notes
// shouldn't get scheduled.

func TestParsePlanSection_BasicEnDash(t *testing.T) {
	body := `# Today

Some notes.

## Plan
- 09:00–09:55 — Deep work on auth refresh
- 10:00–10:30 — Code review

## After
- 11:00 something else (not a plan line)
`
	blocks := parsePlanSection(body)
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2: %+v", len(blocks), blocks)
	}
	if blocks[0].StartH != 9 || blocks[0].StartM != 0 || blocks[0].EndH != 9 || blocks[0].EndM != 55 {
		t.Errorf("first block times wrong: %+v", blocks[0])
	}
	if blocks[0].Text != "Deep work on auth refresh" {
		t.Errorf("first block text = %q, want %q", blocks[0].Text, "Deep work on auth refresh")
	}
	if blocks[1].StartH != 10 || blocks[1].EndM != 30 {
		t.Errorf("second block times wrong: %+v", blocks[1])
	}
}

func TestParsePlanSection_AllDashVariants(t *testing.T) {
	body := "## Plan\n" +
		"- 09:00-09:30 - hyphen variant\n" +
		"- 10:00–10:30 – en-dash variant\n" +
		"- 11:00—11:30 — em-dash variant\n"
	blocks := parsePlanSection(body)
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want 3", len(blocks))
	}
	wantText := []string{"hyphen variant", "en-dash variant", "em-dash variant"}
	for i, b := range blocks {
		if b.Text != wantText[i] {
			t.Errorf("block[%d].Text = %q, want %q", i, b.Text, wantText[i])
		}
	}
}

func TestParsePlanSection_OnlyInsidePlanHeader(t *testing.T) {
	body := `# Today

- 09:00–10:00 — fake plan line above plan section

## Tasks
- 08:00–09:00 — also fake (wrong section)

## Plan
- 10:00–11:00 — real plan line

## Notes
- 12:00–13:00 — fake again
`
	blocks := parsePlanSection(body)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1: %+v", len(blocks), blocks)
	}
	if blocks[0].Text != "real plan line" {
		t.Errorf("got text %q, want %q", blocks[0].Text, "real plan line")
	}
}

func TestParsePlanSection_HeaderVariants(t *testing.T) {
	cases := []struct {
		name string
		head string
	}{
		{"bare", "## Plan"},
		{"with em-dash subtitle", "## Plan — Friday"},
		{"with en-dash subtitle", "## Plan – Friday"},
		{"daily plan", "## Daily Plan"},
		{"todays plan", "## Today's Plan"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := c.head + "\n- 09:00–10:00 — example\n"
			blocks := parsePlanSection(body)
			if len(blocks) != 1 {
				t.Fatalf("header %q: got %d blocks, want 1", c.head, len(blocks))
			}
		})
	}
}

func TestParsePlanSection_EmptyBody(t *testing.T) {
	if blocks := parsePlanSection(""); len(blocks) != 0 {
		t.Errorf("empty body: got %d blocks, want 0", len(blocks))
	}
}

func TestParsePlanSection_NoPlanSection(t *testing.T) {
	body := `# Notes
- 09:00–10:00 — looks like a plan line but no header
`
	if blocks := parsePlanSection(body); len(blocks) != 0 {
		t.Errorf("no plan header: got %d blocks, want 0", len(blocks))
	}
}

func TestParsePlanSection_IgnoresBlankAndNonMatchingLines(t *testing.T) {
	body := `## Plan

- 09:00–10:00 — first

free-form prose between blocks

- 10:30–11:30 — second
- not a time block
`
	blocks := parsePlanSection(body)
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
}

// fuzzyMatch — exercises the substring/longest-match scoring with the
// kinds of plan lines the LLM actually produces.

func TestFuzzyMatch_FindsSubstringMatch(t *testing.T) {
	candidates := []tasks.Task{
		{ID: "a", Text: "Ship the auth refresh"},
		{ID: "b", Text: "Buy groceries"},
	}
	got := fuzzyMatch("Deep work on auth refresh", candidates)
	if got == nil {
		t.Fatalf("expected a match, got nil")
	}
	if got.ID != "a" {
		t.Errorf("expected match on 'a', got %s (%q)", got.ID, got.Text)
	}
}

func TestFuzzyMatch_NoMatchReturnsNil(t *testing.T) {
	candidates := []tasks.Task{
		{ID: "a", Text: "Buy groceries"},
	}
	got := fuzzyMatch("Watch a movie", candidates)
	if got != nil {
		t.Errorf("expected no match, got %+v", got)
	}
}

func TestFuzzyMatch_CaseInsensitive(t *testing.T) {
	candidates := []tasks.Task{
		{ID: "a", Text: "DEPLOY MIGRATION"},
	}
	got := fuzzyMatch("deploy migration", candidates)
	if got == nil || got.ID != "a" {
		t.Errorf("expected case-insensitive match, got %+v", got)
	}
}

func TestFuzzyMatch_RejectsTooShort(t *testing.T) {
	candidates := []tasks.Task{
		{ID: "a", Text: "Buy groceries"},
	}
	if got := fuzzyMatch("xy", candidates); got != nil {
		t.Errorf("expected nil for too-short input, got %+v", got)
	}
}

// buildPlanProposals — the dry-run contract. The whole point of the
// preview UI is that the user can review proposals BEFORE commit.
// If dry_run=true ever calls Schedule, every drawer surface
// silently shifts task times the moment the user opens it.

func TestBuildPlanProposals_DryRunDoesNotWrite(t *testing.T) {
	blocks := []planBlock{
		{StartH: 9, StartM: 0, EndH: 9, EndM: 30, Text: "ship the auth refresh"},
		{StartH: 10, StartM: 0, EndH: 10, EndM: 30, Text: "code review"},
	}
	candidates := []tasks.Task{
		{ID: "t1", Text: "Ship the auth refresh"},
		{ID: "t2", Text: "Code review the new endpoint"},
	}
	start := time.Date(2026, 5, 3, 0, 0, 0, 0, time.Local)

	rec := &recordingScheduler{}
	scheduled, unmatched, proposals := buildPlanProposals(blocks, candidates, start, true, rec)

	if len(rec.scheduleCalls) != 0 {
		t.Errorf("dry-run called Schedule %d times — must be 0: %+v",
			len(rec.scheduleCalls), rec.scheduleCalls)
	}
	if len(rec.triageCalls) != 0 {
		t.Errorf("dry-run called Triage %d times — must be 0", len(rec.triageCalls))
	}
	if len(scheduled) != 0 {
		t.Errorf("dry-run scheduled list must be empty, got %d", len(scheduled))
	}
	if len(unmatched) != 0 {
		t.Errorf("dry-run had unmatched lines, got %d: %v", len(unmatched), unmatched)
	}
	if len(proposals) != 2 {
		t.Fatalf("dry-run should still build proposals, got %d", len(proposals))
	}
	// Proposals must carry the data the drawer renders.
	if proposals[0].TaskID != "t1" {
		t.Errorf("proposals[0].TaskID=%q want t1", proposals[0].TaskID)
	}
	if proposals[0].DurationMinutes != 30 {
		t.Errorf("proposals[0].DurationMinutes=%d want 30", proposals[0].DurationMinutes)
	}
	if proposals[0].PlanLine == "" {
		t.Error("proposals[0].PlanLine empty — UI tooltip needs the raw line")
	}
	if proposals[0].Reason != "ship the auth refresh" {
		t.Errorf("proposals[0].Reason=%q want raw plan text", proposals[0].Reason)
	}
}

func TestBuildPlanProposals_CommitWrites(t *testing.T) {
	blocks := []planBlock{
		{StartH: 14, StartM: 0, EndH: 15, EndM: 0, Text: "deep work"},
	}
	candidates := []tasks.Task{
		{ID: "t1", Text: "Deep work block"},
	}
	start := time.Date(2026, 5, 3, 0, 0, 0, 0, time.Local)

	rec := &recordingScheduler{}
	scheduled, _, _ := buildPlanProposals(blocks, candidates, start, false, rec)

	if len(rec.scheduleCalls) != 1 {
		t.Fatalf("commit should call Schedule once, got %d", len(rec.scheduleCalls))
	}
	if rec.scheduleCalls[0].id != "t1" {
		t.Errorf("schedule call id=%q want t1", rec.scheduleCalls[0].id)
	}
	if rec.scheduleCalls[0].dur != time.Hour {
		t.Errorf("schedule call dur=%v want 1h", rec.scheduleCalls[0].dur)
	}
	if len(scheduled) != 1 || scheduled[0].TaskID != "t1" {
		t.Errorf("scheduled response wrong: %+v", scheduled)
	}
	if len(rec.triageCalls) != 1 || rec.triageCalls[0] != "t1" {
		t.Errorf("triage call wrong: %+v", rec.triageCalls)
	}
}

func TestBuildPlanProposals_UnmatchedReported(t *testing.T) {
	blocks := []planBlock{
		{StartH: 9, StartM: 0, EndH: 9, EndM: 30, Text: "completely unrelated phrase"},
	}
	candidates := []tasks.Task{
		{ID: "t1", Text: "Buy groceries"},
	}
	start := time.Date(2026, 5, 3, 0, 0, 0, 0, time.Local)

	_, unmatched, proposals := buildPlanProposals(blocks, candidates, start, true, &recordingScheduler{})
	if len(unmatched) != 1 {
		t.Errorf("expected 1 unmatched, got %d", len(unmatched))
	}
	if len(proposals) != 0 {
		t.Errorf("expected 0 proposals when nothing matched, got %d", len(proposals))
	}
}

func TestFormatPlanLine(t *testing.T) {
	got := formatPlanLine(planBlock{StartH: 9, StartM: 5, EndH: 10, EndM: 30, Text: "ship it"})
	want := "- 09:05–10:30 — ship it"
	if got != want {
		t.Errorf("formatPlanLine = %q, want %q", got, want)
	}
}
