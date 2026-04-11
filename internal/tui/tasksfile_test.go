package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ===========================================================================
// Tasks.md helper tests
// ===========================================================================

func TestTasksFilePath_EmptyVault(t *testing.T) {
	if got := tasksFilePath(""); got != "" {
		t.Errorf("tasksFilePath(\"\") = %q, want empty string", got)
	}
}

func TestTasksFilePath_JoinsVaultRoot(t *testing.T) {
	vault := t.TempDir()
	want := filepath.Join(vault, "Tasks.md")
	if got := tasksFilePath(vault); got != want {
		t.Errorf("tasksFilePath = %q, want %q", got, want)
	}
}

func TestReadTasksFile_MissingReturnsNilNil(t *testing.T) {
	vault := t.TempDir()
	data, err := readTasksFile(vault)
	if err != nil {
		t.Errorf("missing file should not error, got: %v", err)
	}
	if data != nil {
		t.Errorf("missing file should return nil bytes, got %q", data)
	}
}

func TestReadTasksFile_ReturnsContents(t *testing.T) {
	vault := t.TempDir()
	want := "# Tasks\n\n- [ ] hello\n"
	if err := os.WriteFile(filepath.Join(vault, "Tasks.md"), []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readTasksFile(vault)
	if err != nil {
		t.Fatalf("readTasksFile: %v", err)
	}
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWriteTasksFile_AtomicReplace(t *testing.T) {
	vault := t.TempDir()
	// Seed with v1.
	if err := writeTasksFile(vault, []byte("v1\n")); err != nil {
		t.Fatal(err)
	}
	// Replace with v2.
	if err := writeTasksFile(vault, []byte("v2\n")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(vault, "Tasks.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "v2\n" {
		t.Errorf("got %q, want %q", got, "v2\n")
	}
	// The .tmp file should not be left behind.
	if _, err := os.Stat(filepath.Join(vault, "Tasks.md.tmp")); !os.IsNotExist(err) {
		t.Errorf(".tmp file should be gone after rename")
	}
}

func TestAppendTaskLine_CreatesFileWithHeader(t *testing.T) {
	vault := t.TempDir()
	if err := appendTaskLine(vault, "- [ ] first task"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(vault, "Tasks.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# Tasks\n\n- [ ] first task\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAppendTaskLine_AppendsToExisting(t *testing.T) {
	vault := t.TempDir()
	seed := "# Tasks\n\n- [ ] first\n"
	if err := os.WriteFile(filepath.Join(vault, "Tasks.md"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := appendTaskLine(vault, "- [ ] second"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(vault, "Tasks.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# Tasks\n\n- [ ] first\n- [ ] second\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAppendTaskLine_NormalisesMissingTrailingNewline(t *testing.T) {
	// If the existing Tasks.md doesn't end in a newline, appendTaskLine
	// should still produce a clean separator instead of welding lines
	// together.
	vault := t.TempDir()
	seed := "# Tasks\n\n- [ ] first" // no trailing newline
	if err := os.WriteFile(filepath.Join(vault, "Tasks.md"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := appendTaskLine(vault, "- [ ] second"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(vault, "Tasks.md"))
	if !strings.Contains(string(got), "- [ ] first\n- [ ] second") {
		t.Errorf("welded lines without separator: %q", got)
	}
}

func TestAppendTaskLine_StripsCallerNewlines(t *testing.T) {
	// Some legacy callers passed lines like "\n- [ ] foo\n". appendTaskLine
	// should normalise these to a single trailing newline so we don't end up
	// with double-blank lines accumulating in the file.
	vault := t.TempDir()
	if err := appendTaskLine(vault, "\n- [ ] foo\n\n"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(vault, "Tasks.md"))
	if strings.Count(string(got), "\n\n\n") > 0 {
		t.Errorf("triple newline accumulated: %q", got)
	}
}

func TestAppendTaskLine_NoOpOnEmptyInput(t *testing.T) {
	vault := t.TempDir()
	if err := appendTaskLine(vault, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(vault, "Tasks.md")); !os.IsNotExist(err) {
		t.Errorf("empty taskLine should not create Tasks.md")
	}
}

func TestAppendTaskLine_NoOpOnEmptyVault(t *testing.T) {
	if err := appendTaskLine("", "- [ ] foo"); err != nil {
		t.Errorf("empty vault should be a no-op, got error: %v", err)
	}
}
