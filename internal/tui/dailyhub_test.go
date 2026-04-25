package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/artaeon/granit/internal/profiles"
	"github.com/artaeon/granit/internal/tui/widgets"
)

func newTestHub(t *testing.T) *DailyHub {
	t.Helper()
	reg := widgets.NewRegistry()
	if err := widgets.RegisterBuiltins(reg); err != nil {
		t.Fatal(err)
	}
	h := NewDailyHub(reg)
	return &h
}

func TestDailyHub_OpenWithProfileActivates(t *testing.T) {
	h := newTestHub(t)
	p := &profiles.Profile{
		ID:   "test",
		Name: "Test",
		Dashboard: profiles.DashboardSpec{
			Cells: []profiles.DashboardCell{
				{WidgetID: profiles.WidgetTriageCount, Row: 0, Col: 0, ColSpan: 12},
			},
		},
	}
	h.Open(p)
	if !h.IsActive() {
		t.Error("Open did not activate the hub")
	}
}

func TestDailyHub_RendersHeaderWithProfileName(t *testing.T) {
	h := newTestHub(t)
	p := &profiles.Profile{
		ID: "test", Name: "Mocked Profile",
		Dashboard: profiles.DashboardSpec{
			Cells: []profiles.DashboardCell{
				{WidgetID: profiles.WidgetTriageCount, Row: 0, Col: 0, ColSpan: 12},
			},
		},
	}
	h.Open(p)
	out := h.Render(120, 30, widgets.WidgetCtx{Config: map[string]any{}})
	if !strings.Contains(out, "Mocked Profile") {
		t.Errorf("header should contain profile name, got: %q", out[:200])
	}
}

func TestDailyHub_EmptyProfileRendersHint(t *testing.T) {
	h := newTestHub(t)
	p := &profiles.Profile{ID: "test", Name: "Empty"}
	h.Open(p)
	out := h.Render(80, 20, widgets.WidgetCtx{})
	if !strings.Contains(out, "no dashboard") {
		t.Errorf("empty profile should render hint, got: %q", out[:200])
	}
}

func TestDailyHub_UnknownWidgetRendersErrorStub(t *testing.T) {
	h := newTestHub(t)
	p := &profiles.Profile{
		ID: "test", Name: "Test",
		Dashboard: profiles.DashboardSpec{
			Cells: []profiles.DashboardCell{
				{WidgetID: "no.such.widget", Row: 0, Col: 0, ColSpan: 12},
			},
		},
	}
	h.Open(p)
	out := h.Render(120, 20, widgets.WidgetCtx{Config: map[string]any{}})
	if !strings.Contains(out, "missing widget") {
		t.Errorf("unknown widget should render stub, got: %q", out[:300])
	}
}

func TestDailyHub_TabCyclesFocus(t *testing.T) {
	h := newTestHub(t)
	p := &profiles.Profile{
		ID: "test", Name: "Test",
		Dashboard: profiles.DashboardSpec{
			Cells: []profiles.DashboardCell{
				{WidgetID: profiles.WidgetTriageCount, Row: 0, Col: 0, ColSpan: 6},
				{WidgetID: profiles.WidgetRecentNotes, Row: 0, Col: 6, ColSpan: 6},
			},
		},
	}
	h.Open(p)
	if h.focused != 0 {
		t.Errorf("focused should start at 0, got %d", h.focused)
	}
	h.HandleKey("tab", widgets.WidgetCtx{})
	if h.focused != 1 {
		t.Errorf("after tab, focused should be 1, got %d", h.focused)
	}
	h.HandleKey("tab", widgets.WidgetCtx{})
	if h.focused != 0 {
		t.Errorf("tab should wrap to 0, got %d", h.focused)
	}
	h.HandleKey("shift+tab", widgets.WidgetCtx{})
	if h.focused != 1 {
		t.Errorf("shift+tab from 0 should wrap to 1, got %d", h.focused)
	}
}

func TestDailyHub_AltDigitJumpsToCell(t *testing.T) {
	h := newTestHub(t)
	p := &profiles.Profile{
		ID: "test", Name: "Test",
		Dashboard: profiles.DashboardSpec{
			Cells: []profiles.DashboardCell{
				{WidgetID: profiles.WidgetTriageCount, Row: 0, Col: 0, ColSpan: 4},
				{WidgetID: profiles.WidgetRecentNotes, Row: 0, Col: 4, ColSpan: 4},
				{WidgetID: profiles.WidgetGoalProgress, Row: 0, Col: 8, ColSpan: 4},
			},
		},
	}
	h.Open(p)
	h.HandleKey("alt+3", widgets.WidgetCtx{})
	if h.focused != 2 {
		t.Errorf("alt+3 should focus index 2, got %d", h.focused)
	}
	// Out-of-range digit is ignored — focused stays put.
	h.HandleKey("alt+9", widgets.WidgetCtx{})
	if h.focused != 2 {
		t.Errorf("alt+9 with only 3 cells should be ignored, got %d", h.focused)
	}
}

func TestDailyHub_EscClosesAndDoesNotBubble(t *testing.T) {
	h := newTestHub(t)
	p := &profiles.Profile{
		ID: "test", Name: "Test",
		Dashboard: profiles.DashboardSpec{
			Cells: []profiles.DashboardCell{
				{WidgetID: profiles.WidgetTriageCount, Row: 0, Col: 0, ColSpan: 12},
			},
		},
	}
	h.Open(p)
	bubble, _ := h.HandleKey("esc", widgets.WidgetCtx{})
	if bubble {
		t.Error("esc should be consumed by the hub, not bubbled")
	}
	if h.IsActive() {
		t.Error("esc should close the hub")
	}
}

func TestDailyHub_RoutesKeyToFocusedWidget(t *testing.T) {
	h := newTestHub(t)
	p := &profiles.Profile{
		ID: "test", Name: "Test",
		Dashboard: profiles.DashboardSpec{
			Cells: []profiles.DashboardCell{
				{WidgetID: profiles.WidgetTriageCount, Row: 0, Col: 0, ColSpan: 12},
			},
		},
	}
	h.Open(p)
	called := false
	ctx := widgets.WidgetCtx{
		OpenTriage: func() { called = true },
	}
	h.HandleKey("enter", ctx)
	if !called {
		t.Error("enter on focused triage widget should invoke OpenTriage")
	}
}

func TestSortCells_OrdersByRowThenCol(t *testing.T) {
	cells := []profiles.DashboardCell{
		{WidgetID: "c", Row: 1, Col: 5},
		{WidgetID: "a", Row: 0, Col: 0},
		{WidgetID: "b", Row: 0, Col: 6},
		{WidgetID: "d", Row: 1, Col: 0},
	}
	sortCells(cells)
	wantOrder := []string{"a", "b", "d", "c"}
	for i, c := range cells {
		if c.WidgetID != wantOrder[i] {
			t.Errorf("position %d: got %q want %q (full: %v)",
				i, c.WidgetID, wantOrder[i], cells)
		}
	}
}

func TestHumanAgo_FormatsRanges(t *testing.T) {
	cases := []struct {
		seconds int
		want    string
	}{
		{30, "now"},
		{120, "2m"},
		{3600, "1h"},
		{86400, "1d"},
		{86400 * 30, "1mo"},
	}
	for _, c := range cases {
		got := humanAgo(time.Duration(c.seconds) * time.Second)
		if got != c.want {
			t.Errorf("humanAgo(%ds) = %q, want %q", c.seconds, got, c.want)
		}
	}
}
