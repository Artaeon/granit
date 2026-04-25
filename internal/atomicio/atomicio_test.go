package atomicio

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteNote_CreatesFileWithContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	if err := WriteNote(path, "hello world"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello world" {
		t.Errorf("content: got %q want %q", got, "hello world")
	}
}

func TestWriteState_UsesOwnerOnlyPerm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("perm bits don't translate to Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := WriteState(path, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm: got %o want 600", info.Mode().Perm())
	}
}

func TestWriteNote_UsesWorldReadablePerm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("perm bits don't translate to Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	if err := WriteNote(path, "x"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("perm: got %o want 644", info.Mode().Perm())
	}
}

func TestWriteWithPerm_OverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteWithPerm(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("got %q want new", got)
	}
}

func TestWriteWithPerm_LeavesNoTempOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x")
	if err := WriteWithPerm(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf(".tmp file leaked: %v", err)
	}
}

func TestWriteWithPerm_FailsCleanlyOnUnwritableDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0 doesn't block writes the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses perm checks")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	path := filepath.Join(dir, "x")
	if err := WriteWithPerm(path, []byte("data"), 0o644); err == nil {
		t.Error("expected write to fail on read-only dir")
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf(".tmp file should be cleaned up after error: %v", err)
	}
}
