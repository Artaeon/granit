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

// ── SnapshotNotes — goroutine safety ──

func TestVault_SnapshotNotes_ReturnsIndependentMap(t *testing.T) {
	v := &Vault{Root: "/tmp", Notes: map[string]*Note{
		"a.md": {Path: "/tmp/a.md", RelPath: "a.md"},
		"b.md": {Path: "/tmp/b.md", RelPath: "b.md"},
	}}
	snap := v.SnapshotNotes()

	if len(snap) != 2 {
		t.Fatalf("snapshot has %d entries, want 2", len(snap))
	}
	// Mutating the snapshot must not affect the live Notes map — the
	// whole point is to let a goroutine iterate without racing.
	delete(snap, "a.md")
	snap["c.md"] = &Note{}

	if _, ok := v.Notes["a.md"]; !ok {
		t.Error("snapshot deletion leaked into live Notes")
	}
	if _, ok := v.Notes["c.md"]; ok {
		t.Error("snapshot insertion leaked into live Notes")
	}
}

func TestVault_SnapshotNotes_HandlesEmpty(t *testing.T) {
	v := &Vault{Notes: map[string]*Note{}}
	if got := v.SnapshotNotes(); len(got) != 0 {
		t.Errorf("empty vault snapshot should be empty, got %d entries", len(got))
	}
}

func TestVault_SnapshotNotes_HandlesNilSource(t *testing.T) {
	v := &Vault{}
	if got := v.SnapshotNotes(); got == nil {
		t.Error("nil-source snapshot should return an empty non-nil map")
	}
}
