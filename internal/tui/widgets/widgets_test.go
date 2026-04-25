package widgets

import (
	"strings"
	"testing"
	"time"

	"github.com/artaeon/granit/internal/profiles"
	"github.com/artaeon/granit/internal/tasks"
)

// allBuiltins is a small helper for tests that need to iterate
// every shipped widget.
func allBuiltins() []Widget { return builtinWidgets() }

func TestRegisterBuiltins_RegistersTen(t *testing.T) {
	r := NewRegistry()
	if err := RegisterBuiltins(r); err != nil {
		t.Fatal(err)
	}
	if got := len(r.IDs()); got != 10 {
		t.Errorf("expected 10 built-in widgets, got %d (%v)", got, r.IDs())
	}
}

func TestRegistry_GetUnknownReturnsErr(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Get("not-a-widget"); err == nil {
		t.Error("expected error for unknown widget ID")
	}
}

func TestRegister_RejectsNilAndEmptyID(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(nil); err == nil {
		t.Error("expected error for nil widget")
	}
}

func TestEveryWidget_RendersAtMinSize(t *testing.T) {
	ctx := WidgetCtx{Config: map[string]any{}}
	for _, w := range allBuiltins() {
		t.Run(w.ID(), func(t *testing.T) {
			cols, rows := w.MinSize()
			out := w.Render(ctx, cols, rows)
			if out == "" {
				t.Errorf("%s: render returned empty string at min size", w.ID())
			}
		})
	}
}

func TestEveryWidget_RendersAtLargeSize(t *testing.T) {
	ctx := WidgetCtx{Config: map[string]any{}}
	for _, w := range allBuiltins() {
		t.Run(w.ID(), func(t *testing.T) {
			out := w.Render(ctx, 120, 30)
			if out == "" {
				t.Errorf("%s: render returned empty string at large size", w.ID())
			}
		})
	}
}

func TestEveryWidget_HandleKeyDoesNotPanicOnUnknownKey(t *testing.T) {
	ctx := WidgetCtx{Config: map[string]any{}}
	for _, w := range allBuiltins() {
		t.Run(w.ID(), func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: panicked on unknown key: %v", w.ID(), r)
				}
			}()
			handled, _ := w.HandleKey(ctx, "f99")
			if handled {
				t.Errorf("%s: marked unknown key as handled", w.ID())
			}
		})
	}
}

func TestEveryWidget_DataNeedsAreValidEnumValues(t *testing.T) {
	valid := map[profiles.DataKind]bool{
		profiles.DataTasks: true, profiles.DataCalendar: true,
		profiles.DataHabits: true, profiles.DataGoals: true,
		profiles.DataNotes: true, profiles.DataPomodoro: true,
		profiles.DataPlanner: true, profiles.DataScripture: true,
		profiles.DataBusinessPulse: true, profiles.DataTriage: true,
	}
	for _, w := range allBuiltins() {
		t.Run(w.ID(), func(t *testing.T) {
			for _, k := range w.DataNeeds() {
				if !valid[k] {
					t.Errorf("%s: DataNeeds includes unknown DataKind %q", w.ID(), k)
				}
			}
		})
	}
}

func TestTodayTasksWidget_FilterDueByDate(t *testing.T) {
	all := []tasks.Task{
		{ID: "1", Text: "today", DueDate: "2026-04-25"},
		{ID: "2", Text: "tomorrow", DueDate: "2026-04-26"},
		{ID: "3", Text: "yesterday", DueDate: "2026-04-24"},
		{ID: "4", Text: "done today", Done: true, DueDate: "2026-04-25"},
	}
	due := filterDueByDate(all, "2026-04-25")
	if len(due) != 1 || due[0].ID != "1" {
		t.Errorf("expected only id=1, got %+v", due)
	}
}

func TestTodayOverdueWidget_FilterOverdue(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	all := []tasks.Task{
		{ID: "1", Text: "yesterday", DueDate: "2026-04-24"},
		{ID: "2", Text: "today", DueDate: "2026-04-25"}, // not overdue
		{ID: "3", Text: "tomorrow", DueDate: "2026-04-26"},
		{ID: "4", Text: "done", Done: true, DueDate: "2026-04-20"},
	}
	overdue := filterOverdue(all, now)
	if len(overdue) != 1 || overdue[0].ID != "1" {
		t.Errorf("expected only id=1, got %+v", overdue)
	}
}

func TestTriageCountWidget_RendersZeroAsHealthy(t *testing.T) {
	w := newTriageCountWidget()
	out := w.Render(WidgetCtx{TriageInbox: 0, Config: map[string]any{}}, 12, 3)
	if !strings.Contains(out, "0") {
		t.Errorf("expected zero in output, got %q", out)
	}
}

func TestSparkline_RendersAtTargetWidth(t *testing.T) {
	samples := []BusinessSample{
		{Label: "mon", Value: 1},
		{Label: "tue", Value: 5},
		{Label: "wed", Value: 3},
		{Label: "thu", Value: 8},
	}
	out := sparkline(samples, 4)
	if len([]rune(out)) != 4 {
		t.Errorf("expected 4 runes, got %d (%q)", len([]rune(out)), out)
	}
}

func TestProgressBar_ClampsExtremes(t *testing.T) {
	bar1 := progressBar(-0.5, 10)
	bar2 := progressBar(1.5, 10)
	if !strings.Contains(bar1, "░") || strings.Contains(bar1, "█") {
		t.Errorf("negative pct should render empty: %q", bar1)
	}
	if !strings.Contains(bar2, "█") || strings.Contains(bar2, "░") {
		t.Errorf("over-1 pct should render full: %q", bar2)
	}
}

func TestTodayJotWidget_EnterCallsCreateTask(t *testing.T) {
	called := ""
	ctx := WidgetCtx{
		Config: map[string]any{"buffer": "  buy milk  "},
		CreateTask: func(text string) error {
			called = text
			return nil
		},
	}
	w := newTodayJotWidget()
	handled, _ := w.HandleKey(ctx, "enter")
	if !handled {
		t.Error("enter should be handled")
	}
	if called != "buy milk" {
		t.Errorf("expected trimmed text, got %q", called)
	}
}

func TestTodayJotWidget_BackspaceShortensBuffer(t *testing.T) {
	ctx := WidgetCtx{Config: map[string]any{"buffer": "abc"}}
	w := newTodayJotWidget()
	w.HandleKey(ctx, "backspace")
	if got := ctx.Config["buffer"]; got != "ab" {
		t.Errorf("backspace: got %q want %q", got, "ab")
	}
}

func TestRecentNotesWidget_CursorMoves(t *testing.T) {
	ctx := WidgetCtx{
		RecentNotes: []NoteRef{
			{Path: "a.md", Title: "a"}, {Path: "b.md", Title: "b"},
		},
		Config: map[string]any{"cursor": 0},
	}
	w := newRecentNotesWidget()
	w.HandleKey(ctx, "down")
	if ctx.Config["cursor"] != 1 {
		t.Errorf("cursor should be 1 after down, got %v", ctx.Config["cursor"])
	}
	w.HandleKey(ctx, "down") // already at end
	if ctx.Config["cursor"] != 1 {
		t.Errorf("cursor should clamp at 1, got %v", ctx.Config["cursor"])
	}
}
