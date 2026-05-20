package meals

import (
	"strings"
	"testing"
)

func TestParse_EmptyBody(t *testing.T) {
	if got := Parse(""); len(got) != 0 {
		t.Fatalf("expected empty slice, got %v", got)
	}
}

func TestParse_NoMealsSection(t *testing.T) {
	body := "## Habits\n- [ ] 08:00 Read\n\n## Tasks\n- [ ] write the doc\n"
	if got := Parse(body); len(got) != 0 {
		t.Fatalf("expected empty (no Meals section), got %v", got)
	}
}

func TestParse_ThreeSlots(t *testing.T) {
	body := strings.Join([]string{
		"## Meals",
		"- [x] 08:00 Breakfast — Haferflocken",
		"- [x] 12:30 Lunch — Reste Pasta",
		"- [ ] 19:00 Dinner",
		"",
	}, "\n")
	got := Parse(body)
	if len(got) != 3 {
		t.Fatalf("expected 3 slots, got %d", len(got))
	}
	want := []Slot{
		{Time: "08:00", Name: "Breakfast", Done: true, Text: "Haferflocken"},
		{Time: "12:30", Name: "Lunch", Done: true, Text: "Reste Pasta"},
		{Time: "19:00", Name: "Dinner", Done: false, Text: ""},
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("slot %d: got %+v, want %+v", i, got[i], w)
		}
	}
}

func TestParse_StopsAtNextHeading(t *testing.T) {
	body := "## Meals\n- [x] 08:00 Breakfast\n## Notes\n- [ ] 12:30 not a meal\n"
	got := Parse(body)
	if len(got) != 1 {
		t.Fatalf("expected 1 slot (Notes should close section), got %d", len(got))
	}
}

func TestParse_EnDashAlsoSplits(t *testing.T) {
	body := "## Meals\n- [x] 08:00 Breakfast – Eggs\n"
	got := Parse(body)
	if len(got) != 1 || got[0].Name != "Breakfast" || got[0].Text != "Eggs" {
		t.Fatalf("en-dash split broken: %+v", got)
	}
}

func TestMergeWithDefaults_FillsMissing(t *testing.T) {
	parsed := []Slot{
		{Time: "08:00", Name: "Breakfast", Done: true},
	}
	got := MergeWithDefaults(parsed, DefaultSlots())
	if len(got) != 3 {
		t.Fatalf("expected 3 slots (1 parsed + 2 defaults), got %d", len(got))
	}
	if !got[0].Done || got[0].Name != "Breakfast" {
		t.Errorf("parsed slot lost: %+v", got[0])
	}
	if got[1].Name != "Lunch" || got[1].Done {
		t.Errorf("lunch default wrong: %+v", got[1])
	}
	if got[2].Name != "Dinner" || got[2].Done {
		t.Errorf("dinner default wrong: %+v", got[2])
	}
}

func TestMergeWithDefaults_SortByTime(t *testing.T) {
	parsed := []Slot{
		{Time: "22:00", Name: "Late snack", Done: false},
		{Time: "07:00", Name: "Early breakfast", Done: true},
	}
	got := MergeWithDefaults(parsed, nil)
	if got[0].Time != "07:00" || got[1].Time != "22:00" {
		t.Errorf("not sorted by time: %+v", got)
	}
}

func TestRenderSection_RoundTrip(t *testing.T) {
	slots := []Slot{
		{Time: "08:00", Name: "Breakfast", Done: true, Text: "Eggs"},
		{Time: "12:30", Name: "Lunch", Done: false, Text: ""},
	}
	rendered := RenderSection(slots)
	back := Parse(rendered)
	if len(back) != 2 {
		t.Fatalf("round-trip count mismatch: rendered=%q parsed=%v", rendered, back)
	}
	for i, s := range slots {
		if back[i] != s {
			t.Errorf("round-trip slot %d: got %+v, want %+v", i, back[i], s)
		}
	}
}

