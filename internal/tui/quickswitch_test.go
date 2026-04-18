package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// stubModTime returns a function that maps paths to fixed times for testing.
func stubModTime(times map[string]time.Time) func(string) time.Time {
	return func(path string) time.Time {
		if t, ok := times[path]; ok {
			return t
		}
		return time.Time{}
	}
}

// ---------------------------------------------------------------------------
// Constructor & Initial State
// ---------------------------------------------------------------------------

func TestQuickSwitch_NewQuickSwitch(t *testing.T) {
	qs := NewQuickSwitch()
	if qs.active {
		t.Error("expected inactive after construction")
	}
	if qs.cursor != 0 {
		t.Error("expected cursor=0")
	}
	if len(qs.items) != 0 {
		t.Error("expected no items")
	}
	if qs.result != "" {
		t.Error("expected empty result")
	}
}

// ---------------------------------------------------------------------------
// Open / Close / IsActive
// ---------------------------------------------------------------------------

func TestQuickSwitch_OpenCloseIsActive(t *testing.T) {
	qs := NewQuickSwitch()
	if qs.IsActive() {
		t.Error("should be inactive before Open")
	}

	now := time.Now()
	modTimes := map[string]time.Time{
		"notes/a.md": now.Add(-1 * time.Hour),
		"notes/b.md": now.Add(-2 * time.Hour),
	}
	qs.Open(nil, nil, []string{"notes/a.md", "notes/b.md"}, stubModTime(modTimes))

	if !qs.IsActive() {
		t.Error("should be active after Open")
	}
	if len(qs.items) != 2 {
		t.Errorf("expected 2 items, got %d", len(qs.items))
	}

	qs.Close()
	if qs.IsActive() {
		t.Error("should be inactive after Close")
	}
}

// ---------------------------------------------------------------------------
// SetSize
// ---------------------------------------------------------------------------

func TestQuickSwitch_SetSize(t *testing.T) {
	qs := NewQuickSwitch()
	qs.SetSize(120, 40)
	if qs.width != 120 {
		t.Errorf("expected width=120, got %d", qs.width)
	}
	if qs.height != 40 {
		t.Errorf("expected height=40, got %d", qs.height)
	}
}

// ---------------------------------------------------------------------------
// Open builds items: starred first, then recent, then remaining by modtime
// ---------------------------------------------------------------------------

func TestQuickSwitch_OpenOrderStarredRecentRemaining(t *testing.T) {
	now := time.Now()
	modTimes := map[string]time.Time{
		"notes/starred.md":  now.Add(-10 * time.Minute),
		"notes/recent.md":   now.Add(-5 * time.Minute),
		"notes/old.md":      now.Add(-48 * time.Hour),
		"notes/newer.md":    now.Add(-1 * time.Hour),
	}

	qs := NewQuickSwitch()
	qs.Open(
		[]string{"notes/recent.md"},                                                  // recent
		[]string{"notes/starred.md"},                                                 // starred
		[]string{"notes/starred.md", "notes/recent.md", "notes/old.md", "notes/newer.md"}, // all
		stubModTime(modTimes),
	)

	if len(qs.items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(qs.items))
	}

	// First item should be starred
	if qs.items[0].path != "notes/starred.md" {
		t.Errorf("expected starred file first, got %s", qs.items[0].path)
	}
	if !qs.items[0].starred {
		t.Error("starred file should have starred=true")
	}

	// Second item should be recent
	if qs.items[1].path != "notes/recent.md" {
		t.Errorf("expected recent file second, got %s", qs.items[1].path)
	}

	// Remaining sorted by mod time (newer first): newer.md before old.md
	if qs.items[2].path != "notes/newer.md" {
		t.Errorf("expected newer.md third, got %s", qs.items[2].path)
	}
	if qs.items[3].path != "notes/old.md" {
		t.Errorf("expected old.md fourth, got %s", qs.items[3].path)
	}
}

func TestQuickSwitch_OpenNoDuplicates(t *testing.T) {
	now := time.Now()
	modTimes := map[string]time.Time{
		"notes/a.md": now,
	}

	qs := NewQuickSwitch()
	// Same file in starred, recent, and all
	qs.Open(
		[]string{"notes/a.md"},
		[]string{"notes/a.md"},
		[]string{"notes/a.md"},
		stubModTime(modTimes),
	)

	if len(qs.items) != 1 {
		t.Errorf("expected 1 item (no duplicates), got %d", len(qs.items))
	}
}

// ---------------------------------------------------------------------------
// Navigation — up/down
// ---------------------------------------------------------------------------

