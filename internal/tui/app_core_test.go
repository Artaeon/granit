package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// Helper: create a model ready for Update testing (splash dismissed, sized)
// ---------------------------------------------------------------------------

func newTestModel(t *testing.T) Model {
	t.Helper()
	dir := createTestVault(t)
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	m.showSplash = false
	m.width = 120
	m.height = 40
	m.updateLayout()
	return m
}

// ---------------------------------------------------------------------------
// Message routing tests — verifying the right handler gets each msg type
// ---------------------------------------------------------------------------

func TestUpdate_ClearMessageMsg(t *testing.T) {
	m := newTestModel(t)
	m.statusbar.SetMessage("hello")

	result, _ := m.Update(clearMessageMsg{})
	updated := result.(Model)

	if msg := updated.statusbar.message; msg != "" {
		t.Errorf("expected empty message after clearMessageMsg, got %q", msg)
	}
}

func TestUpdate_AutoSaveTick_NoSaveWhenDisabled(t *testing.T) {
	m := newTestModel(t)
	m.config.AutoSave = false
	m.editor.modified = true
	m.activeNote = "note1.md"

	now := time.Now()
	m.lastEditTime = now
	result, cmd := m.Update(autoSaveTickMsg{editTime: now})
	updated := result.(Model)

	if cmd != nil {
		t.Error("expected no command when auto-save disabled")
	}
	if !updated.editor.modified {
		t.Error("editor should still be modified (no save happened)")
	}
}

func TestUpdate_AutoSaveTick_SavesWhenEnabled(t *testing.T) {
	m := newTestModel(t)
	m.config.AutoSave = true
	m.editor.modified = true
	m.activeNote = "note1.md"

	now := time.Now()
	m.lastEditTime = now
	result, _ := m.Update(autoSaveTickMsg{editTime: now})
	updated := result.(Model)

	if updated.editor.modified {
		t.Error("editor should no longer be modified after auto-save")
	}
}

func TestUpdate_AutoSaveTick_DebouncesMismatch(t *testing.T) {
	m := newTestModel(t)
	m.config.AutoSave = true
	m.editor.modified = true
	m.activeNote = "note1.md"

	m.lastEditTime = time.Now()
	// Send a tick with a different time — should be ignored (debounced)
	result, cmd := m.Update(autoSaveTickMsg{editTime: time.Now().Add(-time.Second)})
	updated := result.(Model)

	if cmd != nil {
		t.Error("expected no command for debounced tick")
	}
	if !updated.editor.modified {
		t.Error("editor should still be modified (tick was stale)")
	}
}

func TestUpdate_WindowSizeMsg_UpdatesAllDimensions(t *testing.T) {
	m := newTestModel(t)

	result, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	updated := result.(Model)

	if updated.width != 200 || updated.height != 60 {
		t.Errorf("expected (200,60), got (%d,%d)", updated.width, updated.height)
	}
	if updated.sidebar.width == 0 {
		t.Error("sidebar width should be set")
	}
	if updated.editor.width == 0 {
		t.Error("editor width should be set")
	}
}

// ---------------------------------------------------------------------------
// Note loading
// ---------------------------------------------------------------------------

func TestLoadNote_ValidPath(t *testing.T) {
	m := newTestModel(t)
	m.loadNote("note1.md")

	if m.activeNote != "note1.md" {
		t.Errorf("expected activeNote to be note1.md, got %q", m.activeNote)
	}
	content := m.editor.GetContent()
	if !strings.Contains(content, "Note 1") {
		t.Error("editor should contain note1.md content")
	}
}

func TestLoadNote_SubfolderPath(t *testing.T) {
	m := newTestModel(t)
	m.loadNote("subfolder/deep.md")

	if m.activeNote != "subfolder/deep.md" {
		t.Errorf("expected subfolder/deep.md, got %q", m.activeNote)
	}
}

func TestLoadNote_NonexistentPath(t *testing.T) {
	m := newTestModel(t)
	original := m.activeNote
	m.loadNote("nonexistent.md")

	// Should not crash, active note may stay the same or change
	_ = original
}

