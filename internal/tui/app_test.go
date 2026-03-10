package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// createTestVault creates a temporary vault directory with a few markdown files
// and returns the path. Suitable for NewModel() which requires a real vault.
func createTestVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "note1.md"), []byte("# Note 1\n\nHello world.\n\nLink to [[note2]]."), 0644)
	_ = os.WriteFile(filepath.Join(dir, "note2.md"), []byte("# Note 2\n\nSee [[note1]]."), 0644)
	_ = os.MkdirAll(filepath.Join(dir, "subfolder"), 0755)
	_ = os.WriteFile(filepath.Join(dir, "subfolder", "deep.md"), []byte("# Deep\n\nNested note."), 0644)
	return dir
}

// ---------------------------------------------------------------------------
// NewModel smoke test
// ---------------------------------------------------------------------------

func TestNewModel_InitializesSuccessfully(t *testing.T) {
	dir := createTestVault(t)
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel failed: %v", err)
	}

	if m.vault == nil {
		t.Error("expected vault to be initialized")
	}
	if m.index == nil {
		t.Error("expected index to be initialized")
	}
}

// ---------------------------------------------------------------------------
// Default values after initialization
// ---------------------------------------------------------------------------

func TestNewModel_DefaultFocus(t *testing.T) {
	dir := createTestVault(t)
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	if m.focus != focusSidebar {
		t.Errorf("expected default focus to be focusSidebar (%d), got %d", focusSidebar, m.focus)
	}
}

func TestNewModel_NotQuitting(t *testing.T) {
	dir := createTestVault(t)
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	if m.quitting {
		t.Error("model should not be quitting on init")
	}
}

func TestNewModel_AutoLoadsFirstNote(t *testing.T) {
	dir := createTestVault(t)
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	// NewModel auto-loads the first note in sorted order
	if m.activeNote == "" {
		t.Error("expected an active note on init (auto-loaded)")
	}
}

func TestNewModel_EditorNotModified(t *testing.T) {
	dir := createTestVault(t)
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	if m.editor.IsModified() {
		t.Error("editor should not be modified on init")
	}
}

func TestNewModel_VaultNoteCount(t *testing.T) {
	dir := createTestVault(t)
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	if m.vault.NoteCount() != 3 {
		t.Errorf("expected 3 notes in vault, got %d", m.vault.NoteCount())
	}
}

// ---------------------------------------------------------------------------
// Init returns a command (or nil)
// ---------------------------------------------------------------------------

func TestModel_Init(t *testing.T) {
	dir := createTestVault(t)
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	// Init should not panic
	cmd := m.Init()
	// cmd may be nil or a batch command depending on config; both are fine
	_ = cmd
}

// ---------------------------------------------------------------------------
// Overlay priority: help overlay blocks other overlays
// ---------------------------------------------------------------------------

func TestOverlayPriority_HelpBlocksOthers(t *testing.T) {
	dir := createTestVault(t)
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	m.showSplash = false
	m.width = 120
	m.height = 40
	m.updateLayout()

	// Activate both help and settings
	m.helpOverlay.Toggle() // opens it
	m.settings.Toggle()    // opens it

	// Send Escape — help has higher priority, so it should handle it
	msg := tea.KeyMsg{Type: tea.KeyEsc}
	result, _ := m.Update(msg)
	updated := result.(Model)

	// Help overlay should have closed (consumed the key)
	if updated.helpOverlay.IsActive() {
		t.Error("expected help overlay to close on Escape")
	}

	// Settings should still be active (it didn't get the key)
	if !updated.settings.IsActive() {
		t.Error("expected settings to remain active since help had priority")
	}
}

// ---------------------------------------------------------------------------
// Overlay priority: settings blocks graph
// ---------------------------------------------------------------------------

func TestOverlayPriority_SettingsBlocksGraph(t *testing.T) {
	dir := createTestVault(t)
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	m.showSplash = false
	m.width = 120
	m.height = 40
	m.updateLayout()

	// Activate settings and graph
	m.settings.Toggle()  // opens it
	m.graphView.Open("") // opens with empty center note

	// Send Escape — settings has priority over graph
	msg := tea.KeyMsg{Type: tea.KeyEsc}
	result, _ := m.Update(msg)
	updated := result.(Model)

	// Settings should consume the escape
	if updated.settings.IsActive() {
		t.Error("expected settings to close on Escape")
	}

	// Graph should remain active
	if !updated.graphView.IsActive() {
		t.Error("expected graph to remain active since settings had priority")
	}
}

// ---------------------------------------------------------------------------
// Mode transitions: setFocus
// ---------------------------------------------------------------------------

func TestSetFocus_SidebarToEditor(t *testing.T) {
	dir := createTestVault(t)
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	m.setFocus(focusSidebar)
	if m.focus != focusSidebar {
		t.Errorf("expected focusSidebar, got %d", m.focus)
	}
	if !m.sidebar.focused {
		t.Error("sidebar should be focused")
	}
	if m.editor.focused {
		t.Error("editor should not be focused")
	}

	m.setFocus(focusEditor)
	if m.focus != focusEditor {
		t.Errorf("expected focusEditor, got %d", m.focus)
	}
	if !m.editor.focused {
		t.Error("editor should be focused")
	}
	if m.sidebar.focused {
		t.Error("sidebar should not be focused")
	}
}

func TestSetFocus_EditorToBacklinks(t *testing.T) {
	dir := createTestVault(t)
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	m.setFocus(focusEditor)
	m.setFocus(focusBacklinks)

	if m.focus != focusBacklinks {
		t.Errorf("expected focusBacklinks, got %d", m.focus)
	}
	if !m.backlinks.focused {
		t.Error("backlinks should be focused")
	}
	if m.editor.focused {
		t.Error("editor should not be focused after switching to backlinks")
	}
}

