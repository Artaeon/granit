package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadScriptures_DefaultsWhenNoFile(t *testing.T) {
	scriptures := LoadScriptures("/nonexistent/path")
	if len(scriptures) == 0 {
		t.Fatal("expected default scriptures when file doesn't exist")
	}
	// Check that defaults have both Text and Source
	for i, s := range scriptures {
		if s.Text == "" {
			t.Errorf("default scripture %d has empty text", i)
		}
		if s.Source == "" {
			t.Errorf("default scripture %d has empty source", i)
		}
	}
}

func TestLoadScriptures_ParsesCustomFile(t *testing.T) {
	dir := t.TempDir()
	granitDir := filepath.Join(dir, ".granit")
	_ = os.MkdirAll(granitDir, 0755)

	content := `# My Scriptures

Be strong and courageous — Joshua 1:9
The Lord is my shepherd — Psalm 23:1
A simple line without source
`
	_ = os.WriteFile(filepath.Join(granitDir, "scriptures.md"), []byte(content), 0644)

	scriptures := LoadScriptures(dir)
	if len(scriptures) != 3 {
		t.Fatalf("expected 3 scriptures, got %d", len(scriptures))
	}

	if scriptures[0].Text != "Be strong and courageous" {
		t.Errorf("expected text, got %q", scriptures[0].Text)
	}
	if scriptures[0].Source != "Joshua 1:9" {
		t.Errorf("expected source 'Joshua 1:9', got %q", scriptures[0].Source)
	}

	// Line without separator — text should be the whole line
	if scriptures[2].Source != "" {
		t.Errorf("expected empty source for line without separator, got %q", scriptures[2].Source)
	}
}

func TestLoadScriptures_EmptyFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	granitDir := filepath.Join(dir, ".granit")
	_ = os.MkdirAll(granitDir, 0755)
	_ = os.WriteFile(filepath.Join(granitDir, "scriptures.md"), []byte("# Just a heading\n\n"), 0644)

	scriptures := LoadScriptures(dir)
	if len(scriptures) == 0 {
		t.Fatal("expected default scriptures for file with only headings")
	}
}

func TestLoadScriptures_EnDashSeparator(t *testing.T) {
	dir := t.TempDir()
	granitDir := filepath.Join(dir, ".granit")
	_ = os.MkdirAll(granitDir, 0755)
	_ = os.WriteFile(filepath.Join(granitDir, "scriptures.md"), []byte("Trust in the Lord – Proverbs 3:5\n"), 0644)

	scriptures := LoadScriptures(dir)
	if len(scriptures) != 1 {
		t.Fatalf("expected 1 scripture, got %d", len(scriptures))
	}
	if scriptures[0].Source != "Proverbs 3:5" {
		t.Errorf("expected en-dash to parse source, got %q", scriptures[0].Source)
	}
}

func TestDailyScripture_Deterministic(t *testing.T) {
	// DailyScripture with no file should return a default and be consistent
	s1 := DailyScripture("/nonexistent")
	s2 := DailyScripture("/nonexistent")
	if s1.Text != s2.Text {
		t.Error("DailyScripture should return same scripture for same day")
	}
}

func TestRandomScripture_DoesNotPanic(t *testing.T) {
	// Should not panic even with no custom file
	s := RandomScripture("/nonexistent")
	if s.Text == "" {
		t.Error("expected non-empty scripture text")
	}
}
