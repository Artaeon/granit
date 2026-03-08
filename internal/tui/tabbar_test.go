package tui

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Basic Operations
// ---------------------------------------------------------------------------

func TestNewTabBar(t *testing.T) {
	tb := NewTabBar()
	if tb == nil {
		t.Fatal("NewTabBar returned nil")
	}
	if len(tb.tabs) != 0 {
		t.Errorf("expected 0 tabs, got %d", len(tb.tabs))
	}
	if tb.maxTabs != 8 {
		t.Errorf("expected maxTabs=8, got %d", tb.maxTabs)
	}
	if tb.activeIdx != -1 {
		t.Errorf("expected activeIdx=-1, got %d", tb.activeIdx)
	}
	if active := tb.GetActive(); active != "" {
		t.Errorf("expected empty active path, got %q", active)
	}
}

func TestAddTab(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("notes/hello.md")

	tabs := tb.Tabs()
	if len(tabs) != 1 {
		t.Fatalf("expected 1 tab, got %d", len(tabs))
	}
	if tabs[0].Path != "notes/hello.md" {
		t.Errorf("expected path %q, got %q", "notes/hello.md", tabs[0].Path)
	}
	if tb.GetActive() != "notes/hello.md" {
		t.Errorf("expected active tab to be the added tab, got %q", tb.GetActive())
	}
	if tabs[0].Modified {
		t.Error("new tab should not be modified")
	}
	if tabs[0].Pinned {
		t.Error("new tab should not be pinned")
	}
}

func TestAddTabDuplicate(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.AddTab("b.md")
	tb.AddTab("a.md") // duplicate — should just activate

	tabs := tb.Tabs()
	if len(tabs) != 2 {
		t.Fatalf("expected 2 tabs (no duplicate), got %d", len(tabs))
	}
	if tb.GetActive() != "a.md" {
		t.Errorf("expected active tab %q, got %q", "a.md", tb.GetActive())
	}
	if tb.activeIdx != 0 {
		t.Errorf("expected activeIdx=0, got %d", tb.activeIdx)
	}
}

func TestAddTabOverflowEvictsOldestUnpinned(t *testing.T) {
	tb := NewTabBar()
	tb.maxTabs = 3

	tb.AddTab("a.md")
	tb.AddTab("b.md")
	tb.AddTab("c.md")
	// Now at capacity. Adding a fourth should evict "a.md" (oldest unpinned).
	tb.AddTab("d.md")

	if tb.HasTab("a.md") {
		t.Error("expected a.md to be evicted")
	}
	if !tb.HasTab("b.md") || !tb.HasTab("c.md") || !tb.HasTab("d.md") {
		t.Error("expected b.md, c.md, d.md to remain")
	}
	if tb.GetActive() != "d.md" {
		t.Errorf("expected active tab d.md, got %q", tb.GetActive())
	}
	if len(tb.Tabs()) != 3 {
		t.Errorf("expected 3 tabs, got %d", len(tb.Tabs()))
	}
}

func TestAddTabOverflowSkipsPinned(t *testing.T) {
	tb := NewTabBar()
	tb.maxTabs = 3

	tb.AddTab("a.md")
	tb.PinTab("a.md")
	tb.AddTab("b.md")
	tb.AddTab("c.md")
	// At capacity. Adding d.md should evict "b.md" (oldest unpinned), not "a.md" (pinned).
	tb.AddTab("d.md")

	if !tb.HasTab("a.md") {
		t.Error("pinned tab a.md should not be evicted")
	}
	if tb.HasTab("b.md") {
		t.Error("expected b.md to be evicted")
	}
	if !tb.HasTab("c.md") || !tb.HasTab("d.md") {
		t.Error("expected c.md and d.md to remain")
	}
}

func TestAddTabOverflowAllPinned(t *testing.T) {
	tb := NewTabBar()
	tb.maxTabs = 2

	tb.AddTab("a.md")
	tb.PinTab("a.md")
	tb.AddTab("b.md")
	tb.PinTab("b.md")

	// All tabs pinned — should not add a new one.
	tb.AddTab("c.md")

	if tb.HasTab("c.md") {
		t.Error("should not add tab when all existing tabs are pinned and at capacity")
	}
	if len(tb.Tabs()) != 2 {
		t.Errorf("expected 2 tabs, got %d", len(tb.Tabs()))
	}
}