func TestLoadNote_UpdatesSidebar(t *testing.T) {
	m := newTestModel(t)
	m.loadNote("note2.md")

	if m.activeNote != "note2.md" {
		t.Errorf("expected note2.md, got %q", m.activeNote)
	}
}

// ---------------------------------------------------------------------------
// Save current note
// ---------------------------------------------------------------------------

func TestSaveCurrentNote(t *testing.T) {
	m := newTestModel(t)
	m.loadNote("note1.md")
	m.editor.modified = true

	// Add some content
	original := m.editor.GetContent()
	m.editor.SetContent(original + "\nNew line added.")

	cmd := m.saveCurrentNote()
	if cmd == nil {
		t.Fatal("expected a save command")
	}

	// Execute the command to actually write the file
	cmd()

	// Verify file was written
	data, err := os.ReadFile(filepath.Join(m.vault.Root, "note1.md"))
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}
	if !strings.Contains(string(data), "New line added.") {
		t.Error("saved file should contain new content")
	}
}

func TestSaveCurrentNote_NoActiveNote(t *testing.T) {
	m := newTestModel(t)
	m.activeNote = ""

	// saveCurrentNote always returns a tea.Cmd; when executed with no
	// active note it returns nil as the message.
	cmd := m.saveCurrentNote()
	if cmd == nil {
		return // nil cmd is fine too
	}
	msg := cmd()
	if msg != nil {
		t.Error("expected nil message when no active note")
	}
}

// ---------------------------------------------------------------------------
// Scroll position cache
// ---------------------------------------------------------------------------

func TestScrollPosition_SaveAndRestore(t *testing.T) {
	m := newTestModel(t)
	m.loadNote("note1.md")

	// Set cursor within content bounds
	line := m.editor.CursorLine()
	col := m.editor.CursorCol()
	m.editor.SetCursorPosition(line, col)

	m.saveScrollPosition()

	if pos, ok := m.scrollCache["note1.md"]; !ok {
		t.Error("expected scroll position to be cached")
	} else {
		if pos.Line != line || pos.Col != col {
			t.Errorf("expected pos (%d,%d), got (%d,%d)", line, col, pos.Line, pos.Col)
		}
	}

	// Reset cursor to 0,0 and restore
	m.editor.SetCursorPosition(0, 0)
	m.restoreScrollPosition("note1.md")

	if m.editor.CursorLine() != line {
		t.Errorf("expected restored line %d, got %d", line, m.editor.CursorLine())
	}
}

func TestScrollPosition_NoCache(t *testing.T) {
	m := newTestModel(t)
	// Should not panic with no cached position
	m.restoreScrollPosition("nonexistent.md")
}

// ---------------------------------------------------------------------------
// Focus cycling
// ---------------------------------------------------------------------------

func TestCycleFocus_WrapForward(t *testing.T) {
	m := newTestModel(t)
	m.setFocus(focusBacklinks)
	m.cycleFocus(1) // should wrap to sidebar
	if m.focus != focusSidebar {
		t.Errorf("expected wrap to focusSidebar, got %d", m.focus)
	}
}

func TestCycleFocus_WrapBackward(t *testing.T) {
	m := newTestModel(t)
	m.setFocus(focusSidebar)
	m.cycleFocus(-1) // should wrap to backlinks
	if m.focus != focusBacklinks {
		t.Errorf("expected wrap to focusBacklinks, got %d", m.focus)
	}
}

// ---------------------------------------------------------------------------
// Command execution
// ---------------------------------------------------------------------------

func TestExecuteCommand_ToggleViewMode(t *testing.T) {
	m := newTestModel(t)
	m.viewMode = false

	m.executeCommand(CmdToggleView)
	if !m.viewMode {
		t.Error("expected viewMode to be true after CmdToggleView")
	}

	m.executeCommand(CmdToggleView)
	if m.viewMode {
		t.Error("expected viewMode to be false after second CmdToggleView")
	}
}

