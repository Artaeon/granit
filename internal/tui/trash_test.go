package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// NewTrash — initial state
// ---------------------------------------------------------------------------

func TestTrashNewTrash(t *testing.T) {
	tr := NewTrash("/tmp/vault")
	if tr.IsActive() {
		t.Error("new trash should not be active")
	}
	if tr.vaultRoot != "/tmp/vault" {
		t.Errorf("unexpected vaultRoot: %s", tr.vaultRoot)
	}
	if len(tr.items) != 0 {
		t.Errorf("expected empty items, got %d", len(tr.items))
	}
	if tr.cursor != 0 {
		t.Errorf("expected cursor 0, got %d", tr.cursor)
	}
}

// ---------------------------------------------------------------------------
// Open / Close / IsActive — state transitions
// ---------------------------------------------------------------------------

func TestTrashOpenCloseIsActive(t *testing.T) {
	tmpDir := t.TempDir()
	tr := NewTrash(tmpDir)

	if tr.IsActive() {
		t.Error("should not be active before Open")
	}

	tr.Open()
	if !tr.IsActive() {
		t.Error("should be active after Open")
	}
	if tr.cursor != 0 {
		t.Errorf("cursor should reset to 0, got %d", tr.cursor)
	}
	if tr.scroll != 0 {
		t.Errorf("scroll should reset to 0, got %d", tr.scroll)
	}

	tr.Close()
	if tr.IsActive() {
		t.Error("should not be active after Close")
	}
}

func TestTrashOpenResetsState(t *testing.T) {
	tmpDir := t.TempDir()
	tr := NewTrash(tmpDir)
	tr.cursor = 5
	tr.scroll = 3
	tr.result = "old result"
	tr.doRestore = true
	tr.doPurge = true

	tr.Open()
	if tr.cursor != 0 {
		t.Errorf("cursor should reset, got %d", tr.cursor)
	}
	if tr.scroll != 0 {
		t.Errorf("scroll should reset, got %d", tr.scroll)
	}
	if tr.result != "" {
		t.Errorf("result should reset, got %q", tr.result)
	}
	if tr.doRestore {
		t.Error("doRestore should reset")
	}
	if tr.doPurge {
		t.Error("doPurge should reset")
	}
}

// ---------------------------------------------------------------------------
// SetSize — dimensions
// ---------------------------------------------------------------------------

func TestTrashSetSize(t *testing.T) {
	tr := NewTrash("/tmp/vault")
	tr.SetSize(100, 50)
	if tr.width != 100 {
		t.Errorf("expected width 100, got %d", tr.width)
	}
	if tr.height != 50 {
		t.Errorf("expected height 50, got %d", tr.height)
	}
}

// ---------------------------------------------------------------------------
// MoveToTrash — adds a file to trash
// ---------------------------------------------------------------------------

