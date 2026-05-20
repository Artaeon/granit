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