func TestRemoveTab(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.AddTab("b.md")
	tb.AddTab("c.md")

	tb.RemoveTab("b.md")
	if tb.HasTab("b.md") {
		t.Error("b.md should be removed")
	}
	if len(tb.Tabs()) != 2 {
		t.Errorf("expected 2 tabs, got %d", len(tb.Tabs()))
	}
}

func TestRemoveTabPinnedNotRemoved(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.PinTab("a.md")

	tb.RemoveTab("a.md")
	if !tb.HasTab("a.md") {
		t.Error("pinned tab should not be removed via RemoveTab")
	}
}

func TestRemoveTabNonexistent(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")

	// Should not panic.
	tb.RemoveTab("nonexistent.md")
	if len(tb.Tabs()) != 1 {
		t.Errorf("expected 1 tab, got %d", len(tb.Tabs()))
	}
}

func TestRemoveTabAdjustsActiveIdx(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.AddTab("b.md")
	tb.AddTab("c.md")
	// Active is c.md (idx=2). Remove b.md (idx=1) — active should adjust to idx=1.
	tb.RemoveTab("b.md")
	if tb.GetActive() != "c.md" {
		t.Errorf("expected active c.md after removing b.md, got %q", tb.GetActive())
	}
}

func TestRemoveTabLastTabSetsActiveNegative(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.RemoveTab("a.md")

	if tb.GetActive() != "" {
		t.Errorf("expected empty active after removing last tab, got %q", tb.GetActive())
	}
	if tb.activeIdx != -1 {
		t.Errorf("expected activeIdx=-1, got %d", tb.activeIdx)
	}
}

func TestSetActive(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.AddTab("b.md")
	tb.AddTab("c.md")

	tb.SetActive("a.md")
	if tb.GetActive() != "a.md" {
		t.Errorf("expected active a.md, got %q", tb.GetActive())
	}
}

func TestSetActiveNonexistent(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.AddTab("b.md")
	// Active is b.md. Setting to nonexistent should keep active unchanged.
	tb.SetActive("nope.md")
	if tb.GetActive() != "b.md" {
		t.Errorf("expected active b.md unchanged, got %q", tb.GetActive())
	}
}

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------

func TestNextTab(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.AddTab("b.md")
	tb.AddTab("c.md")
	// Active is c.md (idx=2). NextTab should wrap to a.md (idx=0).
	path := tb.NextTab()
	if path != "a.md" {
		t.Errorf("expected a.md, got %q", path)
	}
	// Next should go to b.md.
	path = tb.NextTab()
	if path != "b.md" {
		t.Errorf("expected b.md, got %q", path)
	}
}

func TestPrevTab(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.AddTab("b.md")
	tb.AddTab("c.md")
	// Active is c.md (idx=2). PrevTab should go to b.md.
	path := tb.PrevTab()
	if path != "b.md" {
		t.Errorf("expected b.md, got %q", path)
	}
}

func TestPrevTabWraparound(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.AddTab("b.md")
	tb.SetActive("a.md")
	// Active is a.md (idx=0). PrevTab should wrap to b.md (idx=1).
	path := tb.PrevTab()
	if path != "b.md" {
		t.Errorf("expected b.md on wraparound, got %q", path)
	}
}

func TestNextTabSingleTab(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("only.md")

	path := tb.NextTab()
	if path != "only.md" {
		t.Errorf("expected only.md, got %q", path)
	}
}

func TestPrevTabSingleTab(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("only.md")

	path := tb.PrevTab()
	if path != "only.md" {
		t.Errorf("expected only.md, got %q", path)
	}
}

func TestNextTabEmpty(t *testing.T) {
	tb := NewTabBar()
	if path := tb.NextTab(); path != "" {
		t.Errorf("expected empty string, got %q", path)
	}
}

func TestPrevTabEmpty(t *testing.T) {
	tb := NewTabBar()
	if path := tb.PrevTab(); path != "" {
		t.Errorf("expected empty string, got %q", path)
	}
}