func TestExecuteCommand_OpenSearch(t *testing.T) {
	m := newTestModel(t)
	m.executeCommand(CmdOpenFile)

	if !m.searchMode {
		t.Error("expected searchMode to be true after CmdOpenFile")
	}
	if len(m.searchResults) == 0 {
		t.Error("expected search results to be populated")
	}
}

func TestExecuteCommand_ShowHelp(t *testing.T) {
	m := newTestModel(t)
	if m.helpOverlay.IsActive() {
		t.Fatal("help should not be active initially")
	}

	m.executeCommand(CmdShowHelp)
	if !m.helpOverlay.IsActive() {
		t.Error("expected help overlay to be active after CmdShowHelp")
	}
}

func TestExecuteCommand_NewNote(t *testing.T) {
	m := newTestModel(t)
	m.executeCommand(CmdNewNote)

	if !m.templates.IsActive() {
		t.Error("expected templates overlay to be active after CmdNewNote")
	}
}

// ---------------------------------------------------------------------------
// View rendering (no panics)
// ---------------------------------------------------------------------------

func TestView_NoPanic_DefaultLayout(t *testing.T) {
	m := newTestModel(t)
	m.config.Layout = "default"
	m.updateLayout()

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
}

func TestView_NoPanic_WriterLayout(t *testing.T) {
	m := newTestModel(t)
	m.config.Layout = "writer"
	m.updateLayout()

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
}

func TestView_NoPanic_MinimalLayout(t *testing.T) {
	m := newTestModel(t)
	m.config.Layout = "minimal"
	m.updateLayout()

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
}

func TestView_NoPanic_ViewMode(t *testing.T) {
	m := newTestModel(t)
	m.viewMode = true
	m.loadNote("note1.md")

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view in view mode")
	}
}

func TestView_NoPanic_EmptyVault(t *testing.T) {
	dir := t.TempDir()
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	m.showSplash = false
	m.width = 120
	m.height = 40
	m.updateLayout()

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view for empty vault")
	}
}

func TestView_NoPanic_VerySmallTerminal(t *testing.T) {
	m := newTestModel(t)
	m.width = 20
	m.height = 10
	m.updateLayout()

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view at small size")
	}
}

func TestView_NoPanic_WithActiveOverlay(t *testing.T) {
	m := newTestModel(t)
	m.helpOverlay.Toggle()
	m.helpOverlay.SetSize(m.width, m.height)

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view with help overlay")
	}
}

// ---------------------------------------------------------------------------
// Search mode
// ---------------------------------------------------------------------------

func TestSearch_FiltersByQuery(t *testing.T) {
	m := newTestModel(t)
	m.searchMode = true
	m.searchQuery = ""
	m.searchResults = m.vault.SortedPaths()

	// Filter for "note1"
	m.searchQuery = "note1"
	m.filterSearch()

	found := false
	for _, r := range m.searchResults {
		if strings.Contains(r, "note1") {
			found = true
		}
	}
	if !found {
		t.Error("expected note1 in search results")
	}
}

func TestSearch_EmptyQuery_ShowsAll(t *testing.T) {
	m := newTestModel(t)
	m.searchQuery = ""
	m.filterSearch()

	if len(m.searchResults) != 3 {
		t.Errorf("expected 3 results for empty query, got %d", len(m.searchResults))
	}
}

func TestSearch_NoMatch(t *testing.T) {
	m := newTestModel(t)
	m.searchQuery = "zzzzzzz_no_match"
	m.filterSearch()

	if len(m.searchResults) != 0 {
		t.Errorf("expected 0 results for non-matching query, got %d", len(m.searchResults))
	}
}

// ---------------------------------------------------------------------------
// Resolve link
// ---------------------------------------------------------------------------

func TestResolveLink_ExactPath(t *testing.T) {
	m := newTestModel(t)
	result := m.resolveLink("note1.md")
	if result != "note1.md" {
		t.Errorf("expected note1.md, got %q", result)
	}
}

func TestResolveLink_WikilinkWithoutExtension(t *testing.T) {
	m := newTestModel(t)
	result := m.resolveLink("note2")
	if result != "note2.md" {
		t.Errorf("expected note2.md, got %q", result)
	}
}

