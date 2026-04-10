package tui

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// ---------------------------------------------------------------------------
// atomicWriteNote — load-bearing helper used by ~14 call sites
// ---------------------------------------------------------------------------

func TestAtomicWriteNote_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")

	if err := atomicWriteNote(path, "hello"); err != nil {
		t.Fatalf("atomicWriteNote failed: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestAtomicWriteNote_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")

	if err := os.WriteFile(path, []byte("old content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := atomicWriteNote(path, "new content"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new content" {
		t.Errorf("expected 'new content', got %q", got)
	}
}

func TestAtomicWriteNote_LeavesNoTmpOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")

	if err := atomicWriteNote(path, "content"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("expected .tmp cleaned up, stat err = %v", err)
	}
}

func TestAtomicWriteNote_EmptyContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.md")

	if err := atomicWriteNote(path, ""); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Errorf("expected zero-size file, got %d bytes", info.Size())
	}
}

func TestAtomicWriteNote_LargeContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.md")

	// 1 MiB of content
	content := make([]byte, 1<<20)
	for i := range content {
		content[i] = byte('a' + (i % 26))
	}
	if err := atomicWriteNote(path, string(content)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(content) {
		t.Errorf("expected %d bytes, got %d", len(content), len(got))
	}
}

func TestAtomicWriteNote_MultibyteContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unicode.md")

	content := "café 🔥 naïve\n日本語\n"
	if err := atomicWriteNote(path, content); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != content {
		t.Errorf("multi-byte content corrupted: got %q want %q", got, content)
	}
}

// Regression: a failing rename (e.g. target is a directory of the same
// name) must clean up the .tmp file rather than leaving it behind.
func TestAtomicWriteNote_FailedRenameCleansUpTmp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("rename-over-directory semantics differ on Windows")
	}
	dir := t.TempDir()
	// Create a directory at the target path so os.Rename fails.
	target := filepath.Join(dir, "blocked")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}
	// Put something in the target directory so it's non-empty (rename
	// over a non-empty directory always fails on Linux).
	if err := os.WriteFile(filepath.Join(target, "child"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := atomicWriteNote(target, "content"); err == nil {
		t.Error("expected error renaming over a non-empty directory, got nil")
	}
	if _, err := os.Stat(target + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("expected .tmp cleaned up after rename failure, stat err = %v", err)
	}
}

// Regression: writing to a path whose parent directory does not exist
// must surface an error rather than silently dropping the content.
func TestAtomicWriteNote_NoParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing_subdir", "note.md")

	if err := atomicWriteNote(path, "content"); err == nil {
		t.Error("expected error writing to missing parent dir, got nil")
	}
}

// Regression: subsequent writes to the same path must each leave the
// file in the new state (not the previous one) and never half-overwrite.
func TestAtomicWriteNote_RepeatedWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.md")

	for i := 0; i < 50; i++ {
		content := "iteration " + string(rune('a'+(i%26)))
		if err := atomicWriteNote(path, content); err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		got, _ := os.ReadFile(path)
		if string(got) != content {
			t.Errorf("iteration %d: got %q want %q", i, got, content)
		}
	}
}