func TestQuickSwitch_NavigationUpDown(t *testing.T) {
	now := time.Now()
	paths := []string{"notes/a.md", "notes/b.md", "notes/c.md"}
	modTimes := map[string]time.Time{
		"notes/a.md": now.Add(-1 * time.Hour),
		"notes/b.md": now.Add(-2 * time.Hour),
		"notes/c.md": now.Add(-3 * time.Hour),
	}

	qs := NewQuickSwitch()
	qs.Open(nil, nil, paths, stubModTime(modTimes))

	if qs.cursor != 0 {
		t.Error("cursor should start at 0")
	}

	// Down
	qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyDown})
	if qs.cursor != 1 {
		t.Errorf("expected cursor=1, got %d", qs.cursor)
	}

	qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyDown})
	if qs.cursor != 2 {
		t.Errorf("expected cursor=2, got %d", qs.cursor)
	}

	// Cannot go past last
	qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyDown})
	if qs.cursor != 2 {
		t.Errorf("should stay at 2, got %d", qs.cursor)
	}

	// Up
	qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyUp})
	if qs.cursor != 1 {
		t.Errorf("expected cursor=1, got %d", qs.cursor)
	}

	qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyUp})
	if qs.cursor != 0 {
		t.Errorf("expected cursor=0, got %d", qs.cursor)
	}

	// Cannot go above first
	qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyUp})
	if qs.cursor != 0 {
		t.Errorf("should stay at 0, got %d", qs.cursor)
	}
}

func TestQuickSwitch_NavigationCtrlJK(t *testing.T) {
	// Arrow keys + Ctrl+J/K + Ctrl+N/P navigate; raw j/k fall into the
	// query so users can search for files containing those letters
	// (e.g. "jot", "kanban").
	now := time.Now()
	modTimes := map[string]time.Time{
		"a.md": now,
		"b.md": now.Add(-time.Hour),
	}
	qs := NewQuickSwitch()
	qs.Open(nil, nil, []string{"a.md", "b.md"}, stubModTime(modTimes))

	qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if qs.cursor != 1 {
		t.Errorf("expected cursor=1 after Ctrl+J, got %d", qs.cursor)
	}

	qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if qs.cursor != 0 {
		t.Errorf("expected cursor=0 after Ctrl+K, got %d", qs.cursor)
	}
}

func TestQuickSwitch_RawJKTypeIntoQuery(t *testing.T) {
	now := time.Now()
	modTimes := map[string]time.Time{
		"jot.md":    now,
		"kanban.md": now.Add(-time.Hour),
		"other.md":  now.Add(-2 * time.Hour),
	}
	qs := NewQuickSwitch()
	qs.Open(nil, nil, []string{"jot.md", "kanban.md", "other.md"}, stubModTime(modTimes))

	qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if qs.query != "j" {
		t.Errorf("expected query 'j', got %q", qs.query)
	}
	if len(qs.items) == 0 || qs.items[0].path != "jot.md" {
		t.Errorf("expected jot.md to top the filtered list, got %+v", qs.items)
	}
}

// ---------------------------------------------------------------------------
// Selection — enter returns selected file
// ---------------------------------------------------------------------------

func TestQuickSwitch_SelectionEnter(t *testing.T) {
	now := time.Now()
	modTimes := map[string]time.Time{
		"notes/alpha.md": now,
		"notes/beta.md":  now.Add(-time.Hour),
	}

	qs := NewQuickSwitch()
	qs.Open(nil, nil, []string{"notes/alpha.md", "notes/beta.md"}, stubModTime(modTimes))

	// Move to second item
	qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Press Enter
	qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if qs.IsActive() {
		t.Error("should be inactive after selection")
	}

	sel := qs.SelectedFile()
	if sel != "notes/beta.md" {
		t.Errorf("expected 'notes/beta.md', got '%s'", sel)
	}

	// SelectedFile resets
	if qs.SelectedFile() != "" {
		t.Error("SelectedFile should be empty after first read")
	}
}

func TestQuickSwitch_SelectionFirstItem(t *testing.T) {
	now := time.Now()
	modTimes := map[string]time.Time{
		"notes/only.md": now,
	}
	qs := NewQuickSwitch()
	qs.Open(nil, nil, []string{"notes/only.md"}, stubModTime(modTimes))

	qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyEnter})
	sel := qs.SelectedFile()
	if sel != "notes/only.md" {
		t.Errorf("expected 'notes/only.md', got '%s'", sel)
	}
}

// ---------------------------------------------------------------------------
// Esc and Tab close
// ---------------------------------------------------------------------------

func TestQuickSwitch_EscCloses(t *testing.T) {
	now := time.Now()
	qs := NewQuickSwitch()
	qs.Open(nil, nil, []string{"a.md"}, stubModTime(map[string]time.Time{"a.md": now}))

	qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if qs.IsActive() {
		t.Error("Esc should close quick switch")
	}
}

func TestQuickSwitch_TabCloses(t *testing.T) {
	now := time.Now()
	qs := NewQuickSwitch()
	qs.Open(nil, nil, []string{"a.md"}, stubModTime(map[string]time.Time{"a.md": now}))

	qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyTab})
	if qs.IsActive() {
		t.Error("Tab should close quick switch")
	}
}

// ---------------------------------------------------------------------------
// Empty items — no crash
// ---------------------------------------------------------------------------