func TestTrashMoveToTrash(t *testing.T) {
	tmpDir := t.TempDir()
	tr := NewTrash(tmpDir)

	// Create a file to trash
	notePath := filepath.Join(tmpDir, "note.md")
	if err := os.WriteFile(notePath, []byte("# My Note"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := tr.MoveToTrash("note.md"); err != nil {
		t.Fatalf("MoveToTrash failed: %v", err)
	}

	// Original file should be gone
	if _, err := os.Stat(notePath); !os.IsNotExist(err) {
		t.Error("original file should be removed after trash")
	}

	// Trash directory should exist and have files
	trashDir := filepath.Join(tmpDir, ".granit-trash")
	entries, err := os.ReadDir(trashDir)
	if err != nil {
		t.Fatalf("trash dir should exist: %v", err)
	}
	// Should have the trashed file + its JSON sidecar
	if len(entries) != 2 {
		t.Errorf("expected 2 entries in trash dir (file + sidecar), got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// Item listing — shows trashed files after scanning
// ---------------------------------------------------------------------------

func TestTrashItemListing(t *testing.T) {
	tmpDir := t.TempDir()
	tr := NewTrash(tmpDir)

	// Create and trash two files
	for _, name := range []string{"note1.md", "note2.md"} {
		p := filepath.Join(tmpDir, name)
		_ = os.WriteFile(p, []byte("content of "+name), 0644)
		if err := tr.MoveToTrash(name); err != nil {
			t.Fatalf("MoveToTrash(%s) failed: %v", name, err)
		}
	}

	// Open should scan and populate items
	tr.Open()
	if len(tr.items) != 2 {
		t.Errorf("expected 2 items, got %d", len(tr.items))
	}
}

// ---------------------------------------------------------------------------
// Navigation — up/down through trashed items
// ---------------------------------------------------------------------------

func TestTrashNavigation(t *testing.T) {
	tmpDir := t.TempDir()
	tr := NewTrash(tmpDir)

	// Create and trash several files
	for _, name := range []string{"a.md", "b.md", "c.md"} {
		p := filepath.Join(tmpDir, name)
		_ = os.WriteFile(p, []byte("content"), 0644)
		_ = tr.MoveToTrash(name)
	}

	tr.Open()
	tr.SetSize(80, 40)

	if tr.cursor != 0 {
		t.Errorf("cursor should start at 0, got %d", tr.cursor)
	}

	// Navigate down
	tr, _ = tr.Update(tea.KeyMsg{Type: tea.KeyDown})
	if tr.cursor != 1 {
		t.Errorf("expected cursor 1, got %d", tr.cursor)
	}

	tr, _ = tr.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if tr.cursor != 2 {
		t.Errorf("expected cursor 2, got %d", tr.cursor)
	}

	// Should not go beyond last item
	tr, _ = tr.Update(tea.KeyMsg{Type: tea.KeyDown})
	if tr.cursor != 2 {
		t.Errorf("cursor should clamp at 2, got %d", tr.cursor)
	}

	// Navigate up
	tr, _ = tr.Update(tea.KeyMsg{Type: tea.KeyUp})
	if tr.cursor != 1 {
		t.Errorf("expected cursor 1, got %d", tr.cursor)
	}

	tr, _ = tr.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if tr.cursor != 0 {
		t.Errorf("expected cursor 0, got %d", tr.cursor)
	}

	// Should not go below 0
	tr, _ = tr.Update(tea.KeyMsg{Type: tea.KeyUp})
	if tr.cursor != 0 {
		t.Errorf("cursor should clamp at 0, got %d", tr.cursor)
	}
}

// ---------------------------------------------------------------------------
// Cursor bounds — cursor clamped after deletion
// ---------------------------------------------------------------------------

func TestTrashCursorBoundsAfterPurge(t *testing.T) {
	tmpDir := t.TempDir()
	tr := NewTrash(tmpDir)

	// Create and trash two files
	for _, name := range []string{"a.md", "b.md"} {
		p := filepath.Join(tmpDir, name)
		_ = os.WriteFile(p, []byte("content"), 0644)
		_ = tr.MoveToTrash(name)
	}

	tr.Open()
	// Move cursor to last item
	tr.cursor = len(tr.items) - 1

	// Purge the last item
	tr.PurgeSelected()

	// Cursor should be clamped
	if tr.cursor >= len(tr.items) && len(tr.items) > 0 {
		t.Errorf("cursor %d should be < len(items) %d", tr.cursor, len(tr.items))
	}
}

func TestTrashCursorBoundsAfterRestore(t *testing.T) {
	tmpDir := t.TempDir()
	tr := NewTrash(tmpDir)

	// Create and trash two files
	for _, name := range []string{"a.md", "b.md"} {
		p := filepath.Join(tmpDir, name)
		_ = os.WriteFile(p, []byte("content"), 0644)
		_ = tr.MoveToTrash(name)
	}

	tr.Open()
	// Move cursor to last item
	tr.cursor = len(tr.items) - 1

	// Restore the last item
	result := tr.RestoreFile()
	if result == "" {
		t.Error("RestoreFile should return original path")
	}

	// Cursor should be clamped
	if len(tr.items) > 0 && tr.cursor >= len(tr.items) {
		t.Errorf("cursor %d should be < len(items) %d", tr.cursor, len(tr.items))
	}
}

// ---------------------------------------------------------------------------
// RestoreFile — returns restore path and removes from list
// ---------------------------------------------------------------------------

func TestTrashRestoreFile(t *testing.T) {
	tmpDir := t.TempDir()
	tr := NewTrash(tmpDir)

	// Create and trash a file
	notePath := filepath.Join(tmpDir, "restore_me.md")
	_ = os.WriteFile(notePath, []byte("restore content"), 0644)
	_ = tr.MoveToTrash("restore_me.md")

	tr.Open()
	if len(tr.items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(tr.items))
	}

	origPath := tr.RestoreFile()
	if origPath != "restore_me.md" {
		t.Errorf("expected restore path 'restore_me.md', got %q", origPath)
	}

	// File should be restored
	data, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("restored file should exist: %v", err)
	}
	if string(data) != "restore content" {
		t.Errorf("unexpected restored content: %s", string(data))
	}

	// Item should be removed from list
	if len(tr.items) != 0 {
		t.Errorf("expected 0 items after restore, got %d", len(tr.items))
	}

	// ShouldRestore flag should be set
	if !tr.ShouldRestore() {
		t.Error("ShouldRestore should return true after restore")
	}
	// Second call should return false
	if tr.ShouldRestore() {
		t.Error("ShouldRestore should return false on second call")
	}
}

func TestTrashRestoreEmptyList(t *testing.T) {
	tmpDir := t.TempDir()
	tr := NewTrash(tmpDir)
	tr.Open()

	result := tr.RestoreFile()
	if result != "" {
		t.Errorf("expected empty result for empty list, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// PurgeSelected — permanently deletes selected item
// ---------------------------------------------------------------------------

func TestTrashPurgeSelected(t *testing.T) {
	tmpDir := t.TempDir()
	tr := NewTrash(tmpDir)

	// Create and trash a file
	notePath := filepath.Join(tmpDir, "purge_me.md")
	_ = os.WriteFile(notePath, []byte("purge content"), 0644)
	_ = tr.MoveToTrash("purge_me.md")

	tr.Open()
	if len(tr.items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(tr.items))
	}

	trashPath := tr.items[0].TrashPath
	tr.PurgeSelected()

	// Item should be removed from list
	if len(tr.items) != 0 {
		t.Errorf("expected 0 items after purge, got %d", len(tr.items))
	}

	// File should be deleted from trash dir
	trashDir := filepath.Join(tmpDir, ".granit-trash")
	if _, err := os.Stat(filepath.Join(trashDir, trashPath)); !os.IsNotExist(err) {
		t.Error("purged file should not exist in trash dir")
	}

	// Sidecar should also be deleted
	if _, err := os.Stat(filepath.Join(trashDir, trashPath+".json")); !os.IsNotExist(err) {
		t.Error("purged sidecar should not exist in trash dir")
	}

	// doPurge flag should be set
	if !tr.doPurge {
		t.Error("doPurge should be true after purge")
	}
}

func TestTrashPurgeEmptyList(t *testing.T) {
	tmpDir := t.TempDir()
	tr := NewTrash(tmpDir)
	tr.Open()

	// Should not panic
	tr.PurgeSelected()
	if len(tr.items) != 0 {
		t.Error("items should still be empty")
	}
}

// ---------------------------------------------------------------------------
// Empty trash — handles gracefully (no items to navigate)
// ---------------------------------------------------------------------------

func TestTrashEmptyTrash(t *testing.T) {
	tmpDir := t.TempDir()
	tr := NewTrash(tmpDir)
	tr.Open()

	if len(tr.items) != 0 {
		t.Errorf("expected 0 items in empty vault, got %d", len(tr.items))
	}

	// Navigation should not crash
	tr, _ = tr.Update(tea.KeyMsg{Type: tea.KeyDown})
	tr, _ = tr.Update(tea.KeyMsg{Type: tea.KeyUp})
	if tr.cursor != 0 {
		t.Errorf("cursor should stay at 0, got %d", tr.cursor)
	}

	// Restore/purge should not crash
	tr.RestoreFile()
	tr.PurgeSelected()
}

func TestTrashEmptyView(t *testing.T) {
	tmpDir := t.TempDir()
	tr := NewTrash(tmpDir)
	tr.SetSize(100, 40)
	tr.Open()

	view := tr.View()
	if view == "" {
		t.Error("expected non-empty view for empty trash")
	}
}

// ---------------------------------------------------------------------------
// Multiple items — correct cursor behavior with several trashed files
// ---------------------------------------------------------------------------

func TestTrashMultipleItemsCursorBehavior(t *testing.T) {
	tmpDir := t.TempDir()
	tr := NewTrash(tmpDir)

	// Create and trash 5 files
	for i := 0; i < 5; i++ {
		name := filepath.Join(tmpDir, string(rune('a'+i))+".md")
		_ = os.WriteFile(name, []byte("content"), 0644)
		_ = tr.MoveToTrash(string(rune('a'+i)) + ".md")
		// Small delay to ensure different timestamps for sorting
		time.Sleep(time.Millisecond)
	}

	tr.Open()
	if len(tr.items) != 5 {
		t.Fatalf("expected 5 items, got %d", len(tr.items))
	}

	// Navigate to middle (cursor at 2)
	tr.cursor = 2

	// Purge middle item
	tr.PurgeSelected()
	if len(tr.items) != 4 {
		t.Errorf("expected 4 items, got %d", len(tr.items))
	}
	if tr.cursor != 2 {
		t.Errorf("cursor should stay at 2 (next item), got %d", tr.cursor)
	}

	// Purge all remaining items one by one
	for len(tr.items) > 0 {
		tr.cursor = len(tr.items) - 1
		tr.PurgeSelected()
	}
	if tr.cursor != 0 {
		t.Errorf("cursor should be 0 when all items purged, got %d", tr.cursor)
	}
}

func TestTrashEscCloses(t *testing.T) {
	tmpDir := t.TempDir()
	tr := NewTrash(tmpDir)
	tr.Open()

	tr, _ = tr.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if tr.IsActive() {
		t.Error("Esc should close the trash overlay")
	}
}

func TestTrashUpdateInactive(t *testing.T) {
	tmpDir := t.TempDir()
	tr := NewTrash(tmpDir)

	// Update while inactive should be a no-op
	_, cmd := tr.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Error("expected nil cmd when inactive")
	}
}

func TestTrashSortingNewestFirst(t *testing.T) {
	tmpDir := t.TempDir()
	trashDir := filepath.Join(tmpDir, ".granit-trash")
	_ = os.MkdirAll(trashDir, 0755)

	// Manually create two sidecar files with known timestamps
	older := TrashItem{
		OrigPath:  "old.md",
		TrashPath: "old_content",
		DeletedAt: time.Now().Add(-2 * time.Hour),
	}
	newer := TrashItem{
		OrigPath:  "new.md",
		TrashPath: "new_content",
		DeletedAt: time.Now().Add(-1 * time.Minute),
	}

	// Write content files
	_ = os.WriteFile(filepath.Join(trashDir, "old_content"), []byte("old"), 0644)
	_ = os.WriteFile(filepath.Join(trashDir, "new_content"), []byte("new"), 0644)

	// Write sidecar JSON files
	for _, item := range []TrashItem{older, newer} {
		data, _ := json.Marshal(item)
		_ = os.WriteFile(filepath.Join(trashDir, item.TrashPath+".json"), data, 0644)
	}

	tr := NewTrash(tmpDir)
	tr.Open()

	if len(tr.items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(tr.items))
	}

	// Newest should be first
	if tr.items[0].OrigPath != "new.md" {
		t.Errorf("expected newest first, got %s", tr.items[0].OrigPath)
	}
	if tr.items[1].OrigPath != "old.md" {
		t.Errorf("expected oldest second, got %s", tr.items[1].OrigPath)
	}
}

func TestTrashTimeAgo(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{10 * time.Second, "just now"},
		{5 * time.Minute, "5m ago"},
		{1 * time.Minute, "1m ago"},
		{3 * time.Hour, "3h ago"},
		{1 * time.Hour, "1h ago"},
		{48 * time.Hour, "2d ago"},
		{24 * time.Hour, "1d ago"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			then := time.Now().Add(-tc.duration)
			result := timeAgo(then)
			if result != tc.expected {
				t.Errorf("timeAgo(-%v) = %q, want %q", tc.duration, result, tc.expected)
			}
		})
	}
}

func TestTrashViewWithItems(t *testing.T) {
	tmpDir := t.TempDir()
	tr := NewTrash(tmpDir)

	// Create and trash a file
	notePath := filepath.Join(tmpDir, "view_test.md")
	_ = os.WriteFile(notePath, []byte("test content"), 0644)
	_ = tr.MoveToTrash("view_test.md")

	tr.SetSize(100, 40)
	tr.Open()

	view := tr.View()
	if view == "" {
		t.Error("expected non-empty view with items")
	}
}