func TestMoveLeft(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.AddTab("b.md")
	tb.AddTab("c.md")
	// Active is c.md (idx=2). Move left.
	ok := tb.MoveLeft()
	if !ok {
		t.Error("expected MoveLeft to return true")
	}
	tabs := tb.Tabs()
	if tabs[1].Path != "c.md" || tabs[2].Path != "b.md" {
		t.Errorf("expected c.md at idx 1 and b.md at idx 2, got %v", tabs)
	}
	if tb.GetActive() != "c.md" {
		t.Errorf("expected active c.md, got %q", tb.GetActive())
	}
}

func TestMoveRight(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.AddTab("b.md")
	tb.AddTab("c.md")
	tb.SetActive("a.md")
	// Active is a.md (idx=0). Move right.
	ok := tb.MoveRight()
	if !ok {
		t.Error("expected MoveRight to return true")
	}
	tabs := tb.Tabs()
	if tabs[0].Path != "b.md" || tabs[1].Path != "a.md" {
		t.Errorf("expected b.md at idx 0 and a.md at idx 1, got %v", tabs)
	}
	if tb.GetActive() != "a.md" {
		t.Errorf("expected active a.md, got %q", tb.GetActive())
	}
}

func TestMoveLeftAtStart(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.AddTab("b.md")
	tb.SetActive("a.md")

	ok := tb.MoveLeft()
	if ok {
		t.Error("MoveLeft at start should return false")
	}
	// Tab order should be unchanged.
	tabs := tb.Tabs()
	if tabs[0].Path != "a.md" || tabs[1].Path != "b.md" {
		t.Errorf("tab order should be unchanged, got %v", tabs)
	}
}

func TestMoveRightAtEnd(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.AddTab("b.md")
	// Active is b.md (last position, idx=1).
	ok := tb.MoveRight()
	if ok {
		t.Error("MoveRight at end should return false")
	}
	tabs := tb.Tabs()
	if tabs[0].Path != "a.md" || tabs[1].Path != "b.md" {
		t.Errorf("tab order should be unchanged, got %v", tabs)
	}
}

func TestMoveLeftSingleTab(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	if tb.MoveLeft() {
		t.Error("MoveLeft with single tab should return false")
	}
}

func TestMoveRightSingleTab(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	if tb.MoveRight() {
		t.Error("MoveRight with single tab should return false")
	}
}

// ---------------------------------------------------------------------------
// Pin System
// ---------------------------------------------------------------------------

func TestPinTab(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.PinTab("a.md")

	tabs := tb.Tabs()
	if !tabs[0].Pinned {
		t.Error("expected tab to be pinned")
	}
}

func TestUnpinTab(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.PinTab("a.md")
	tb.UnpinTab("a.md")

	tabs := tb.Tabs()
	if tabs[0].Pinned {
		t.Error("expected tab to be unpinned")
	}
}

func TestPinnedTabSurvivesEviction(t *testing.T) {
	tb := NewTabBar()
	tb.maxTabs = 3

	tb.AddTab("a.md")
	tb.PinTab("a.md")
	tb.AddTab("b.md")
	tb.AddTab("c.md")
	// Full. Adding d.md should evict b.md (first unpinned), not a.md (pinned).
	tb.AddTab("d.md")

	if !tb.HasTab("a.md") {
		t.Error("pinned a.md should survive eviction")
	}
	if tb.HasTab("b.md") {
		t.Error("unpinned b.md should be evicted")
	}
}

func TestPinTabNonexistent(t *testing.T) {
	tb := NewTabBar()
	// Should not panic.
	tb.PinTab("nonexistent.md")
	tb.UnpinTab("nonexistent.md")
}

// ---------------------------------------------------------------------------
// Modified Tracking
// ---------------------------------------------------------------------------

func TestSetModifiedTrue(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.SetModified("a.md", true)

	tabs := tb.Tabs()
	if !tabs[0].Modified {
		t.Error("expected tab to be marked modified")
	}
}

func TestSetModifiedFalse(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.SetModified("a.md", true)
	tb.SetModified("a.md", false)

	tabs := tb.Tabs()
	if tabs[0].Modified {
		t.Error("expected modified flag cleared")
	}
}

