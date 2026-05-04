package serveapi

import "testing"

// TestFindLayout_CaseInsensitive locks the contract that preset
// names are matched case-insensitively. UI buttons render with the
// user's casing (e.g. "Focus") but the user might activate via
// keyboard or URL with a different casing — both should hit the
// same record.
func TestFindLayout_CaseInsensitive(t *testing.T) {
	layouts := []dashboardLayout{
		{Name: "Focus"},
		{Name: "Morning"},
		{Name: "shutdown"},
	}
	cases := []struct {
		query string
		want  int
	}{
		{"Focus", 0},
		{"focus", 0},
		{"FOCUS", 0},
		{"Morning", 1},
		{"morning", 1},
		{"shutdown", 2},
		{"Shutdown", 2},
		{"missing", -1},
		{"", -1},
	}
	for _, c := range cases {
		if got := findLayout(layouts, c.query); got != c.want {
			t.Errorf("findLayout(%q) = %d, want %d", c.query, got, c.want)
		}
	}
}

// TestCloneWidgets_Independence verifies the deep-enough copy
// semantics: mutating the clone (including its Config map) must not
// reach back into the source. This matters because we copy widgets
// into Layouts on save AND copy them out on activate — a shared
// reference would let a later edit silently mutate a saved preset.
func TestCloneWidgets_Independence(t *testing.T) {
	src := []dashboardWidget{
		{
			ID:      "w-1",
			Type:    "today-tasks",
			Enabled: true,
			Config:  map[string]interface{}{"limit": 5},
		},
		{
			ID:      "w-2",
			Type:    "scripture",
			Enabled: false,
			Config:  nil, // nil-config widget — clone must not allocate
		},
	}
	clone := cloneWidgets(src)

	if len(clone) != len(src) {
		t.Fatalf("len(clone)=%d, want %d", len(clone), len(src))
	}

	// Mutate the clone's first widget — top-level field.
	clone[0].Enabled = false
	if !src[0].Enabled {
		t.Errorf("mutating clone[0].Enabled changed src[0].Enabled — clone is shallow")
	}

	// Mutate the clone's first widget's Config map.
	clone[0].Config["limit"] = 99
	if v := src[0].Config["limit"]; v != 5 {
		t.Errorf("mutating clone[0].Config['limit'] changed src — got %v, want 5", v)
	}

	// nil-config widget round-trip.
	if clone[1].Config != nil {
		t.Errorf("clone[1].Config should remain nil, got %v", clone[1].Config)
	}

	// Mutate the clone's slice — adding to clone must not extend src.
	clone = append(clone, dashboardWidget{ID: "w-3"})
	if len(src) != 2 {
		t.Errorf("append to clone leaked into src — len(src)=%d, want 2", len(src))
	}
}
