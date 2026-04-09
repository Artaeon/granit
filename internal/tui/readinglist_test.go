package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadingList_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	rl := &ReadingList{vaultRoot: dir}
	rl.items = []ReadingItem{
		{URL: "https://example.com", Title: "Example", Priority: 1},
		{URL: "https://test.com", Title: "Test", ReadAt: "2026-04-09"},
	}
	rl.saveItems()

	// Verify file
	data, err := os.ReadFile(filepath.Join(dir, ".granit", "readinglist.json"))
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	var loaded []ReadingItem
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 items, got %d", len(loaded))
	}

	// Reload
	rl2 := &ReadingList{vaultRoot: dir}
	rl2.loadItems()
	if len(rl2.items) != 2 {
		t.Fatalf("expected 2 items after load, got %d", len(rl2.items))
	}
	if rl2.items[0].Title != "Example" {
		t.Errorf("expected 'Example', got %q", rl2.items[0].Title)
	}
}

func TestReadingList_LoadMissing(t *testing.T) {
	rl := &ReadingList{vaultRoot: t.TempDir()}
	rl.loadItems()
	if rl.items != nil {
		t.Error("expected nil for missing file")
	}
}

func TestReadingList_FilteredUnread(t *testing.T) {
	rl := &ReadingList{
		tab: 0, // unread
		items: []ReadingItem{
			{Title: "Unread 1"},
			{Title: "Read 1", ReadAt: "2026-04-09"},
			{Title: "Unread 2"},
		},
	}
	indices := rl.filtered()
	if len(indices) != 2 {
		t.Errorf("expected 2 unread items, got %d", len(indices))
	}
}

func TestReadingList_FilteredArchive(t *testing.T) {
	rl := &ReadingList{
		tab: 1, // archive (read items)
		items: []ReadingItem{
			{Title: "Unread 1"},
			{Title: "Read 1", ReadAt: "2026-04-09"},
		},
	}
	indices := rl.filtered()
	if len(indices) != 1 {
		t.Errorf("expected 1 archived item, got %d", len(indices))
	}
}

func TestReadingList_FilteredWithQuery(t *testing.T) {
	rl := &ReadingList{
		tab:         0,
		filterQuery: "go",
		items: []ReadingItem{
			{Title: "Learning Go", URL: "https://go.dev"},
			{Title: "Rust Book", URL: "https://rust-lang.org"},
			{Title: "Go Patterns", URL: "https://patterns.dev", Tags: []string{"golang"}},
		},
	}
	indices := rl.filtered()
	if len(indices) != 2 {
		t.Errorf("expected 2 items matching 'go', got %d", len(indices))
	}
}

func TestReadingList_EmptyVaultRootGuard(t *testing.T) {
	rl := &ReadingList{vaultRoot: ""}
	rl.items = []ReadingItem{{Title: "Test"}}
	rl.saveItems() // should not panic or create files in cwd
}