func TestSetModifiedNonexistent(t *testing.T) {
	tb := NewTabBar()
	// Should not panic.
	tb.SetModified("nope.md", true)
}

func TestModifiedIndicatorInRender(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("test.md")
	tb.SetModified("test.md", true)

	rendered := tb.Render(80, "test.md")
	// The modified indicator uses a styled "*" character, which should appear
	// in the rendered output. We check for presence of "test" (the basename).
	if !strings.Contains(rendered, "test") {
		t.Error("expected rendered output to contain tab name")
	}
}

// ---------------------------------------------------------------------------
// Close Operations
// ---------------------------------------------------------------------------

func TestCloseActive(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.AddTab("b.md")
	tb.AddTab("c.md")
	tb.SetActive("b.md")

	newActive := tb.CloseActive()
	if tb.HasTab("b.md") {
		t.Error("b.md should be closed")
	}
	// After closing b.md (idx=1), c.md was at idx=2 and shifts to idx=1.
	// The active index stays at 1 so the new active should be c.md.
	if newActive != "c.md" {
		t.Errorf("expected new active c.md, got %q", newActive)
	}
	if len(tb.Tabs()) != 2 {
		t.Errorf("expected 2 tabs, got %d", len(tb.Tabs()))
	}
}

func TestCloseActiveLastTab(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")

	newActive := tb.CloseActive()
	if newActive != "" {
		t.Errorf("expected empty string after closing last tab, got %q", newActive)
	}
	if len(tb.Tabs()) != 0 {
		t.Errorf("expected 0 tabs, got %d", len(tb.Tabs()))
	}
	if tb.activeIdx != -1 {
		t.Errorf("expected activeIdx=-1, got %d", tb.activeIdx)
	}
}

func TestCloseActivePinnedTabStays(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.PinTab("a.md")

	newActive := tb.CloseActive()
	if newActive != "a.md" {
		t.Errorf("expected a.md (pinned cannot be closed), got %q", newActive)
	}
	if !tb.HasTab("a.md") {
		t.Error("pinned tab should remain after CloseActive")
	}
}

func TestCloseActiveWithMultiple(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.AddTab("b.md")
	tb.AddTab("c.md")
	// Active is c.md (last). Closing should make b.md active.
	newActive := tb.CloseActive()
	if newActive != "b.md" {
		t.Errorf("expected b.md after closing last tab, got %q", newActive)
	}
}

