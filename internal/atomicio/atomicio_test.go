package atomicio

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
	matches, _ := filepath.Glob(path + ".tmp.*")
	if len(matches) > 0 {
		t.Errorf("temp files leaked after failed write: %v", matches)
	}
}

func TestWriteWithPerm_PreservesExistingPerms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("perm bits don't translate to Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.md")
	// Create with permissive perm, then tighten.
	if err := os.WriteFile(path, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	// WriteNote requests 0o644, but the existing 0o600 should survive.
	if err := WriteNote(path, "v2"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm not preserved: got %o want 600", info.Mode().Perm())
	}
}

func TestWriteWithPerm_RejectsWritingThroughSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "real.md")
	if err := os.WriteFile(target, []byte("real"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := WriteNote(link, "via symlink"); err == nil {
		t.Error("expected refusal to write through symlink")
	}
	// The symlink target must NOT have been overwritten.
	got, _ := os.ReadFile(target)
	if string(got) != "real" {
		t.Errorf("symlink target was modified: %q", got)
	}
}

func TestWriteWithPerm_ConcurrentWritersDoNotCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shared")
	const writers = 20
	var wg sync.WaitGroup
	wg.Add(writers)
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		go func(n int) {
			defer wg.Done()
			payload := []byte("writer-" + strconv.Itoa(n))
			if err := WriteWithPerm(path, payload, 0o644); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent write failed: %v", err)
	}
	// Exactly one writer's content survives — and it's a valid one,
	// not a torn mix of two payloads.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), "writer-") {
		t.Errorf("torn write detected: %q", got)
	}
	// No temp files leaked (every writer either wrote+renamed or
	// failed and cleaned up).
	leftovers, _ := filepath.Glob(path + ".tmp.*")
	if len(leftovers) > 0 {
		t.Errorf("temp files leaked after concurrent writes: %v", leftovers)
	}
}

func TestWriteWithPerm_RejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteNote(subdir, "data"); err == nil {
		t.Error("expected refusal to write to a directory")
	}
}