func TestApplyPatch_TogglesDone(t *testing.T) {
	slots := []Slot{{Time: "08:00", Name: "Breakfast", Done: false}}
	tr := true
	got, changed := ApplyPatch(slots, "08:00", "", &tr, nil)
	if !changed || !got[0].Done {
		t.Fatalf("toggle to done failed: %+v changed=%v", got, changed)
	}
	got2, changed2 := ApplyPatch(got, "08:00", "", &tr, nil)
	if changed2 {
		t.Errorf("no-op patch should not report changed")
	}
	if !got2[0].Done {
		t.Errorf("no-op patch dropped state: %+v", got2)
	}
}

func TestApplyPatch_AddsMissingSlot(t *testing.T) {
	tr := true
	got, changed := ApplyPatch(nil, "12:30", "Lunch", &tr, nil)
	if !changed || len(got) != 1 || !got[0].Done {
		t.Fatalf("missing-slot append failed: %+v changed=%v", got, changed)
	}
}

func TestApplyPatch_DoesNotAddOnEmptyPayload(t *testing.T) {
	got, changed := ApplyPatch(nil, "12:30", "Lunch", nil, nil)
	if changed || len(got) != 0 {
		t.Fatalf("empty patch should be no-op on empty list: %+v changed=%v", got, changed)
	}
}

func TestAggregate(t *testing.T) {
	done, total := Aggregate([]Slot{
		{Done: true}, {Done: false}, {Done: true},
	})
	if done != 2 || total != 3 {
		t.Fatalf("aggregate wrong: %d/%d", done, total)
	}
}

func TestDetectHeading_Missing(t *testing.T) {
	marker, level := DetectHeading("# Daily Note\n\nsome notes\n")
	if marker != "## Meals" || level != 0 {
		t.Fatalf("expected default fallback, got marker=%q level=%d", marker, level)
	}
}

func TestDetectHeading_Level3(t *testing.T) {
	body := "## Tasks\n\n### Meals\n- [ ] 08:00 Breakfast\n"
	marker, level := DetectHeading(body)
	if marker != "### Meals" || level != 3 {
		t.Fatalf("expected level-3, got marker=%q level=%d", marker, level)
	}
}

func TestDetectHeading_CaseFold(t *testing.T) {
	body := "#### meals\n- [ ] 12:30 Lunch\n"
	marker, level := DetectHeading(body)
	if marker != "#### meals" || level != 4 {
		t.Fatalf("case-fold detection broke: %q %d", marker, level)
	}
}

func TestRewriteHeadingLevel(t *testing.T) {
	in := "## Meals\n- [x] 08:00 Breakfast\n"
	cases := []struct {
		level int
		want  string
	}{
		{0, in},                                    // no-op
		{2, in},                                    // identity
		{3, "### Meals\n- [x] 08:00 Breakfast\n"},  // bump
		{4, "#### Meals\n- [x] 08:00 Breakfast\n"}, // higher
	}
	for _, c := range cases {
		got := RewriteHeadingLevel(in, c.level)
		if got != c.want {
			t.Errorf("level=%d: got %q, want %q", c.level, got, c.want)
		}
	}
}

func TestWriteSection_PreservesFreeFormNotesInsideSection(t *testing.T) {
	// The whole point of the line-preserving rewriter: a user can
	// jot a thought inside the Meals section without it being eaten
	// the next time they tick a row.
	body := strings.Join([]string{
		"## Meals",
		"- [ ] 08:00 Breakfast",
		"feeling pretty hungry this morning",
		"- [ ] 12:30 Lunch",
		"- [ ] 19:00 Dinner",
		"",
		"## Notes",
		"random thoughts",
		"",
	}, "\n")

	parsed := Parse(body)
	tr := true
	updated, _ := ApplyPatch(parsed, "08:00", "Breakfast", &tr, nil)
	out := WriteSection(body, updated)

	if !strings.Contains(out, "feeling pretty hungry this morning") {
		t.Errorf("free-form note inside section was dropped: %q", out)
	}
	if !strings.Contains(out, "- [x] 08:00 Breakfast") {
		t.Errorf("breakfast tick not written: %q", out)
	}
	if !strings.Contains(out, "## Notes\nrandom thoughts") {
		t.Errorf("tail content outside section corrupted: %q", out)
	}
}

