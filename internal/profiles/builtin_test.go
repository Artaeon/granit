package profiles

import (
	"encoding/json"
	"testing"
)

// validLayouts lists every layout name string from
// internal/tui/layouts.go. Kept as a string set rather than an
// import to avoid pulling tui into the profiles package; the
// contract is enforced via this fixture which gets reviewed when
// the layout list changes.
var validLayouts = map[string]bool{
	"default": true, "writer": true, "minimal": true, "reading": true,
	"dashboard": true, "zen": true, "research": true, "cornell": true,
	"focus": true, "cockpit": true, "stacked": true, "preview": true,
	"presenter": true, "kanban": true, "widescreen": true,
}

// validWidgets lists the v1 widget IDs ship by commit 4. New
// built-in profiles must reference IDs in this set; Lua-defined
// widgets are out of scope for this test.
var validWidgets = map[string]bool{
	WidgetTodayJot: true, WidgetTodayCalendar: true,
	WidgetTodayTasks: true, WidgetTodayOverdue: true,
	WidgetTriageCount: true, WidgetGoalProgress: true,
	WidgetHabitStreak: true, WidgetRecentNotes: true,
	WidgetScripture: true, WidgetBusinessPulse: true,
}

func TestBuiltinProfiles_AllReferencesResolve(t *testing.T) {
	for _, p := range BuiltinProfiles() {
		if p.DefaultLayout != "" && !validLayouts[p.DefaultLayout] {
			t.Errorf("profile %s: unknown layout %q", p.ID, p.DefaultLayout)
		}
		for _, c := range p.Dashboard.Cells {
			if !validWidgets[c.WidgetID] {
				t.Errorf("profile %s: dashboard cell references unknown widget %q", p.ID, c.WidgetID)
			}
		}
	}
}

func TestBuiltinProfiles_HasClassic(t *testing.T) {
	found := false
	for _, p := range BuiltinProfiles() {
		if p.ID == DefaultProfileID {
			found = true
			if len(p.EnabledModules) != 0 {
				t.Errorf("classic profile must have empty EnabledModules (means 'all on'), got %v", p.EnabledModules)
			}
			break
		}
	}
	if !found {
		t.Errorf("BuiltinProfiles must contain the default profile %q", DefaultProfileID)
	}
}

func TestBuiltinProfiles_NoDuplicateIDs(t *testing.T) {
	seen := make(map[string]bool)
	for _, p := range BuiltinProfiles() {
		if seen[p.ID] {
			t.Errorf("duplicate built-in profile ID: %q", p.ID)
		}
		seen[p.ID] = true
	}
}

func TestBuiltinProfiles_DashboardCellsDoNotOverlap(t *testing.T) {
	// Power users will hand-edit profile JSON. Make sure our
	// shipped examples don't model a hand-overlapped grid since
	// readers will copy them.
	for _, p := range BuiltinProfiles() {
		occupied := make(map[[2]int]string) // (row, col) → widgetID
		for _, c := range p.Dashboard.Cells {
			rowSpan := c.RowSpan
			if rowSpan < 1 {
				rowSpan = 1
			}
			colSpan := c.ColSpan
			if colSpan < 1 {
				colSpan = 1
			}
			if c.Col+colSpan > 12 {
				t.Errorf("profile %s: cell %q extends past col 12 (col=%d span=%d)",
					p.ID, c.WidgetID, c.Col, colSpan)
			}
			for r := c.Row; r < c.Row+rowSpan; r++ {
				for col := c.Col; col < c.Col+colSpan; col++ {
					key := [2]int{r, col}
					if other, taken := occupied[key]; taken {
						t.Errorf("profile %s: cell %q overlaps %q at (%d,%d)",
							p.ID, c.WidgetID, other, r, col)
					}
					occupied[key] = c.WidgetID
				}
			}
		}
	}
}

func TestBuiltinProfiles_JSONRoundTrip(t *testing.T) {
	for _, p := range BuiltinProfiles() {
		data, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			t.Errorf("profile %s: marshal: %v", p.ID, err)
			continue
		}
		var back Profile
		if err := json.Unmarshal(data, &back); err != nil {
			t.Errorf("profile %s: round-trip unmarshal: %v\n%s", p.ID, err, data)
			continue
		}
		if back.ID != p.ID {
			t.Errorf("profile %s: ID lost in round trip: %q", p.ID, back.ID)
		}
		if len(back.Dashboard.Cells) != len(p.Dashboard.Cells) {
			t.Errorf("profile %s: cell count changed in round trip: %d → %d",
				p.ID, len(p.Dashboard.Cells), len(back.Dashboard.Cells))
		}
	}
}

func TestRegisterBuiltins_RegistersAll(t *testing.T) {
	r := New("")
	if err := RegisterBuiltins(r); err != nil {
		t.Fatal(err)
	}
	expected := len(BuiltinProfiles())
	if got := len(r.All()); got != expected {
		t.Errorf("registry has %d profiles after RegisterBuiltins, want %d", got, expected)
	}
	classic, ok := r.Get(DefaultProfileID)
	if !ok {
		t.Fatal("classic not registered")
	}
	if !classic.BuiltIn {
		t.Error("classic should be marked BuiltIn after RegisterBuiltins")
	}
}
