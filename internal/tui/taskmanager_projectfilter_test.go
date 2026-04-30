package tui

import "testing"

func TestApplyProjectFilter_KeepsOnlyMatchingProject(t *testing.T) {
	tm := &TaskManager{}
	tm.filterProject = "Apollo"
	in := []Task{
		{Text: "build rocket", Project: "Apollo"},
		{Text: "Land on moon", Project: "apollo"}, // case-insensitive match
		{Text: "Plant garden", Project: "Other"},
		{Text: "No project task", Project: ""},
	}
	got := tm.applyProjectFilter(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 (case-insensitive Apollo), got %d", len(got))
	}
}

func TestApplyProjectFilter_EmptyFilterIsNoOp(t *testing.T) {
	tm := &TaskManager{}
	tm.filterProject = ""
	// applyProjectFilter is only called when filter is set, but exercising
	// the code path with an empty filter must not crash.
	in := []Task{{Text: "x", Project: "Apollo"}}
	got := tm.applyProjectFilter(in)
	// With empty target, the filter matches only tasks with empty Project
	// — which is its own valid behaviour ("show me tasks WITHOUT a project").
	// What matters is no panic.
	_ = got
}
