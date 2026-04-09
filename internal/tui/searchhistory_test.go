package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppendToHistory_Basic(t *testing.T) {
	h := appendToHistory(nil, "hello")
	if len(h) != 1 || h[0] != "hello" {
		t.Errorf("expected [hello], got %v", h)
	}
}

func TestAppendToHistory_NoDuplicateConsecutive(t *testing.T) {
	h := []string{"hello"}
	h = appendToHistory(h, "hello")
	if len(h) != 1 {
		t.Errorf("expected 1 entry (no dup), got %d", len(h))
	}
}

func TestAppendToHistory_AllowsDuplicateNonConsecutive(t *testing.T) {
	h := []string{"hello", "world"}
	h = appendToHistory(h, "hello")
	if len(h) != 3 {
		t.Errorf("expected 3 entries, got %d", len(h))
	}
}

func TestAppendToHistory_Empty(t *testing.T) {
	h := appendToHistory(nil, "")
	if len(h) != 0 {
		t.Errorf("expected 0 entries for empty query, got %d", len(h))
	}
}

func TestAppendToHistory_MaxCap(t *testing.T) {
	var h []string
	for i := 0; i < 25; i++ {
		h = appendToHistory(h, string(rune('a'+i)))
	}
	if len(h) != maxSearchHistory {
		t.Errorf("expected %d entries (capped), got %d", maxSearchHistory, len(h))
	}
	// Oldest entries should be trimmed
	if h[0] != string(rune('a'+5)) {
		t.Errorf("expected oldest to be trimmed, first is %q", h[0])
	}
}

func TestSearchHistory_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	h := SearchHistory{
		ContentSearch: []string{"foo", "bar"},
		FindReplace:   []string{"old", "new"},
	}
	saveSearchHistory(dir, h)

	loaded := loadSearchHistory(dir)
	if len(loaded.ContentSearch) != 2 {
		t.Fatalf("expected 2 content search entries, got %d", len(loaded.ContentSearch))
	}
	if loaded.ContentSearch[0] != "foo" {
		t.Errorf("expected 'foo', got %q", loaded.ContentSearch[0])
	}
	if len(loaded.FindReplace) != 2 {
		t.Fatalf("expected 2 find/replace entries, got %d", len(loaded.FindReplace))
	}
}

func TestSearchHistory_LoadMissing(t *testing.T) {
	h := loadSearchHistory(t.TempDir())
	if len(h.ContentSearch) != 0 || len(h.FindReplace) != 0 {
		t.Error("expected empty history for missing file")
	}
}

func TestSearchHistory_LoadCorrupt(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, ".granit"), 0755)
	_ = os.WriteFile(filepath.Join(dir, ".granit", "search-history.json"), []byte("not json"), 0600)

	h := loadSearchHistory(dir)
	// Should not panic, return empty
	if len(h.ContentSearch) != 0 {
		t.Error("expected empty history for corrupt file")
	}
}

func TestSearchHistory_LoadTruncatesOversize(t *testing.T) {
	dir := t.TempDir()
	h := SearchHistory{}
	for i := 0; i < 30; i++ {
		h.ContentSearch = append(h.ContentSearch, string(rune('a'+i%26)))
	}
	saveSearchHistory(dir, h)

	loaded := loadSearchHistory(dir)
	if len(loaded.ContentSearch) != maxSearchHistory {
		t.Errorf("expected %d entries after load cap, got %d", maxSearchHistory, len(loaded.ContentSearch))
	}
}