func TestQuickSwitch_EmptyItems(t *testing.T) {
	qs := NewQuickSwitch()
	qs.Open(nil, nil, nil, func(s string) time.Time { return time.Time{} })

	if len(qs.items) != 0 {
		t.Error("expected no items")
	}

	// Enter on empty should not panic
	qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if qs.SelectedFile() != "" {
		t.Error("expected empty selection on empty items")
	}

	// Up/Down on empty should not panic
	qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyUp})
	qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyDown})
}

// ---------------------------------------------------------------------------
// Inactive Update is no-op
// ---------------------------------------------------------------------------

func TestQuickSwitch_InactiveUpdateNoop(t *testing.T) {
	qs := NewQuickSwitch()
	qs2, cmd := qs.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("expected nil cmd for inactive update")
	}
	if qs2.active {
		t.Error("should remain inactive")
	}
}

// ---------------------------------------------------------------------------
// Starred flag propagated to recent files
// ---------------------------------------------------------------------------

func TestQuickSwitch_StarredInRecent(t *testing.T) {
	now := time.Now()
	modTimes := map[string]time.Time{
		"notes/a.md": now,
		"notes/b.md": now.Add(-time.Hour),
	}

	qs := NewQuickSwitch()
	// a.md is in both starred and recent
	qs.Open(
		[]string{"notes/a.md", "notes/b.md"}, // recent
		[]string{"notes/a.md"},                // starred
		[]string{"notes/a.md", "notes/b.md"}, // all
		stubModTime(modTimes),
	)

	// First item is starred
	if !qs.items[0].starred {
		t.Error("a.md should be starred")
	}
	// Second item (b.md via recent) is not starred
	if qs.items[1].starred {
		t.Error("b.md should not be starred")
	}
}

// ---------------------------------------------------------------------------
// formatRelativeTime
// ---------------------------------------------------------------------------

func TestQuickSwitch_FormatRelativeTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		t        time.Time
		expected string
	}{
		{"zero", time.Time{}, ""},
		{"just now", now.Add(-10 * time.Second), "just now"},
		{"minutes", now.Add(-5 * time.Minute), "5m ago"},
		{"hours", now.Add(-3 * time.Hour), "3h ago"},
		{"days", now.Add(-2 * 24 * time.Hour), "2d ago"},
		{"weeks", now.Add(-14 * 24 * time.Hour), "2w ago"},
		{"months", now.Add(-60 * 24 * time.Hour), "2mo ago"},
		{"years", now.Add(-400 * 24 * time.Hour), "1y ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatRelativeTime(now, tt.t)
			if got != tt.expected {
				t.Errorf("formatRelativeTime(%s): expected %q, got %q", tt.name, tt.expected, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// View does not panic
// ---------------------------------------------------------------------------

func TestQuickSwitch_ViewNoPanic(t *testing.T) {
	qs := NewQuickSwitch()
	qs.SetSize(120, 40)

	// Empty items
	qs.Open(nil, nil, nil, func(s string) time.Time { return time.Time{} })
	v := qs.View()
	if v == "" {
		t.Error("expected non-empty view even with no items")
	}

	// With items
	now := time.Now()
	qs.Open(nil, nil, []string{"a.md", "b.md"}, stubModTime(map[string]time.Time{
		"a.md": now,
		"b.md": now.Add(-time.Hour),
	}))
	v = qs.View()
	if v == "" {
		t.Error("expected non-empty view with items")
	}
}

func TestQuickSwitch_FuzzyFilterByBasename(t *testing.T) {
	now := time.Now()
	modTimes := map[string]time.Time{
		"projects/granit.md":  now,
		"projects/grafana.md": now,
		"random/notes.md":     now,
	}
	qs := NewQuickSwitch()
	qs.Open(nil, nil, []string{"projects/granit.md", "projects/grafana.md", "random/notes.md"}, stubModTime(modTimes))

	// Type "gran" — both grafana and granit fuzzy-match, granit is the
	// stronger fit (4 contiguous chars at start of basename).
	for _, ch := range "gran" {
		qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	if len(qs.items) < 1 || qs.items[0].path != "projects/granit.md" {
		t.Errorf("expected granit first for 'gran', got %+v", qs.items)
	}
	for _, it := range qs.items {
		if it.path == "random/notes.md" {
			t.Errorf("non-matching item appeared in filtered list: %+v", qs.items)
		}
	}
}

func TestQuickSwitch_BackspaceShrinksQuery(t *testing.T) {
	now := time.Now()
	qs := NewQuickSwitch()
	qs.Open(nil, nil, []string{"alpha.md"}, stubModTime(map[string]time.Time{"alpha.md": now}))

	qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if qs.query != "ab" {
		t.Fatalf("expected query 'ab', got %q", qs.query)
	}
	qs, _ = qs.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if qs.query != "a" {
		t.Errorf("expected query 'a' after backspace, got %q", qs.query)
	}
}
