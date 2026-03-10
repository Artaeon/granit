package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScan(t *testing.T) {
	// Create temp dir with test .md files
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "test.md"), []byte("# Hello\nworld"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "note2.md"), []byte("Link to [[test]]"), 0644)
	_ = os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	_ = os.WriteFile(filepath.Join(dir, "sub", "nested.md"), []byte("# Nested"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	paths := v.SortedPaths()
	if len(paths) != 3 {
		t.Errorf("expected 3 notes, got %d", len(paths))
	}
}

func TestGetNote(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "hello.md"), []byte("# Hello\nworld"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	_ = v.Scan()

	note := v.GetNote("hello.md")
	if note == nil {
		t.Fatal("expected note, got nil")
	}
	// Title is derived from filename (without .md extension), not from content
	if note.Title != "hello" {
		t.Errorf("expected title 'hello', got '%s'", note.Title)
	}
}

func TestNoteCount(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.md"), []byte("note a"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "b.md"), []byte("note b"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	_ = v.Scan()

	if v.NoteCount() != 2 {
		t.Errorf("expected 2 notes, got %d", v.NoteCount())
	}
}

func TestScanSkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "visible.md"), []byte("visible"), 0644)
	_ = os.MkdirAll(filepath.Join(dir, ".hidden"), 0755)
	_ = os.WriteFile(filepath.Join(dir, ".hidden", "secret.md"), []byte("secret"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	_ = v.Scan()

	if v.NoteCount() != 1 {
		t.Errorf("expected 1 note (hidden dir skipped), got %d", v.NoteCount())
	}
}