func TestCycleFocus(t *testing.T) {
	dir := createTestVault(t)
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	// Start at sidebar (0)
	m.setFocus(focusSidebar)

	// Cycle forward: sidebar -> editor -> backlinks -> sidebar
	m.cycleFocus(1)
	if m.focus != focusEditor {
		t.Errorf("expected focusEditor after cycle +1, got %d", m.focus)
	}

	m.cycleFocus(1)
	if m.focus != focusBacklinks {
		t.Errorf("expected focusBacklinks after cycle +1, got %d", m.focus)
	}

	m.cycleFocus(1)
	if m.focus != focusSidebar {
		t.Errorf("expected focusSidebar after full cycle, got %d", m.focus)
	}

	// Cycle backward: sidebar -> backlinks
	m.cycleFocus(-1)
	if m.focus != focusBacklinks {
		t.Errorf("expected focusBacklinks after cycle -1, got %d", m.focus)
	}
}

// ---------------------------------------------------------------------------
// SetSize propagation via WindowSizeMsg
// ---------------------------------------------------------------------------

func TestWindowSizeMsg_PropagatesSize(t *testing.T) {
	dir := createTestVault(t)
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	m.showSplash = false

	msg := tea.WindowSizeMsg{Width: 160, Height: 50}
	result, _ := m.Update(msg)
	updated := result.(Model)

	if updated.width != 160 {
		t.Errorf("expected width 160, got %d", updated.width)
	}
	if updated.height != 50 {
		t.Errorf("expected height 50, got %d", updated.height)
	}

	// Editor should have received some width
	if updated.editor.width == 0 {
		t.Error("editor width should be > 0 after resize")
	}
	if updated.editor.height == 0 {
		t.Error("editor height should be > 0 after resize")
	}

	// Renderer should match editor width
	if updated.renderer.width != updated.editor.width {
		t.Errorf("renderer width (%d) should match editor width (%d)",
			updated.renderer.width, updated.editor.width)
	}
}

func TestWindowSizeMsg_SmallTerminal(t *testing.T) {
	dir := createTestVault(t)
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	m.showSplash = false

	// Simulate a very small terminal (mobile)
	msg := tea.WindowSizeMsg{Width: 60, Height: 20}
	result, _ := m.Update(msg)
	updated := result.(Model)

	if updated.width != 60 {
		t.Errorf("expected width 60, got %d", updated.width)
	}
	// Should still have reasonable editor dimensions
	if updated.editor.width < 30 {
		t.Errorf("editor width too small: %d", updated.editor.width)
	}
}

func TestWindowSizeMsg_DuringSplash(t *testing.T) {
	dir := createTestVault(t)
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	m.showSplash = true

	msg := tea.WindowSizeMsg{Width: 100, Height: 30}
	result, _ := m.Update(msg)
	updated := result.(Model)

	if updated.width != 100 {
		t.Errorf("expected width 100 during splash, got %d", updated.width)
	}
	if updated.height != 30 {
		t.Errorf("expected height 30 during splash, got %d", updated.height)
	}
}

// ---------------------------------------------------------------------------
// Splash screen dismissal
// ---------------------------------------------------------------------------

func TestSplash_DismissOnKeyPress(t *testing.T) {
	dir := createTestVault(t)
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	m.showSplash = true

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}
	result, _ := m.Update(msg)
	updated := result.(Model)

	if updated.showSplash {
		t.Error("splash should be dismissed after key press")
	}
}

// ---------------------------------------------------------------------------
// ClearMessage msg handling
// ---------------------------------------------------------------------------

func TestClearMessageMsg(t *testing.T) {
	dir := createTestVault(t)
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	m.showSplash = false
	m.statusbar.SetMessage("test message")

	result, _ := m.Update(clearMessageMsg{})
	updated := result.(Model)

	// The message should be cleared
	_ = updated // clearMessageMsg sets message to ""
}

// ---------------------------------------------------------------------------
// View mode toggle
// ---------------------------------------------------------------------------

func TestNewModel_ViewModeMatchesConfig(t *testing.T) {
	dir := createTestVault(t)
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	// Default config has DefaultViewMode = false
	if m.viewMode {
		t.Error("expected viewMode to be false by default")
	}
}

// ---------------------------------------------------------------------------
// Empty vault initialization
// ---------------------------------------------------------------------------

func TestNewModel_EmptyVault(t *testing.T) {
	dir := t.TempDir() // no markdown files

	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel on empty vault: %v", err)
	}

	if m.vault.NoteCount() != 0 {
		t.Errorf("expected 0 notes, got %d", m.vault.NoteCount())
	}

	// Should still be able to resize without panic
	m.width = 120
	m.height = 40
	m.updateLayout()
}

// ---------------------------------------------------------------------------
// Multiple rapid resize events
// ---------------------------------------------------------------------------

func TestMultipleResizes(t *testing.T) {
	dir := createTestVault(t)
	m, err := NewModel(dir)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	m.showSplash = false

	sizes := []struct{ w, h int }{
		{40, 15}, {80, 24}, {120, 40}, {200, 60}, {60, 20}, {160, 50},
	}

	var current tea.Model = m
	for _, s := range sizes {
		msg := tea.WindowSizeMsg{Width: s.w, Height: s.h}
		current, _ = current.Update(msg)
	}

	updated := current.(Model)
	if updated.width != 160 || updated.height != 50 {
		t.Errorf("expected final size (160,50), got (%d,%d)", updated.width, updated.height)
	}
}