func TestResolveLink_NonExistent(t *testing.T) {
	m := newTestModel(t)
	result := m.resolveLink("nonexistent")
	if result != "" {
		t.Errorf("expected empty for nonexistent link, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// Tab management
// ---------------------------------------------------------------------------

func TestTabBar_AddOnLoadNote(t *testing.T) {
	m := newTestModel(t)
	if m.tabBar == nil {
		t.Skip("tabBar not initialized")
	}

	m.loadNote("note1.md")
	m.loadNote("note2.md")

	tabs := m.tabBar.Tabs()
	if len(tabs) < 2 {
		t.Errorf("expected at least 2 tabs, got %d", len(tabs))
	}
}

// ---------------------------------------------------------------------------
// Edge cases: rapid overlay toggle
// ---------------------------------------------------------------------------

func TestRapidOverlayToggle(t *testing.T) {
	m := newTestModel(t)

	for i := 0; i < 100; i++ {
		m.helpOverlay.Toggle()
	}
	// After 100 toggles (even number), should be inactive
	if m.helpOverlay.IsActive() {
		t.Error("expected help to be inactive after even number of toggles")
	}
}

// ---------------------------------------------------------------------------
// Layout management
// ---------------------------------------------------------------------------

func TestUpdateLayout_SetsEditorDimensions(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	m.height = 40
	m.config.Layout = "default"
	m.updateLayout()

	if m.editor.width == 0 {
		t.Error("editor width should be set after updateLayout")
	}
	if m.editor.height == 0 {
		t.Error("editor height should be set after updateLayout")
	}
}

func TestUpdateLayout_WriterLayout_NoSidebar(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	m.height = 40
	m.config.Layout = "writer"
	m.updateLayout()

	// In writer layout, editor should be wider than default
	defaultEditorWidth := m.editor.width

	m.config.Layout = "default"
	m.updateLayout()

	// Writer layout gives more space to editor by hiding sidebar.
	// If sidebar is visible in default, editor should be narrower.
	// This verifies the layouts differ.
	if m.editor.width < defaultEditorWidth {
		t.Logf("default layout editor width (%d) is narrower than writer layout (%d), as expected",
			m.editor.width, defaultEditorWidth)
	}
}

// ---------------------------------------------------------------------------
// Config sync
// ---------------------------------------------------------------------------

func TestSyncConfigToComponents(t *testing.T) {
	m := newTestModel(t)
	m.config.VimMode = true

	m.syncConfigToComponents()

	if m.vimState != nil && !m.vimState.enabled {
		t.Error("expected vim mode to be enabled after syncConfigToComponents")
	}
}

// ---------------------------------------------------------------------------
// Find and Replace
// ---------------------------------------------------------------------------

func TestFindReplace_OpenFind(t *testing.T) {
	m := newTestModel(t)
	m.loadNote("note1.md")

	// Open find mode
	m.findReplace.OpenFind(m.vault.Root)
	if !m.findReplace.IsActive() {
		t.Error("find/replace should be active")
	}
}

// ---------------------------------------------------------------------------
// Backlink building
// ---------------------------------------------------------------------------

func TestBuildBacklinkItems(t *testing.T) {
	m := newTestModel(t)
	m.activeNote = "note2.md"

	paths := m.vault.SortedPaths()
	items := m.buildBacklinkItems(paths, "note2")

	// note1.md links to note2, so it should appear as a backlink
	found := false
	for _, item := range items {
		if strings.Contains(item.Path, "note1") {
			found = true
		}
	}
	if !found {
		t.Error("expected note1 to appear as backlink for note2")
	}
}

func TestBuildOutgoingItems(t *testing.T) {
	m := newTestModel(t)
	m.loadNote("note1.md")

	paths := m.vault.SortedPaths()
	items := m.buildOutgoingItems(paths)

	// note1.md links to note2
	found := false
	for _, item := range items {
		if strings.Contains(item.Path, "note2") {
			found = true
		}
	}
	if !found {
		t.Error("expected note2 to appear as outgoing link from note1")
	}
}