func TestWriteSection_NoSection_AppendsFresh(t *testing.T) {
	body := "# Today\n\nsome content\n"
	out := WriteSection(body, []Slot{
		{Time: "08:00", Name: "Breakfast", Done: true},
	})
	if !strings.Contains(out, "## Meals\n- [x] 08:00 Breakfast\n") {
		t.Errorf("missing-section append failed: %q", out)
	}
	if !strings.Contains(out, "some content") {
		t.Errorf("existing content lost: %q", out)
	}
}

func TestWriteSection_AppendsNewSlotsBeforeNextHeading(t *testing.T) {
	// User has Breakfast + Lunch, ticks Dinner (which wasn't yet
	// materialised). Dinner should land inside the Meals section,
	// not after the next heading.
	body := strings.Join([]string{
		"## Meals",
		"- [x] 08:00 Breakfast",
		"- [x] 12:30 Lunch",
		"",
		"## Notes",
		"keep this safe",
		"",
	}, "\n")
	tr := true
	updated, _ := ApplyPatch(Parse(body), "19:00", "Dinner", &tr, nil)
	out := WriteSection(body, updated)

	mealsIdx := strings.Index(out, "## Meals")
	notesIdx := strings.Index(out, "## Notes")
	dinnerIdx := strings.Index(out, "Dinner")
	if dinnerIdx < mealsIdx || dinnerIdx > notesIdx {
		t.Errorf("Dinner not placed inside Meals section: %q", out)
	}
	if !strings.Contains(out, "keep this safe") {
		t.Errorf("Notes section corrupted: %q", out)
	}
}

func TestWriteSection_PreservesUnusualHeadingLevel(t *testing.T) {
	body := "### Meals\n- [ ] 08:00 Breakfast\n"
	tr := true
	updated, _ := ApplyPatch(Parse(body), "08:00", "", &tr, nil)
	out := WriteSection(body, updated)
	if !strings.HasPrefix(out, "### Meals\n") {
		t.Errorf("heading level not preserved: %q", out)
	}
	// Whole-line check — substring "## Meals" would false-positive
	// inside "### Meals".
	for _, line := range strings.Split(out, "\n") {
		if line == "## Meals" {
			t.Errorf("duplicate ## Meals heading appeared: %q", out)
		}
	}
}

func TestRoundTrip_PatchPreservesAcrossExistingSection(t *testing.T) {
	// Simulate: user has a daily note with `### Meals` already.
	// We parse → patch → render with the existing level → upsert
	// should rewrite in place, not append a new section.
	body := strings.Join([]string{
		"# Daily 2026-05-20",
		"",
		"## Tasks",
		"- [ ] write the doc",
		"",
		"### Meals",
		"- [ ] 08:00 Breakfast",
		"- [ ] 12:30 Lunch",
		"- [ ] 19:00 Dinner",
		"",
		"## Notes",
		"random thoughts",
		"",
	}, "\n")

	parsed := Parse(body)
	if len(parsed) != 3 {
		t.Fatalf("expected 3 parsed slots, got %d", len(parsed))
	}

	tr := true
	updated, changed := ApplyPatch(parsed, "08:00", "", &tr, nil)
	if !changed || !updated[0].Done {
		t.Fatalf("breakfast toggle failed: %+v", updated)
	}

	rendered := RenderSection(updated)
	marker, level := DetectHeading(body)
	if level != 3 {
		t.Fatalf("heading detection should return level 3, got %d", level)
	}
	rendered = RewriteHeadingLevel(rendered, level)
	if !strings.HasPrefix(rendered, "### Meals\n") {
		t.Errorf("rewritten section missing level-3 prefix: %q", rendered)
	}
	// Whole-line check — substring would false-positive because
	// "### Meals" trivially contains "## Meals".
	for _, line := range strings.Split(rendered, "\n") {
		if line == "## Meals" {
			t.Errorf("rewritten section still carries default ## marker line")
		}
	}
	_ = marker
}