func TestCloseActiveEmpty(t *testing.T) {
	tb := NewTabBar()
	result := tb.CloseActive()
	if result != "" {
		t.Errorf("expected empty string for empty tab bar, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

func TestRenderNoTabs(t *testing.T) {
	tb := NewTabBar()
	rendered := tb.Render(80, "")
	if rendered == "" {
		t.Error("expected non-empty render even with no tabs")
	}
	// Should contain the underline separator.
	if !strings.Contains(rendered, "─") {
		t.Error("expected underline in empty tab bar render")
	}
}

func TestRenderSingleTab(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("notes/hello.md")
	rendered := tb.Render(80, "notes/hello.md")
	if !strings.Contains(rendered, "hello") {
		t.Errorf("expected tab name 'hello' in rendered output, got: %q", rendered)
	}
}

func TestRenderMultipleTabs(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.AddTab("b.md")
	tb.AddTab("c.md")
	rendered := tb.Render(200, "b.md")

	for _, name := range []string{"a", "b", "c"} {
		if !strings.Contains(rendered, name) {
			t.Errorf("expected %q in rendered output", name)
		}
	}
}

func TestRenderActiveTabHighlighted(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.AddTab("b.md")
	rendered := tb.Render(80, "a.md")

	// The active tab's underline uses "━" (heavy rule), while inactive uses "─".
	if !strings.Contains(rendered, "━") {
		t.Error("expected heavy underline for active tab")
	}
	if !strings.Contains(rendered, "─") {
		t.Error("expected dim underline for inactive tab")
	}
}

func TestRenderPinnedIndicator(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("pinned.md")
	tb.PinTab("pinned.md")

	rendered := tb.Render(80, "pinned.md")
	// Pinned tabs show a "*" indicator.
	if !strings.Contains(rendered, "*") {
		t.Error("expected pin indicator '*' in rendered output")
	}
	// Pinned tabs should not have the close "x" indicator.
	if strings.Contains(rendered, " x") {
		t.Error("pinned tab should not show close indicator")
	}
}

func TestRenderUnpinnedHasCloseIndicator(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("open.md")
	rendered := tb.Render(80, "open.md")
	if !strings.Contains(rendered, "x") {
		t.Error("unpinned tab should show close indicator 'x'")
	}
}

func TestRenderModifiedIndicator(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("dirty.md")
	tb.SetModified("dirty.md", true)

	rendered := tb.Render(80, "dirty.md")
	// Modified shows a styled "*" dot.
	if !strings.Contains(rendered, "*") {
		t.Error("expected modified indicator '*' in rendered output")
	}
}

func TestRenderOverflow(t *testing.T) {
	tb := NewTabBar()
	tb.maxTabs = 20
	// Add many tabs that won't all fit in a narrow width.
	for i := 0; i < 15; i++ {
		tb.AddTab("note" + tbItoa(i) + ".md")
	}
	tb.SetActive("note0.md")
	rendered := tb.Render(60, "note0.md")

	// Should contain the overflow indicator "+N".
	if !strings.Contains(rendered, "+") {
		t.Error("expected overflow indicator '+N' in narrow render")
	}
}

func TestRenderTwoLineOutput(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	rendered := tb.Render(80, "a.md")
	lines := strings.Split(rendered, "\n")
	if len(lines) < 2 {
		t.Errorf("expected at least 2 lines (tabs + underline), got %d", len(lines))
	}
}

// ---------------------------------------------------------------------------
// Edge Cases
// ---------------------------------------------------------------------------

func TestTabPathsWithSpaces(t *testing.T) {
	tb := NewTabBar()
	path := "my notes/hello world.md"
	tb.AddTab(path)

	if !tb.HasTab(path) {
		t.Error("expected tab with spaces in path to be added")
	}
	if tb.GetActive() != path {
		t.Errorf("expected active %q, got %q", path, tb.GetActive())
	}
	rendered := tb.Render(80, path)
	if !strings.Contains(rendered, "hello world") {
		t.Error("expected space-containing name in rendered output")
	}
}

func TestTabPathsDeepNesting(t *testing.T) {
	tb := NewTabBar()
	path := "vault/area/topic/subtopic/deeply/nested/note.md"
	tb.AddTab(path)

	// tbBaseName should extract just "note" (without extension).
	rendered := tb.Render(80, path)
	if !strings.Contains(rendered, "note") {
		t.Error("expected basename 'note' in rendered output for deep path")
	}
}

func TestEmptyStringPath(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("")

	if len(tb.Tabs()) != 1 {
		t.Errorf("expected 1 tab, got %d", len(tb.Tabs()))
	}
	if !tb.HasTab("") {
		t.Error("expected empty-string tab to be findable")
	}
}

func TestManyTabs(t *testing.T) {
	tb := NewTabBar()
	tb.maxTabs = 100

	for i := 0; i < 25; i++ {
		tb.AddTab("note" + tbItoa(i) + ".md")
	}

	if len(tb.Tabs()) != 25 {
		t.Errorf("expected 25 tabs, got %d", len(tb.Tabs()))
	}
	// Render should not panic.
	rendered := tb.Render(120, "note12.md")
	if rendered == "" {
		t.Error("expected non-empty render with many tabs")
	}
}

func TestManyTabsDefaultMaxEviction(t *testing.T) {
	tb := NewTabBar()
	// Default maxTabs=8. Add 20 tabs — should evict down to 8.
	for i := 0; i < 20; i++ {
		tb.AddTab("file" + tbItoa(i) + ".md")
	}
	if len(tb.Tabs()) != 8 {
		t.Errorf("expected 8 tabs after eviction, got %d", len(tb.Tabs()))
	}
	// Most recent tabs should survive.
	if !tb.HasTab("file19.md") {
		t.Error("expected most recent tab to survive")
	}
}

// ---------------------------------------------------------------------------
// TabBar helper functions
// ---------------------------------------------------------------------------

func TestTbBaseName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"notes/hello.md", "hello"},
		{"deep/path/to/file.txt", "file"},
		{"noext", "noext"},
		{"multi.dots.name.md", "multi.dots.name"},
		{"", ""},
		{"just-a-file.md", "just-a-file"},
	}
	for _, tt := range tests {
		got := tbBaseName(tt.path)
		if got != tt.want {
			t.Errorf("tbBaseName(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestTbTruncName(t *testing.T) {
	tests := []struct {
		name   string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is a long name", 10, "this is a\u2026"},
		{"abc", 3, "abc"},
		{"abcd", 3, "ab\u2026"},
		{"", 5, ""},
	}
	for _, tt := range tests {
		got := tbTruncName(tt.name, tt.maxLen)
		if got != tt.want {
			t.Errorf("tbTruncName(%q, %d) = %q, want %q", tt.name, tt.maxLen, got, tt.want)
		}
	}
}

func TestTbItoa(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{-5, "-5"},
		{100, "100"},
		{999, "999"},
	}
	for _, tt := range tests {
		got := tbItoa(tt.n)
		if got != tt.want {
			t.Errorf("tbItoa(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// HasTab / Tabs copy isolation
// ---------------------------------------------------------------------------

func TestHasTab(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")

	if !tb.HasTab("a.md") {
		t.Error("expected HasTab to return true for existing tab")
	}
	if tb.HasTab("nope.md") {
		t.Error("expected HasTab to return false for non-existing tab")
	}
}

func TestTabsCopyIsolation(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")

	tabs := tb.Tabs()
	tabs[0].Path = "modified.md"
	tabs[0].Pinned = true

	// The original tab bar should not be affected.
	if tb.Tabs()[0].Path != "a.md" {
		t.Error("modifying Tabs() return value should not affect TabBar internals")
	}
	if tb.Tabs()[0].Pinned {
		t.Error("modifying Tabs() return value should not affect pinned status")
	}
}

// ---------------------------------------------------------------------------
// Combined workflows
// ---------------------------------------------------------------------------

func TestWorkflowOpenEditSaveClose(t *testing.T) {
	tb := NewTabBar()

	// Open a file.
	tb.AddTab("notes/project.md")
	if tb.GetActive() != "notes/project.md" {
		t.Fatal("expected active tab to be project.md")
	}

	// Mark as modified.
	tb.SetModified("notes/project.md", true)
	if !tb.Tabs()[0].Modified {
		t.Fatal("expected modified flag")
	}

	// Save — clear modified.
	tb.SetModified("notes/project.md", false)
	if tb.Tabs()[0].Modified {
		t.Fatal("expected modified flag cleared after save")
	}

	// Close.
	result := tb.CloseActive()
	if result != "" {
		t.Errorf("expected empty string after closing last tab, got %q", result)
	}
}

func TestWorkflowNavigateAndReorder(t *testing.T) {
	tb := NewTabBar()
	tb.AddTab("a.md")
	tb.AddTab("b.md")
	tb.AddTab("c.md")
	tb.AddTab("d.md")

	// Navigate backward twice from d.md.
	tb.PrevTab() // c.md
	tb.PrevTab() // b.md
	if tb.GetActive() != "b.md" {
		t.Fatalf("expected b.md, got %q", tb.GetActive())
	}

	// Move b.md to the left.
	tb.MoveLeft()
	tabs := tb.Tabs()
	if tabs[0].Path != "b.md" || tabs[1].Path != "a.md" {
		t.Errorf("expected [b, a, c, d], got [%s, %s, %s, %s]",
			tabs[0].Path, tabs[1].Path, tabs[2].Path, tabs[3].Path)
	}

	// Navigate forward to wrap around.
	tb.NextTab() // a.md
	tb.NextTab() // c.md
	tb.NextTab() // d.md
	tb.NextTab() // b.md (wraparound)
	if tb.GetActive() != "b.md" {
		t.Errorf("expected wraparound to b.md, got %q", tb.GetActive())
	}
}
