package serveapi

import (
	"strings"
	"testing"
	"time"
)

func TestBuildDailyPlan_AllSections(t *testing.T) {
	now := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	got := buildDailyPlan(MorningSaveBody{
		Scripture: struct {
			Text   string `json:"text"`
			Source string `json:"source"`
		}{Text: "Iron sharpens iron", Source: "Proverbs 27:17"},
		Goal:     "Ship the wizard",
		Tasks:    []string{"task one", "task two"},
		Habits:   []string{"morning movement", "read 20 pages"},
		Thoughts: "feeling focused",
	}, now)

	for _, want := range []string{
		"## Daily Plan — Friday, May 1, 2026",
		`> *"Iron sharpens iron"* — Proverbs 27:17`,
		"### Today's Goal",
		"**Ship the wizard**",
		"### Tasks",
		"- task one",
		"- task two",
		"### Habits",
		"- [ ] morning movement",
		"- [ ] read 20 pages",
		"### Thoughts",
		"feeling focused",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("buildDailyPlan output missing %q\n--- got ---\n%s", want, got)
		}
	}
}

func TestBuildDailyPlan_OmitsEmptySections(t *testing.T) {
	got := buildDailyPlan(MorningSaveBody{
		Goal: "ship",
	}, time.Now())
	for _, banned := range []string{"### Tasks", "### Habits", "### Thoughts", "> *"} {
		if strings.Contains(got, banned) {
			t.Errorf("expected section %q to be omitted, got:\n%s", banned, got)
		}
	}
	if !strings.Contains(got, "**ship**") {
		t.Error("expected goal to render bold")
	}
}

func TestUpsertDailyPlan_AppendsWhenAbsent(t *testing.T) {
	raw := "---\nfm\n---\n\n# Title\n\n## Tasks\n- [ ]\n\n## Notes\n"
	plan := "## Daily Plan — Foo\n\nbody\n"
	got := upsertDailyPlan(raw, plan)

	if !strings.Contains(got, "## Tasks") {
		t.Error("Tasks section should be preserved on append")
	}
	if !strings.Contains(got, "## Notes") {
		t.Error("Notes section should be preserved on append")
	}
	if !strings.Contains(got, plan) {
		t.Error("plan should be appended")
	}
	// plan should come after both ## Tasks and ## Notes
	idxTasks := strings.Index(got, "## Tasks")
	idxNotes := strings.Index(got, "## Notes")
	idxPlan := strings.Index(got, "## Daily Plan")
	if !(idxTasks < idxPlan && idxNotes < idxPlan) {
		t.Errorf("plan should be appended last: tasks=%d notes=%d plan=%d", idxTasks, idxNotes, idxPlan)
	}
}

func TestUpsertDailyPlan_ReplacesExistingPlan_PreservesSurroundingSections(t *testing.T) {
	// Original has ## Tasks BEFORE ## Daily Plan, ## Notes AFTER ## Daily Plan
	raw := "---\nfm\n---\n\n# Title\n\n## Tasks\n- [ ] keep\n\n## Daily Plan — Old\nold body\n\n## Notes\nnotes content\n"
	plan := "## Daily Plan — New\n\nnew body\n"
	got := upsertDailyPlan(raw, plan)

	if !strings.Contains(got, "## Tasks") || !strings.Contains(got, "- [ ] keep") {
		t.Errorf("Tasks before Daily Plan should be preserved:\n%s", got)
	}
	if !strings.Contains(got, "## Notes") || !strings.Contains(got, "notes content") {
		t.Errorf("Notes after Daily Plan should be preserved:\n%s", got)
	}
	if strings.Contains(got, "Daily Plan — Old") || strings.Contains(got, "old body") {
		t.Error("old plan should have been replaced")
	}
	if !strings.Contains(got, "Daily Plan — New") || !strings.Contains(got, "new body") {
		t.Errorf("new plan should be present:\n%s", got)
	}
}

func TestUpsertDailyPlan_ReplacesPlanAtEOF(t *testing.T) {
	// ## Daily Plan is the LAST section; nothing after.
	raw := "---\nfm\n---\n\n# Title\n\n## Tasks\n- [ ] keep\n\n## Daily Plan — Old\nold body\nmore old\n"
	plan := "## Daily Plan — New\nnew body\n"
	got := upsertDailyPlan(raw, plan)

	if !strings.Contains(got, "## Tasks") {
		t.Error("Tasks before should be preserved")
	}
	if strings.Contains(got, "old body") {
		t.Errorf("old plan body should be gone:\n%s", got)
	}
	if !strings.Contains(got, "new body") {
		t.Errorf("new plan body should be present:\n%s", got)
	}
	// Idempotence: running it again with the same plan should be a no-op (or close to)
	got2 := upsertDailyPlan(got, plan)
	if got != got2 {
		t.Errorf("idempotence failure:\nfirst:  %q\nsecond: %q", got, got2)
	}
}

func TestUpsertDailyPlan_DoesNotEatNotesAtEnd(t *testing.T) {
	// Regression: the "section runs to EOF" branch must NOT consume a
	// later top-level heading that follows the plan.
	raw := "## Daily Plan — Old\nold\n\n## After\nshould stay\n"
	got := upsertDailyPlan(raw, "## Daily Plan — New\nnew\n")
	if !strings.Contains(got, "## After") || !strings.Contains(got, "should stay") {
		t.Errorf("trailing heading was eaten:\n%s", got)
	}
}
