package serveapi

import (
	"testing"

	"github.com/artaeon/granit/internal/tasks"
)

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
